package hotkey

import (
	"sync"
	"time"
)

// SimulatedListener allows programmatic triggering of hotkey events for testing and CI.
type SimulatedListener struct {
	mu          sync.RWMutex
	shortcut    string
	mode        Mode
	isListening bool
	eventChan   chan Event
}

// NewSimulatedListener creates a mock hotkey listener.
func NewSimulatedListener(shortcut string, mode Mode) *SimulatedListener {
	if mode == "" {
		mode = ModePushToTalk
	}
	if shortcut == "" {
		shortcut = "Cmd+Shift+Space"
	}
	return &SimulatedListener{
		shortcut: shortcut,
		mode:     mode,
	}
}

// Start begins listening and returns the event channel.
func (s *SimulatedListener) Start() (<-chan Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isListening {
		return nil, ErrAlreadyListening
	}

	s.isListening = true
	s.eventChan = make(chan Event, 32)
	return s.eventChan, nil
}

// Stop ends hotkey monitoring and closes the event channel.
func (s *SimulatedListener) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isListening {
		return ErrNotListening
	}

	s.isListening = false
	if s.eventChan != nil {
		close(s.eventChan)
		s.eventChan = nil
	}
	return nil
}

// IsListening returns current monitoring state.
func (s *SimulatedListener) IsListening() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isListening
}

// Mode returns the active dictation mode.
func (s *SimulatedListener) Mode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// SetMode dynamically changes the active dictation mode.
func (s *SimulatedListener) SetMode(m Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = m
}

// Shortcut returns the configured shortcut string.
func (s *SimulatedListener) Shortcut() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shortcut
}

// Emit sends a simulated hotkey event to the listener's channel.
func (s *SimulatedListener) Emit(eventType EventType) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isListening || s.eventChan == nil {
		return false
	}

	ev := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Shortcut:  s.shortcut,
	}

	select {
	case s.eventChan <- ev:
		return true
	default:
		return false
	}
}

// SimulateHold simulates holding down the hotkey for duration, then releasing.
func (s *SimulatedListener) SimulateHold(duration time.Duration) {
	s.Emit(EventKeyDown)
	time.Sleep(duration)
	s.Emit(EventKeyUp)
}
