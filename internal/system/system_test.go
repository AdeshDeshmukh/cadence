package system

import (
	"context"
	"testing"
)

func TestParseSystemCommand(t *testing.T) {
	tests := []struct {
		input       string
		wantOK      bool
		wantType    IntentType
		wantApp     string
		wantWindow  WindowAction
		wantVol     *int
		wantMute    *bool
		wantClip    bool
		wantWinOnly bool
	}{
		// Launch
		{
			input:    "open chrome",
			wantOK:   true,
			wantType: IntentLaunchApp,
			wantApp:  "chrome",
		},
		{
			input:    "please launch Visual Studio Code",
			wantOK:   true,
			wantType: IntentLaunchApp,
			wantApp:  "visual studio code",
		},
		{
			input:    "start application Slack",
			wantOK:   true,
			wantType: IntentLaunchApp,
			wantApp:  "slack",
		},

		// Focus
		{
			input:    "switch to spotify",
			wantOK:   true,
			wantType: IntentFocusApp,
			wantApp:  "spotify",
		},
		{
			input:    "focus iterm",
			wantOK:   true,
			wantType: IntentFocusApp,
			wantApp:  "iterm",
		},
		{
			input:    "bring up notes",
			wantOK:   true,
			wantType: IntentFocusApp,
			wantApp:  "notes",
		},

		// Quit
		{
			input:    "quit terminal",
			wantOK:   true,
			wantType: IntentQuitApp,
			wantApp:  "terminal",
		},
		{
			input:    "exit safari",
			wantOK:   true,
			wantType: IntentQuitApp,
			wantApp:  "safari",
		},
		{
			input:    "kill app calculator",
			wantOK:   true,
			wantType: IntentQuitApp,
			wantApp:  "calculator",
		},

		// Window actions
		{
			input:      "minimize window",
			wantOK:     true,
			wantType:   IntentWindow,
			wantWindow: WindowActionMinimize,
		},
		{
			input:      "fullscreen",
			wantOK:     true,
			wantType:   IntentWindow,
			wantWindow: WindowActionMaximize,
		},
		{
			input:      "close window",
			wantOK:     true,
			wantType:   IntentWindow,
			wantWindow: WindowActionClose,
		},
		{
			input:      "hide window",
			wantOK:     true,
			wantType:   IntentWindow,
			wantWindow: WindowActionHide,
		},

		// Screenshot
		{
			input:    "take a screenshot",
			wantOK:   true,
			wantType: IntentScreenshot,
		},
		{
			input:       "screenshot window to clipboard",
			wantOK:      true,
			wantType:    IntentScreenshot,
			wantWinOnly: true,
			wantClip:    true,
		},

		// Volume
		{
			input:    "mute",
			wantOK:   true,
			wantType: IntentVolume,
			wantMute: boolPtr(true),
		},
		{
			input:    "unmute volume",
			wantOK:   true,
			wantType: IntentVolume,
			wantMute: boolPtr(false),
		},
		{
			input:    "set volume to 80",
			wantOK:   true,
			wantType: IntentVolume,
			wantVol:  intPtr(80),
		},
		{
			input:    "volume 50%",
			wantOK:   true,
			wantType: IntentVolume,
			wantVol:  intPtr(50),
		},

		// Unknown / empty
		{
			input:  "",
			wantOK: false,
		},
		{
			input:  "hello world how are you",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, ok := ParseSystemCommand(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ParseSystemCommand(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if cmd.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", cmd.Type, tt.wantType)
			}
			if tt.wantApp != "" && cmd.AppName != tt.wantApp {
				t.Errorf("AppName = %q, want %q", cmd.AppName, tt.wantApp)
			}
			if tt.wantWindow != "" && cmd.WindowAction != tt.wantWindow {
				t.Errorf("WindowAction = %q, want %q", cmd.WindowAction, tt.wantWindow)
			}
			if tt.wantVol != nil {
				if cmd.VolumeLevel == nil || *cmd.VolumeLevel != *tt.wantVol {
					t.Errorf("VolumeLevel = %v, want %v", cmd.VolumeLevel, *tt.wantVol)
				}
			}
			if tt.wantMute != nil {
				if cmd.MuteState == nil || *cmd.MuteState != *tt.wantMute {
					t.Errorf("MuteState = %v, want %v", cmd.MuteState, *tt.wantMute)
				}
			}
			if tt.wantClip != cmd.ScreenshotOpts.CopyToClipboard {
				t.Errorf("CopyToClipboard = %v, want %v", cmd.ScreenshotOpts.CopyToClipboard, tt.wantClip)
			}
			if tt.wantWinOnly != cmd.ScreenshotOpts.ActiveWindowOnly {
				t.Errorf("ActiveWindowOnly = %v, want %v", cmd.ScreenshotOpts.ActiveWindowOnly, tt.wantWinOnly)
			}
		})
	}
}

