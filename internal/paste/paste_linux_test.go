//go:build linux

package paste

import (
	"os"
	"testing"
)

func TestLinuxDesktopDetection(t *testing.T) {
	origWayland := os.Getenv("WAYLAND_DISPLAY")
	origSession := os.Getenv("XDG_SESSION_TYPE")
	origDisplay := os.Getenv("DISPLAY")
	defer func() {
		_ = os.Setenv("WAYLAND_DISPLAY", origWayland)
		_ = os.Setenv("XDG_SESSION_TYPE", origSession)
		_ = os.Setenv("DISPLAY", origDisplay)
	}()

	// Case 1: Wayland display
	_ = os.Setenv("WAYLAND_DISPLAY", "wayland-0")
	_ = os.Setenv("DISPLAY", "")
	_ = os.Setenv("XDG_SESSION_TYPE", "")
	if dt := DetectLinuxDesktop(); dt != DesktopWayland {
		t.Errorf("expected DesktopWayland, got %v", dt)
	}

	// Case 2: Wayland session type
	_ = os.Setenv("WAYLAND_DISPLAY", "")
	_ = os.Setenv("XDG_SESSION_TYPE", "wayland")
	if dt := DetectLinuxDesktop(); dt != DesktopWayland {
		t.Errorf("expected DesktopWayland, got %v", dt)
	}

	// Case 3: X11 display
	_ = os.Setenv("WAYLAND_DISPLAY", "")
	_ = os.Setenv("XDG_SESSION_TYPE", "")
	_ = os.Setenv("DISPLAY", ":0")
	if dt := DetectLinuxDesktop(); dt != DesktopX11 {
		t.Errorf("expected DesktopX11, got %v", dt)
	}

	// Case 4: Headless / unknown
	_ = os.Setenv("WAYLAND_DISPLAY", "")
	_ = os.Setenv("XDG_SESSION_TYPE", "")
	_ = os.Setenv("DISPLAY", "")
	if dt := DetectLinuxDesktop(); dt != DesktopUnknown {
		t.Errorf("expected DesktopUnknown, got %v", dt)
	}
}

func TestLinuxTiers_Names(t *testing.T) {
	x11Type := NewX11TypeTier()
	if x11Type.Name() != "x11_xdotool_type" {
		t.Errorf("unexpected name: %s", x11Type.Name())
	}

	x11Clip := NewX11ClipboardTier()
	if x11Clip.Name() != "x11_clipboard_keystroke" {
		t.Errorf("unexpected name: %s", x11Clip.Name())
	}

	wlType := NewWaylandTypeTier()
	if wlType.Name() != "wayland_typing" {
		t.Errorf("unexpected name: %s", wlType.Name())
	}

	wlClip := NewWaylandClipboardTier()
	if wlClip.Name() != "wayland_clipboard_keystroke" {
		t.Errorf("unexpected name: %s", wlClip.Name())
	}

	mem := NewFallbackMemoryTier()
	if mem.Name() != "fallback_memory" {
		t.Errorf("unexpected name: %s", mem.Name())
	}
}

func TestLinuxClipboard_ReadWrite(t *testing.T) {
	testMsg := "cadence_linux_clipboard_test_456"
	if err := SetSystemClipboard(testMsg); err != nil {
		t.Fatalf("SetSystemClipboard failed: %v", err)
	}

	readBack, err := GetSystemClipboard()
	if err != nil {
		t.Fatalf("GetSystemClipboard failed: %v", err)
	}

	if readBack != testMsg {
		t.Fatalf("expected clipboard text %q, got %q", testMsg, readBack)
	}
}

func TestLinuxCascade_DefaultOrder(t *testing.T) {
	c := NewDefaultCascade()
	tiers := c.Tiers()

	expected := []string{
		"x11_xdotool_type",
		"x11_clipboard_keystroke",
		"wayland_typing",
		"wayland_clipboard_keystroke",
		"fallback_memory",
	}

	if len(tiers) != len(expected) {
		t.Fatalf("expected %d tiers, got %d: %v", len(expected), len(tiers), tiers)
	}

	for i, name := range expected {
		if tiers[i] != name {
			t.Errorf("tier %d: expected %s, got %s", i, name, tiers[i])
		}
	}
}

func TestLinuxCascade_Execution(t *testing.T) {
	c := NewDefaultCascade()
	res, err := c.Paste("hello from linux test")
	if err != nil {
		t.Fatalf("cascade paste failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected res.Success to be true")
	}

	if res.Method == "" {
		t.Errorf("expected non-empty res.Method")
	}
}

func TestLinuxNativeReverter(t *testing.T) {
	reverter := NewNativeReverter()
	// In headless/test environment, Revert should safely return nil or not crash
	err := reverter.Revert()
	// Non-error or expected graceful failure
	_ = err
}

func TestX11Tiers_DisplayMissing(t *testing.T) {
	origDisplay := os.Getenv("DISPLAY")
	defer func() { _ = os.Setenv("DISPLAY", origDisplay) }()
	_ = os.Setenv("DISPLAY", "")

	t1 := NewX11TypeTier()
	if err := t1.Paste("test"); err == nil {
		t.Errorf("expected error when DISPLAY is empty")
	}

	t2 := NewX11ClipboardTier()
	if err := t2.Paste("test"); err == nil {
		t.Errorf("expected error when DISPLAY is empty")
	}
}
