package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/AdeshDeshmukh/cadence/internal/developer"
)

// ConfirmationMode represents the policy determining when agent actions require explicit user approval.
type ConfirmationMode string

const (
	// ModeAlwaysAsk requires explicit confirmation for all non-trivial tool actions.
	ModeAlwaysAsk ConfirmationMode = "always_ask"

	// ModeAutoApproveSafe automatically executes safe (read-only) actions, prompting for risky and destructive actions (Default).
	ModeAutoApproveSafe ConfirmationMode = "auto_approve_safe"

	// ModeAutoApproveRisky automatically executes safe and moderate-risk actions, prompting only for destructive actions.
	ModeAutoApproveRisky ConfirmationMode = "auto_approve_risky"

	// ModeAutoApproveAll auto-approves all actions regardless of risk (autonomous/unattended mode).
	ModeAutoApproveAll ConfirmationMode = "auto_approve_all"

	// ModeReadOnly permits only safe read-only actions; all risky and destructive actions are immediately blocked.
	ModeReadOnly ConfirmationMode = "read_only"
)

// IsValidConfirmationMode checks whether a string is a valid confirmation mode.
func IsValidConfirmationMode(m string) bool {
	switch ConfirmationMode(strings.ToLower(strings.TrimSpace(m))) {
	case ModeAlwaysAsk, ModeAutoApproveSafe, ModeAutoApproveRisky, ModeAutoApproveAll, ModeReadOnly:
		return true
	default:
		return false
	}
}

// ConfirmationDecision holds the outcome of evaluating a tool action against the active confirmation policy.
type ConfirmationDecision struct {
	Allowed              bool                `json:"allowed"`
	RequiresConfirmation bool                `json:"requires_confirmation"`
	RiskLevel            developer.RiskLevel `json:"risk_level"`
	Reason               string              `json:"reason"`
	Prompt               string              `json:"prompt,omitempty"`
}

// ConfirmationPolicy manages action authorization rules based on evaluated risk tiers.
type ConfirmationPolicy struct {
	mu   sync.RWMutex
	mode ConfirmationMode
}

// NewConfirmationPolicy creates a ConfirmationPolicy with a specified mode.
func NewConfirmationPolicy(mode ConfirmationMode) *ConfirmationPolicy {
	if !IsValidConfirmationMode(string(mode)) {
		mode = ModeAutoApproveSafe
	}
	return &ConfirmationPolicy{
		mode: mode,
	}
}

// DefaultConfirmationPolicy initializes a ConfirmationPolicy in ModeAutoApproveSafe.
func DefaultConfirmationPolicy() *ConfirmationPolicy {
	return NewConfirmationPolicy(ModeAutoApproveSafe)
}

// Mode returns the active confirmation policy mode.
func (cp *ConfirmationPolicy) Mode() ConfirmationMode {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.mode
}

// SetMode updates the active confirmation mode.
func (cp *ConfirmationPolicy) SetMode(mode ConfirmationMode) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	norm := ConfirmationMode(strings.ToLower(strings.TrimSpace(string(mode))))
	if !IsValidConfirmationMode(string(norm)) {
		return fmt.Errorf("invalid confirmation mode %q (supported: %s, %s, %s, %s, %s)",
			mode, ModeAlwaysAsk, ModeAutoApproveSafe, ModeAutoApproveRisky, ModeAutoApproveAll, ModeReadOnly)
	}
	cp.mode = norm
	return nil
}

// AssessRisk calculates the developer.RiskAssessment for a tool and action invocation.
func (cp *ConfirmationPolicy) AssessRisk(tool, action string, params map[string]any) developer.RiskAssessment {
	t := strings.ToLower(strings.TrimSpace(tool))
	a := strings.ToLower(strings.TrimSpace(action))
	if a == "" && params != nil {
		if act, ok := params["action"].(string); ok {
			a = strings.ToLower(strings.TrimSpace(act))
		}
	}

	switch t {
	case "exec":
		cmdStr := ""
		if params != nil {
			if c, ok := params["command"].(string); ok {
				cmdStr = strings.TrimSpace(c)
			}
		}
		if cmdStr == "" {
			return developer.RiskAssessment{
				Level:  developer.RiskRisky,
				Reason: "Shell execution without specific command",
			}
		}
		return developer.ClassifyCommand(cmdStr)

	case "fs":
		switch a {
		case "read", "list", "info", "search":
			return developer.RiskAssessment{
				Level:  developer.RiskSafe,
				Reason: fmt.Sprintf("Read-only filesystem operation (%s)", a),
			}
		case "write":
			return developer.RiskAssessment{
				Level:  developer.RiskRisky,
				Reason: "Filesystem write / modification",
			}
		case "delete", "remove":
			return developer.RiskAssessment{
				Level:  developer.RiskDestructive,
				Reason: "Permanent filesystem file deletion",
			}
		default:
			return developer.RiskAssessment{
				Level:  developer.RiskRisky,
				Reason: fmt.Sprintf("Filesystem operation (%s)", a),
			}
		}

	case "system":
		switch a {
		case "status", "focus", "volume":
			return developer.RiskAssessment{
				Level:  developer.RiskSafe,
				Reason: fmt.Sprintf("System inspection / window focus (%s)", a),
			}
		case "launch", "screenshot":
			return developer.RiskAssessment{
				Level:  developer.RiskRisky,
				Reason: fmt.Sprintf("Desktop application launch or capture (%s)", a),
			}
		case "quit":
			return developer.RiskAssessment{
				Level:  developer.RiskDestructive,
				Reason: "Application termination / process quit",
			}
		default:
			return developer.RiskAssessment{
				Level:  developer.RiskRisky,
				Reason: fmt.Sprintf("System controller action (%s)", a),
			}
		}

	case "clipboard":
		switch a {
		case "get", "set":
			return developer.RiskAssessment{
				Level:  developer.RiskSafe,
				Reason: fmt.Sprintf("Clipboard access (%s)", a),
			}
		case "clear":
			return developer.RiskAssessment{
				Level:  developer.RiskRisky,
				Reason: "Clipboard purge / clear buffer",
			}
		default:
			return developer.RiskAssessment{
				Level:  developer.RiskSafe,
				Reason: "Clipboard operation",
			}
		}

	case "web_search", "echo", "system_info":
		return developer.RiskAssessment{
			Level:  developer.RiskSafe,
			Reason: fmt.Sprintf("Non-destructive utility / telemetry (%s)", t),
		}

	default:
		return developer.RiskAssessment{
			Level:  developer.RiskRisky,
			Reason: fmt.Sprintf("Custom tool invocation (%s)", t),
		}
	}
}

