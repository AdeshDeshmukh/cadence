package audio

import (
	"sync"
	"time"
)

type MockCapture struct {
	chunks  []Chunk
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
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
	stopCh := make(chan struct{})
	m.stopCh = stopCh
	m.mu.Unlock()

	out := make(chan Chunk)

	go func() {
		defer close(out)
		for _, chunk := range m.chunks {
			select {
			case <-stopCh:
				return
			default:
			}

			select {
			case out <- chunk:
			case <-stopCh:
				return
			}

			if d := chunk.Duration(); d > 0 {
				t := time.NewTimer(d)
				select {
				case <-t.C:
				case <-stopCh:
					t.Stop()
					return
				}
			}
		}
	}()

	return out, nil
}

func (m *MockCapture) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		m.running = false
		if m.stopCh != nil {
			close(m.stopCh)
			m.stopCh = nil
		}
	}
	return nil
}
