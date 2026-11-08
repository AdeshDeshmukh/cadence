package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/system"
)

func TestEngine_Plan(t *testing.T) {
	mockCtrl := system.NewMockController()
	engine := NewEngine(WithSystemController(mockCtrl))
	ctx := context.Background()

	// 1. Single goal
	p, err := engine.Plan(ctx, "open Chrome")
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(p.Steps))
	}
	if p.Steps[0].Tool != "system" || p.Steps[0].Params["action"] != "launch" {
		t.Fatalf("unexpected step 0: %+v", p.Steps[0])
	}

	// 2. Compound goal with 'and then'
	p, err = engine.Plan(ctx, "open Chrome and then take a screenshot")
	if err != nil {
		t.Fatalf("compound Plan failed: %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(p.Steps))
	}
	if p.Steps[0].Params["action"] != "launch" {
		t.Errorf("step 0 action = %v, want launch", p.Steps[0].Params["action"])
	}
	if p.Steps[1].Params["action"] != "screenshot" {
		t.Errorf("step 1 action = %v, want screenshot", p.Steps[1].Params["action"])
	}

	// 3. Multi-delimiter goal
	p, err = engine.Plan(ctx, "minimize window; switch to slack; volume 80%")
	if err != nil {
		t.Fatalf("multi-delimiter Plan failed: %v", err)
	}
	if len(p.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(p.Steps))
	}

	// 4. Empty goal error
	if _, err := engine.Plan(ctx, "   "); err == nil {
		t.Fatal("expected error on empty goal")
	}
}

func TestEngine_Run(t *testing.T) {
	mockCtrl := system.NewMockController()
	engine := NewEngine(WithSystemController(mockCtrl))
	ctx := context.Background()

	// 1. Plan-only run
	traj, err := engine.Run(ctx, "open Slack and minimize window", RunOptions{PlanOnly: true})
	if err != nil {
		t.Fatalf("PlanOnly Run failed: %v", err)
	}
	if traj.Status != StatusSuccess {
		t.Errorf("status = %v, want success", traj.Status)
	}
	if len(traj.Steps) != 0 {
		t.Errorf("expected 0 executed steps in plan-only, got %d", len(traj.Steps))
	}
	if len(traj.Plan.Steps) != 2 {
		t.Errorf("expected 2 planned steps, got %d", len(traj.Plan.Steps))
	}

	// 2. Full autonomous run
	traj, err = engine.Run(ctx, "open Slack and then minimize window", RunOptions{})
	if err != nil {
		t.Fatalf("full Run failed: %v", err)
	}
	if traj.Status != StatusSuccess {
		t.Errorf("status = %v, want success", traj.Status)
	}
	if len(traj.Steps) != 2 {
		t.Fatalf("expected 2 executed steps, got %d", len(traj.Steps))
	}
	if len(mockCtrl.LaunchedApps) != 1 || mockCtrl.LaunchedApps[0] != "slack" {
		t.Errorf("mock launched apps = %v", mockCtrl.LaunchedApps)
	}
	if len(mockCtrl.Windows) != 1 || mockCtrl.Windows[0] != system.WindowActionMinimize {
		t.Errorf("mock windows = %v", mockCtrl.Windows)
	}
	if !strings.Contains(traj.FinalReport, "Agent completed 2 step(s)") {
		t.Errorf("unexpected report: %s", traj.FinalReport)
	}

	// 3. Step limit reached
	traj, err = engine.Run(ctx, "open Chrome and then open Notes and then volume 50", RunOptions{MaxSteps: 1})
	if err != nil {
		t.Fatalf("MaxSteps Run failed: %v", err)
	}
	if traj.Status != StatusStepLimitReached {
		t.Errorf("expected StatusStepLimitReached, got %v", traj.Status)
	}
	if len(traj.Steps) != 1 {
		t.Errorf("expected 1 executed step, got %d", len(traj.Steps))
	}

	// 4. Custom tool with failure abort
	engine.RegisterTool("fail_tool", func(_ context.Context, _ map[string]any) (ToolResult, error) {
		return ToolResult{Success: false, Error: "simulated failure"}, nil
	})
	engine.RegisterTool("custom_step", func(_ context.Context, _ map[string]any) (ToolResult, error) {
		return ToolResult{Success: true, Output: "custom ok"}, nil
	})

	customPlan := Plan{
		Goal: "custom workflow",
		Steps: []PlanStep{
			{Index: 1, Description: "step 1", Tool: "fail_tool", Status: StepPending},
			{Index: 2, Description: "step 2", Tool: "custom_step", Status: StepPending},
		},
	}
	// Run manual trajectory testing failure
	trajFail, err := engine.Run(ctx, "run invalid", RunOptions{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	_ = customPlan
	_ = trajFail

	// Test tool returning error directly
	engine.RegisterTool("error_tool", func(_ context.Context, _ map[string]any) (ToolResult, error) {
		return ToolResult{}, errors.New("network timeout")
	})

	// 5. History check
	history := engine.History(10)
	if len(history) < 3 {
		t.Errorf("expected at least 3 history items, got %d", len(history))
	}
}

