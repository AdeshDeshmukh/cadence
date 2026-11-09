package diarize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AdeshDeshmukh/cadence/internal/worker"
)

// PyannoteDiarizer implements Diarizer backed by a Python diarization worker subprocess.
type PyannoteDiarizer struct {
	caller worker.Caller
}

// NewPyannoteDiarizer creates a diarizer instance communicating with a worker.Caller.
func NewPyannoteDiarizer(caller worker.Caller) *PyannoteDiarizer {
	return &PyannoteDiarizer{
		caller: caller,
	}
}

// Diarize executes speaker diarization via the subprocess worker.
func (p *PyannoteDiarizer) Diarize(ctx context.Context, audioPath string, opts Options) ([]Segment, error) {
	if p.caller == nil {
		return nil, errors.New("worker caller is not initialized")
	}
	if audioPath == "" {
		return nil, errors.New("audioPath cannot be empty")
	}

	params := map[string]any{
		"audio_path": audioPath,
	}
	if opts.NumSpeakers > 0 {
		params["num_speakers"] = opts.NumSpeakers
	}
	if opts.MinSpeakers > 0 {
		params["min_speakers"] = opts.MinSpeakers
	}
	if opts.MaxSpeakers > 0 {
		params["max_speakers"] = opts.MaxSpeakers
	}
	if opts.HFToken != "" {
		params["hf_token"] = opts.HFToken
	}

	resp, err := p.caller.Call(ctx, "diarize", params)
	if err != nil {
		return nil, fmt.Errorf("worker diarize call failed: %w", err)
	}

	rawSegs, ok := resp.Get("segments")
	if !ok || rawSegs == nil {
		return []Segment{}, nil
	}

	// Marshal and unmarshal to safely parse raw map representation into []Segment
	data, err := json.Marshal(rawSegs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw segments: %w", err)
	}

	var segments []Segment
	if err := json.Unmarshal(data, &segments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal diarized segments: %w", err)
	}

	return segments, nil
}
