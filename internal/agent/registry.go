package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/system"
)

// ToolParam describes a parameter accepted by a tool.
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "string", "int", "bool", "object", "array"
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolDefinition contains full metadata and the execution handler for an agent tool.
type ToolDefinition struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"` // "filesystem", "clipboard", "web", "system", "shell", "utility"
	Description string      `json:"description"`
	Parameters  []ToolParam `json:"parameters"`
	Handler     ToolHandler `json:"-"`
}

// SearchResult represents a single item returned from a web search.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchProvider defines a web search execution function.
type SearchProvider func(ctx context.Context, query string, limit int) ([]SearchResult, error)

// Registry manages tool registration, metadata lookup, and execution.
type Registry struct {
	mu                 sync.RWMutex
	tools              map[string]ToolDefinition
	systemCtrl         system.Controller
	searchProvider     SearchProvider
	permissions        *PermissionEngine
	confirmationPolicy *ConfirmationPolicy
}

// NewRegistry initializes a ToolRegistry pre-populated with standard Cadence agent tools.
func NewRegistry(ctrl system.Controller, searchProvider SearchProvider) *Registry {
	if ctrl == nil {
		ctrl = system.NewDefaultController()
	}
	r := &Registry{
		tools:          make(map[string]ToolDefinition),
		systemCtrl:     ctrl,
		searchProvider: searchProvider,
		permissions:    NewDefaultPermissionEngine(),
	}

	r.registerBuiltinTools()
	return r
}

// SetPermissions configures the active permission engine.
func (r *Registry) SetPermissions(pe *PermissionEngine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissions = pe
}

// Permissions returns the active permission engine.
func (r *Registry) Permissions() *PermissionEngine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.permissions
}

// SetConfirmationPolicy configures the active confirmation policy engine.
func (r *Registry) SetConfirmationPolicy(cp *ConfirmationPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.confirmationPolicy = cp
}

// ConfirmationPolicy returns the active confirmation policy engine.
func (r *Registry) ConfirmationPolicy() *ConfirmationPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.confirmationPolicy
}

// Register registers a tool definition in the registry.
func (r *Registry) Register(tool ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(tool.Name) == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if tool.Handler == nil {
		return fmt.Errorf("tool %q must have an executable handler", tool.Name)
	}
	r.tools[tool.Name] = tool
	if r.permissions != nil {
		_ = r.permissions.Grant(PermissionRule{
			Tool:        tool.Name,
			Action:      "*",
			Level:       PermissionAllow,
			Description: fmt.Sprintf("Automatically allowed registered tool %s", tool.Name),
		})
	}
	return nil
}

// Get retrieves a tool definition by name.
func (r *Registry) Get(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tool definitions in alphabetical order.
func (r *Registry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// Categories returns the unique list of tool categories in the registry.
func (r *Registry) Categories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool)
	var cats []string
	for _, t := range r.tools {
		if !seen[t.Category] && t.Category != "" {
			seen[t.Category] = true
			cats = append(cats, t.Category)
		}
	}
	return cats
}

// Call executes the named tool with the provided parameter map.
func (r *Registry) Call(ctx context.Context, name string, params map[string]any) (ToolResult, error) {
	r.mu.RLock()
	tool, exists := r.tools[name]
	r.mu.RUnlock()

	if !exists {
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool %q is not registered", name),
		}, nil
	}

	// Validate required parameters
	for _, p := range tool.Parameters {
		if p.Required {
			val, ok := params[p.Name]
			if !ok || val == nil || (p.Type == "string" && strings.TrimSpace(fmt.Sprint(val)) == "") {
				return ToolResult{
					Success: false,
					Error:   fmt.Sprintf("missing required parameter %q for tool %q", p.Name, name),
				}, nil
			}
		}
	}

	// Permission enforcement check
	r.mu.RLock()
	perms := r.permissions
	r.mu.RUnlock()

	if perms != nil {
		action := ""
		if params != nil {
			if a, ok := params["action"].(string); ok {
				action = a
			}
		}
		decision := perms.Check(name, action, params)
		if !decision.Granted {
			return ToolResult{
				Success: false,
				Error:   fmt.Sprintf("permission denied: %s", decision.Reason),
			}, nil
		}
	}

	// Confirmation policy enforcement
	r.mu.RLock()
	cPol := r.confirmationPolicy
	r.mu.RUnlock()

	if cPol != nil {
		action := ""
		if params != nil {
			if a, ok := params["action"].(string); ok {
				action = a
			}
		}
		decision := cPol.Evaluate(name, action, params)
		if !decision.Allowed {
			if decision.RequiresConfirmation {
				return ToolResult{
					Success: false,
					Error:   fmt.Sprintf("confirmation required (%s): %s", decision.RiskLevel, decision.Reason),
				}, nil
			}
			return ToolResult{
				Success: false,
				Error:   fmt.Sprintf("action prohibited by confirmation policy (%s): %s", decision.RiskLevel, decision.Reason),
			}, nil
		}
	}

	return tool.Handler(ctx, params)
}

