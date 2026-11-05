package paste

import "sync"

// MockReverter records calls to Revert for testing.
type MockReverter struct {
	mu         sync.Mutex
	CallCount  int
	RevertFunc func() error
}

// NewMockReverter creates a MockReverter.
func NewMockReverter() *MockReverter {
	return &MockReverter{}
}

// Revert implements Reverter.
func (m *MockReverter) Revert() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallCount++
	if m.RevertFunc != nil {
		return m.RevertFunc()
	}
	return nil
}

// Count returns the number of times Revert was invoked.
func (m *MockReverter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CallCount
}
