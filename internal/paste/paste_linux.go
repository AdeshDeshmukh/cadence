//go:build linux

package paste

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	linuxClipboardMu sync.RWMutex
	linuxClipboard   string
)

// LinuxDesktopType represents the display server detected.
type LinuxDesktopType string

const (
	DesktopX11     LinuxDesktopType = "x11"
	DesktopWayland LinuxDesktopType = "wayland"
	DesktopUnknown LinuxDesktopType = "unknown"
)

// DetectLinuxDesktop detects whether the current environment is Wayland, X11, or headless.
func DetectLinuxDesktop() LinuxDesktopType {
	if os.Getenv("WAYLAND_DISPLAY") != "" || strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return DesktopWayland
	}
	if os.Getenv("DISPLAY") != "" || strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "x11") {
		return DesktopX11
	}
	return DesktopUnknown
}

// X11TypeTier types text directly using xdotool without modifying the clipboard.
type X11TypeTier struct {
	Timeout time.Duration
}

func NewX11TypeTier() *X11TypeTier {
	return &X11TypeTier{
		Timeout: 2 * time.Second,
	}
}

func (x *X11TypeTier) Name() string {
	return "x11_xdotool_type"
}

func (x *X11TypeTier) Paste(text string) error {
	if os.Getenv("DISPLAY") == "" {
		return errors.New("DISPLAY environment variable not set")
	}
	bin, err := exec.LookPath("xdotool")
	if err != nil {
		return fmt.Errorf("xdotool not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), x.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "type", "--clearmodifiers", "--delay", "0", "--", text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xdotool typing failed: %w", err)
	}
	return nil
}

// X11ClipboardTier writes text to the X11 clipboard (via xclip or xsel) and triggers Ctrl+V via xdotool.
type X11ClipboardTier struct {
	Timeout           time.Duration
	RestoreDelay      time.Duration
	PreserveClipboard bool
}

func NewX11ClipboardTier() *X11ClipboardTier {
	return &X11ClipboardTier{
		Timeout:           2 * time.Second,
		RestoreDelay:      50 * time.Millisecond,
		PreserveClipboard: true,
	}
}

func (x *X11ClipboardTier) Name() string {
	return "x11_clipboard_keystroke"
}

func (x *X11ClipboardTier) Paste(text string) error {
	if os.Getenv("DISPLAY") == "" {
		return errors.New("DISPLAY environment variable not set")
	}
	xdotoolBin, err := exec.LookPath("xdotool")
	if err != nil {
		return fmt.Errorf("xdotool not found: %w", err)
	}

	var originalClipboard string
	if x.PreserveClipboard {
		originalClipboard, _ = GetSystemClipboard()
	}

	if err := writeX11Clipboard(text); err != nil {
		return fmt.Errorf("failed to copy to X11 clipboard: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), x.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, xdotoolBin, "key", "--clearmodifiers", "ctrl+v")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xdotool ctrl+v failed: %w", err)
	}

	if x.PreserveClipboard && originalClipboard != "" {
		go func(restoreText string) {
			time.Sleep(x.RestoreDelay)
			_ = SetSystemClipboard(restoreText)
		}(originalClipboard)
	}

	return nil
}

// WaylandTypeTier types text using wtype or ydotool on Wayland.
type WaylandTypeTier struct {
	Timeout time.Duration
}

func NewWaylandTypeTier() *WaylandTypeTier {
	return &WaylandTypeTier{
		Timeout: 2 * time.Second,
	}
}

func (w *WaylandTypeTier) Name() string {
	return "wayland_typing"
}

