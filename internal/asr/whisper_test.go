package asr

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/worker"
)

type mockWorkerCaller struct {
	mu         sync.Mutex
	lastMethod string
	lastParams map[string]any
	resp       *worker.Response
	err        error
}

func (m *mockWorkerCaller) Call(
	_ context.Context,
	method string,
	params map[string]any,
) (*worker.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMethod = method
	m.lastParams = params
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockWorkerCaller) Ping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

func (m *mockWorkerCaller) Stop() error {
	return nil
}

func TestWhisperEngine_Name(t *testing.T) {
	engine := NewWhisperEngine(&mockWorkerCaller{})
	if name := engine.Name(); name != "whisper" {
		t.Fatalf("expected 'whisper', got %q", name)
	}
}

func TestWhisperEngine_TranscribeEmpty(t *testing.T) {
	engine := NewWhisperEngine(&mockWorkerCaller{})
	res, err := engine.Transcribe(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error on nil chunks: %v", err)
	}
	if !res.Final {
		t.Fatal("expected Final to be true")
	}

	res, err = engine.Transcribe(context.Background(), []audio.Chunk{})
	if err != nil {
		t.Fatalf("unexpected error on empty chunks: %v", err)
	}
	if !res.Final {
		t.Fatal("expected Final to be true")
	}
}

func TestWhisperEngine_TranscribeSuccess(t *testing.T) {
	mockResp := worker.NewResponse("req-1", true, map[string]any{
		"text":       "Cadence local dictation",
		"language":   "en",
		"confidence": 0.96,
		"words": []any{
			map[string]any{"text": "Cadence", "start_ms": float64(100), "end_ms": float64(300), "confidence": 0.98},
			map[string]any{"text": "local", "start_ms": float64(350), "end_ms": float64(500), "confidence": 0.95},
			map[string]any{"text": "dictation", "start_ms": float64(550), "end_ms": float64(800), "confidence": 0.96},
		},
	})

	caller := &mockWorkerCaller{resp: mockResp}
	engine := NewWhisperEngine(caller)

	chunks := []audio.Chunk{
		{
			Samples:    []float32{0.1, -0.2, 0.3},
			SampleRate: 16000,
		},
		{
			Samples:    []float32{-0.1, 0.2},
			SampleRate: 16000,
		},
	}

	res, err := engine.Transcribe(context.Background(), chunks)
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}

	if res.Text != "Cadence local dictation" {
		t.Fatalf("expected text 'Cadence local dictation', got %q", res.Text)
	}
	if res.Language != "en" {
		t.Fatalf("expected language 'en', got %q", res.Language)
	}
	if res.Confidence != 0.96 {
		t.Fatalf("expected confidence 0.96, got %f", res.Confidence)
	}
	if len(res.Words) != 3 {
		t.Fatalf("expected 3 words, got %d", len(res.Words))
	}
	if res.Words[0].Text != "Cadence" || res.Words[0].StartMs != 100 {
		t.Fatalf("unexpected first word: %+v", res.Words[0])
	}

	caller.mu.Lock()
	audioParam, ok := caller.lastParams["audio"].([]float32)
	caller.mu.Unlock()

	if !ok || len(audioParam) != 5 {
		t.Fatalf("expected 5 concatenated audio samples, got %v", audioParam)
	}
}

func TestWhisperEngine_TranscribeError(t *testing.T) {
	caller := &mockWorkerCaller{err: errors.New("inference crash")}
	engine := NewWhisperEngine(caller)

	chunks := []audio.Chunk{
		{
			Samples:    []float32{0.1, 0.2},
			SampleRate: 16000,
		},
	}

	_, err := engine.Transcribe(context.Background(), chunks)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = engine.Transcribe(cancelCtx, chunks)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWhisperEngine_Stream(t *testing.T) {
	mockResp := worker.NewResponse("req-2", true, map[string]any{
		"text":       "Streaming dictation",
		"language":   "en",
		"confidence": 0.99,
	})

	caller := &mockWorkerCaller{resp: mockResp}
	engine := NewWhisperEngine(caller)

	in := make(chan audio.Chunk, 2)
	in <- audio.Chunk{Samples: []float32{0.1, 0.2}, SampleRate: 16000}
	in <- audio.Chunk{Samples: []float32{0.3, 0.4}, SampleRate: 16000}
	close(in)

	out, err := engine.Stream(context.Background(), in)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	select {
	case res, ok := <-out:
		if !ok {
			t.Fatal("expected result from stream channel")
		}
		if res.Text != "Streaming dictation" || !res.Final {
			t.Fatalf("unexpected stream result: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream result")
	}
}

func TestWhisperEngine_StreamPartial(t *testing.T) {
	mockResp := worker.NewResponse("req-partial", true, map[string]any{
		"text":  "hello world",
		"final": false,
	})

	caller := &mockWorkerCaller{resp: mockResp}
	engine := NewWhisperEngine(caller)

	in := make(chan audio.Chunk, 1)
	// 9600 samples = 600ms at 16kHz
	in <- audio.Chunk{Samples: make([]float32, 9600), SampleRate: 16000}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := engine.Stream(ctx, in)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	select {
	case res := <-out:
		if res.Text != "hello world" {
			t.Fatalf("expected 'hello world', got %q", res.Text)
		}
		if res.Final {
			t.Fatal("expected partial result to not be final")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partial result")
	}

	close(in)
}

func TestWhisperEngine_StreamCancel(t *testing.T) {
	caller := &mockWorkerCaller{}
	engine := NewWhisperEngine(caller)

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan audio.Chunk, 1)

	out, err := engine.Stream(ctx, in)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	cancel()

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("expected closed out channel on cancellation")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("stream did not terminate on context cancel")
	}
}
