package pipeline

import (
	"sync"
	"time"
)

type Session struct {
	ID        string
	State     State
	RawText   string
	FinalText string
	StartedAt time.Time
	EndedAt   time.Time
	mu        sync.RWMutex
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
