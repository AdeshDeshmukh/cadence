package system

import (
	"testing"
)

func TestParseSelfCommand(t *testing.T) {
	tests := []struct {
		input      string
		wantOK     bool
		wantAction SelfActionType
		wantMode   OperatingMode
	}{
		// Recording
		{"start recording", true, ActionStartRecording, ""},
		{"cadence start listening", true, ActionStartRecording, ""},
		{"please begin recording", true, ActionStartRecording, ""},
		{"stop", true, ActionStopRecording, ""},
		{"hey cadence stop recording", true, ActionStopRecording, ""},
		{"cancel recording", true, ActionCancelRecording, ""},
		{"cadence discard session", true, ActionCancelRecording, ""},

		// Modes
		{"switch to code mode", true, ActionSwitchMode, ModeCode},
		{"cadence code mode", true, ActionSwitchMode, ModeCode},
		{"enter command mode", true, ActionSwitchMode, ModeCommand},
		{"set mode to dictation", true, ActionSwitchMode, ModeDictation},
		{"enable agent mode", true, ActionSwitchMode, ModeAgent},
		{"agent mode", true, ActionSwitchMode, ModeAgent},

		// Status
		{"what mode are you in?", true, ActionQueryStatus, ""},
		{"cadence what is your mode", true, ActionQueryStatus, ""},
		{"current mode", true, ActionQueryStatus, ""},
		{"report mode", true, ActionQueryStatus, ""},

		// Mute / Unmute
		{"mute cadence", true, ActionMute, ""},
		{"unmute microphone", true, ActionUnmute, ""},

		// Non-matching
		{"write a python function", false, "", ""},
		{"open slack", false, "", ""},
		{"", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, ok := ParseSelfCommand(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ParseSelfCommand(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if cmd.Action != tt.wantAction {
				t.Errorf("Action = %v, want %v", cmd.Action, tt.wantAction)
			}
			if tt.wantMode != "" && cmd.TargetMode != tt.wantMode {
				t.Errorf("TargetMode = %v, want %v", cmd.TargetMode, tt.wantMode)
			}
		})
	}
}

func TestFormatSelfFeedback(t *testing.T) {
	// Status idle
	msg := FormatSelfFeedback(SelfCommand{Action: ActionQueryStatus}, ModeDictation, false)
	if msg != "Cadence is in dictation mode (idle)" {
		t.Errorf("unexpected status message: %s", msg)
	}

	// Status recording
	msg = FormatSelfFeedback(SelfCommand{Action: ActionQueryStatus}, ModeCode, true)
	if msg != "Cadence is in code mode (recording)" {
		t.Errorf("unexpected status message: %s", msg)
	}

	// Mode switch
	msg = FormatSelfFeedback(SelfCommand{Action: ActionSwitchMode, TargetMode: ModeCommand}, ModeDictation, false)
	if msg != "Switched to command mode" {
		t.Errorf("unexpected switch message: %s", msg)
	}
}

func TestIsValidMode(t *testing.T) {
	valid := []string{"dictation", "code", "command", "agent", "CODE", " Dictation "}
	for _, m := range valid {
		if !IsValidMode(m) {
			t.Errorf("expected %q to be valid", m)
		}
	}

	invalid := []string{"invalid", "unknown", "", "super"}
	for _, m := range invalid {
		if IsValidMode(m) {
			t.Errorf("expected %q to be invalid", m)
		}
	}
}
