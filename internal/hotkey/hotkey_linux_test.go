//go:build linux

package hotkey

import (
	"errors"
	"testing"
	"time"
)

func TestLinuxEvdevListener_Lifecycle(t *testing.T) {
	l := NewLinuxEvdevListener("Ctrl+Shift+Space", ModePushToTalk, "")
	if l.IsListening() {
		t.Fatalf("expected not listening initially")
	}

	// Double stop returns ErrNotListening
	if err := l.Stop(); !errors.Is(err, ErrNotListening) {
		t.Fatalf("expected ErrNotListening on early stop, got %v", err)
	}

	// Start
	ch, err := l.Start()
	if err != nil {
		t.Fatalf("unexpected error starting: %v", err)
	}
	if !l.IsListening() {
		t.Fatalf("expected listening after Start()")
	}

	// Double start returns ErrAlreadyListening
	if _, err := l.Start(); !errors.Is(err, ErrAlreadyListening) {
		t.Fatalf("expected ErrAlreadyListening, got %v", err)
	}

	// Emit key down and key up
	go func() {
		l.Emit(EventKeyDown)
		time.Sleep(10 * time.Millisecond)
		l.Emit(EventKeyUp)
	}()

	select {
	case ev1 := <-ch:
		if ev1.Type != EventKeyDown || ev1.Shortcut != "Ctrl+Shift+Space" {
			t.Fatalf("unexpected event 1: %+v", ev1)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for key down event")
	}

	select {
	case ev2 := <-ch:
		if ev2.Type != EventKeyUp || ev2.Shortcut != "Ctrl+Shift+Space" {
			t.Fatalf("unexpected event 2: %+v", ev2)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for key up event")
	}

	// Stop
	if err := l.Stop(); err != nil {
		t.Fatalf("unexpected error stopping: %v", err)
	}

	if l.IsListening() {
		t.Fatalf("expected not listening after Stop()")
	}
}

func TestLinuxEvdevListener_ModeAndShortcut(t *testing.T) {
	l := NewLinuxEvdevListener("", "", "")
	if l.Mode() != ModePushToTalk {
		t.Fatalf("expected default push to talk, got %s", l.Mode())
	}
	if l.Shortcut() != "Ctrl+Shift+Space" {
		t.Fatalf("expected default shortcut, got %s", l.Shortcut())
	}

	l.SetMode(ModeToggle)
	if l.Mode() != ModeToggle {
		t.Fatalf("expected mode toggle, got %s", l.Mode())
	}
}

func TestLinuxDefaultListener(t *testing.T) {
	l := NewDefaultListener("Ctrl+D", ModePushToTalk)
	if l.Shortcut() != "Ctrl+D" {
		t.Fatalf("expected shortcut Ctrl+D, got %s", l.Shortcut())
	}
	ch, err := l.Start()
	if err != nil {
		t.Fatalf("failed to start default listener: %v", err)
	}
	defer l.Stop()

	if !l.IsListening() {
		t.Fatalf("expected listening")
	}
	_ = ch
}

func TestFindKeyboardDevices_DoesNotPanic(t *testing.T) {
	// Should gracefully return devices or error in headless/container environments without panicking
	_, _ = FindKeyboardDevices()
}
