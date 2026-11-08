package agent

import (
	"time"
)

// StepStatus indicates the lifecycle phase of an individual plan step.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// ExecutionStatus describes the overall outcome of an agent execution trajectory.
type ExecutionStatus string

const (
	StatusInProgress       ExecutionStatus = "in_progress"
	StatusSuccess          ExecutionStatus = "success"
	StatusFailed           ExecutionStatus = "failed"
	StatusStepLimitReached ExecutionStatus = "step_limit_reached"
)

// PlanStep represents a single actionable item in a decomposed goal.
type PlanStep struct {
	Index       int            `json:"index"`
	Description string         `json:"description"`
	Tool        string         `json:"tool"`
	Params      map[string]any `json:"params"`
	Status      StepStatus     `json:"status"`
}

// Plan captures the decomposition of a high-level goal into structured steps.
type Plan struct {
	Goal      string     `json:"goal"`
	Steps     []PlanStep `json:"steps"`
	CreatedAt time.Time  `json:"created_at"`
}

// ToolCall holds the invocation parameters for a tool.
type ToolCall struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params"`
}

// ToolResult captures the direct output and status of an invoked tool.
type ToolResult struct {
	Success bool           `json:"success"`
	Output  string         `json:"output"`
	Error   string         `json:"error,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// Observation provides evaluated insights after a tool invocation.
type Observation struct {
	Success    bool   `json:"success"`
	Summary    string `json:"summary"`
	NextAction string `json:"next_action,omitempty"`
}

// TrajectoryStep documents a complete iteration of the agent loop (Thought -> Action -> Observation).
type TrajectoryStep struct {
	Index       int           `json:"index"`
	Thought     string        `json:"thought"`
	Action      ToolCall      `json:"action"`
	Observation Observation   `json:"observation"`
	Duration    time.Duration `json:"duration"`
}

// ExecutionTrajectory tracks the end-to-end execution of a goal through the agent loop.
type ExecutionTrajectory struct {
	ID          string           `json:"id"`
	Goal        string           `json:"goal"`
	Plan        Plan             `json:"plan"`
	Steps       []TrajectoryStep `json:"steps"`
	FinalReport string           `json:"final_report"`
	Status      ExecutionStatus  `json:"status"`
	StartTime   time.Time        `json:"start_time"`
	EndTime     time.Time        `json:"end_time"`
}

// RunOptions configures an agent loop execution run.
type RunOptions struct {
	MaxSteps int  `json:"max_steps,omitempty"`
	PlanOnly bool `json:"plan_only,omitempty"`
}

