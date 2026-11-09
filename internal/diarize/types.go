package diarize

import (
	"context"
)

// Segment represents a diarized temporal interval attributed to a specific speaker.
type Segment struct {
	StartMs    int64   `json:"start_ms"`
	EndMs      int64   `json:"end_ms"`
	Speaker    string  `json:"speaker"`
	Confidence float64 `json:"confidence"`
}

// Options contains optional parameters for a diarization run.
type Options struct {
	NumSpeakers int    `json:"num_speakers,omitempty"`
	MinSpeakers int    `json:"min_speakers,omitempty"`
	MaxSpeakers int    `json:"max_speakers,omitempty"`
	HFToken     string `json:"hf_token,omitempty"`
}

// Diarizer defines the contract for speaker diarization backends.
type Diarizer interface {
	Diarize(ctx context.Context, audioPath string, opts Options) ([]Segment, error)
}

// Result summarizes a completed diarization operation across audio segments.
type Result struct {
	MeetingID  string    `json:"meeting_id"`
	Backend    string    `json:"backend"`
	Speakers   []string  `json:"speakers"`
	Segments   []Segment `json:"segments"`
	DurationMs int64     `json:"duration_ms"`
}