func TestExecuteSystemCommand(t *testing.T) {
	ctx := context.Background()
	mock := NewMockController()

	// Launch
	msg, err := ExecuteSystemCommand(ctx, mock, SystemCommand{Type: IntentLaunchApp, AppName: "Visual Studio Code"})
	if err != nil {
		t.Fatalf("LaunchApp failed: %v", err)
	}
	if msg != "Launched Visual Studio Code" {
		t.Errorf("unexpected message: %s", msg)
	}
	if len(mock.LaunchedApps) != 1 || mock.LaunchedApps[0] != "Visual Studio Code" {
		t.Errorf("mock.LaunchedApps = %v", mock.LaunchedApps)
	}

	// Focus
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{Type: IntentFocusApp, AppName: "iTerm"})
	if err != nil {
		t.Fatalf("FocusApp failed: %v", err)
	}
	if msg != "Switched to iTerm" {
		t.Errorf("unexpected message: %s", msg)
	}
	if len(mock.FocusedApps) != 1 || mock.FocusedApps[0] != "iTerm" {
		t.Errorf("mock.FocusedApps = %v", mock.FocusedApps)
	}

	// Quit
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{Type: IntentQuitApp, AppName: "Slack"})
	if err != nil {
		t.Fatalf("QuitApp failed: %v", err)
	}
	if msg != "Quit Slack" {
		t.Errorf("unexpected message: %s", msg)
	}
	if len(mock.QuitApps) != 1 || mock.QuitApps[0] != "Slack" {
		t.Errorf("mock.QuitApps = %v", mock.QuitApps)
	}

	// Window
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{Type: IntentWindow, WindowAction: WindowActionMinimize})
	if err != nil {
		t.Fatalf("ControlWindow failed: %v", err)
	}
	if msg != "Window action executed: minimize" {
		t.Errorf("unexpected message: %s", msg)
	}
	if len(mock.Windows) != 1 || mock.Windows[0] != WindowActionMinimize {
		t.Errorf("mock.Windows = %v", mock.Windows)
	}

	// Screenshot clipboard
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{
		Type:           IntentScreenshot,
		ScreenshotOpts: ScreenshotOptions{CopyToClipboard: true},
	})
	if err != nil {
		t.Fatalf("CaptureScreenshot failed: %v", err)
	}
	if msg != "Screenshot copied to clipboard" {
		t.Errorf("unexpected message: %s", msg)
	}

	// Screenshot file
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{
		Type:           IntentScreenshot,
		ScreenshotOpts: ScreenshotOptions{SavePath: "/tmp/snap.png"},
	})
	if err != nil {
		t.Fatalf("CaptureScreenshot failed: %v", err)
	}
	if msg != "Screenshot saved to /tmp/snap.png" {
		t.Errorf("unexpected message: %s", msg)
	}

	// Volume level
	lvl := 75
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{
		Type:        IntentVolume,
		VolumeLevel: &lvl,
	})
	if err != nil {
		t.Fatalf("SetVolume failed: %v", err)
	}
	if msg != "Volume set to 75%" {
		t.Errorf("unexpected message: %s", msg)
	}
	if mock.Volume != 75 {
		t.Errorf("mock.Volume = %d, want 75", mock.Volume)
	}

	// Volume mute
	mute := true
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{
		Type:      IntentVolume,
		MuteState: &mute,
	})
	if err != nil {
		t.Fatalf("MuteVolume failed: %v", err)
	}
	if msg != "Volume muted" {
		t.Errorf("unexpected message: %s", msg)
	}
	if !mock.Muted {
		t.Errorf("mock.Muted = false, want true")
	}

	// Volume unmute
	unmute := false
	msg, err = ExecuteSystemCommand(ctx, mock, SystemCommand{
		Type:      IntentVolume,
		MuteState: &unmute,
	})
	if err != nil {
		t.Fatalf("MuteVolume(false) failed: %v", err)
	}
	if msg != "Volume unmuted" {
		t.Errorf("unexpected message: %s", msg)
	}
	if mock.Muted {
		t.Errorf("mock.Muted = true, want false")
	}
}

func TestNormalizeAppName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vscode", "Visual Studio Code"},
		{"VS Code", "Visual Studio Code"},
		{"chrome", "Google Chrome"},
		{"slack", "Slack"},
		{"iterm2", "iTerm"},
		{"safari", "Safari"},
		{"settings", "System Settings"},
		{"CustomApp", "CustomApp"},
	}

	for _, tt := range tests {
		got := normalizeAppName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeAppName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }
