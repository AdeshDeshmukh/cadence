package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PermissionLevel represents the authorization degree for a tool invocation.
type PermissionLevel string

const (
	PermissionAllow PermissionLevel = "allow"
	PermissionDeny  PermissionLevel = "deny"
	PermissionAsk   PermissionLevel = "ask"
)

// IsValidPermissionLevel validates whether a string is an accepted permission level.
func IsValidPermissionLevel(lvl string) bool {
	switch PermissionLevel(strings.ToLower(strings.TrimSpace(lvl))) {
	case PermissionAllow, PermissionDeny, PermissionAsk:
		return true
	default:
		return false
	}
}

// PermissionRule defines access control criteria for a tool or action.
type PermissionRule struct {
	Tool        string          `json:"tool"`             // Tool name (e.g., "fs", "exec", "clipboard", "system") or "*"
	Action      string          `json:"action,omitempty"` // Sub-action (e.g., "read", "write", "list") or "*"
	Level       PermissionLevel `json:"level"`            // "allow", "deny", or "ask"
	Scope       []string        `json:"scope,omitempty"`  // Allowed directory paths or allowed command prefixes
	Description string          `json:"description,omitempty"`
}

// PermissionDecision represents the outcome of evaluating a tool invocation against active policies.
type PermissionDecision struct {
	Granted      bool            `json:"granted"`
	Level        PermissionLevel `json:"level"`
	Reason       string          `json:"reason"`
	MatchedRule  *PermissionRule `json:"matched_rule,omitempty"`
	RequiresUser bool            `json:"requires_user"`
}

var sensitiveSystemPathSubstrings = []string{
	"/etc/shadow",
	"/etc/sudoers",
	"/etc/master.passwd",
	".ssh/id_",
	".ssh/authorized_keys",
	".gnupg/",
	".aws/credentials",
	".kube/config",
}

// PermissionEngine enforces per-tool, per-action, and scoped access control for Cadence agents.
type PermissionEngine struct {
	mu           sync.RWMutex
	rules        []PermissionRule
	defaultLevel PermissionLevel
}

// NewPermissionEngine creates a permission engine with a custom default policy and initial rules.
func NewPermissionEngine(defaultLevel PermissionLevel, rules ...PermissionRule) *PermissionEngine {
	if !IsValidPermissionLevel(string(defaultLevel)) {
		defaultLevel = PermissionAsk
	}
	pe := &PermissionEngine{
		defaultLevel: defaultLevel,
		rules:        make([]PermissionRule, 0, len(rules)),
	}
	for _, r := range rules {
		_ = pe.Grant(r)
	}
	return pe
}

// NewDefaultPermissionEngine initializes a permission engine with standard safe Cadence defaults.
func NewDefaultPermissionEngine() *PermissionEngine {
	pe := &PermissionEngine{
		defaultLevel: PermissionAsk,
		rules:        make([]PermissionRule, 0),
	}
	for _, r := range DefaultRules() {
		_ = pe.Grant(r)
	}
	return pe
}

// DefaultRules returns Cadence's out-of-the-box safe permission rules.
func DefaultRules() []PermissionRule {
	return []PermissionRule{
		// Safe utilities and standard system tools are pre-approved
		{Tool: "echo", Action: "*", Level: PermissionAllow, Description: "Echo diagnostic tool"},
		{Tool: "system_info", Action: "*", Level: PermissionAllow, Description: "System telemetry and battery info"},
		{Tool: "web_search", Action: "*", Level: PermissionAllow, Description: "Web search and documentation lookup"},
		{Tool: "fs", Action: "*", Level: PermissionAllow, Description: "Filesystem access (guarded against sensitive paths)"},
		{Tool: "clipboard", Action: "*", Level: PermissionAllow, Description: "System clipboard access"},
		{Tool: "system", Action: "*", Level: PermissionAllow, Description: "System automation and app control"},

		// Arbitrary shell command execution requires confirmation by default
		{Tool: "exec", Action: "*", Level: PermissionAsk, Description: "Arbitrary shell command execution"},
	}
}

