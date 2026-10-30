package vad_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/vad"
	"github.com/AdeshDeshmukh/cadence/internal/worker"
)

type mockVADCaller struct {
	mu        sync.Mutex
	isSpeech  bool
	callCount int
	lastReset bool
	failCall  bool
}

func (m *mockVADCaller) Call(_ context.Context, method string, _ map[string]any) (*worker.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	if m.failCall {
		return nil, errors.New("worker connection error")
	}

	if method == "reset" {
		m.lastReset = true
		return worker.NewResponse("1", true, map[string]any{"reset": true}), nil
	}

	if method == "detect" {
		return worker.NewResponse("1", true, map[string]any{
			"is_speech":   m.isSpeech,
			"probability": 0.85,
		}), nil
	}

	return nil, errors.New("unknown method")
}

func (m *mockVADCaller) Ping(_ context.Context) error {
	return nil
}

func (m *mockVADCaller) Stop() error {
	return nil
}

func (m *mockVADCaller) setSpeech(val bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isSpeech = val
}

func TestSileroDetector_SpeechOnsetAndHangover(t *testing.T) {
	mock := &mockVADCaller{isSpeech: false}
	detector := vad.NewSileroDetector(mock,
		vad.WithSileroThreshold(0.5),
		vad.WithSileroHangover(100*time.Millisecond),
	)

	chunk50ms := audio.Chunk{
		Samples:    make([]float32, 800), // 800 samples at 16kHz = 50ms
		SampleRate: 16000,
	}

	// 1. Initial silence
	ev := detector.Process(chunk50ms)
	if ev.Type != vad.Silence {
		t.Fatalf("expected initial silence, got %v", ev.Type)
	}

	// 2. Speech begins: 50ms (not yet 80ms min speech)
	mock.setSpeech(true)
	ev = detector.Process(chunk50ms)
	if ev.Type != vad.Silence {
		t.Fatalf("expected silence under min speech duration, got %v", ev.Type)
	}

	// 3. Speech continues: reaches 100ms >= 80ms -> SpeechStart
	ev = detector.Process(chunk50ms)
	if ev.Type != vad.SpeechStart {
		t.Fatalf("expected speech_start, got %v", ev.Type)
	}
	if !detector.IsSpeaking() {
		t.Fatal("expected IsSpeaking() == true")
	}

	// 4. Speech continues -> returns Silence while ongoing
	ev = detector.Process(chunk50ms)
	if ev.Type != vad.Silence {
		t.Fatalf("expected ongoing speech event Silence, got %v", ev.Type)
	}

	// 5. Speech ends: 50ms silence (hangover is 100ms)
	mock.setSpeech(false)
	ev = detector.Process(chunk50ms)
	if ev.Type != vad.Silence {
		t.Fatalf("expected hangover silence, got %v", ev.Type)
	}
	if !detector.IsSpeaking() {
		t.Fatal("expected still speaking during hangover")
	}

	// 6. Another 50ms silence: reaches 100ms hangover -> SpeechEnd
	ev = detector.Process(chunk50ms)
	if ev.Type != vad.SpeechEnd {
		t.Fatalf("expected speech_end after hangover, got %v", ev.Type)
	}
	if detector.IsSpeaking() {
		t.Fatal("expected IsSpeaking() == false after SpeechEnd")
	}
}

func TestSileroDetector_Reset(t *testing.T) {
	mock := &mockVADCaller{isSpeech: true}
	detector := vad.NewSileroDetector(mock)

	chunk := audio.Chunk{Samples: make([]float32, 1600), SampleRate: 16000} // 100ms
	ev := detector.Process(chunk)
	if ev.Type != vad.SpeechStart {
		t.Fatalf("expected speech_start, got %v", ev.Type)
	}

	detector.Reset()
	if detector.IsSpeaking() {
		t.Fatal("expected IsSpeaking() == false after reset")
	}

	mock.mu.Lock()
	resetCalled := mock.lastReset
	mock.mu.Unlock()

	if !resetCalled {
		t.Fatal("expected reset call to worker")
	}
}

func TestSileroDetector_WorkerFailureFallback(t *testing.T) {
	mock := &mockVADCaller{failCall: true}
	detector := vad.NewSileroDetector(mock)

	chunk := audio.Chunk{Samples: make([]float32, 800), SampleRate: 16000}
	ev := detector.Process(chunk)
	if ev.Type != vad.Silence {
		t.Fatalf("expected silence on worker failure, got %v", ev.Type)
	}
}

func TestSileroDetector_ConcurrentRace(t *testing.T) {
	mock := &mockVADCaller{isSpeech: true}
	detector := vad.NewSileroDetector(mock)

	chunk := audio.Chunk{Samples: make([]float32, 160), SampleRate: 16000}
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%4 == 0 {
				detector.Reset()
			} else {
				detector.Process(chunk)
				_ = detector.IsSpeaking()
			}
		}(i)
	}

	wg.Wait()
}
