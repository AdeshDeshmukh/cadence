package diarize

import (
	"context"
	"fmt"
)

// MockDiarizer provides deterministic diarization output for unit testing and CI.
type MockDiarizer struct {
	Segments []Segment
	Backend  string
	Err      error
}

// NewMockDiarizer returns a mock diarizer with pre-defined segments.
func NewMockDiarizer(segments ...Segment) *MockDiarizer {
	return &MockDiarizer{
		Segments: segments,
		Backend:  "mock",
	}
}

// Diarize returns preconfigured mock segments or dynamically generates segments based on options.
func (m *MockDiarizer) Diarize(_ context.Context, _ string, opts Options) ([]Segment, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.Segments) > 0 {
		return m.Segments, nil
	}

	// Dynamic fallback segments if none explicitly provided
	k := opts.NumSpeakers
	if k <= 0 {
		k = 2
	}

	var out []Segment
	for i := 0; i < k; i++ {
		out = append(out, Segment{
			StartMs:    int64(i * 15000),
			EndMs:      int64((i + 1) * 15000),
			Speaker:    fmt.Sprintf("SPEAKER_%02d", i),
			Confidence: 0.95,
		})
	}
	return out, nil
}