// Grant adds or updates an explicit rule. Specific rules override generic or wildcard rules.
func (pe *PermissionEngine) Grant(rule PermissionRule) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	tool := strings.ToLower(strings.TrimSpace(rule.Tool))
	if tool == "" {
		return errors.New("tool name cannot be empty")
	}
	action := strings.ToLower(strings.TrimSpace(rule.Action))
	if action == "" {
		action = "*"
	}
	lvl := PermissionLevel(strings.ToLower(strings.TrimSpace(string(rule.Level))))
	if !IsValidPermissionLevel(string(lvl)) {
		lvl = PermissionAllow
	}

	normalized := PermissionRule{
		Tool:        tool,
		Action:      action,
		Level:       lvl,
		Scope:       rule.Scope,
		Description: rule.Description,
	}

	// Update existing matching rule if tool and action match
	for i, existing := range pe.rules {
		if existing.Tool == tool && existing.Action == action {
			pe.rules[i] = normalized
			return nil
		}
	}

	pe.rules = append(pe.rules, normalized)
	return nil
}

// Revoke removes an explicit rule matching tool and action.
func (pe *PermissionEngine) Revoke(tool, action string) bool {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	t := strings.ToLower(strings.TrimSpace(tool))
	a := strings.ToLower(strings.TrimSpace(action))
	if a == "" {
		a = "*"
	}

	idx := -1
	for i, r := range pe.rules {
		if r.Tool == t && r.Action == a {
			idx = i
			break
		}
	}
	if idx >= 0 {
		pe.rules = append(pe.rules[:idx], pe.rules[idx+1:]...)
		return true
	}
	return false
}

// Rules returns a snapshot slice of all active permission rules.
func (pe *PermissionEngine) Rules() []PermissionRule {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	copied := make([]PermissionRule, len(pe.rules))
	copy(copied, pe.rules)
	return copied
}

// DefaultLevel returns the fallback permission level.
func (pe *PermissionEngine) DefaultLevel() PermissionLevel {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.defaultLevel
}

// SetDefaultLevel updates the fallback permission level.
func (pe *PermissionEngine) SetDefaultLevel(lvl PermissionLevel) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if IsValidPermissionLevel(string(lvl)) {
		pe.defaultLevel = lvl
	}
}

// Check evaluates whether a tool invocation with parameters is authorized.
func (pe *PermissionEngine) Check(tool, action string, params map[string]any) PermissionDecision {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	t := strings.ToLower(strings.TrimSpace(tool))
	a := strings.ToLower(strings.TrimSpace(action))
	if a == "" && params != nil {
		if act, ok := params["action"].(string); ok {
			a = strings.ToLower(strings.TrimSpace(act))
		}
	}
	if a == "" {
		a = "*"
	}

	// 1. Safety check for sensitive system paths in filesystem operations
	if t == "fs" {
		targetPath := extractPathFromParams(params)
		if isSensitiveSystemPath(targetPath) {
			// Find if there is an explicit allow rule specifically for this exact tool and action
			var explicitRule *PermissionRule
			for _, r := range pe.rules {
				if r.Tool == t && (r.Action == a || r.Action == "*") && r.Level == PermissionAllow {
					explicitRule = &r
					break
				}
			}
			if explicitRule == nil || len(explicitRule.Scope) == 0 {
				return PermissionDecision{
					Granted:      false,
					Level:        PermissionDeny,
					Reason:       fmt.Sprintf("access to sensitive system path %q is denied by security policy", targetPath),
					RequiresUser: false,
				}
			}
		}
	}

	// 2. Match rules with specificity precedence:
	//    (1) tool == t && action == a
	//    (2) tool == t && action == "*"
	//    (3) tool == "*" && action == a
	//    (4) tool == "*" && action == "*"
	var matched *PermissionRule
	matchPrecedence := 0

	for _, rule := range pe.rules {
		rTool := strings.ToLower(rule.Tool)
		rAct := strings.ToLower(rule.Action)

		if rTool == t && rAct == a {
			ruleCopy := rule
			matched = &ruleCopy
			matchPrecedence = 4
			break
		} else if rTool == t && rAct == "*" && matchPrecedence < 3 {
			ruleCopy := rule
			matched = &ruleCopy
			matchPrecedence = 3
		} else if rTool == "*" && rAct == a && matchPrecedence < 2 {
			ruleCopy := rule
			matched = &ruleCopy
			matchPrecedence = 2
		} else if rTool == "*" && rAct == "*" && matchPrecedence < 1 {
			ruleCopy := rule
			matched = &ruleCopy
			matchPrecedence = 1
		}
	}

	effectiveLevel := pe.defaultLevel
	var matchedRule *PermissionRule
	if matched != nil {
		effectiveLevel = matched.Level
		matchedRule = matched
	}

	// 3. Scope validation if rule specifies scopes
	if matchedRule != nil && len(matchedRule.Scope) > 0 {
		if t == "fs" {
			targetPath := extractPathFromParams(params)
			if !isPathInAllowedScopes(targetPath, matchedRule.Scope) {
				return PermissionDecision{
					Granted:      false,
					Level:        PermissionDeny,
					Reason:       fmt.Sprintf("target path %q is outside allowed scope (%s)", targetPath, strings.Join(matchedRule.Scope, ", ")),
					MatchedRule:  matchedRule,
					RequiresUser: false,
				}
			}
		} else if t == "exec" {
			cmdStr := ""
			if c, ok := params["command"].(string); ok {
				cmdStr = strings.TrimSpace(c)
			}
			if !isCommandInAllowedScopes(cmdStr, matchedRule.Scope) {
				return PermissionDecision{
					Granted:      false,
					Level:        PermissionDeny,
					Reason:       fmt.Sprintf("command %q does not match permitted command scopes (%s)", cmdStr, strings.Join(matchedRule.Scope, ", ")),
					MatchedRule:  matchedRule,
					RequiresUser: false,
				}
			}
		}
	}

	// 4. Check user approval override in parameters (e.g. interactive confirmation)
	isApproved := false
	if params != nil {
		if app, ok := params["approved"].(bool); ok && app {
			isApproved = true
		}
	}

	switch effectiveLevel {
	case PermissionAllow:
		return PermissionDecision{
			Granted:      true,
			Level:        PermissionAllow,
			Reason:       fmt.Sprintf("tool %s action %s is authorized by policy", t, a),
			MatchedRule:  matchedRule,
			RequiresUser: false,
		}

	case PermissionDeny:
		return PermissionDecision{
			Granted:      false,
			Level:        PermissionDeny,
			Reason:       fmt.Sprintf("tool %s action %s is explicitly denied by policy", t, a),
			MatchedRule:  matchedRule,
			RequiresUser: false,
		}

	case PermissionAsk:
		if isApproved {
			return PermissionDecision{
				Granted:      true,
				Level:        PermissionAsk,
				Reason:       fmt.Sprintf("tool %s action %s authorized via user confirmation", t, a),
				MatchedRule:  matchedRule,
				RequiresUser: false,
			}
		}
		return PermissionDecision{
			Granted:      false,
			Level:        PermissionAsk,
			Reason:       fmt.Sprintf("confirmation required for tool %s action %s", t, a),
			MatchedRule:  matchedRule,
			RequiresUser: true,
		}

	default:
		return PermissionDecision{
			Granted:      false,
			Level:        PermissionDeny,
			Reason:       fmt.Sprintf("unrecognized permission level: %s", effectiveLevel),
			RequiresUser: false,
		}
	}
}

