package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/system"
)

// ToolHandler defines a function signature for invoking an agent tool.
type ToolHandler func(ctx context.Context, params map[string]any) (ToolResult, error)

// Engine orchestrates the autonomous Agent loop (plan -> tool call -> observe -> report).
type Engine struct {
	mu         sync.RWMutex
	tools      map[string]ToolHandler
	history    []*ExecutionTrajectory
	maxSteps   int
	systemCtrl system.Controller
}

// Option configures dependencies for the Agent Engine.
type Option func(*Engine)

// WithSystemController configures a desktop system automation controller.
func WithSystemController(c system.Controller) Option {
	return func(e *Engine) { e.systemCtrl = c }
}

// WithMaxSteps sets the global default maximum step limit.
func WithMaxSteps(n int) Option {
	return func(e *Engine) {
		if n > 0 {
			e.maxSteps = n
		}
	}
}

// NewEngine creates a new Agent loop engine with default built-in tools.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		tools:    make(map[string]ToolHandler),
		maxSteps: 10,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.systemCtrl == nil {
		e.systemCtrl = system.NewDefaultController()
	}

	// Register built-in tools
	e.registerBuiltins()

	return e
}

func (e *Engine) registerBuiltins() {
	// 1. Echo tool
	e.RegisterTool("echo", func(_ context.Context, params map[string]any) (ToolResult, error) {
		msg, _ := params["message"].(string)
		return ToolResult{
			Success: true,
			Output:  msg,
		}, nil
	})

	// 2. Shell execution tool
	e.RegisterTool("exec", func(ctx context.Context, params map[string]any) (ToolResult, error) {
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
	})

	// 3. System action tool
	e.RegisterTool("system", func(ctx context.Context, params map[string]any) (ToolResult, error) {
		action, _ := params["action"].(string)
		target, _ := params["target"].(string)

		switch strings.ToLower(action) {
		case "launch":
			if err := e.systemCtrl.LaunchApp(ctx, target); err != nil {
				return ToolResult{Success: false, Error: err.Error()}, nil
			}
			return ToolResult{Success: true, Output: fmt.Sprintf("Launched %s", target)}, nil

		case "focus":
			if err := e.systemCtrl.FocusApp(ctx, target); err != nil {
				return ToolResult{Success: false, Error: err.Error()}, nil
			}
			return ToolResult{Success: true, Output: fmt.Sprintf("Switched to %s", target)}, nil

		case "quit":
			if err := e.systemCtrl.QuitApp(ctx, target); err != nil {
				return ToolResult{Success: false, Error: err.Error()}, nil
			}
			return ToolResult{Success: true, Output: fmt.Sprintf("Quit %s", target)}, nil

		case "window":
			wAct := system.WindowAction(target)
			if err := e.systemCtrl.ControlWindow(ctx, wAct); err != nil {
				return ToolResult{Success: false, Error: err.Error()}, nil
			}
			return ToolResult{Success: true, Output: fmt.Sprintf("Window %s executed", target)}, nil

		case "screenshot":
			clip, _ := params["clipboard"].(bool)
			win, _ := params["window"].(bool)
			path, _ := params["path"].(string)
			res, err := e.systemCtrl.CaptureScreenshot(ctx, system.ScreenshotOptions{
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
				if err := e.systemCtrl.MuteVolume(ctx, m); err != nil {
					return ToolResult{Success: false, Error: err.Error()}, nil
				}
				if m {
					return ToolResult{Success: true, Output: "Volume muted"}, nil
				}
				return ToolResult{Success: true, Output: "Volume unmuted"}, nil
			}
			if lvl, ok := params["level"].(float64); ok {
				if err := e.systemCtrl.SetVolume(ctx, int(lvl)); err != nil {
					return ToolResult{Success: false, Error: err.Error()}, nil
				}
				return ToolResult{Success: true, Output: fmt.Sprintf("Volume set to %d%%", int(lvl))}, nil
			}
			return ToolResult{Success: false, Error: "missing volume parameter"}, nil

		default:
			return ToolResult{Success: false, Error: fmt.Sprintf("unknown system action: %s", action)}, nil
		}
	})
}

// RegisterTool adds or overrides a named tool handler.
func (e *Engine) RegisterTool(name string, handler ToolHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools[name] = handler
}

// Plan decomposes a high-level goal into structured executable steps.
func (e *Engine) Plan(_ context.Context, goal string) (Plan, error) {
	trimmed := strings.TrimSpace(goal)
	if trimmed == "" {
		return Plan{}, fmt.Errorf("goal cannot be empty")
	}

	// Decompose compound phrases using common sequential conjunctions
	delimiters := []string{" and then ", " then ", " and ", " after that ", "; "}
	phrases := []string{trimmed}

	for _, delim := range delimiters {
		var newPhrases []string
		for _, p := range phrases {
			parts := strings.Split(p, delim)
			for _, part := range parts {
				clean := strings.TrimSpace(part)
				if clean != "" {
					newPhrases = append(newPhrases, clean)
				}
			}
		}
		phrases = newPhrases
	}

	var steps []PlanStep
	for idx, phrase := range phrases {
		step := e.decomposePhrase(idx+1, phrase)
		steps = append(steps, step)
	}

	return Plan{
		Goal:      trimmed,
		Steps:     steps,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (e *Engine) decomposePhrase(index int, phrase string) PlanStep {
	// Check if this matches a system automation command
	if sysCmd, ok := system.ParseSystemCommand(phrase); ok {
		switch sysCmd.Type {
		case system.IntentLaunchApp:
			return PlanStep{
				Index:       index,
				Description: fmt.Sprintf("Launch %s", sysCmd.AppName),
				Tool:        "system",
				Params:      map[string]any{"action": "launch", "target": sysCmd.AppName},
				Status:      StepPending,
			}
		case system.IntentFocusApp:
			return PlanStep{
				Index:       index,
				Description: fmt.Sprintf("Switch to %s", sysCmd.AppName),
				Tool:        "system",
				Params:      map[string]any{"action": "focus", "target": sysCmd.AppName},
				Status:      StepPending,
			}
		case system.IntentQuitApp:
			return PlanStep{
				Index:       index,
				Description: fmt.Sprintf("Quit %s", sysCmd.AppName),
				Tool:        "system",
				Params:      map[string]any{"action": "quit", "target": sysCmd.AppName},
				Status:      StepPending,
			}
		case system.IntentWindow:
			return PlanStep{
				Index:       index,
				Description: fmt.Sprintf("Window %s", sysCmd.WindowAction),
				Tool:        "system",
				Params:      map[string]any{"action": "window", "target": string(sysCmd.WindowAction)},
				Status:      StepPending,
			}
		case system.IntentScreenshot:
			return PlanStep{
				Index:       index,
				Description: "Capture screenshot",
				Tool:        "system",
				Params: map[string]any{
					"action":    "screenshot",
					"clipboard": sysCmd.ScreenshotOpts.CopyToClipboard,
					"window":    sysCmd.ScreenshotOpts.ActiveWindowOnly,
					"path":      sysCmd.ScreenshotOpts.SavePath,
				},
				Status: StepPending,
			}
		case system.IntentVolume:
			params := map[string]any{"action": "volume"}
			if sysCmd.MuteState != nil {
				params["mute"] = *sysCmd.MuteState
			}
			if sysCmd.VolumeLevel != nil {
				params["level"] = float64(*sysCmd.VolumeLevel)
			}
			return PlanStep{
				Index:       index,
				Description: "Adjust volume",
				Tool:        "system",
				Params:      params,
				Status:      StepPending,
			}
		}
	}

	// Shell command invocation
	lower := strings.ToLower(phrase)
	if strings.HasPrefix(lower, "run ") || strings.HasPrefix(lower, "exec ") || strings.HasPrefix(lower, "execute ") {
		cmd := strings.TrimPrefix(lower, "run ")
		cmd = strings.TrimPrefix(cmd, "exec ")
		cmd = strings.TrimPrefix(cmd, "execute ")
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("Execute command: %s", cmd),
			Tool:        "exec",
			Params:      map[string]any{"command": cmd},
			Status:      StepPending,
		}
	}

	// Default fallback to echo diagnostic step
	return PlanStep{
		Index:       index,
		Description: phrase,
		Tool:        "echo",
		Params:      map[string]any{"message": phrase},
		Status:      StepPending,
	}
}

// Run executes the complete agent loop (plan -> tool call -> observe -> report).
func (e *Engine) Run(ctx context.Context, goal string, opts RunOptions) (*ExecutionTrajectory, error) {
	plan, err := e.Plan(ctx, goal)
	if err != nil {
		return nil, err
	}

	maxSteps := e.maxSteps
	if opts.MaxSteps > 0 {
		maxSteps = opts.MaxSteps
	}

	trajID := fmt.Sprintf("traj-%d", time.Now().UnixNano())
	trajectory := &ExecutionTrajectory{
		ID:        trajID,
		Goal:      goal,
		Plan:      plan,
		Status:    StatusInProgress,
		StartTime: time.Now().UTC(),
	}

	// If plan-only mode is requested, return early without tool calls
	if opts.PlanOnly {
		trajectory.Status = StatusSuccess
		trajectory.EndTime = time.Now().UTC()
		trajectory.FinalReport = fmt.Sprintf("Generated plan with %d steps for goal: %q", len(plan.Steps), goal)
		e.recordTrajectory(trajectory)
		return trajectory, nil
	}

	var stepOutputs []string
	executedCount := 0

	for i, planStep := range plan.Steps {
		if executedCount >= maxSteps {
			trajectory.Status = StatusStepLimitReached
			break
		}

		stepStart := time.Now()
		thought := fmt.Sprintf("Executing step %d/%d: %s", planStep.Index, len(plan.Steps), planStep.Description)

		e.mu.RLock()
		handler, exists := e.tools[planStep.Tool]
		e.mu.RUnlock()

		var tRes ToolResult
		var callErr error

		if !exists {
			callErr = fmt.Errorf("unregistered tool: %s", planStep.Tool)
			tRes = ToolResult{Success: false, Error: callErr.Error()}
		} else {
			tRes, callErr = handler(ctx, planStep.Params)
			if callErr != nil {
				tRes = ToolResult{Success: false, Error: callErr.Error()}
			}
		}

		obs := Observation{
			Success: tRes.Success,
		}
		if tRes.Success {
			obs.Summary = tRes.Output
			if obs.Summary == "" {
				obs.Summary = fmt.Sprintf("Completed %s successfully", planStep.Description)
			}
			stepOutputs = append(stepOutputs, obs.Summary)
			plan.Steps[i].Status = StepCompleted
		} else {
			obs.Summary = fmt.Sprintf("Failed: %s", tRes.Error)
			plan.Steps[i].Status = StepFailed
		}

		trajectory.Steps = append(trajectory.Steps, TrajectoryStep{
			Index:       planStep.Index,
			Thought:     thought,
			Action:      ToolCall{Tool: planStep.Tool, Params: planStep.Params},
			Observation: obs,
			Duration:    time.Since(stepStart),
		})

		executedCount++

		// Abort on step failure
		if !tRes.Success {
			trajectory.Status = StatusFailed
			trajectory.FinalReport = fmt.Sprintf("Agent stopped at step %d (%s): %s", planStep.Index, planStep.Description, tRes.Error)
			trajectory.EndTime = time.Now().UTC()
			e.recordTrajectory(trajectory)
			return trajectory, nil
		}
	}

	trajectory.EndTime = time.Now().UTC()
	if trajectory.Status == StatusInProgress {
		trajectory.Status = StatusSuccess
	}

	if len(stepOutputs) > 0 {
		trajectory.FinalReport = fmt.Sprintf("Agent completed %d step(s): %s", len(stepOutputs), strings.Join(stepOutputs, "; "))
	} else {
		trajectory.FinalReport = "Agent completed execution."
	}

	e.recordTrajectory(trajectory)
	return trajectory, nil
}

func (e *Engine) recordTrajectory(t *ExecutionTrajectory) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = append(e.history, t)
	if len(e.history) > 50 {
		e.history = e.history[len(e.history)-50:]
	}
}

// History returns recent execution trajectories up to the specified limit.
func (e *Engine) History(limit int) []*ExecutionTrajectory {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 || limit > len(e.history) {
		limit = len(e.history)
	}

	result := make([]*ExecutionTrajectory, limit)
	start := len(e.history) - limit
	for i := 0; i < limit; i++ {
		result[i] = e.history[start+i]
	}
	return result
}

