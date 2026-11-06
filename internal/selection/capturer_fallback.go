//go:build !darwin

package selection

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
)

// FallbackCapturer implements selection capture on Linux (Wayland/X11) and other systems.
type FallbackCapturer struct {
	preserveClipboard bool
	restoreDelay      time.Duration
}

// NewFallbackCapturer creates a FallbackCapturer.
func NewFallbackCapturer() *FallbackCapturer {
	return &FallbackCapturer{
		preserveClipboard: true,
		restoreDelay:      60 * time.Millisecond,
	}
}

func (f *FallbackCapturer) Capture(ctx context.Context) (Result, error) {
	// 1. Tier 1: Linux Primary Selection (highlighted text automatically goes into PRIMARY)
	if text, err := f.capturePrimary(ctx); err == nil && strings.TrimSpace(text) != "" {
		return Result{
			Text:   text,
			Method: "primary_selection",
		}, nil
	}

	// 2. Tier 2: Keystroke Ctrl+C with clipboard preservation
	if text, err := f.captureKeystroke(ctx); err == nil && strings.TrimSpace(text) != "" {
		return Result{
			Text:   text,
			Method: "clipboard_keystroke",
		}, nil
	}

	return Result{}, ErrNoSelection
}

func (f *FallbackCapturer) capturePrimary(ctx context.Context) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 350*time.Millisecond)
	defer cancel()

	if _, err := exec.LookPath("wl-paste"); err == nil {
		out, err := exec.CommandContext(callCtx, "wl-paste", "--primary", "--no-newline").Output()
		if err == nil && len(out) > 0 {
			return string(out), nil
		}
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		out, err := exec.CommandContext(callCtx, "xclip", "-selection", "primary", "-o").Output()
		if err == nil && len(out) > 0 {
			return string(out), nil
		}
	}

	if _, err := exec.LookPath("xsel"); err == nil {
		out, err := exec.CommandContext(callCtx, "xsel", "-p", "-o").Output()
		if err == nil && len(out) > 0 {
			return string(out), nil
		}
	}

	return "", ErrNoSelection
}

func (f *FallbackCapturer) captureKeystroke(ctx context.Context) (string, error) {
	var originalClipboard string
	if f.preserveClipboard {
		originalClipboard, _ = paste.GetSystemClipboard()
	}

	callCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	var sent bool
	if _, err := exec.LookPath("xdotool"); err == nil {
		if err := exec.CommandContext(callCtx, "xdotool", "key", "ctrl+c").Run(); err == nil {
			sent = true
		}
	} else if _, err := exec.LookPath("wtype"); err == nil {
		if err := exec.CommandContext(callCtx, "wtype", "-M", "ctrl", "-k", "c").Run(); err == nil {
			sent = true
		}
	}

	if !sent {
		return "", ErrNoSelection
	}

	time.Sleep(50 * time.Millisecond)

	newClipboard, err := paste.GetSystemClipboard()
	if err != nil || strings.TrimSpace(newClipboard) == "" {
		return "", ErrNoSelection
	}

	if f.preserveClipboard && originalClipboard != "" {
		go func(orig string) {
			time.Sleep(f.restoreDelay)
			_ = paste.SetSystemClipboard(orig)
		}(originalClipboard)
	}

	return newClipboard, nil
}

// NewDefaultCapturer returns the platform default selection capturer.
func NewDefaultCapturer() Capturer {
	return NewFallbackCapturer()
}
