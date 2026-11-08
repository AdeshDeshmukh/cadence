package system

import (
	"fmt"
	"regexp"
	"strings"
)

// OperatingMode defines the current active behavior and processing pipeline of Cadence.
type OperatingMode string

const (
	ModeDictation OperatingMode = "dictation" // Standard prose dictation with punctuation & postprocessing
	ModeCode      OperatingMode = "code"      // Developer syntax & casing formatting (camel, snake, docstring)
	ModeCommand   OperatingMode = "command"   // Voice shell commands & system execution
	ModeAgent     OperatingMode = "agent"     // Autonomous goal-directed multi-step agent mode
)

// ValidModes returns a slice of all supported operating modes.
func ValidModes() []OperatingMode {
	return []OperatingMode{ModeDictation, ModeCode, ModeCommand, ModeAgent}
}

// IsValidMode checks whether a given string is a valid OperatingMode.
func IsValidMode(m string) bool {
	switch OperatingMode(strings.ToLower(strings.TrimSpace(m))) {
	case ModeDictation, ModeCode, ModeCommand, ModeAgent:
		return true
	default:
		return false
	}
}

// SelfActionType identifies an internal Cadence self-orchestration action.
type SelfActionType string

const (
	ActionStartRecording  SelfActionType = "start_recording"
	ActionStopRecording   SelfActionType = "stop_recording"
	ActionCancelRecording SelfActionType = "cancel_recording"
	ActionSwitchMode      SelfActionType = "switch_mode"
	ActionQueryStatus     SelfActionType = "query_status"
	ActionMute            SelfActionType = "mute"
	ActionUnmute          SelfActionType = "unmute"
)

// SelfCommand represents a parsed voice instruction targeted at Cadence itself.
type SelfCommand struct {
	Action      SelfActionType `json:"action"`
	TargetMode  OperatingMode  `json:"target_mode,omitempty"`
	RawInput    string         `json:"raw_input"`
	Description string         `json:"description,omitempty"`
}

var (
	reSwitchMode = regexp.MustCompile(`(?i)^(?:switch\s+to|enter|enable|set\s+mode\s+to|change\s+to)\s+(dictation|code|command|agent)(?:\s+mode)?$`)
	reDirectMode = regexp.MustCompile(`(?i)^(dictation|code|command|agent)\s+mode$`)
)

// ParseSelfCommand extracts a self-control instruction from natural speech input.
func ParseSelfCommand(input string) (SelfCommand, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return SelfCommand{}, false
	}
	clean := strings.ToLower(s)

	// Strip common wake / address prefixes
	for {
		orig := clean
		clean = strings.TrimPrefix(clean, "hey cadence ")
		clean = strings.TrimPrefix(clean, "cadence ")
		clean = strings.TrimPrefix(clean, "please ")
		clean = strings.TrimPrefix(clean, "can you ")
		clean = strings.TrimPrefix(clean, "could you ")
		clean = strings.TrimSpace(clean)
		if clean == orig {
			break
		}
	}

	// 1. Recording lifecycle actions
	switch clean {
	case "start recording", "start listening", "begin recording", "listen", "record", "start dictation":
		return SelfCommand{
			Action:      ActionStartRecording,
			RawInput:    input,
			Description: "Start audio recording session",
		}, true

	case "stop recording", "stop listening", "stop", "finish recording", "done recording", "end recording", "end dictation":
		return SelfCommand{
			Action:      ActionStopRecording,
			RawInput:    input,
			Description: "Stop audio recording session and transcribe",
		}, true

	case "cancel recording", "cancel", "discard recording", "abort", "discard session", "cancel session":
		return SelfCommand{
			Action:      ActionCancelRecording,
			RawInput:    input,
			Description: "Cancel current audio recording session",
		}, true
	}

	// 2. Mode switching ("switch to code mode", "code mode", "enter agent mode", etc.)
	if m := reSwitchMode.FindStringSubmatch(clean); len(m) == 2 {
		mode := OperatingMode(strings.ToLower(m[1]))
		return SelfCommand{
			Action:      ActionSwitchMode,
			TargetMode:  mode,
			RawInput:    input,
			Description: fmt.Sprintf("Switch operating mode to %s", mode),
		}, true
	}

	if m := reDirectMode.FindStringSubmatch(clean); len(m) == 2 {
		mode := OperatingMode(strings.ToLower(m[1]))
		return SelfCommand{
			Action:      ActionSwitchMode,
			TargetMode:  mode,
			RawInput:    input,
			Description: fmt.Sprintf("Switch operating mode to %s", mode),
		}, true
	}

	// 3. Status queries ("what mode are you in?", "what is your mode?", "status")
	switch clean {
	case "what mode are you in", "what mode are you in?", "what is your mode", "what is your mode?",
		"current mode", "what mode is cadence in", "what mode is cadence in?", "mode status",
		"report mode", "report state", "cadence status":
		return SelfCommand{
			Action:      ActionQueryStatus,
			RawInput:    input,
			Description: "Query current operating mode and status",
		}, true
	}

	// 4. Mute / Unmute
	switch clean {
	case "mute cadence", "mute microphone", "mute mic", "silence cadence":
		return SelfCommand{
			Action:      ActionMute,
			RawInput:    input,
			Description: "Mute microphone",
		}, true
	case "unmute cadence", "unmute microphone", "unmute mic":
		return SelfCommand{
			Action:      ActionUnmute,
			RawInput:    input,
			Description: "Unmute microphone",
		}, true
	}

	return SelfCommand{}, false
}

// FormatSelfFeedback generates a user-friendly spoken response for a completed self-control action.
func FormatSelfFeedback(cmd SelfCommand, currentMode OperatingMode, isRecording bool) string {
	switch cmd.Action {
	case ActionStartRecording:
		return "Cadence recording started"
	case ActionStopRecording:
		return "Cadence recording stopped"
	case ActionCancelRecording:
		return "Cadence recording cancelled"
	case ActionSwitchMode:
		return fmt.Sprintf("Switched to %s mode", cmd.TargetMode)
	case ActionQueryStatus:
		recStatus := "idle"
		if isRecording {
			recStatus = "recording"
		}
		return fmt.Sprintf("Cadence is in %s mode (%s)", currentMode, recStatus)
	case ActionMute:
		return "Cadence microphone muted"
	case ActionUnmute:
		return "Cadence microphone unmuted"
	default:
		return "Action completed"
	}
}
