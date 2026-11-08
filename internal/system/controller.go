package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// WindowAction represents an operation on a window.
type WindowAction string

const (
	WindowActionMinimize WindowAction = "minimize"
	WindowActionMaximize WindowAction = "maximize"
	WindowActionClose    WindowAction = "close"
	WindowActionHide     WindowAction = "hide"
)

// ScreenshotOptions configures a screenshot capture.
type ScreenshotOptions struct {
	ActiveWindowOnly bool   `json:"active_window_only"`
	SavePath         string `json:"save_path,omitempty"`
	CopyToClipboard  bool   `json:"copy_to_clipboard"`
}

// ScreenshotResult contains metadata about the captured screenshot.
type ScreenshotResult struct {
	Path              string    `json:"path,omitempty"`
	CopiedToClipboard bool      `json:"copied_to_clipboard"`
	Timestamp         time.Time `json:"timestamp"`
}

// Controller defines the interface for desktop system automation.
type Controller interface {
	LaunchApp(ctx context.Context, appName string) error
	FocusApp(ctx context.Context, appName string) error
	QuitApp(ctx context.Context, appName string) error
	ControlWindow(ctx context.Context, action WindowAction) error
	CaptureScreenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error)
	SetVolume(ctx context.Context, level int) error
	MuteVolume(ctx context.Context, mute bool) error
}

// NewDefaultController creates an OS-appropriate controller (Darwin on macOS, Mock on others).
func NewDefaultController() Controller {
	if runtime.GOOS == "darwin" {
		return NewDarwinController()
	}
	return NewMockController()
}

// DarwinController executes system commands via native macOS AppleScript and binaries.
type DarwinController struct{}

// NewDarwinController creates a new macOS system controller.
func NewDarwinController() *DarwinController {
	return &DarwinController{}
}

func (d *DarwinController) LaunchApp(ctx context.Context, appName string) error {
	normalized := normalizeAppName(appName)
	cmd := exec.CommandContext(ctx, "open", "-a", normalized)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launch app %q: %w (output: %s)", appName, err, string(out))
	}
	return nil
}

func (d *DarwinController) FocusApp(ctx context.Context, appName string) error {
	normalized := normalizeAppName(appName)
	script := fmt.Sprintf(`tell application "%s" to activate`, normalized)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fallback to open -a
		openCmd := exec.CommandContext(ctx, "open", "-a", normalized)
		if oOut, oErr := openCmd.CombinedOutput(); oErr != nil {
			return fmt.Errorf("focus app %q: %w (output: %s)", appName, err, string(out)+string(oOut))
		}
	}
	return nil
}

func (d *DarwinController) QuitApp(ctx context.Context, appName string) error {
	normalized := normalizeAppName(appName)
	script := fmt.Sprintf(`tell application "%s" to quit`, normalized)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("quit app %q: %w (output: %s)", appName, err, string(out))
	}
	return nil
}

func (d *DarwinController) ControlWindow(ctx context.Context, action WindowAction) error {
	var script string
	switch action {
	case WindowActionMinimize:
		script = `tell application "System Events"
			set frontApp to first application process whose frontmost is true
			tell frontApp
				if (count of windows) > 0 then
					set value of attribute "AXMinimized" of window 1 to true
				end if
			end tell
		end tell`
	case WindowActionClose:
		script = `tell application "System Events"
			set frontApp to first application process whose frontmost is true
			tell frontApp
				if (count of windows) > 0 then
					perform action "AXPress" of (first button whose subrole is "AXCloseButton") of window 1
				end if
			end tell
		end tell`
	case WindowActionHide:
		script = `tell application "System Events"
			set frontApp to first application process whose frontmost is true
			set visible of frontApp to false
		end tell`
	case WindowActionMaximize:
		script = `tell application "System Events"
			set frontApp to first application process whose frontmost is true
			tell frontApp
				if (count of windows) > 0 then
					set value of attribute "AXFullScreen" of window 1 to true
				end if
			end tell
		end tell`
	default:
		return fmt.Errorf("unsupported window action: %s", action)
	}

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("control window (%s): %w (output: %s)", action, err, string(out))
	}
	return nil
}

