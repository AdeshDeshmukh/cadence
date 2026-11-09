package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/developer"
	"github.com/AdeshDeshmukh/cadence/internal/system"
)

func TestConfirmationPolicy_Defaults(t *testing.T) {
	cp := DefaultConfirmationPolicy()
	if cp.Mode() != ModeAutoApproveSafe {
		t.Fatalf("expected default mode %s, got %s", ModeAutoApproveSafe, cp.Mode())
	}

	// 1. Safe action (read file) -> auto-approved
	dec := cp.Evaluate("fs", "read", map[string]any{"path": "file.txt"})
	if !dec.Allowed || dec.RequiresConfirmation || dec.RiskLevel != developer.RiskSafe {
		t.Fatalf("expected safe read to be auto-approved, got %+v", dec)
	}

	// 2. Risky action (write file) -> confirmation required
	dec = cp.Evaluate("fs", "write", map[string]any{"path": "file.txt"})
	if dec.Allowed || !dec.RequiresConfirmation || dec.RiskLevel != developer.RiskRisky {
		t.Fatalf("expected risky write to require confirmation, got %+v", dec)
	}

	// 3. Destructive action (delete file) -> confirmation required
	dec = cp.Evaluate("fs", "delete", map[string]any{"path": "file.txt"})
	if dec.Allowed || !dec.RequiresConfirmation || dec.RiskLevel != developer.RiskDestructive {
		t.Fatalf("expected destructive delete to require confirmation, got %+v", dec)
	}
}

func TestConfirmationPolicy_ModeAlwaysAsk(t *testing.T) {
	cp := NewConfirmationPolicy(ModeAlwaysAsk)

	// Safe read action requires confirmation under always_ask
	dec := cp.Evaluate("fs", "read", map[string]any{"path": "main.go"})
	if dec.Allowed || !dec.RequiresConfirmation {
		t.Fatalf("expected read to require confirmation under always_ask, got %+v", dec)
	}

	// Diagnostic echo is always permitted
	dec = cp.Evaluate("echo", "run", map[string]any{"message": "ping"})
	if !dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected echo to be allowed under always_ask, got %+v", dec)
	}
}

func TestConfirmationPolicy_ModeAutoApproveRisky(t *testing.T) {
	cp := NewConfirmationPolicy(ModeAutoApproveRisky)

	// 1. Safe read -> auto-approved
	dec := cp.Evaluate("fs", "read", map[string]any{"path": "main.go"})
	if !dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected safe read to be auto-approved, got %+v", dec)
	}

	// 2. Risky write -> auto-approved
	dec = cp.Evaluate("fs", "write", map[string]any{"path": "main.go"})
	if !dec.Allowed || dec.RequiresConfirmation || dec.RiskLevel != developer.RiskRisky {
		t.Fatalf("expected risky write to be auto-approved under auto_approve_risky, got %+v", dec)
	}

	// 3. Destructive delete -> confirmation required
	dec = cp.Evaluate("fs", "delete", map[string]any{"path": "main.go"})
	if dec.Allowed || !dec.RequiresConfirmation || dec.RiskLevel != developer.RiskDestructive {
		t.Fatalf("expected destructive delete to require confirmation, got %+v", dec)
	}
}

func TestConfirmationPolicy_ModeAutoApproveAll(t *testing.T) {
	cp := NewConfirmationPolicy(ModeAutoApproveAll)

	// Even destructive actions are auto-approved in headless/unattended mode
	dec := cp.Evaluate("fs", "delete", map[string]any{"path": "old.log"})
	if !dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected destructive action to be auto-approved under auto_approve_all, got %+v", dec)
	}
}

func TestConfirmationPolicy_ModeReadOnly(t *testing.T) {
	cp := NewConfirmationPolicy(ModeReadOnly)

	// 1. Safe read -> allowed
	dec := cp.Evaluate("fs", "read", map[string]any{"path": "info.txt"})
	if !dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected safe read to be allowed under read_only, got %+v", dec)
	}

	// 2. Risky write -> completely prohibited without prompting
	dec = cp.Evaluate("fs", "write", map[string]any{"path": "info.txt"})
	if dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected risky write to be prohibited under read_only, got %+v", dec)
	}

	// 3. Destructive delete -> prohibited
	dec = cp.Evaluate("fs", "delete", map[string]any{"path": "info.txt"})
	if dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected destructive delete to be prohibited under read_only, got %+v", dec)
	}
}

