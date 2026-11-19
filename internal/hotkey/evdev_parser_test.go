package hotkey

import (
	"bytes"
	"testing"
)

func TestEvdev_BinaryEncodeDecode(t *testing.T) {
	orig := InputEvent{
		Sec:   1700000000,
		Usec:  123456,
		Type:  EvdevTypeKey,
		Code:  KeySpace,
		Value: EvdevValuePress,
	}

	encoded := EncodeInputEvent(orig)
	if len(encoded) != 24 {
		t.Fatalf("expected 24 bytes, got %d", len(encoded))
	}

	decoded, err := ReadInputEvent(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("ReadInputEvent failed: %v", err)
	}

	if decoded != orig {
		t.Fatalf("expected %+v, got %+v", orig, decoded)
	}
}

func TestModifierTracker(t *testing.T) {
	var tracker ModifierTracker

	// Initial
	if mods := tracker.CurrentModifiers(); mods.Ctrl || mods.Alt || mods.Shift || mods.Cmd {
		t.Fatalf("expected all false initially, got %+v", mods)
	}

	// Press LeftCtrl
	tracker.Update(KeyLeftCtrl, EvdevValuePress)
	if !tracker.CurrentModifiers().Ctrl {
		t.Errorf("expected Ctrl to be true after LeftCtrl press")
	}

	// Press RightCtrl
	tracker.Update(KeyRightCtrl, EvdevValuePress)
	if !tracker.CurrentModifiers().Ctrl {
		t.Errorf("expected Ctrl to remain true")
	}

	// Release LeftCtrl (RightCtrl still held)
	tracker.Update(KeyLeftCtrl, EvdevValueRelease)
	if !tracker.CurrentModifiers().Ctrl {
		t.Errorf("expected Ctrl to remain true while RightCtrl is held")
	}

	// Release RightCtrl
	tracker.Update(KeyRightCtrl, EvdevValueRelease)
	if tracker.CurrentModifiers().Ctrl {
		t.Errorf("expected Ctrl to be false after releasing all Ctrl keys")
	}

	// Test Meta/Super
	tracker.Update(KeyLeftMeta, EvdevValuePress)
	if !tracker.CurrentModifiers().Cmd {
		t.Errorf("expected Cmd to be true after LeftMeta press")
	}
	tracker.Update(KeyLeftMeta, EvdevValueRelease)
	if tracker.CurrentModifiers().Cmd {
		t.Errorf("expected Cmd to be false after LeftMeta release")
	}
}

func TestShortcutMatcher_PrimaryWithModifiers(t *testing.T) {
	target, err := ParseShortcut("Ctrl+Shift+Space")
	if err != nil {
		t.Fatalf("ParseShortcut failed: %v", err)
	}

	matcher := NewShortcutMatcher(target)

	// Step 1: Press LeftCtrl
	trig, evType := matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftCtrl, Value: EvdevValuePress})
	if trig {
		t.Fatalf("unexpected trigger on LeftCtrl alone: %v", evType)
	}

	// Step 2: Press LeftShift
	trig, evType = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftShift, Value: EvdevValuePress})
	if trig {
		t.Fatalf("unexpected trigger on LeftShift alone: %v", evType)
	}

	// Step 3: Press Space (all required modifiers active)
	trig, evType = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeySpace, Value: EvdevValuePress})
	if !trig || evType != EventKeyDown {
		t.Fatalf("expected EventKeyDown, got trig=%v, evType=%v", trig, evType)
	}

	// Step 4: Autorepeat Space (should not trigger duplicate KeyDown)
	trig, _ = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeySpace, Value: EvdevValueAutorepeat})
	if trig {
		t.Fatalf("expected no trigger on autorepeat")
	}

	// Step 5: Release Space
	trig, evType = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeySpace, Value: EvdevValueRelease})
	if !trig || evType != EventKeyUp {
		t.Fatalf("expected EventKeyUp, got trig=%v, evType=%v", trig, evType)
	}

	// Step 6: Release modifiers
	trig, _ = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftShift, Value: EvdevValueRelease})
	if trig {
		t.Fatalf("unexpected trigger on Shift release")
	}
	trig, _ = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftCtrl, Value: EvdevValueRelease})
	if trig {
		t.Fatalf("unexpected trigger on Ctrl release")
	}
}

func TestShortcutMatcher_ModifierReleaseAbortsHold(t *testing.T) {
	target, err := ParseShortcut("Ctrl+D")
	if err != nil {
		t.Fatalf("ParseShortcut failed: %v", err)
	}

	matcher := NewShortcutMatcher(target)

	// Press Ctrl then D
	matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftCtrl, Value: EvdevValuePress})
	trig, evType := matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyD, Value: EvdevValuePress})
	if !trig || evType != EventKeyDown {
		t.Fatalf("expected EventKeyDown on Ctrl+D")
	}

	// Release Ctrl while D is still down
	trig, evType = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftCtrl, Value: EvdevValueRelease})
	if !trig || evType != EventKeyUp {
		t.Fatalf("expected EventKeyUp when required modifier is released while holding key")
	}
}

func TestShortcutMatcher_ModifierOnly(t *testing.T) {
	target, err := ParseShortcut("Option")
	if err != nil {
		t.Fatalf("ParseShortcut failed: %v", err)
	}

	matcher := NewShortcutMatcher(target)

	// Press LeftAlt
	trig, evType := matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftAlt, Value: EvdevValuePress})
	if !trig || evType != EventKeyDown {
		t.Fatalf("expected EventKeyDown on Option press")
	}

	// Release LeftAlt
	trig, evType = matcher.ProcessEvent(InputEvent{Type: EvdevTypeKey, Code: KeyLeftAlt, Value: EvdevValueRelease})
	if !trig || evType != EventKeyUp {
		t.Fatalf("expected EventKeyUp on Option release")
	}
}