func (d *DarwinController) CaptureScreenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error) {
	var args []string

	// Active window only
	if opts.ActiveWindowOnly {
		args = append(args, "-w")
	}

	// Clipboard or file
	now := time.Now().UTC()
	if opts.CopyToClipboard {
		args = append(args, "-c")
	}

	savePath := opts.SavePath
	if !opts.CopyToClipboard {
		if savePath == "" {
			home, _ := os.UserHomeDir()
			desktop := filepath.Join(home, "Desktop")
			if _, err := os.Stat(desktop); err != nil {
				desktop = os.TempDir()
			}
			savePath = filepath.Join(desktop, fmt.Sprintf("Screenshot_%s.png", now.Format("2006-01-02_15.04.05")))
		}
		args = append(args, savePath)
	}

	cmd := exec.CommandContext(ctx, "screencapture", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return ScreenshotResult{}, fmt.Errorf("screencapture: %w (output: %s)", err, string(out))
	}

	return ScreenshotResult{
		Path:              savePath,
		CopiedToClipboard: opts.CopyToClipboard,
		Timestamp:         now,
	}, nil
}

func (d *DarwinController) SetVolume(ctx context.Context, level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	script := fmt.Sprintf("set volume output volume %d", level)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set volume to %d: %w (output: %s)", level, err, string(out))
	}
	return nil
}

func (d *DarwinController) MuteVolume(ctx context.Context, mute bool) error {
	val := "false"
	if mute {
		val = "true"
	}
	script := fmt.Sprintf("set volume output muted %s", val)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mute volume (%s): %w (output: %s)", val, err, string(out))
	}
	return nil
}

// normalizeAppName maps common spoken app names to canonical macOS application bundle names.
func normalizeAppName(spoken string) string {
	s := strings.ToLower(strings.TrimSpace(spoken))
	switch s {
	case "vs code", "vscode", "code", "visual studio code":
		return "Visual Studio Code"
	case "iterm", "iterm2":
		return "iTerm"
	case "terminal":
		return "Terminal"
	case "chrome", "google chrome":
		return "Google Chrome"
	case "safari":
		return "Safari"
	case "slack":
		return "Slack"
	case "spotify":
		return "Spotify"
	case "finder":
		return "Finder"
	case "notes", "apple notes":
		return "Notes"
	case "calculator":
		return "Calculator"
	case "system settings", "settings", "preferences":
		return "System Settings"
	default:
		return spoken
	}
}

// MockController provides an in-memory test implementation of Controller.
type MockController struct {
	mu           sync.Mutex
	LaunchedApps []string
	FocusedApps  []string
	QuitApps     []string
	Windows      []WindowAction
	Screenshots  []ScreenshotOptions
	Volume       int
	Muted        bool
}

// NewMockController creates a new mock system controller.
func NewMockController() *MockController {
	return &MockController{Volume: 50}
}

func (m *MockController) LaunchApp(_ context.Context, appName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LaunchedApps = append(m.LaunchedApps, appName)
	return nil
}

func (m *MockController) FocusApp(_ context.Context, appName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FocusedApps = append(m.FocusedApps, appName)
	return nil
}

func (m *MockController) QuitApp(_ context.Context, appName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QuitApps = append(m.QuitApps, appName)
	return nil
}

func (m *MockController) ControlWindow(_ context.Context, action WindowAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Windows = append(m.Windows, action)
	return nil
}

func (m *MockController) CaptureScreenshot(_ context.Context, opts ScreenshotOptions) (ScreenshotResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Screenshots = append(m.Screenshots, opts)
	return ScreenshotResult{
		Path:              opts.SavePath,
		CopiedToClipboard: opts.CopyToClipboard,
		Timestamp:         time.Now().UTC(),
	}, nil
}

func (m *MockController) SetVolume(_ context.Context, level int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	m.Volume = level
	return nil
}

func (m *MockController) MuteVolume(_ context.Context, mute bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Muted = mute
	return nil
}
