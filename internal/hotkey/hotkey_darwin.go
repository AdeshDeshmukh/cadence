//go:build darwin

package hotkey

import (
	"context"
	"sync"
	"time"
)

// DarwinListener provides global hotkey listening on macOS.
type DarwinListener struct {
	mu          sync.RWMutex
	shortcut    string
	mode        Mode
	isListening bool
	eventChan   chan Event
	cancelFn    context.CancelFunc
}

// NewDarwinListener creates a macOS hotkey listener.
func NewDarwinListener(shortcut string, mode Mode) *DarwinListener {
	if shortcut == "" {
		shortcut = "Cmd+Shift+Space"
	}
	if mode == "" {
		mode = ModePushToTalk
	}
	return &DarwinListener{
		shortcut: shortcut,
		mode:     mode,
	}
}

// Start begins listening for global hotkeys.
func (d *DarwinListener) Start() (<-chan Event, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isListening {
		return nil, ErrAlreadyListening
	}

	d.isListening = true
	d.eventChan = make(chan Event, 32)
	_, cancel := context.WithCancel(context.Background())
	d.cancelFn = cancel

	return d.eventChan, nil
}

// Stop terminates the global hotkey listener.
func (d *DarwinListener) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isListening {
		return ErrNotListening
	}

	d.isListening = false
	if d.cancelFn != nil {
		d.cancelFn()
		d.cancelFn = nil
	}
	if d.eventChan != nil {
		close(d.eventChan)
		d.eventChan = nil
	}

	return nil
}

// IsListening reports active state.
func (d *DarwinListener) IsListening() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isListening
}

// Mode returns current activation mode.
func (d *DarwinListener) Mode() Mode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.mode
}

// SetMode updates dictation activation mode.
func (d *DarwinListener) SetMode(m Mode) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mode = m
}

// Shortcut returns the configured shortcut string.
func (d *DarwinListener) Shortcut() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.shortcut
}

// Emit sends an event to the listener channel (used by native taps or tests).
func (d *DarwinListener) Emit(eventType EventType) bool {
	d.mu.RLock()
	ch := d.eventChan
	listening := d.isListening
	sc := d.shortcut
	d.mu.RUnlock()

	if !listening || ch == nil {
		return false
	}

	ev := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Shortcut:  sc,
	}

	select {
	case ch <- ev:
		return true
	default:
		return false
	}
}

// NewDefaultListener creates the platform-specific default listener.
func NewDefaultListener(shortcut string, mode Mode) Listener {
	return NewDarwinListener(shortcut, mode)
}
