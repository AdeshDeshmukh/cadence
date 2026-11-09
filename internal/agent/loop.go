package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/system"
)

// Engine orchestrates the autonomous Agent loop (plan -> tool call -> observe -> report).
type Engine struct {
	mu             sync.RWMutex
	registry       *Registry
	history        []*ExecutionTrajectory
	maxSteps       int
	systemCtrl     system.Controller
	searchProvider SearchProvider
}

// Option configures dependencies for the Agent Engine.
type Option func(*Engine)

// WithSystemController configures a desktop system automation controller.
func WithSystemController(c system.Controller) Option {
	return func(e *Engine) { e.systemCtrl = c }
}

// WithSearchProvider configures a custom web search provider.
func WithSearchProvider(sp SearchProvider) Option {
	return func(e *Engine) { e.searchProvider = sp }
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
		maxSteps: 10,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.systemCtrl == nil {
		e.systemCtrl = system.NewDefaultController()
	}

	e.registry = NewRegistry(e.systemCtrl, e.searchProvider)

	return e
}

// Registry returns the active tool registry.
func (e *Engine) Registry() *Registry {
	return e.registry
}

// RegisterTool adds or overrides a named tool handler in the registry.
func (e *Engine) RegisterTool(name string, handler ToolHandler) {
	_ = e.registry.Register(ToolDefinition{
		Name:        name,
		Category:    "custom",
		Description: fmt.Sprintf("Custom tool %s", name),
		Handler:     handler,
	})
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

var (
	reReadFile      = regexp.MustCompile(`(?i)^(?:read\s+file|read|cat)\s+(.+)$`)
	reWriteFile     = regexp.MustCompile(`(?i)^(?:write\s+file|write\s+to)\s+(\S+)\s+(?:with\s+content\s+|content\s+)?(.+)$`)
	reListDir       = regexp.MustCompile(`(?i)^(?:list\s+files\s+in|list\s+directory|ls|list\s+dir)(?:\s+(.+))?$`)
	reSearchFiles   = regexp.MustCompile(`(?i)^(?:search\s+files\s+for|find\s+files\s+matching)\s+(.+)$`)
	reSearchWeb     = regexp.MustCompile(`(?i)^(?:search\s+(?:the\s+)?web\s+for|search\s+for|google)\s+(.+)$`)
	reClipboardCopy = regexp.MustCompile(`(?i)^(?:copy\s+to\s+clipboard|copy|set\s+clipboard\s+to)\s+(.+)$`)
)

func (e *Engine) decomposePhrase(index int, phrase string) PlanStep {
	clean := strings.TrimSpace(phrase)
	lower := strings.ToLower(clean)

	// 1. Clipboard tool intents
	if lower == "get clipboard" || lower == "read clipboard" || lower == "show clipboard" || lower == "paste clipboard" {
		return PlanStep{
			Index:       index,
			Description: "Get clipboard content",
			Tool:        "clipboard",
			Params:      map[string]any{"action": "get"},
			Status:      StepPending,
		}
	}
	if lower == "clear clipboard" || lower == "empty clipboard" {
		return PlanStep{
			Index:       index,
			Description: "Clear clipboard",
			Tool:        "clipboard",
			Params:      map[string]any{"action": "clear"},
			Status:      StepPending,
		}
	}
	if m := reClipboardCopy.FindStringSubmatch(clean); len(m) == 2 {
		txt := strings.TrimSpace(m[1])
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("Copy %q to clipboard", txt),
			Tool:        "clipboard",
			Params:      map[string]any{"action": "set", "text": txt},
			Status:      StepPending,
		}
	}

	// 2. Web search intents
	if m := reSearchWeb.FindStringSubmatch(clean); len(m) == 2 {
		q := strings.TrimSpace(m[1])
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("Search web for: %s", q),
			Tool:        "web_search",
			Params:      map[string]any{"query": q},
			Status:      StepPending,
		}
	}

	// 3. System info intents
	if lower == "check battery" || lower == "battery status" || lower == "battery" {
		return PlanStep{
			Index:       index,
			Description: "Check battery status",
			Tool:        "system_info",
			Params:      map[string]any{"action": "battery"},
			Status:      StepPending,
		}
	}
	if lower == "check os" || lower == "os info" || lower == "system info" || lower == "system specs" {
		return PlanStep{
			Index:       index,
			Description: "Check OS environment",
			Tool:        "system_info",
			Params:      map[string]any{"action": "os"},
			Status:      StepPending,
		}
	}
	if lower == "check network" || lower == "network info" || lower == "ip address" || lower == "my ip" {
		return PlanStep{
			Index:       index,
			Description: "Check network interfaces",
			Tool:        "system_info",
			Params:      map[string]any{"action": "network"},
			Status:      StepPending,
		}
	}
	if lower == "hostname" || lower == "check hostname" {
		return PlanStep{
			Index:       index,
			Description: "Check host name",
			Tool:        "system_info",
			Params:      map[string]any{"action": "hostname"},
			Status:      StepPending,
		}
	}

	// 4. Filesystem tool intents
	if m := reWriteFile.FindStringSubmatch(clean); len(m) == 3 {
		filePath := strings.TrimSpace(m[1])
		fileContent := strings.TrimSpace(m[2])
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("Write file %s", filePath),
			Tool:        "fs",
			Params:      map[string]any{"action": "write", "path": filePath, "content": fileContent},
			Status:      StepPending,
		}
	}
	if m := reReadFile.FindStringSubmatch(clean); len(m) == 2 {
		filePath := strings.TrimSpace(m[1])
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("Read file %s", filePath),
			Tool:        "fs",
			Params:      map[string]any{"action": "read", "path": filePath},
			Status:      StepPending,
		}
	}
	if m := reListDir.FindStringSubmatch(clean); len(m) == 2 {
		dirPath := strings.TrimSpace(m[1])
		if dirPath == "" {
			dirPath = "."
		}
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("List directory %s", dirPath),
			Tool:        "fs",
			Params:      map[string]any{"action": "list", "path": dirPath},
			Status:      StepPending,
		}
	}
	if m := reSearchFiles.FindStringSubmatch(clean); len(m) == 2 {
		pat := strings.TrimSpace(m[1])
		return PlanStep{
			Index:       index,
			Description: fmt.Sprintf("Search files matching %s", pat),
			Tool:        "fs",
			Params:      map[string]any{"action": "search", "path": ".", "pattern": pat},
			Status:      StepPending,
		}
	}

	// 5. System automation command
	if sysCmd, ok := system.ParseSystemCommand(clean); ok {
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

	// 6. Shell command invocation
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
		Description: clean,
		Tool:        "echo",
		Params:      map[string]any{"message": clean},
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

		// Dispatch via registry
		tRes, callErr := e.registry.Call(ctx, planStep.Tool, planStep.Params)
		if callErr != nil {
			tRes = ToolResult{Success: false, Error: callErr.Error()}
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
