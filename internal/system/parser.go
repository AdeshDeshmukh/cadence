package system

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// IntentType identifies what system action the user wants to perform.
type IntentType string

const (
	IntentLaunchApp  IntentType = "launch_app"
	IntentFocusApp   IntentType = "focus_app"
	IntentQuitApp    IntentType = "quit_app"
	IntentWindow     IntentType = "window_action"
	IntentScreenshot IntentType = "screenshot"
	IntentVolume     IntentType = "volume"
)

// SystemCommand represents a parsed system intent and its associated parameters.
type SystemCommand struct {
	Type           IntentType        `json:"type"`
	AppName        string            `json:"app_name,omitempty"`
	WindowAction   WindowAction      `json:"window_action,omitempty"`
	ScreenshotOpts ScreenshotOptions `json:"screenshot_opts,omitempty"`
	VolumeLevel    *int              `json:"volume_level,omitempty"`
	MuteState      *bool             `json:"mute_state,omitempty"`
	RawInput       string            `json:"raw_input"`
}

var (
	reVolumeSet = regexp.MustCompile(`(?i)^(?:set\s+)?volume\s+(?:to\s+)?(\d{1,3})(?:%)?$`)
	reOpenApp   = regexp.MustCompile(`(?i)^(?:open|launch|start)\s+(?:application\s+|app\s+)?(.+)$`)
	reFocusApp  = regexp.MustCompile(`(?i)^(?:switch\s+to|focus|bring\s+up)\s+(?:application\s+|app\s+)?(.+)$`)
	reQuitApp   = regexp.MustCompile(`(?i)^(?:quit|exit|kill)\s+(?:application\s+|app\s+)?(.+)$`)
)

// ParseSystemCommand parses natural voice speech into a structured SystemCommand.
func ParseSystemCommand(input string) (SystemCommand, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return SystemCommand{}, false
	}
	clean := strings.ToLower(s)

	// Strip common conversational voice prefixes: "please ", "cadence ", "can you "
	for {
		orig := clean
		clean = strings.TrimPrefix(clean, "please ")
		clean = strings.TrimPrefix(clean, "cadence ")
		clean = strings.TrimPrefix(clean, "can you ")
		clean = strings.TrimPrefix(clean, "could you ")
		clean = strings.TrimSpace(clean)
		if clean == orig {
			break
		}
	}

	// 1. Screenshot Intents
	if clean == "take screenshot" || clean == "take a screenshot" || clean == "screenshot" ||
		clean == "capture screen" || clean == "screen capture" || strings.HasPrefix(clean, "screenshot") {
		opts := ScreenshotOptions{}
		if strings.Contains(clean, "window") || strings.Contains(clean, "active") {
			opts.ActiveWindowOnly = true
		}
		if strings.Contains(clean, "clipboard") || strings.Contains(clean, "copy") {
			opts.CopyToClipboard = true
		}
		return SystemCommand{
			Type:           IntentScreenshot,
			ScreenshotOpts: opts,
			RawInput:       input,
		}, true
	}

	// 2. Window Control Intents
	switch clean {
	case "minimize window", "minimize", "minimize the window", "minimize active window":
		return SystemCommand{
			Type:         IntentWindow,
			WindowAction: WindowActionMinimize,
			RawInput:     input,
		}, true
	case "maximize window", "maximize", "fullscreen", "full screen", "maximize the window":
		return SystemCommand{
			Type:         IntentWindow,
			WindowAction: WindowActionMaximize,
			RawInput:     input,
		}, true
	case "close window", "close the window", "close active window":
		return SystemCommand{
			Type:         IntentWindow,
			WindowAction: WindowActionClose,
			RawInput:     input,
		}, true
	case "hide window", "hide app", "hide application", "hide the window":
		return SystemCommand{
			Type:         IntentWindow,
			WindowAction: WindowActionHide,
			RawInput:     input,
		}, true
	}

	// 3. Volume Intents
	if clean == "mute" || clean == "mute volume" || clean == "silence" || clean == "mute audio" {
		t := true
		return SystemCommand{
			Type:      IntentVolume,
			MuteState: &t,
			RawInput:  input,
		}, true
	}
	if clean == "unmute" || clean == "unmute volume" || clean == "unmute audio" {
		f := false
		return SystemCommand{
			Type:      IntentVolume,
			MuteState: &f,
			RawInput:  input,
		}, true
	}
	if m := reVolumeSet.FindStringSubmatch(clean); len(m) == 2 {
		lvl, err := strconv.Atoi(m[1])
		if err == nil {
			if lvl > 100 {
				lvl = 100
			}
			return SystemCommand{
				Type:        IntentVolume,
				VolumeLevel: &lvl,
				RawInput:    input,
			}, true
		}
	}

	// 4. Focus App Intents ("switch to <app>", "focus <app>", "bring up <app>")
	if m := reFocusApp.FindStringSubmatch(clean); len(m) == 2 {
		target := strings.TrimSpace(m[1])
		if target != "" {
			return SystemCommand{
				Type:     IntentFocusApp,
				AppName:  target,
				RawInput: input,
			}, true
		}
	}

	// 5. Quit App Intents ("quit <app>", "exit <app>", "kill <app>")
	if m := reQuitApp.FindStringSubmatch(clean); len(m) == 2 {
		target := strings.TrimSpace(m[1])
		if target != "" {
			return SystemCommand{
				Type:     IntentQuitApp,
				AppName:  target,
				RawInput: input,
			}, true
		}
	}

	// 6. Launch App Intents ("open <app>", "launch <app>", "start <app>")
	if m := reOpenApp.FindStringSubmatch(clean); len(m) == 2 {
		target := strings.TrimSpace(m[1])
		if target != "" {
			return SystemCommand{
				Type:     IntentLaunchApp,
				AppName:  target,
				RawInput: input,
			}, true
		}
	}

	return SystemCommand{}, false
}

