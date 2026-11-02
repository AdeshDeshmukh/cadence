package polish

import (
	"context"
	"time"
)

// Request defines the input data and context for LLM polishing.
type Request struct {
	Text        string            `json:"text"`
	Profile     string            `json:"profile,omitempty"`
	Application string            `json:"application,omitempty"`
	Selection   string            `json:"selection,omitempty"`
	CustomRules []string          `json:"custom_rules,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Response contains the result of LLM transformation and execution metrics.
type Response struct {
	Text      string        `json:"text"`
	Model     string        `json:"model"`
	TokensIn  int           `json:"tokens_in,omitempty"`
	TokensOut int           `json:"tokens_out,omitempty"`
	Latency   time.Duration `json:"latency"`
}

// Polisher defines the contract for text polishing backends.
type Polisher interface {
	Name() string
	Polish(ctx context.Context, req Request) (Response, error)
}

// NoopPolisher is a pass-through polisher that returns input text unaltered.
type NoopPolisher struct{}

// NewNoopPolisher returns a zero-overhead pass-through polisher.
func NewNoopPolisher() *NoopPolisher {
	return &NoopPolisher{}
}

func (NoopPolisher) Name() string {
	return "noop"
}

func (NoopPolisher) Polish(_ context.Context, req Request) (Response, error) {
	return Response{
		Text:    req.Text,
		Model:   "noop",
		Latency: 0,
	}, nil
}

// MockPolisher provides custom or static responses for testing.
type MockPolisher struct {
	NameVal     string
	TransformFn func(req Request) (Response, error)
}

// NewMockPolisher creates a configurable mock polisher.
func NewMockPolisher(fn func(req Request) (Response, error)) *MockPolisher {
	return &MockPolisher{
		NameVal:     "mock",
		TransformFn: fn,
	}
}

func (m *MockPolisher) Name() string {
	if m.NameVal != "" {
		return m.NameVal
	}
	return "mock"
}

func (m *MockPolisher) Polish(_ context.Context, req Request) (Response, error) {
	if m.TransformFn != nil {
		return m.TransformFn(req)
	}
	return Response{
		Text:    req.Text,
		Model:   m.Name(),
		Latency: 1 * time.Millisecond,
	}, nil
}
