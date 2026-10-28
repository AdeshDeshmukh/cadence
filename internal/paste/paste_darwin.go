//go:build darwin

package paste

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// AccessibilityTier attempts direct insertion into the focused AX element.
type AccessibilityTier struct{}

func NewAccessibilityTier() *AccessibilityTier {
	return &AccessibilityTier{}
}

func (a *AccessibilityTier) Name() string {
	return "accessibility_ax"
}

func (a *AccessibilityTier) Paste(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	script := fmt.Sprintf(`
		tell application "System Events"
			set frontApp to first application process whose frontmost is true
			set focusedEl to value of attribute "AXFocusedUIElement" of frontApp
			set value of focusedEl to %q
		end tell
	`, text)

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("accessibility insert failed: %w", err)
	}
	return nil
}

// ClipboardKeystrokeTier writes text to clipboard, emits Cmd+V, and restores previous clipboard.
type ClipboardKeystrokeTier struct {
	RestoreDelay      time.Duration
	PreserveClipboard bool
}

func NewClipboardKeystrokeTier() *ClipboardKeystrokeTier {
	return &ClipboardKeystrokeTier{
		RestoreDelay:      50 * time.Millisecond,
		PreserveClipboard: true,
	}
}

func (c *ClipboardKeystrokeTier) Name() string {
	return "clipboard_keystroke"
}

func (c *ClipboardKeystrokeTier) Paste(text string) error {
	var originalClipboard string

	if c.PreserveClipboard {
		originalClipboard, _ = GetSystemClipboard()
	}

	// 1. Set text into system clipboard
	if err := SetSystemClipboard(text); err != nil {
		return fmt.Errorf("failed to copy text to clipboard: %w", err)
	}

	// 2. Synthesize Cmd+V keystroke
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "osascript", "-e", `tell application "System Events" to keystroke "v" using command down`)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("synthesize Cmd+V keystroke failed: %w", err)
	}

	// 3. Restore previous clipboard if requested
	if c.PreserveClipboard && originalClipboard != "" {
		go func(restoreText string) {
			time.Sleep(c.RestoreDelay)
			_ = SetSystemClipboard(restoreText)
		}(originalClipboard)
	}

	return nil
}

// KeystrokeTypeTier types the characters out directly using AppleScript keystroke.
type KeystrokeTypeTier struct{}

func NewKeystrokeTypeTier() *KeystrokeTypeTier {
	return &KeystrokeTypeTier{}
}

func (k *KeystrokeTypeTier) Name() string {
	return "keystroke_type"
}

func (k *KeystrokeTypeTier) Paste(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	script := fmt.Sprintf(`tell application "System Events" to keystroke %q`, text)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keystroke typing failed: %w", err)
	}
	return nil
}

// GetSystemClipboard reads current text from macOS pasteboard.
func GetSystemClipboard() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pbpaste")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// SetSystemClipboard copies text to macOS pasteboard via pbcopy.
func SetSystemClipboard(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := io.WriteString(stdin, text); err != nil {
		_ = stdin.Close()
		return err
	}
	_ = stdin.Close()

	return cmd.Wait()
}

// NewDefaultCascade returns the macOS 3-tier cascade.
func NewDefaultCascade() *Cascade {
	return NewCascade(
		NewAccessibilityTier(),
		NewClipboardKeystrokeTier(),
		NewKeystrokeTypeTier(),
	)
}