// ExecuteSystemCommand executes a parsed SystemCommand using the provided Controller.
func ExecuteSystemCommand(ctx context.Context, ctrl Controller, cmd SystemCommand) (string, error) {
	if ctrl == nil {
		return "", fmt.Errorf("system controller not initialized")
	}

	switch cmd.Type {
	case IntentLaunchApp:
		if err := ctrl.LaunchApp(ctx, cmd.AppName); err != nil {
			return "", err
		}
		return fmt.Sprintf("Launched %s", cmd.AppName), nil

	case IntentFocusApp:
		if err := ctrl.FocusApp(ctx, cmd.AppName); err != nil {
			return "", err
		}
		return fmt.Sprintf("Switched to %s", cmd.AppName), nil

	case IntentQuitApp:
		if err := ctrl.QuitApp(ctx, cmd.AppName); err != nil {
			return "", err
		}
		return fmt.Sprintf("Quit %s", cmd.AppName), nil

	case IntentWindow:
		if err := ctrl.ControlWindow(ctx, cmd.WindowAction); err != nil {
			return "", err
		}
		return fmt.Sprintf("Window action executed: %s", cmd.WindowAction), nil

	case IntentScreenshot:
		res, err := ctrl.CaptureScreenshot(ctx, cmd.ScreenshotOpts)
		if err != nil {
			return "", err
		}
		if res.CopiedToClipboard {
			return "Screenshot copied to clipboard", nil
		}
		return fmt.Sprintf("Screenshot saved to %s", res.Path), nil

	case IntentVolume:
		if cmd.MuteState != nil {
			if err := ctrl.MuteVolume(ctx, *cmd.MuteState); err != nil {
				return "", err
			}
			if *cmd.MuteState {
				return "Volume muted", nil
			}
			return "Volume unmuted", nil
		}
		if cmd.VolumeLevel != nil {
			if err := ctrl.SetVolume(ctx, *cmd.VolumeLevel); err != nil {
				return "", err
			}
			return fmt.Sprintf("Volume set to %d%%", *cmd.VolumeLevel), nil
		}
		return "Volume unchanged", nil

	default:
		return "", fmt.Errorf("unknown intent type: %s", cmd.Type)
	}
}