func TestConfirmationPolicy_ExplicitApproval(t *testing.T) {
	cp := NewConfirmationPolicy(ModeAlwaysAsk)

	// Action with approved=true should be authorized
	dec := cp.Evaluate("fs", "delete", map[string]any{
		"path":     "temp.txt",
		"approved": true,
	})
	if !dec.Allowed || dec.RequiresConfirmation {
		t.Fatalf("expected approved=true action to be authorized, got %+v", dec)
	}
}

func TestConfirmationPolicy_AssessRisk(t *testing.T) {
	cp := DefaultConfirmationPolicy()

	// 1. Exec tool classifies via developer.ClassifyCommand
	safeExec := cp.AssessRisk("exec", "run", map[string]any{"command": "git status"})
	if safeExec.Level != developer.RiskSafe {
		t.Errorf("git status expected safe, got %s", safeExec.Level)
	}

	riskyExec := cp.AssessRisk("exec", "run", map[string]any{"command": "git commit -m 'fix'"})
	if riskyExec.Level != developer.RiskRisky {
		t.Errorf("git commit expected risky, got %s", riskyExec.Level)
	}

	destructExec := cp.AssessRisk("exec", "run", map[string]any{"command": "rm -rf /tmp/data"})
	if destructExec.Level != developer.RiskDestructive {
		t.Errorf("rm -rf expected destructive, got %s", destructExec.Level)
	}

	// 2. System tool actions
	if cp.AssessRisk("system", "status", nil).Level != developer.RiskSafe {
		t.Errorf("system.status expected safe")
	}
	if cp.AssessRisk("system", "launch", nil).Level != developer.RiskRisky {
		t.Errorf("system.launch expected risky")
	}
	if cp.AssessRisk("system", "quit", nil).Level != developer.RiskDestructive {
		t.Errorf("system.quit expected destructive")
	}
}

func TestRegistry_ConfirmationPolicyEnforcement(t *testing.T) {
	mockCtrl := system.NewMockController()
	reg := NewRegistry(mockCtrl, nil)
	ctx := context.Background()

	// Configure AutoApproveSafe policy on registry
	cp := NewConfirmationPolicy(ModeAutoApproveSafe)
	reg.SetConfirmationPolicy(cp)

	// 1. Safe tool call (echo) succeeds
	res, err := reg.Call(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil || !res.Success {
		t.Fatalf("expected echo to succeed: %v, res: %+v", err, res)
	}

	// 2. Risky tool call (system launch) fails requiring confirmation
	res, err = reg.Call(ctx, "system", map[string]any{"action": "launch", "target": "Slack"})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.Success || !strings.Contains(res.Error, "confirmation required") {
		t.Fatalf("expected confirmation required error, got: %+v", res)
	}

	// 3. Risky tool call with approved: true succeeds
	res, err = reg.Call(ctx, "system", map[string]any{
		"action":   "launch",
		"target":   "Slack",
		"approved": true,
	})
	if err != nil || !res.Success {
		t.Fatalf("expected approved launch to succeed, got: %+v", res)
	}
}

func TestEngine_ConfirmationPolicyIntegration(t *testing.T) {
	mockCtrl := system.NewMockController()
	cp := NewConfirmationPolicy(ModeAutoApproveSafe)
	engine := NewEngine(
		WithSystemController(mockCtrl),
		WithConfirmationPolicy(cp),
	)
	ctx := context.Background()

	// 1. Run without AutoApprove should fail on risky step (launch Slack)
	traj, err := engine.Run(ctx, "open Slack", RunOptions{AutoApprove: false})
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if traj.Status != StatusFailed {
		t.Fatalf("expected run to fail due to confirmation required, got status %s", traj.Status)
	}
	if len(traj.Steps) == 0 || !strings.Contains(traj.Steps[0].Observation.Summary, "confirmation required") {
		t.Fatalf("expected observation to report confirmation required, got: %+v", traj.Steps)
	}

	// 2. Run with AutoApprove: true should complete successfully
	traj, err = engine.Run(ctx, "open Slack", RunOptions{AutoApprove: true})
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if traj.Status != StatusSuccess {
		t.Fatalf("expected run with AutoApprove to succeed, got status %s", traj.Status)
	}
}