func (w *WaylandTypeTier) Paste(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()

	if bin, err := exec.LookPath("wtype"); err == nil {
		cmd := exec.CommandContext(ctx, bin, "--", text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	if bin, err := exec.LookPath("ydotool"); err == nil {
		cmd := exec.CommandContext(ctx, bin, "type", "--", text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return errors.New("no Wayland typing utility (wtype or ydotool) found or execution failed")
}

// WaylandClipboardTier sets clipboard via wl-copy and invokes Ctrl+V keystroke using wtype or ydotool.
type WaylandClipboardTier struct {
	Timeout           time.Duration
	RestoreDelay      time.Duration
	PreserveClipboard bool
}

func NewWaylandClipboardTier() *WaylandClipboardTier {
	return &WaylandClipboardTier{
		Timeout:           2 * time.Second,
		RestoreDelay:      50 * time.Millisecond,
		PreserveClipboard: true,
	}
}

func (w *WaylandClipboardTier) Name() string {
	return "wayland_clipboard_keystroke"
}

func (w *WaylandClipboardTier) Paste(text string) error {
	wlCopyBin, err := exec.LookPath("wl-copy")
	if err != nil {
		return fmt.Errorf("wl-copy not found: %w", err)
	}

	var originalClipboard string
	if w.PreserveClipboard {
		originalClipboard, _ = GetSystemClipboard()
	}

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, wlCopyBin)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wl-copy failed: %w", err)
	}

	// Emit Ctrl+V keystroke using wtype or ydotool
	pasted := false
	if wtypeBin, err := exec.LookPath("wtype"); err == nil {
		if err := exec.CommandContext(ctx, wtypeBin, "-M", "ctrl", "-k", "v").Run(); err == nil {
			pasted = true
		}
	}

	if !pasted {
		if ydotoolBin, err := exec.LookPath("ydotool"); err == nil {
			// KEY_LEFTCTRL (29), KEY_V (47)
			if err := exec.CommandContext(ctx, ydotoolBin, "key", "29:1", "47:1", "47:0", "29:0").Run(); err == nil {
				pasted = true
			}
		}
	}

	if !pasted {
		return errors.New("wl-copy copied text to clipboard but no keystroke synthesizer (wtype/ydotool) available")
	}

	if w.PreserveClipboard && originalClipboard != "" {
		go func(restoreText string) {
			time.Sleep(w.RestoreDelay)
			_ = SetSystemClipboard(restoreText)
		}(originalClipboard)
	}

	return nil
}

func writeX11Clipboard(text string) error {
	if bin, err := exec.LookPath("xclip"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}

	if bin, err := exec.LookPath("xsel"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-b", "-i")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}

	return errors.New("neither xclip nor xsel found for X11 clipboard")
}

// GetSystemClipboard returns clipboard text from Wayland (wl-paste), X11 (xclip/xsel), or in-memory fallback.
func GetSystemClipboard() (string, error) {
	if bin, err := exec.LookPath("wl-paste"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, "--no-newline").Output()
		if err == nil {
			return string(out), nil
		}
	}

	if bin, err := exec.LookPath("xclip"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, "-selection", "clipboard", "-o").Output()
		if err == nil {
			return string(out), nil
		}
	}

	if bin, err := exec.LookPath("xsel"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, "-b", "-o").Output()
		if err == nil {
			return string(out), nil
		}
	}

	linuxClipboardMu.RLock()
	defer linuxClipboardMu.RUnlock()
	return linuxClipboard, nil
}

// SetSystemClipboard copies text to Wayland, X11, or in-memory fallback.
func SetSystemClipboard(text string) error {
	if bin, err := exec.LookPath("wl-copy"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	if bin, err := exec.LookPath("xclip"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	if bin, err := exec.LookPath("xsel"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-b", "-i")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	linuxClipboardMu.Lock()
	linuxClipboard = text
	linuxClipboardMu.Unlock()
	return nil
}

// NewDefaultCascade returns the tiered Linux cascade:
// X11 direct typing -> X11 clipboard+keystroke -> Wayland typing -> Wayland clipboard+keystroke -> In-memory fallback
func NewDefaultCascade() *Cascade {
	return NewCascade(
		NewX11TypeTier(),
		NewX11ClipboardTier(),
		NewWaylandTypeTier(),
		NewWaylandClipboardTier(),
		NewFallbackMemoryTier(),
	)
}

// NativeReverter sends an Undo shortcut (Ctrl+Z) on Linux via xdotool, wtype, or ydotool.
type NativeReverter struct{}

func NewNativeReverter() *NativeReverter {
	return &NativeReverter{}
}

func (n *NativeReverter) Revert() error {
	if bin, err := exec.LookPath("xdotool"); err == nil && os.Getenv("DISPLAY") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, "key", "--clearmodifiers", "ctrl+z").Run()
	}
	if bin, err := exec.LookPath("wtype"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, "-M", "ctrl", "-k", "z").Run()
	}
	if bin, err := exec.LookPath("ydotool"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, "key", "29:1", "44:1", "44:0", "29:0").Run()
	}
	return nil
}
