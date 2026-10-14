package asr_test

import (
	"context"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

func TestMockEngine_Transcribe(t *testing.T) {
	engine := asr.MockEngine{Text: "hello world"}

	if name := engine.Name(); name != "mock" {
		t.Fatalf("expected engine name 'mock', got %q", name)
	}

	res, err := engine.Transcribe(context.Background(), []audio.Chunk{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "hello world" || !res.Final {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestMockEngine_Stream(t *testing.T) {
	engine := asr.MockEngine{Text: "streaming voice"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	audioCh := make(chan audio.Chunk)
	close(audioCh)

	out, err := engine.Stream(ctx, audioCh)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	var results []asr.PartialResult
	for r := range out {
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 partial results, got %d", len(results))
	}
	if !results[1].Final {
		t.Fatalf("expected last partial result to be final")
	}
}
