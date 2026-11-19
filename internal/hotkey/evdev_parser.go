package hotkey

import (
	"encoding/binary"
	"io"
)

// InputEvent represents a decoded Linux input subsystem event (struct input_event).
type InputEvent struct {
	Sec   uint64
	Usec  uint64
	Type  uint16
	Code  uint16
	Value int32
}

// ReadInputEvent decodes a 24-byte 64-bit Linux input_event frame from r.
func ReadInputEvent(r io.Reader) (InputEvent, error) {
	var buf [24]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return InputEvent{}, err
	}

	return InputEvent{
		Sec:   binary.LittleEndian.Uint64(buf[0:8]),
		Usec:  binary.LittleEndian.Uint64(buf[8:16]),
		Type:  binary.LittleEndian.Uint16(buf[16:18]),
		Code:  binary.LittleEndian.Uint16(buf[18:20]),
		Value: int32(binary.LittleEndian.Uint32(buf[20:24])),
	}, nil
}

// EncodeInputEvent encodes an InputEvent into a 24-byte little-endian binary slice (for testing).
func EncodeInputEvent(ev InputEvent) []byte {
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint64(buf[0:8], ev.Sec)
	binary.LittleEndian.PutUint64(buf[8:16], ev.Usec)
	binary.LittleEndian.PutUint16(buf[16:18], ev.Type)
	binary.LittleEndian.PutUint16(buf[18:20], ev.Code)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(ev.Value))
	return buf
}

// ModifierTracker maintains the live state of keyboard modifier keys.
type ModifierTracker struct {
	leftCtrl, rightCtrl   bool
	leftAlt, rightAlt     bool
	leftShift, rightShift bool
	leftMeta, rightMeta   bool
}

// Update records key state changes for modifier keys.
func (m *ModifierTracker) Update(code uint16, value int32) {
	pressed := value == EvdevValuePress || value == EvdevValueAutorepeat

	switch code {
	case KeyLeftCtrl:
		m.leftCtrl = pressed
	case KeyRightCtrl:
		m.rightCtrl = pressed
	case KeyLeftAlt:
		m.leftAlt = pressed
	case KeyRightAlt:
		m.rightAlt = pressed
	case KeyLeftShift:
		m.leftShift = pressed
	case KeyRightShift:
		m.rightShift = pressed
	case KeyLeftMeta:
		m.leftMeta = pressed
	case KeyRightMeta:
		m.rightMeta = pressed
	}
}

// CurrentModifiers returns the current Modifiers matching Cadence's format.
func (m *ModifierTracker) CurrentModifiers() Modifiers {
	return Modifiers{
		Ctrl:  m.leftCtrl || m.rightCtrl,
		Alt:   m.leftAlt || m.rightAlt,
		Shift: m.leftShift || m.rightShift,
		Cmd:   m.leftMeta || m.rightMeta,
	}
}

// ShortcutMatcher tracks hotkey state and determines when a configured shortcut triggers.
type ShortcutMatcher struct {
	target      Shortcut
	tracker     ModifierTracker
	primaryDown bool
}

// NewShortcutMatcher creates a matcher for the given parsed shortcut.
func NewShortcutMatcher(target Shortcut) *ShortcutMatcher {
	return &ShortcutMatcher{
		target: target,
	}
}

// ProcessEvent inspects an evdev event and returns whether a hotkey key_down or key_up occurred.
func (m *ShortcutMatcher) ProcessEvent(ev InputEvent) (bool, EventType) {
	if ev.Type != EvdevTypeKey {
		return false, ""
	}

	// Update live modifier keys
	m.tracker.Update(ev.Code, ev.Value)

	// Check if this is a modifier-only shortcut (e.g. "Option" or "Ctrl")
	if m.target.Key == "" {
		modType := ClassifyEvdevModifier(ev.Code)
		matchesMod := false
		switch modType {
		case ModifierCtrl:
			matchesMod = m.target.Modifiers.Ctrl
		case ModifierAlt:
			matchesMod = m.target.Modifiers.Alt
		case ModifierShift:
			matchesMod = m.target.Modifiers.Shift
		case ModifierMeta:
			matchesMod = m.target.Modifiers.Cmd
		}

		if matchesMod {
			if ev.Value == EvdevValuePress && !m.primaryDown {
				m.primaryDown = true
				return true, EventKeyDown
			}
			if ev.Value == EvdevValueRelease && m.primaryDown {
				m.primaryDown = false
				return true, EventKeyUp
			}
		}
		return false, ""
	}

	// Standard shortcut with a primary key (e.g. "Cmd+Shift+Space" or "Ctrl+D")
	keyName := EvdevCodeToKey(ev.Code)
	if keyName == m.target.Key {
		if ev.Value == EvdevValuePress && !m.primaryDown {
			currentMods := m.tracker.CurrentModifiers()
			if m.modifiersMatch(currentMods) {
				m.primaryDown = true
				return true, EventKeyDown
			}
		} else if ev.Value == EvdevValueRelease && m.primaryDown {
			m.primaryDown = false
			return true, EventKeyUp
		}
	} else if ev.Value == EvdevValueRelease && m.primaryDown {
		// If a required modifier is released while holding the primary key, trigger key up
		currentMods := m.tracker.CurrentModifiers()
		if !m.modifiersMatch(currentMods) {
			m.primaryDown = false
			return true, EventKeyUp
		}
	}

	return false, ""
}

func (m *ShortcutMatcher) modifiersMatch(current Modifiers) bool {
	return current.Ctrl == m.target.Modifiers.Ctrl &&
		current.Alt == m.target.Modifiers.Alt &&
		current.Shift == m.target.Modifiers.Shift &&
		current.Cmd == m.target.Modifiers.Cmd
}
