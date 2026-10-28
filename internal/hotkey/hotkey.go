package hotkey

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrAlreadyListening is returned when Start() is called while listener is already active.
	ErrAlreadyListening = errors.New("hotkey: listener is already running")

	// ErrNotListening is returned when Stop() is called on an inactive listener.
	ErrNotListening = errors.New("hotkey: listener is not running")

	// ErrInvalidShortcut is returned when a shortcut definition cannot be parsed.
	ErrInvalidShortcut = errors.New("hotkey: invalid shortcut specification")
)

// Mode defines the dictation activation behavior.
type Mode string

const (
	// ModePushToTalk records while key is held down; stops on release.
	ModePushToTalk Mode = "push_to_talk"

	// ModeToggle records on first press; stops on second press.
	ModeToggle Mode = "toggle"
)

// EventType indicates whether the hotkey was pressed or released.
type EventType string

const (
	// EventKeyDown signifies hotkey press (initiates recording).
	EventKeyDown EventType = "key_down"

	// EventKeyUp signifies hotkey release (terminates recording in Push-to-Talk).
	EventKeyUp EventType = "key_up"
)

// Event represents a global hotkey trigger.
type Event struct {
	Type      EventType
	Timestamp time.Time
	Shortcut  string
}

// Modifiers represents modifier keys held down with the primary key.
type Modifiers struct {
	Cmd   bool
	Alt   bool // Option on macOS
	Ctrl  bool
	Shift bool
	Fn    bool
}

// Shortcut represents a parsed key combination.
type Shortcut struct {
	Modifiers Modifiers
	Key       string
}

// String formats the shortcut in canonical format (e.g. "Cmd+Shift+Space").
func (s Shortcut) String() string {
	var parts []string
	if s.Modifiers.Cmd {
		parts = append(parts, "Cmd")
	}
	if s.Modifiers.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if s.Modifiers.Alt {
		parts = append(parts, "Option")
	}
	if s.Modifiers.Shift {
		parts = append(parts, "Shift")
	}
	if s.Modifiers.Fn {
		parts = append(parts, "Fn")
	}
	if s.Key != "" {
		parts = append(parts, formatKey(s.Key))
	}
	return strings.Join(parts, "+")
}

func formatKey(k string) string {
	if len(k) <= 1 {
		return strings.ToUpper(k)
	}
	return strings.ToUpper(k[:1]) + strings.ToLower(k[1:])
}

// ParseShortcut parses strings such as "Cmd+Shift+Space", "Option", or "Ctrl+D".
func ParseShortcut(str string) (Shortcut, error) {
	str = strings.TrimSpace(str)
	if str == "" {
		return Shortcut{}, ErrInvalidShortcut
	}

	parts := strings.Split(str, "+")
	var sc Shortcut

	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		switch strings.ToLower(part) {
		case "cmd", "command":
			sc.Modifiers.Cmd = true
		case "alt", "opt", "option":
			sc.Modifiers.Alt = true
		case "ctrl", "control":
			sc.Modifiers.Ctrl = true
		case "shift":
			sc.Modifiers.Shift = true
		case "fn":
			sc.Modifiers.Fn = true
		default:
			if sc.Key != "" {
				return Shortcut{}, fmt.Errorf("%w: multiple primary keys defined (%s, %s)", ErrInvalidShortcut, sc.Key, part)
			}
			sc.Key = strings.ToUpper(part)
		}
	}

	// At least one modifier or key must be defined
	if !sc.Modifiers.Cmd && !sc.Modifiers.Alt && !sc.Modifiers.Ctrl && !sc.Modifiers.Shift && !sc.Modifiers.Fn && sc.Key == "" {
		return Shortcut{}, ErrInvalidShortcut
	}

	return sc, nil
}

// Listener is the interface for background global hotkey monitoring.
type Listener interface {
	Start() (<-chan Event, error)
	Stop() error
	IsListening() bool
	Mode() Mode
	SetMode(m Mode)
	Shortcut() string
}
