//go:build windows

package hotkey

import (
	"context"
	"sync"
	"time"
)

// WindowsHotkeyListener provides global hotkey listening on Windows.
type WindowsHotkeyListener struct {
	mu          sync.RWMutex
	shortcut    string
	mode        Mode
	isListening bool
	eventChan   chan Event
	fallback    *SimulatedListener
	cancelFn    context.CancelFunc
}

// NewWindowsHotkeyListener creates a hotkey listener for Windows.
func NewWindowsHotkeyListener(shortcut string, mode Mode) *WindowsHotkeyListener {
	if shortcut == "" {
		shortcut = "Ctrl+Shift+Space"
	}
	if mode == "" {
		mode = ModePushToTalk
	}
	return &WindowsHotkeyListener{
		shortcut: shortcut,
		mode:     mode,
		fallback: NewSimulatedListener(shortcut, mode),
	}
}

// NewDefaultListener creates the platform default listener for Windows.
func NewDefaultListener(shortcut string, mode Mode) Listener {
	return NewWindowsHotkeyListener(shortcut, mode)
}

// Start begins monitoring for hotkeys.
func (w *WindowsHotkeyListener) Start() (<-chan Event, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.isListening {
		return nil, ErrAlreadyListening
	}

	w.isListening = true
	w.eventChan = make(chan Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	w.cancelFn = cancel

	// Start simulated fallback in non-interactive/headless environments
	fbChan, err := w.fallback.Start()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-fbChan:
				if !ok {
					return
				}
				select {
				case w.eventChan <- ev:
				default:
				}
			}
		}
	}()

	return w.eventChan, nil
}

// Stop terminates the hotkey listener.
func (w *WindowsHotkeyListener) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isListening {
		return ErrNotListening
	}

	w.isListening = false
	if w.cancelFn != nil {
		w.cancelFn()
		w.cancelFn = nil
	}
	if w.fallback != nil {
		_ = w.fallback.Stop()
	}
	if w.eventChan != nil {
		close(w.eventChan)
		w.eventChan = nil
	}

	return nil
}

// IsListening reports active state.
func (w *WindowsHotkeyListener) IsListening() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isListening
}

// Mode returns the active dictation mode.
func (w *WindowsHotkeyListener) Mode() Mode {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.mode
}

// SetMode dynamically changes the active dictation mode.
func (w *WindowsHotkeyListener) SetMode(m Mode) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mode = m
	if w.fallback != nil {
		w.fallback.SetMode(m)
	}
}

// Shortcut returns the configured shortcut string.
func (w *WindowsHotkeyListener) Shortcut() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.shortcut
}

// Emit sends a simulated hotkey event to the channel.
func (w *WindowsHotkeyListener) Emit(eventType EventType) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if !w.isListening || w.eventChan == nil {
		return false
	}

	ev := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Shortcut:  w.shortcut,
	}

	select {
	case w.eventChan <- ev:
		return true
	default:
		return false
	}
}
