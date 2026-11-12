package plugins

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// Registry tracks all registered plugins and enables capability lookup.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Manifest
}

// NewRegistry initializes an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*Manifest),
	}
}

// Register validates and adds a plugin manifest to the registry.
func (r *Registry) Register(m *Manifest) error {
	if err := ValidateManifest(m); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.plugins[m.ID] = m
	return nil
}

// Get returns the plugin manifest for the given ID.
func (r *Registry) Get(id string) (*Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.plugins[id]
	return m, ok
}

// List returns all registered manifests sorted by ID.
func (r *Registry) List() []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]*Manifest, 0, len(r.plugins))
	for _, m := range r.plugins {
		res = append(res, m)
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].ID < res[j].ID
	})
	return res
}

// Unregister removes a plugin by ID.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.plugins[id]; !ok {
		return errors.New("plugin not found: " + id)
	}
	delete(r.plugins, id)
	return nil
}

// FindTool searches all registered plugins for an agent tool matching name.
func (r *Registry) FindTool(toolName string) (*Manifest, *ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.plugins {
		for _, t := range m.Tools {
			if strings.EqualFold(t.Name, toolName) {
				return m, &t, true
			}
		}
	}
	return nil, nil, false
}

// FindCommand searches all registered plugins for a command matching name or voice trigger.
func (r *Registry) FindCommand(trigger string) (*Manifest, *CommandDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	triggerLower := strings.ToLower(strings.TrimSpace(trigger))
	for _, m := range r.plugins {
		for _, c := range m.Commands {
			if strings.ToLower(c.Name) == triggerLower {
				return m, &c, true
			}
			for _, a := range c.Aliases {
				if strings.ToLower(a) == triggerLower {
					return m, &c, true
				}
			}
			for _, vt := range c.VoiceTriggers {
				if strings.ToLower(vt) == triggerLower {
					return m, &c, true
				}
			}
		}
	}
	return nil, nil, false
}
