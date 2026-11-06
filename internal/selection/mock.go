package selection

import (
	"context"
	"sync"
)

// MockCapturer provides a configurable mock for unit and integration testing.
type MockCapturer struct {
	mu        sync.RWMutex
	text      string
	method    string
	err       error
	callCount int
}

// NewMockCapturer creates a MockCapturer with initial text and method.
func NewMockCapturer(text, method string) *MockCapturer {
	if method == "" {
		method = "mock"
	}
	return &MockCapturer{
		text:   text,
		method: method,
	}
}

// SetText updates the mock selected text.
func (m *MockCapturer) SetText(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text = text
	m.err = nil
}

// SetError updates the mock error returned by Capture.
func (m *MockCapturer) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// Capture implements Capturer.
func (m *MockCapturer) Capture(ctx context.Context) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.err != nil {
		return Result{}, m.err
	}
	if m.text == "" {
		return Result{}, ErrNoSelection
	}
	return Result{
		Text:   m.text,
		Method: m.method,
	}, nil
}

// CallCount returns the number of times Capture was called.
func (m *MockCapturer) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}