// Evaluate evaluates a tool invocation against the active confirmation policy and returns a decision.
func (cp *ConfirmationPolicy) Evaluate(tool, action string, params map[string]any) ConfirmationDecision {
	cp.mu.RLock()
	mode := cp.mode
	cp.mu.RUnlock()

	assessment := cp.AssessRisk(tool, action, params)
	level := assessment.Level

	// Check for explicit approved flag in params
	isApproved := false
	if params != nil {
		if app, ok := params["approved"].(bool); ok && app {
			isApproved = true
		}
	}

	if isApproved {
		return ConfirmationDecision{
			Allowed:              true,
			RequiresConfirmation: false,
			RiskLevel:            level,
			Reason:               fmt.Sprintf("Action approved by caller (risk: %s)", level),
		}
	}

	promptMsg := fmt.Sprintf("Execute %s action %s? (Risk: %s - %s)", tool, action, level, assessment.Reason)

	switch mode {
	case ModeAutoApproveAll:
		return ConfirmationDecision{
			Allowed:              true,
			RequiresConfirmation: false,
			RiskLevel:            level,
			Reason:               fmt.Sprintf("Auto-approved under %s policy (risk: %s)", mode, level),
		}

	case ModeReadOnly:
		if level == developer.RiskSafe {
			return ConfirmationDecision{
				Allowed:              true,
				RequiresConfirmation: false,
				RiskLevel:            level,
				Reason:               "Safe read-only action approved",
			}
		}
		return ConfirmationDecision{
			Allowed:              false,
			RequiresConfirmation: false,
			RiskLevel:            level,
			Reason:               fmt.Sprintf("Action rejected: %s risk prohibited in read-only mode", level),
		}

	case ModeAutoApproveRisky:
		if level == developer.RiskSafe || level == developer.RiskRisky {
			return ConfirmationDecision{
				Allowed:              true,
				RequiresConfirmation: false,
				RiskLevel:            level,
				Reason:               fmt.Sprintf("Auto-approved under %s policy (risk: %s)", mode, level),
			}
		}
		return ConfirmationDecision{
			Allowed:              false,
			RequiresConfirmation: true,
			RiskLevel:            level,
			Reason:               fmt.Sprintf("Confirmation required for destructive action (%s)", assessment.Reason),
			Prompt:               promptMsg,
		}

	case ModeAlwaysAsk:
		if tool == "echo" {
			return ConfirmationDecision{
				Allowed:              true,
				RequiresConfirmation: false,
				RiskLevel:            level,
				Reason:               "Echo diagnostic is unconditionally permitted",
			}
		}
		return ConfirmationDecision{
			Allowed:              false,
			RequiresConfirmation: true,
			RiskLevel:            level,
			Reason:               fmt.Sprintf("Confirmation required under always_ask policy (%s)", assessment.Reason),
			Prompt:               promptMsg,
		}

	case ModeAutoApproveSafe:
		fallthrough
	default:
		if level == developer.RiskSafe {
			return ConfirmationDecision{
				Allowed:              true,
				RequiresConfirmation: false,
				RiskLevel:            level,
				Reason:               fmt.Sprintf("Auto-approved safe action (%s)", assessment.Reason),
			}
		}
		return ConfirmationDecision{
			Allowed:              false,
			RequiresConfirmation: true,
			RiskLevel:            level,
			Reason:               fmt.Sprintf("Confirmation required for %s action (%s)", level, assessment.Reason),
			Prompt:               promptMsg,
		}
	}
}

// ModeDescription provides human-readable documentation for each confirmation policy mode.
func ModeDescription(m ConfirmationMode) string {
	switch m {
	case ModeAlwaysAsk:
		return "Prompt for confirmation on all tool actions"
	case ModeAutoApproveSafe:
		return "Auto-approve safe (read-only) actions, prompt for risky & destructive actions"
	case ModeAutoApproveRisky:
		return "Auto-approve safe and moderate-risk actions, prompt only for destructive actions"
	case ModeAutoApproveAll:
		return "Auto-approve all actions without prompting (unattended/autonomous mode)"
	case ModeReadOnly:
		return "Permit only safe read-only operations, block all mutations and destructive actions"
	default:
		return "Standard confirmation policy"
	}
}
