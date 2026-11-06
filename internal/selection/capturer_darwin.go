//go:build darwin

package selection

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
)

// DarwinCapturer implements selection capture on macOS using Accessibility and Cmd+C fallback.
type DarwinCapturer struct {
	preserveClipboard bool
	restoreDelay      time.Duration
}

// NewDarwinCapturer creates a new DarwinCapturer.
func NewDarwinCapturer() *DarwinCapturer {
	return &DarwinCapturer{
		preserveClipboard: true,
		restoreDelay:      60 * time.Millisecond,
	}
}

func (d *DarwinCapturer) Capture(ctx context.Context) (Result, error) {
	// 1. Tier 1: Accessibility API direct attribute query
	if text, err := d.captureAX(ctx); err == nil && strings.TrimSpace(text) != "" {
		return Result{
			Text:   text,
			Method: "accessibility_ax",
		}, nil
	}

	// 2. Tier 2: Keystroke Cmd+C with clipboard preservation
	if text, err := d.captureKeystroke(ctx); err == nil && strings.TrimSpace(text) != "" {
		return Result{
			Text:   text,
			Method: "clipboard_keystroke",
		}, nil
	}

	return Result{}, ErrNoSelection
}

func (d *DarwinCapturer) captureAX(ctx context.Context) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 350*time.Millisecond)
	defer cancel()

	script := `
		tell application "System Events"
			set frontApp to first application process whose frontmost is true
			set focusedEl to value of attribute "AXFocusedUIElement" of frontApp
			set selText to value of attribute "AXSelectedText" of focusedEl
			return selText
		end tell
	`
	cmd := exec.CommandContext(callCtx, "osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (d *DarwinCapturer) captureKeystroke(ctx context.Context) (string, error) {
	var originalClipboard string
	if d.preserveClipboard {
		originalClipboard, _ = paste.GetSystemClipboard()
	}

	sentinel := fmt.Sprintf("__cad_sel_%d__", time.Now().UnixNano())
	_ = paste.SetSystemClipboard(sentinel)

	callCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(callCtx, "osascript", "-e", `tell application "System Events" to keystroke "c" using command down`)
	if err := cmd.Run(); err != nil {
		if d.preserveClipboard && originalClipboard != "" {
			_ = paste.SetSystemClipboard(originalClipboard)
		}
		return "", err
	}

	time.Sleep(50 * time.Millisecond)

	newClipboard, err := paste.GetSystemClipboard()
	if err != nil || newClipboard == sentinel || strings.TrimSpace(newClipboard) == "" {
		if d.preserveClipboard && originalClipboard != "" {
			_ = paste.SetSystemClipboard(originalClipboard)
		}
		return "", ErrNoSelection
	}

	if d.preserveClipboard && originalClipboard != "" {
		go func(orig string) {
			time.Sleep(d.restoreDelay)
			_ = paste.SetSystemClipboard(orig)
		}(originalClipboard)
	}

	return newClipboard, nil
}

// NewDefaultCapturer returns the platform default selection capturer.
func NewDefaultCapturer() Capturer {
	return NewDarwinCapturer()
}
