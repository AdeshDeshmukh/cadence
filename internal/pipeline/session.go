package pipeline

import (
	"sync"
	"time"
)

type Session struct {
	ID          string
	State       State
	RawText     string
	FinalText   string
	PartialText string
	StartedAt   time.Time
	EndedAt     time.Time
	onPartial   func(string)
	mu          sync.RWMutex
}

func NewSession(id string) *Session {
	return &Session{
		ID: id, State: Idle, StartedAt: time.Now(),
	}
}

func (s *Session) SetState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
	if state.Terminal() {
		s.EndedAt = time.Now()
	}
}

func (s *Session) GetState() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *Session) SetOnPartial(fn func(string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPartial = fn
}

func (s *Session) UpdatePartial(text string) {
	s.mu.Lock()
	s.PartialText = text
	cb := s.onPartial
	s.mu.Unlock()

	if cb != nil {
		cb(text)
	}
}

func (s *Session) GetPartial() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PartialText
}