// ExportJSONSchema exports registered tools in standard function calling schema.
func (r *Registry) ExportJSONSchema() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var schemas []map[string]any
	for _, tool := range r.tools {
		properties := make(map[string]any)
		var required []string

		for _, p := range tool.Parameters {
			properties[p.Name] = map[string]any{
				"type":        p.Type,
				"description": p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}

		schema := map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		}
		schemas = append(schemas, schema)
	}
	return schemas
}

func (r *Registry) registerBuiltinTools() {
	// 1. Filesystem Tool
	_ = r.Register(ToolDefinition{
		Name:        "fs",
		Category:    "filesystem",
		Description: "Read, write, list, and search files and directories on the local filesystem.",
		Parameters: []ToolParam{
			{Name: "action", Type: "string", Description: "Action to perform: read, write, list, search, or info", Required: true},
			{Name: "path", Type: "string", Description: "Target file or directory path", Required: true},
			{Name: "content", Type: "string", Description: "Content to write (for write action)", Required: false},
			{Name: "pattern", Type: "string", Description: "Glob pattern or keyword for search (for search action)", Required: false},
		},
		Handler: r.handleFS,
	})

	// 2. Clipboard Tool
	_ = r.Register(ToolDefinition{
		Name:        "clipboard",
		Category:    "clipboard",
		Description: "Inspect, copy text to, or clear the system clipboard.",
		Parameters: []ToolParam{
			{Name: "action", Type: "string", Description: "Action to perform: get, set, or clear", Required: true},
			{Name: "text", Type: "string", Description: "Text string to write to clipboard (for set action)", Required: false},
		},
		Handler: r.handleClipboard,
	})

	// 3. Web Search Tool
	_ = r.Register(ToolDefinition{
		Name:        "web_search",
		Category:    "web",
		Description: "Search the web for query terms and retrieve relevant summary snippets and URLs.",
		Parameters: []ToolParam{
			{Name: "query", Type: "string", Description: "Search query string", Required: true},
			{Name: "limit", Type: "int", Description: "Maximum number of search results (default 5)", Required: false},
		},
		Handler: r.handleWebSearch,
	})

	// 4. System Info Tool
	_ = r.Register(ToolDefinition{
		Name:        "system_info",
		Category:    "system",
		Description: "Inspect OS environment, battery status, network IP/interfaces, and running host details.",
		Parameters: []ToolParam{
			{Name: "action", Type: "string", Description: "Action: os, battery, network, or hostname", Required: true},
		},
		Handler: r.handleSystemInfo,
	})

	// 5. System Action Tool (app, window, screenshot, volume)
	_ = r.Register(ToolDefinition{
		Name:        "system",
		Category:    "system",
		Description: "Desktop application management, window manipulation, screenshots, and volume control.",
		Parameters: []ToolParam{
			{Name: "action", Type: "string", Description: "Action: launch, focus, quit, window, screenshot, volume", Required: true},
			{Name: "target", Type: "string", Description: "Application name or window action", Required: false},
			{Name: "clipboard", Type: "bool", Description: "Copy screenshot to clipboard", Required: false},
			{Name: "window", Type: "bool", Description: "Screenshot active window only", Required: false},
			{Name: "path", Type: "string", Description: "Path to save screenshot", Required: false},
			{Name: "mute", Type: "bool", Description: "Mute or unmute volume", Required: false},
			{Name: "level", Type: "int", Description: "Volume level 0-100", Required: false},
		},
		Handler: r.handleSystemAction,
	})

	// 6. Shell execution tool
	_ = r.Register(ToolDefinition{
		Name:        "exec",
		Category:    "shell",
		Description: "Safely execute a shell command line string and return its output.",
		Parameters: []ToolParam{
			{Name: "command", Type: "string", Description: "Command string to run", Required: true},
		},
		Handler: r.handleExec,
	})

	// 7. Echo utility tool
	_ = r.Register(ToolDefinition{
		Name:        "echo",
		Category:    "utility",
		Description: "Echoes back the provided message for diagnostics and pipeline validation.",
		Parameters: []ToolParam{
			{Name: "message", Type: "string", Description: "Message string to echo", Required: true},
		},
		Handler: func(_ context.Context, params map[string]any) (ToolResult, error) {
			msg, _ := params["message"].(string)
			return ToolResult{Success: true, Output: msg}, nil
		},
	})
}

