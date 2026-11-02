//go:build !darwin

package paste

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	fallbackClipboardMu sync.RWMutex
	fallbackClipboard   string
)

// FallbackMemoryTier is an in-memory mock tier for headless or non-Darwin platforms.
type FallbackMemoryTier struct {
	clipboard *Clipboard
}

func NewFallbackMemoryTier() *FallbackMemoryTier {
	return &FallbackMemoryTier{
		clipboard: NewClipboard(),
	}
}

func (f *FallbackMemoryTier) Name() string {
	return "fallback_memory"
}

func (f *FallbackMemoryTier) Paste(text string) error {
	_, err := f.clipboard.Paste(text)
	return err
}

// NewDefaultCascade returns the fallback cascade for non-macOS systems.
func NewDefaultCascade() *Cascade {
	return NewCascade(NewFallbackMemoryTier())
}

// GetSystemClipboard returns clipboard text from Wayland/X11 or memory fallback.
func GetSystemClipboard() (string, error) {
	if _, err := exec.LookPath("wl-paste"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, "wl-paste", "--no-newline").Output()
		if err == nil {
			return string(out), nil
		}
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-o").Output()
		if err == nil {
			return string(out), nil
		}
	}

	fallbackClipboardMu.RLock()
	defer fallbackClipboardMu.RUnlock()
	return fallbackClipboard, nil
}

// SetSystemClipboard copies text to Wayland/X11 or memory fallback.
func SetSystemClipboard(text string) error {
	if _, err := exec.LookPath("wl-copy"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "wl-copy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	fallbackClipboardMu.Lock()
	fallbackClipboard = text
	fallbackClipboardMu.Unlock()
	return nil
}
