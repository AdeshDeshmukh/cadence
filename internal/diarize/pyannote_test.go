package diarize

import (
	"context"
	"errors"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/worker"
)

type mockWorkerCaller struct {
	lastMethod string
	lastParams map[string]any
	resp       *worker.Response
	err        error
}

func (m *mockWorkerCaller) Call(_ context.Context, method string, params map[string]any) (*worker.Response, error) {
	m.lastMethod = method
	m.lastParams = params
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockWorkerCaller) Ping(_ context.Context) error { return nil }
func (m *mockWorkerCaller) Stop() error                  { return nil }

func TestPyannoteDiarizer(t *testing.T) {
	ctx := context.Background()

	// 1. Error on uninitialized caller
	pNil := NewPyannoteDiarizer(nil)
	if _, err := pNil.Diarize(ctx, "/path/audio.wav", Options{}); err == nil {
		t.Fatal("expected error on nil caller")
	}

	// 2. Error on empty audioPath
	caller := &mockWorkerCaller{}
	p := NewPyannoteDiarizer(caller)
	if _, err := p.Diarize(ctx, "", Options{}); err == nil {
		t.Fatal("expected error on empty audioPath")
	}

	// 3. Worker error propagation
	caller.err = errors.New("subprocess crashed")
	if _, err := p.Diarize(ctx, "/tmp/meeting.wav", Options{}); err == nil {
		t.Fatal("expected error when worker fails")
	}

	// 4. Successful diarization call and option forwarding
	caller.err = nil
	caller.resp = worker.NewResponse("req-1", true, map[string]any{
		"segments": []any{
			map[string]any{
				"start_ms":   float64(0),
				"end_ms":     float64(10000),
				"speaker":    "SPEAKER_00",
				"confidence": 0.92,
			},
			map[string]any{
				"start_ms":   float64(10000),
				"end_ms":     float64(20000),
				"speaker":    "SPEAKER_01",
				"confidence": 0.88,
			},
		},
	})

	opts := Options{
		NumSpeakers: 2,
		MinSpeakers: 1,
		MaxSpeakers: 3,
		HFToken:     "hf_test_token",
	}

	segs, err := p.Diarize(ctx, "/tmp/meeting.wav", opts)
	if err != nil {
		t.Fatalf("unexpected diarize error: %v", err)
	}

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	if segs[0].Speaker != "SPEAKER_00" || segs[1].Speaker != "SPEAKER_01" {
		t.Fatalf("unexpected segment speakers: %+v", segs)
	}

	// Verify params sent to worker
	if caller.lastParams["audio_path"] != "/tmp/meeting.wav" {
		t.Fatalf("unexpected audio_path param: %v", caller.lastParams["audio_path"])
	}
	if caller.lastParams["num_speakers"] != 2 || caller.lastParams["hf_token"] != "hf_test_token" {
		t.Fatalf("unexpected params forwarded: %+v", caller.lastParams)
	}
}
