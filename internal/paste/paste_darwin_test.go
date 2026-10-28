//go:build darwin

package paste

import (
	"testing"
)

func TestSystemClipboard_ReadWrite(t *testing.T) {
	original, _ := GetSystemClipboard()

	testMsg := "cadence_paste_test_123"
	if err := SetSystemClipboard(testMsg); err != nil {
		t.Fatalf("failed to set system clipboard: %v", err)
	}

	readBack, err := GetSystemClipboard()
	if err != nil {
		t.Fatalf("failed to get system clipboard: %v", err)
	}
	if readBack != testMsg {
		t.Fatalf("expected clipboard text %q, got %q", testMsg, readBack)
	}

	// Restore original clipboard
	if original != "" {
		_ = SetSystemClipboard(original)
	}
}

func TestDarwinTiers_Names(t *testing.T) {
	ax := NewAccessibilityTier()
	if ax.Name() != "accessibility_ax" {
		t.Fatalf("unexpected ax tier name: %s", ax.Name())
	}

	cb := NewClipboardKeystrokeTier()
	if cb.Name() != "clipboard_keystroke" {
		t.Fatalf("unexpected cb tier name: %s", cb.Name())
	}

	ks := NewKeystrokeTypeTier()
	if ks.Name() != "keystroke_type" {
		t.Fatalf("unexpected ks tier name: %s", ks.Name())
	}
}
