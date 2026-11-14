package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// PluginState represents the lifecycle state of a plugin.
type PluginState string

const (
	StateInstalled PluginState = "installed"
	StateActive    PluginState = "active"
	StateInactive  PluginState = "inactive"
	StateError     PluginState = "error"
)

// PluginStatus contains the runtime and persistent state of an installed plugin.
type PluginStatus struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Description string      `json:"description,omitempty"`
	InstallDir  string      `json:"install_dir"`
	State       PluginState `json:"state"`
	Enabled     bool        `json:"enabled"`
	PID         int         `json:"pid,omitempty"`
	Error       string      `json:"error,omitempty"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	Manifest    *Manifest   `json:"manifest,omitempty"`
}

type pluginInstance struct {
	record     storage.PluginRecord
	manifest   *Manifest
	state      PluginState
	startedAt  *time.Time
	lastError  string
	sandbox    *SandboxProcess
	hostClient *HostClient
}

// Manager orchestrates plugin discovery, lifecycle, sandboxing, and dispatching.
type Manager struct {
	mu         sync.RWMutex
	store      storage.Store
	registry   *Registry
	pluginsDir string
	instances  map[string]*pluginInstance
	launcher   CommandLauncher
}

// NewManager creates a plugin manager backed by the given store and registry.
func NewManager(store storage.Store, registry *Registry, pluginsDir string) *Manager {
	if registry == nil {
		registry = NewRegistry()
	}
	if pluginsDir == "" {
		home, _ := os.UserHomeDir()
		pluginsDir = filepath.Join(home, ".config", "cadence", "plugins")
	}

	return &Manager{
		store:      store,
		registry:   registry,
		pluginsDir: pluginsDir,
		instances:  make(map[string]*pluginInstance),
		launcher:   DefaultCommandLauncher,
	}
}

// SetLauncher configures a custom process launcher (useful for integration testing).
func (m *Manager) SetLauncher(l CommandLauncher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.launcher = l
}

// Registry returns the underlying plugin registry.
func (m *Manager) Registry() *Registry {
	return m.registry
}

// Install reads a plugin from the specified source path, validates its manifest,
// copies its directory to the managed plugins location, and persists the record.
func (m *Manager) Install(srcPath string) (*PluginStatus, error) {
	if srcPath == "" {
		return nil, errors.New("plugin source path must be provided")
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("stat plugin source: %w", err)
	}

	var srcDir string
	if info.IsDir() {
		srcDir = srcPath
	} else {
		srcDir = filepath.Dir(srcPath)
	}

	// Validate manifest from source
	manifest, err := LoadManifest(srcPath)
	if err != nil {
		return nil, fmt.Errorf("load source manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	targetDir := filepath.Join(m.pluginsDir, manifest.ID)
	if err := os.MkdirAll(m.pluginsDir, 0755); err != nil {
		return nil, fmt.Errorf("create plugins root directory: %w", err)
	}

	// If destination already exists, remove it first
	_ = os.RemoveAll(targetDir)

	if err := copyDirectory(srcDir, targetDir); err != nil {
		return nil, fmt.Errorf("copy plugin files: %w", err)
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	rec := storage.PluginRecord{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Version:      manifest.Version,
		InstallDir:   targetDir,
		Enabled:      true,
		ManifestJSON: string(manifestBytes),
		InstalledAt:  time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := m.store.SavePlugin(rec); err != nil {
		return nil, fmt.Errorf("save plugin to storage: %w", err)
	}

	_ = m.registry.Register(manifest)

	m.mu.Lock()
	m.instances[manifest.ID] = &pluginInstance{
		record:   rec,
		manifest: manifest,
		state:    StateInstalled,
	}
	m.mu.Unlock()

	return &PluginStatus{
		ID:          manifest.ID,
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		InstallDir:  targetDir,
		State:       StateInstalled,
		Enabled:     true,
		Manifest:    manifest,
	}, nil
}

// Uninstall deactivates the plugin, removes its files, and deletes it from storage and registry.
func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	inst, exists := m.instances[id]
	m.mu.Unlock()

	if !exists {
		// Try loading from store
		rec, err := m.store.GetPlugin(id)
		if err != nil {
			return fmt.Errorf("plugin not found: %s", id)
		}
		inst = &pluginInstance{record: *rec}
	}

	// Deactivate if currently active
	if inst.sandbox != nil {
		_ = m.Deactivate(id)
	}

	// Remove files
	installDir := inst.record.InstallDir
	if installDir == "" {
		installDir = filepath.Join(m.pluginsDir, id)
	}
	_ = os.RemoveAll(installDir)

	// Remove from storage and registry
	_ = m.store.DeletePlugin(id)
	_ = m.registry.Unregister(id)

	m.mu.Lock()
	delete(m.instances, id)
	m.mu.Unlock()

	return nil
}

// Activate spawns the plugin subprocess in a sandbox and performs the initialization handshake.
func (m *Manager) Activate(ctx context.Context, id string) (*PluginStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, exists := m.instances[id]
	if !exists {
		rec, err := m.store.GetPlugin(id)
		if err != nil {
			return nil, fmt.Errorf("plugin not found: %s", id)
		}
		var mf Manifest
		if err := json.Unmarshal([]byte(rec.ManifestJSON), &mf); err != nil {
			return nil, fmt.Errorf("parse stored manifest: %w", err)
		}
		inst = &pluginInstance{
			record:   *rec,
			manifest: &mf,
			state:    StateInstalled,
		}
		m.instances[id] = inst
	}

	// If already active, return current status
	if inst.state == StateActive && inst.sandbox != nil {
		return m.statusLocked(inst), nil
	}

	manifest := inst.manifest
	if manifest == nil {
		var mf Manifest
		if err := json.Unmarshal([]byte(inst.record.ManifestJSON), &mf); err != nil {
			return nil, fmt.Errorf("manifest missing for plugin %s", id)
		}
		manifest = &mf
		inst.manifest = manifest
	}

	installDir := inst.record.InstallDir
	if installDir == "" {
		installDir = filepath.Join(m.pluginsDir, id)
	}

	sbCfg := SandboxConfig{
		PluginID:    id,
		InstallDir:  installDir,
		Entrypoint:  manifest.Entrypoint,
		Permissions: manifest.Permissions,
		Launcher:    m.launcher,
	}

	sp, stdout, stdin, err := SpawnSandbox(ctx, sbCfg)
	if err != nil {
		inst.state = StateError
		inst.lastError = err.Error()
		return m.statusLocked(inst), fmt.Errorf("spawn sandbox: %w", err)
	}

	client := NewHostClient(stdout, stdin)

	// Perform init handshake with timeout
	handshakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cfgPayload := make(map[string]any)
	for _, f := range manifest.ConfigSchema {
		if f.Default != nil {
			cfgPayload[f.Name] = f.Default
		}
	}

	initResp, err := client.SendCall(handshakeCtx, Message{
		Type: TypeInit,
		Payload: map[string]any{
			"config":  cfgPayload,
			"version": manifest.Version,
		},
	})
	if err != nil {
		_ = sp.Stop(1 * time.Second)
		inst.state = StateError
		inst.lastError = fmt.Sprintf("init handshake failed: %v", err)
		return m.statusLocked(inst), fmt.Errorf("handshake failed: %w", err)
	}
	if initResp.Error != "" {
		_ = sp.Stop(1 * time.Second)
		inst.state = StateError
		inst.lastError = initResp.Error
		return m.statusLocked(inst), fmt.Errorf("handshake rejected: %s", initResp.Error)
	}

	now := time.Now()
	inst.sandbox = sp
	inst.hostClient = client
	inst.state = StateActive
	inst.startedAt = &now
	inst.lastError = ""
	inst.record.Enabled = true

	_ = m.store.SetPluginEnabled(id, true)
	_ = m.registry.Register(manifest)

	// Monitor process termination in background
	go func(instRef *pluginInstance, proc *SandboxProcess, pid int) {
		<-proc.Exited()
		m.mu.Lock()
		defer m.mu.Unlock()

		if instRef.sandbox == proc && instRef.state == StateActive {
			exitErr := proc.ExitError()
			if exitErr != nil {
				instRef.lastError = fmt.Sprintf("process %d exited: %v", pid, exitErr)
				instRef.state = StateError
			} else {
				instRef.state = StateInactive
			}
			instRef.sandbox = nil
			instRef.hostClient = nil
			instRef.startedAt = nil
		}
	}(inst, sp, sp.PID())

	return m.statusLocked(inst), nil
}

// Deactivate stops the sandboxed process and marks the plugin inactive.
func (m *Manager) Deactivate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, exists := m.instances[id]
	if !exists {
		rec, err := m.store.GetPlugin(id)
		if err != nil {
			return fmt.Errorf("plugin not found: %s", id)
		}
		inst = &pluginInstance{record: *rec, state: StateInactive}
		m.instances[id] = inst
	}

	if inst.sandbox != nil {
		_ = inst.sandbox.Stop(2 * time.Second)
		inst.sandbox = nil
		inst.hostClient = nil
	}

	inst.state = StateInactive
	inst.startedAt = nil
	inst.record.Enabled = false
	_ = m.store.SetPluginEnabled(id, false)

	return nil
}

// GetStatus returns the status of a specific plugin.
func (m *Manager) GetStatus(id string) (*PluginStatus, error) {
	m.mu.RLock()
	inst, exists := m.instances[id]
	m.mu.RUnlock()

	if exists {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.statusLocked(inst), nil
	}

	rec, err := m.store.GetPlugin(id)
	if err != nil {
		if m.registry != nil {
			if mf, ok := m.registry.Get(id); ok {
				return &PluginStatus{
					ID:          mf.ID,
					Name:        mf.Name,
					Version:     mf.Version,
					Description: mf.Description,
					State:       StateInstalled,
					Enabled:     true,
					Manifest:    mf,
				}, nil
			}
		}
		return nil, fmt.Errorf("plugin not found: %s", id)
	}

	var mf Manifest
	_ = json.Unmarshal([]byte(rec.ManifestJSON), &mf)

	return &PluginStatus{
		ID:          rec.ID,
		Name:        rec.Name,
		Version:     rec.Version,
		Description: mf.Description,
		InstallDir:  rec.InstallDir,
		State:       StateInactive,
		Enabled:     rec.Enabled,
		Manifest:    &mf,
	}, nil
}

// List returns the status of all installed plugins.
func (m *Manager) List() ([]PluginStatus, error) {
	records, err := m.store.ListPlugins()
	if err != nil {
		return nil, fmt.Errorf("list plugins from storage: %w", err)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]PluginStatus, 0, len(records))
	seen := make(map[string]bool)

	for _, rec := range records {
		seen[rec.ID] = true
		if inst, ok := m.instances[rec.ID]; ok {
			statuses = append(statuses, *m.statusLocked(inst))
		} else {
			var mf Manifest
			_ = json.Unmarshal([]byte(rec.ManifestJSON), &mf)
			statuses = append(statuses, PluginStatus{
				ID:          rec.ID,
				Name:        rec.Name,
				Version:     rec.Version,
				Description: mf.Description,
				InstallDir:  rec.InstallDir,
				State:       StateInactive,
				Enabled:     rec.Enabled,
				Manifest:    &mf,
			})
		}
	}

	// Also include any in-memory instances not yet in storage
	for id, inst := range m.instances {
		if !seen[id] {
			seen[id] = true
			statuses = append(statuses, *m.statusLocked(inst))
		}
	}

	// Also include any manifests registered directly in the registry
	if m.registry != nil {
		for _, mf := range m.registry.List() {
			if !seen[mf.ID] {
				seen[mf.ID] = true
				statuses = append(statuses, PluginStatus{
					ID:          mf.ID,
					Name:        mf.Name,
					Version:     mf.Version,
					Description: mf.Description,
					State:       StateInstalled,
					Enabled:     true,
					Manifest:    mf,
				})
			}
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].ID < statuses[j].ID
	})

	return statuses, nil
}

// LoadAll loads all installed plugins from storage and activates those marked enabled.
func (m *Manager) LoadAll(ctx context.Context) error {
	records, err := m.store.ListPlugins()
	if err != nil {
		return err
	}

	for _, rec := range records {
		var mf Manifest
		if err := json.Unmarshal([]byte(rec.ManifestJSON), &mf); err == nil {
			_ = m.registry.Register(&mf)
		}

		if rec.Enabled {
			_, _ = m.Activate(ctx, rec.ID)
		}
	}

	return nil
}

// DispatchTranscriptHook sends a transcription event sequentially to all active plugins
// declared with CapTranscriptionHook, chaining any text modifications.
func (m *Manager) DispatchTranscriptHook(ctx context.Context, event TranscriptEvent) (*TranscriptResult, error) {
	m.mu.RLock()
	activeClients := make([]struct {
		id string
		hc *HostClient
	}, 0)

	for id, inst := range m.instances {
		if inst.state == StateActive && inst.hostClient != nil && inst.manifest != nil {
			hasCap := false
			for _, cap := range inst.manifest.Capabilities {
				if cap == CapTranscriptionHook {
					hasCap = true
					break
				}
			}
			if hasCap {
				activeClients = append(activeClients, struct {
					id string
					hc *HostClient
				}{id: id, hc: inst.hostClient})
			}
		}
	}
	m.mu.RUnlock()

	result := &TranscriptResult{
		ModifiedText: event.Text,
	}

	for _, client := range activeClients {
		ev := event
		ev.Text = result.ModifiedText

		payload := map[string]any{
			"session_id":  ev.SessionID,
			"text":        ev.Text,
			"raw_text":    ev.RawText,
			"application": ev.Application,
		}

		resp, err := client.hc.SendCall(ctx, Message{
			Type:    TypeTranscriptHook,
			Payload: payload,
		})
		if err != nil {
			continue
		}

		if mod, ok := resp.Payload["modified_text"].(string); ok && mod != "" {
			result.ModifiedText = mod
		}
		if cancel, ok := resp.Payload["cancel_paste"].(bool); ok && cancel {
			result.CancelPaste = true
			break
		}
		if tags, ok := resp.Payload["tags"].([]any); ok {
			for _, t := range tags {
				if ts, ok := t.(string); ok {
					result.Tags = append(result.Tags, ts)
				}
			}
		}
	}

	return result, nil
}

// DispatchToolCall dispatches a tool execution to the plugin that registered the tool.
func (m *Manager) DispatchToolCall(ctx context.Context, toolName string, args map[string]any) (*ToolCallResult, error) {
	mf, _, found := m.registry.FindTool(toolName)
	if !found {
		return nil, fmt.Errorf("tool %q not registered by any plugin", toolName)
	}

	m.mu.RLock()
	inst, ok := m.instances[mf.ID]
	var client *HostClient
	if ok && inst.state == StateActive {
		client = inst.hostClient
	}
	m.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("plugin %q providing tool %q is not active", mf.ID, toolName)
	}

	resp, err := client.SendCall(ctx, Message{
		Type: TypeToolCall,
		Payload: map[string]any{
			"tool_name": toolName,
			"arguments": args,
		},
	})
	if err != nil {
		return nil, err
	}

	return &ToolCallResult{
		Result: resp.Payload["result"],
		Error:  resp.Error,
	}, nil
}

// DispatchCommand routes a command execution to the registering plugin.
func (m *Manager) DispatchCommand(ctx context.Context, cmdName string, args []string) (*CommandResult, error) {
	mf, _, found := m.registry.FindCommand(cmdName)
	if !found {
		return nil, fmt.Errorf("command %q not registered by any plugin", cmdName)
	}

	m.mu.RLock()
	inst, ok := m.instances[mf.ID]
	var client *HostClient
	if ok && inst.state == StateActive {
		client = inst.hostClient
	}
	m.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("plugin %q providing command %q is not active", mf.ID, cmdName)
	}

	resp, err := client.SendCall(ctx, Message{
		Type: TypeCommand,
		Payload: map[string]any{
			"command": cmdName,
			"args":    args,
		},
	})
	if err != nil {
		return nil, err
	}

	out, _ := resp.Payload["output"].(string)
	return &CommandResult{
		Output: out,
		Error:  resp.Error,
	}, nil
}

func (m *Manager) statusLocked(inst *pluginInstance) *PluginStatus {
	st := &PluginStatus{
		ID:         inst.record.ID,
		Name:       inst.record.Name,
		Version:    inst.record.Version,
		InstallDir: inst.record.InstallDir,
		State:      inst.state,
		Enabled:    inst.record.Enabled,
		Error:      inst.lastError,
		StartedAt:  inst.startedAt,
		Manifest:   inst.manifest,
	}
	if inst.manifest != nil && inst.manifest.Description != "" {
		st.Description = inst.manifest.Description
	}
	if inst.sandbox != nil {
		st.PID = inst.sandbox.PID()
	}
	return st
}

// copyDirectory recursively copies files and directories from src to dst.
func copyDirectory(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
