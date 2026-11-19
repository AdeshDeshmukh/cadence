//go:build linux

package hotkey

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LinuxEvdevListener provides global hotkey listening via the Linux input subsystem (/dev/input/event*).
type LinuxEvdevListener struct {
	mu             sync.RWMutex
	shortcut       string
	parsedShortcut Shortcut
	mode           Mode
	devicePath     string
	isListening    bool
	useFallback    bool
	eventChan      chan Event
	fallback       *SimulatedListener
	cancelFn       context.CancelFunc
	file           *os.File
}

// NewLinuxEvdevListener creates a Linux evdev hotkey listener.
func NewLinuxEvdevListener(shortcut string, mode Mode, devicePath string) *LinuxEvdevListener {
	if shortcut == "" {
		shortcut = "Ctrl+Shift+Space"
	}
	if mode == "" {
		mode = ModePushToTalk
	}

	parsed, _ := ParseShortcut(shortcut)

	return &LinuxEvdevListener{
		shortcut:       shortcut,
		parsedShortcut: parsed,
		mode:           mode,
		devicePath:     devicePath,
		fallback:       NewSimulatedListener(shortcut, mode),
	}
}

// NewDefaultListener creates the platform-specific default listener for Linux.
func NewDefaultListener(shortcut string, mode Mode) Listener {
	return NewLinuxEvdevListener(shortcut, mode, "")
}

// FindKeyboardDevices searches /dev/input for accessible keyboard event devices.
func FindKeyboardDevices() ([]string, error) {
	// 1. Try /dev/input/by-id/*-kbd*
	matches, _ := filepath.Glob("/dev/input/by-id/*-kbd*")
	if len(matches) > 0 {
		var valid []string
		for _, m := range matches {
			if f, err := os.Open(m); err == nil {
				_ = f.Close()
				valid = append(valid, m)
			}
		}
		if len(valid) > 0 {
			return valid, nil
		}
	}

	// 2. Scan /dev/input/event*
	eventFiles, err := filepath.Glob("/dev/input/event*")
	if err != nil || len(eventFiles) == 0 {
		return nil, fmt.Errorf("no /dev/input/event* devices found: %w", err)
	}

	var readable []string
	for _, ef := range eventFiles {
		f, err := os.Open(ef)
		if err == nil {
			_ = f.Close()
			readable = append(readable, ef)
		}
	}

	if len(readable) == 0 {
		return nil, fmt.Errorf("no readable /dev/input/event* devices (permission denied or no input devices)")
	}

	return readable, nil
}

// Start begins listening for global hotkeys via evdev.
func (l *LinuxEvdevListener) Start() (<-chan Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isListening {
		return nil, ErrAlreadyListening
	}

	l.isListening = true
	l.eventChan = make(chan Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFn = cancel

	devPath := l.devicePath
	if devPath == "" {
		devs, err := FindKeyboardDevices()
		if err != nil || len(devs) == 0 {
			// Headless / non-root / container fallback
			l.useFallback = true
			ch, err := l.fallback.Start()
			if err != nil {
				return nil, err
			}
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case ev, ok := <-ch:
						if !ok {
							return
						}
						select {
						case l.eventChan <- ev:
						default:
						}
					}
				}
			}()
			return l.eventChan, nil
		}
		devPath = devs[0]
	}

	file, err := os.Open(devPath)
	if err != nil {
		// Fallback to simulated if device cannot be opened
		l.useFallback = true
		ch, err := l.fallback.Start()
		if err != nil {
			return nil, err
		}
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-ch:
					if !ok {
						return
					}
					select {
					case l.eventChan <- ev:
					default:
					}
				}
			}
		}()
		return l.eventChan, nil
	}

	l.file = file
	matcher := NewShortcutMatcher(l.parsedShortcut)

	go func() {
		defer func() {
			_ = file.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ev, err := ReadInputEvent(file)
			if err != nil {
				return
			}

			triggered, evType := matcher.ProcessEvent(ev)
			if triggered {
				hotkeyEvent := Event{
					Type:      evType,
					Timestamp: time.Now(),
					Shortcut:  l.shortcut,
				}
				select {
				case <-ctx.Done():
					return
				case l.eventChan <- hotkeyEvent:
				default:
				}
			}
		}
	}()

	return l.eventChan, nil
}

// Stop terminates the global hotkey listener.
func (l *LinuxEvdevListener) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.isListening {
		return ErrNotListening
	}

	l.isListening = false
	if l.cancelFn != nil {
		l.cancelFn()
		l.cancelFn = nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	if l.useFallback && l.fallback != nil {
		_ = l.fallback.Stop()
	}
	if l.eventChan != nil {
		close(l.eventChan)
		l.eventChan = nil
	}

	return nil
}

// IsListening reports whether monitoring is active.
func (l *LinuxEvdevListener) IsListening() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isListening
}

// Mode returns current activation mode.
func (l *LinuxEvdevListener) Mode() Mode {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.mode
}

// SetMode dynamically changes the active dictation mode.
func (l *LinuxEvdevListener) SetMode(m Mode) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.mode = m
	if l.fallback != nil {
		l.fallback.SetMode(m)
	}
}

// Shortcut returns the configured shortcut string.
func (l *LinuxEvdevListener) Shortcut() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.shortcut
}

// Emit sends a hotkey event to the listener's channel (for testing or programmatic invocation).
func (l *LinuxEvdevListener) Emit(eventType EventType) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.isListening || l.eventChan == nil {
		return false
	}

	ev := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Shortcut:  l.shortcut,
	}

	select {
	case l.eventChan <- ev:
		return true
	default:
		return false
	}
}
