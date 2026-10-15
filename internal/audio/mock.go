package audio

import (
	"sync"
	"time"
)

type MockCapture struct {
	chunks  []Chunk
	mu      sync.Mutex
	running bool
}

func NewMockCapture(chunks []Chunk) *MockCapture {
	return &MockCapture{chunks: chunks}
}

func (m *MockCapture) Devices() ([]Device, error) {
	return []Device{{
		ID: "mock-default", Name: "Mock Microphone",
		SampleRate: 16000, Input: true, Default: true,
	}}, nil
}

func (m *MockCapture) Start() (<-chan Chunk, error) {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()

	out := make(chan Chunk)

	go func() {
		defer close(out)
		for _, chunk := range m.chunks {
			m.mu.Lock()
			running := m.running
			m.mu.Unlock()
			if !running {
				return
			}
			out <- chunk
			if d := chunk.Duration(); d > 0 {
				time.Sleep(d)
			}
		}
	}()

	return out, nil
}

func (m *MockCapture) Stop() error {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	return nil
}