func (r *Registry) handleFS(_ context.Context, params map[string]any) (ToolResult, error) {
	action, _ := params["action"].(string)
	p, _ := params["path"].(string)
	p = strings.TrimSpace(p)

	switch strings.ToLower(action) {
	case "read":
		if p == "" {
			return ToolResult{Success: false, Error: "missing file path"}, nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		const maxRead = 256 * 1024
		content := string(data)
		truncated := false
		if len(content) > maxRead {
			content = content[:maxRead] + "\n...[truncated]"
			truncated = true
		}
		lines := len(strings.Split(string(data), "\n"))
		return ToolResult{
			Success: true,
			Output:  content,
			Data: map[string]any{
				"path":      p,
				"bytes":     len(data),
				"lines":     lines,
				"truncated": truncated,
			},
		}, nil

	case "write":
		if p == "" {
			return ToolResult{Success: false, Error: "missing file path"}, nil
		}
		content, _ := params["content"].(string)
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Wrote %d bytes to %s", len(content), p),
			Data: map[string]any{
				"path":  p,
				"bytes": len(content),
			},
		}, nil

	case "list":
		if p == "" {
			p = "."
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		var names []string
		for _, e := range entries {
			typeIndicator := ""
			if e.IsDir() {
				typeIndicator = "/"
			}
			names = append(names, e.Name()+typeIndicator)
		}
		return ToolResult{
			Success: true,
			Output:  strings.Join(names, "\n"),
			Data: map[string]any{
				"path":  p,
				"count": len(entries),
				"items": names,
			},
		}, nil

	case "search":
		pattern, _ := params["pattern"].(string)
		if pattern == "" {
			pattern = "*"
		}
		if p == "" {
			p = "."
		}
		var matched []string
		_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if len(matched) >= 100 {
				return filepath.SkipDir
			}
			if m, _ := filepath.Match(pattern, info.Name()); m {
				matched = append(matched, path)
			}
			return nil
		})
		return ToolResult{
			Success: true,
			Output:  strings.Join(matched, "\n"),
			Data: map[string]any{
				"pattern": pattern,
				"count":   len(matched),
				"matches": matched,
			},
		}, nil

	case "info":
		if p == "" {
			return ToolResult{Success: false, Error: "missing file path"}, nil
		}
		fi, err := os.Stat(p)
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		out := fmt.Sprintf("%s (Size: %d bytes, Mode: %s, IsDir: %v, Modified: %s)",
			fi.Name(), fi.Size(), fi.Mode(), fi.IsDir(), fi.ModTime().Format(time.RFC3339))
		return ToolResult{
			Success: true,
			Output:  out,
			Data: map[string]any{
				"name":     fi.Name(),
				"size":     fi.Size(),
				"is_dir":   fi.IsDir(),
				"mod_time": fi.ModTime().Format(time.RFC3339),
			},
		}, nil

	default:
		return ToolResult{Success: false, Error: fmt.Sprintf("unsupported fs action: %s", action)}, nil
	}
}

func (r *Registry) handleClipboard(_ context.Context, params map[string]any) (ToolResult, error) {
	action, _ := params["action"].(string)
	switch strings.ToLower(action) {
	case "get":
		text, err := paste.GetSystemClipboard()
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{
			Success: true,
			Output:  text,
			Data:    map[string]any{"text": text, "length": len(text)},
		}, nil

	case "set":
		text, _ := params["text"].(string)
		if err := paste.SetSystemClipboard(text); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Copied %d character(s) to clipboard", len(text)),
			Data:    map[string]any{"length": len(text)},
		}, nil

	case "clear":
		if err := paste.SetSystemClipboard(""); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{
			Success: true,
			Output:  "Clipboard cleared",
		}, nil

	default:
		return ToolResult{Success: false, Error: fmt.Sprintf("unsupported clipboard action: %s", action)}, nil
	}
}

func (r *Registry) handleWebSearch(ctx context.Context, params map[string]any) (ToolResult, error) {
	query, _ := params["query"].(string)
	if strings.TrimSpace(query) == "" {
		return ToolResult{Success: false, Error: "missing search query"}, nil
	}
	limit := 5
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if r.searchProvider != nil {
		results, err := r.searchProvider(ctx, query, limit)
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		var lines []string
		for idx, item := range results {
			lines = append(lines, fmt.Sprintf("%d. %s (%s)\n   %s", idx+1, item.Title, item.URL, item.Snippet))
		}
		return ToolResult{
			Success: true,
			Output:  strings.Join(lines, "\n"),
			Data: map[string]any{
				"query":   query,
				"count":   len(results),
				"results": results,
			},
		}, nil
	}

	// Default offline search summary response
	simulatedResults := []SearchResult{
		{
			Title:   fmt.Sprintf("Information about %q", query),
			URL:     fmt.Sprintf("https://duckduckgo.com/?q=%s", strings.ReplaceAll(query, " ", "+")),
			Snippet: fmt.Sprintf("Summary results for query %q. Online provider not configured; search query captured.", query),
		},
	}
	return ToolResult{
		Success: true,
		Output:  fmt.Sprintf("1. %s (%s)\n   %s", simulatedResults[0].Title, simulatedResults[0].URL, simulatedResults[0].Snippet),
		Data: map[string]any{
			"query":   query,
			"count":   1,
			"results": simulatedResults,
		},
	}, nil
}

