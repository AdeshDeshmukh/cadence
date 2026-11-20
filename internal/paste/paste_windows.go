//go:build windows

package paste

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	winClipboardMu sync.RWMutex
	winClipboard   string
)

// Win32TypeTier types text directly using Windows Forms SendKeys.
type Win32TypeTier struct {
	Timeout time.Duration
}

func NewWin32TypeTier() *Win32TypeTier {
	return &Win32TypeTier{
		Timeout: 2 * time.Second,
	}
}

func (w *Win32TypeTier) Name() string {
	return "win32_keystroke_type"
}

func (w *Win32TypeTier) Paste(text string) error {
	psBin, err := exec.LookPath("powershell")
	if err != nil {
		return fmt.Errorf("powershell not found: %w", err)
	}

	escaped := escapeSendKeys(text)
	cmdText := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(%q)`, escaped)

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-Command", cmdText)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SendKeys typing failed: %w", err)
	}
	return nil
}

// Win32ClipboardTier writes text to the Windows clipboard and sends Ctrl+V.
type Win32ClipboardTier struct {
	Timeout           time.Duration
	RestoreDelay      time.Duration
	PreserveClipboard bool
}

func NewWin32ClipboardTier() *Win32ClipboardTier {
	return &Win32ClipboardTier{
		Timeout:           2 * time.Second,
		RestoreDelay:      50 * time.Millisecond,
		PreserveClipboard: true,
	}
}

func (w *Win32ClipboardTier) Name() string {
	return "win32_clipboard_keystroke"
}

func (w *Win32ClipboardTier) Paste(text string) error {
	psBin, err := exec.LookPath("powershell")
	if err != nil {
		return fmt.Errorf("powershell not found: %w", err)
	}

	var originalClipboard string
	if w.PreserveClipboard {
		originalClipboard, _ = GetSystemClipboard()
	}

	if err := SetSystemClipboard(text); err != nil {
		return fmt.Errorf("failed to set Windows clipboard: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()

	// Synthesize Ctrl+V with SendKeys
	cmdText := `Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait("^v")`
	cmd := exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-Command", cmdText)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Ctrl+V keystroke synthesis failed: %w", err)
	}

	if w.PreserveClipboard && originalClipboard != "" {
		go func(restoreText string) {
			time.Sleep(w.RestoreDelay)
			_ = SetSystemClipboard(restoreText)
		}(originalClipboard)
	}

	return nil
}

// escapeSendKeys escapes special characters for System.Windows.Forms.SendKeys.
// Characters with special meaning: +, ^, %, ~, (, ), [, ], {, }
func escapeSendKeys(text string) string {
	var sb strings.Builder
	for _, r := range text {
		switch r {
		case '+', '^', '%', '~', '(', ')', '[', ']', '{', '}':
			sb.WriteRune('{')
			sb.WriteRune(r)
			sb.WriteRune('}')
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// GetSystemClipboard returns clipboard text from Windows PowerShell or memory fallback.
func GetSystemClipboard() (string, error) {
	if psBin, err := exec.LookPath("powershell"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard")
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimRight(string(out), "\r\n"), nil
		}
	}

	winClipboardMu.RLock()
	defer winClipboardMu.RUnlock()
	return winClipboard, nil
}

// SetSystemClipboard copies text to the Windows clipboard via PowerShell or memory fallback.
func SetSystemClipboard(text string) error {
	winClipboardMu.Lock()
	winClipboard = text
	winClipboardMu.Unlock()

	if psBin, err := exec.LookPath("powershell"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-Command", "$Input | Set-Clipboard")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
	}

	return nil
}

// NewDefaultCascade returns the Windows cascade:
// Win32 direct typing -> Win32 clipboard+keystroke -> In-memory fallback
func NewDefaultCascade() *Cascade {
	return NewCascade(
		NewWin32TypeTier(),
		NewWin32ClipboardTier(),
		NewFallbackMemoryTier(),
	)
}

// NativeReverter sends an Undo shortcut (Ctrl+Z) on Windows via SendKeys.
type NativeReverter struct{}

func NewNativeReverter() *NativeReverter {
	return &NativeReverter{}
}

func (n *NativeReverter) Revert() error {
	if psBin, err := exec.LookPath("powershell"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmdText := `Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait("^z")`
		return exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-Command", cmdText).Run()
	}
	return nil
}