// extractPathFromParams finds path references from standard parameter field names.
func extractPathFromParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	for _, key := range []string{"path", "target", "file", "filepath", "source", "pattern"} {
		if val, ok := params[key].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

// isSensitiveSystemPath detects attempts to touch forbidden system credential or security files.
func isSensitiveSystemPath(targetPath string) bool {
	if targetPath == "" {
		return false
	}
	cleaned := filepath.Clean(targetPath)
	lower := strings.ToLower(cleaned)

	for _, sub := range sensitiveSystemPathSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// isPathInAllowedScopes checks if targetPath is a child of any scope prefix.
func isPathInAllowedScopes(targetPath string, allowedScopes []string) bool {
	if len(allowedScopes) == 0 {
		return true
	}
	if targetPath == "" {
		return false
	}

	cleanTarget := filepath.Clean(targetPath)
	if !filepath.IsAbs(cleanTarget) {
		if wd, err := os.Getwd(); err == nil {
			cleanTarget = filepath.Join(wd, cleanTarget)
		}
	}

	for _, scope := range allowedScopes {
		cleanScope := filepath.Clean(scope)
		if !filepath.IsAbs(cleanScope) {
			if wd, err := os.Getwd(); err == nil {
				cleanScope = filepath.Join(wd, cleanScope)
			}
		}
		rel, err := filepath.Rel(cleanScope, cleanTarget)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// isCommandInAllowedScopes verifies if cmd starts with any permitted command token.
func isCommandInAllowedScopes(cmd string, allowedScopes []string) bool {
	if len(allowedScopes) == 0 {
		return true
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	firstToken := fields[0]

	for _, scope := range allowedScopes {
		cleanScope := strings.TrimSpace(scope)
		if strings.EqualFold(firstToken, cleanScope) || strings.HasPrefix(cmd, cleanScope) {
			return true
		}
	}
	return false
}
