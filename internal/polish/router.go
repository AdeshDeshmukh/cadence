package polish

import (
	"strings"
	"sync"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
)

// Router automatically maps applications and categories to prompt profiles.
type Router struct {
	mu           sync.RWMutex
	customRoutes map[string]string // app name (lowercased) -> profile name
}

// NewRouter creates a new Router pre-populated with optional custom routes.
func NewRouter(routes map[string]string) *Router {
	r := &Router{
		customRoutes: make(map[string]string),
	}
	for k, v := range routes {
		r.SetRoute(k, v)
	}
	return r
}

// Route resolves the prompt profile name for the given application and category.
func (r *Router) Route(appName string, category appcontext.Category) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normApp := strings.ToLower(strings.TrimSpace(appName))
	if normApp != "" {
		// Exact custom match
		if prof, ok := r.customRoutes[normApp]; ok && prof != "" {
			return prof
		}

		// Substring custom match
		for pattern, prof := range r.customRoutes {
			if strings.Contains(normApp, pattern) && prof != "" {
				return prof
			}
		}
	}

	// Category default mapping
	switch category {
	case appcontext.CategoryCode, appcontext.CategoryTerminal:
		return ProfileNameCode
	case appcontext.CategoryMedical:
		return ProfileNameMedical
	case appcontext.CategoryLegal:
		return ProfileNameLegal
	default:
		return ProfileNameGeneral
	}
}

// SetRoute registers or overrides a route for an application name pattern.
func (r *Router) SetRoute(appPattern, profile string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(appPattern))
	val := strings.ToLower(strings.TrimSpace(profile))
	if key != "" && val != "" {
		r.customRoutes[key] = val
	}
}

// GetRoute returns the custom route for an application pattern, if any.
func (r *Router) GetRoute(appPattern string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(appPattern))
	prof, ok := r.customRoutes[key]
	return prof, ok
}

// ListRoutes returns all configured custom routing rules.
func (r *Router) ListRoutes() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	copyMap := make(map[string]string, len(r.customRoutes))
	for k, v := range r.customRoutes {
		copyMap[k] = v
	}
	return copyMap
}

// DeleteRoute removes a custom route rule.
func (r *Router) DeleteRoute(appPattern string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(appPattern))
	delete(r.customRoutes, key)
}