func (r *Registry) handleSystemInfo(_ context.Context, params map[string]any) (ToolResult, error) {
	action, _ := params["action"].(string)
	switch strings.ToLower(action) {
	case "os":
		info := fmt.Sprintf("OS: %s, Arch: %s, CPUs: %d, Go: %s",
			runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
		return ToolResult{
			Success: true,
			Output:  info,
			Data: map[string]any{
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
				"cpus":    runtime.NumCPU(),
				"version": runtime.Version(),
			},
		}, nil

	case "hostname":
		h, err := os.Hostname()
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{
			Success: true,
			Output:  h,
			Data:    map[string]any{"hostname": h},
		}, nil

	case "network":
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		var ips []string
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
		out := "No external IP detected"
		if len(ips) > 0 {
			out = strings.Join(ips, ", ")
		}
		return ToolResult{
			Success: true,
			Output:  out,
			Data:    map[string]any{"ips": ips},
		}, nil

	case "battery":
		// On macOS, try pmset -g batt
		if runtime.GOOS == "darwin" {
			cmd := exec.Command("pmset", "-g", "batt")
			if out, err := cmd.Output(); err == nil {
				return ToolResult{Success: true, Output: strings.TrimSpace(string(out))}, nil
			}
		}
		return ToolResult{
			Success: true,
			Output:  "Battery status: AC Power (simulator/default)",
		}, nil

	default:
		return ToolResult{Success: false, Error: fmt.Sprintf("unsupported system_info action: %s", action)}, nil
	}
}

func (r *Registry) handleSystemAction(ctx context.Context, params map[string]any) (ToolResult, error) {
	action, _ := params["action"].(string)
	target, _ := params["target"].(string)

	switch strings.ToLower(action) {
	case "launch":
		if err := r.systemCtrl.LaunchApp(ctx, target); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Launched %s", target)}, nil

	case "focus":
		if err := r.systemCtrl.FocusApp(ctx, target); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Switched to %s", target)}, nil

	case "quit":
		if err := r.systemCtrl.QuitApp(ctx, target); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Quit %s", target)}, nil

	case "window":
		wAct := system.WindowAction(target)
		if err := r.systemCtrl.ControlWindow(ctx, wAct); err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Window action executed: %s", target)}, nil

	case "screenshot":
		clip, _ := params["clipboard"].(bool)
		win, _ := params["window"].(bool)
		path, _ := params["path"].(string)
		res, err := r.systemCtrl.CaptureScreenshot(ctx, system.ScreenshotOptions{
			ActiveWindowOnly: win,
			CopyToClipboard:  clip,
			SavePath:         path,
		})
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}, nil
		}
		msg := fmt.Sprintf("Screenshot saved to %s", res.Path)
		if res.CopiedToClipboard {
			msg = "Screenshot copied to clipboard"
		}
		return ToolResult{Success: true, Output: msg}, nil

	case "volume":
		if m, ok := params["mute"].(bool); ok {
			if err := r.systemCtrl.MuteVolume(ctx, m); err != nil {
				return ToolResult{Success: false, Error: err.Error()}, nil
			}
			if m {
				return ToolResult{Success: true, Output: "Volume muted"}, nil
			}
			return ToolResult{Success: true, Output: "Volume unmuted"}, nil
		}
		if lvl, ok := params["level"].(float64); ok {
			if err := r.systemCtrl.SetVolume(ctx, int(lvl)); err != nil {
				return ToolResult{Success: false, Error: err.Error()}, nil
			}
			return ToolResult{Success: true, Output: fmt.Sprintf("Volume set to %d%%", int(lvl))}, nil
		}
		return ToolResult{Success: false, Error: "missing volume parameter"}, nil

	default:
		return ToolResult{Success: false, Error: fmt.Sprintf("unknown system action: %s", action)}, nil
	}
}

func (r *Registry) handleExec(ctx context.Context, params map[string]any) (ToolResult, error) {
	cmdStr, _ := params["command"].(string)
	if strings.TrimSpace(cmdStr) == "" {
		return ToolResult{Success: false, Error: "missing command parameter"}, nil
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  string(out),
			Error:   err.Error(),
		}, nil
	}
	return ToolResult{
		Success: true,
		Output:  strings.TrimSpace(string(out)),
	}, nil
}
