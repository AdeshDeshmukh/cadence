package vad_test

import (
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/vad"
)

func TestSimpleVAD(t *testing.T) {
	detector := vad.NewSimple(0.1)

	// Silence chunk
	silenceChunk := audio.Chunk{
		Samples:    []float32{0.0, 0.01, -0.01, 0.0},
		SampleRate: 16000,
	}
	ev := detector.Process(silenceChunk)
	if ev.Type != vad.Silence {
		t.Fatalf("expected silence, got %v", ev.Type)
	}

	// Speech chunk (loud samples)
	speechChunk := audio.Chunk{
		Samples:    []float32{0.5, -0.5, 0.6, -0.4},
		SampleRate: 16000,
	}
	ev = detector.Process(speechChunk)
	if ev.Type != vad.SpeechStart {
		t.Fatalf("expected speech_start, got %v", ev.Type)
	}

	// Ongoing speech chunk
	ev = detector.Process(speechChunk)
	if ev.Type != vad.SpeechStart {
		t.Fatalf("expected ongoing speech_start, got %v", ev.Type)
	}

	// Transition back to silence
	ev = detector.Process(silenceChunk)
	if ev.Type != vad.SpeechEnd {
		t.Fatalf("expected speech_end, got %v", ev.Type)
	}

	// Reset detector
	detector.Reset()
	ev = detector.Process(silenceChunk)
	if ev.Type != vad.Silence {
		t.Fatalf("expected silence after reset, got %v", ev.Type)
	}
}

func TestSimpleVAD_EmptyChunk(t *testing.T) {
	detector := vad.NewSimple(0.1)
	ev := detector.Process(audio.Chunk{})
	if ev.Type != vad.Silence {
		t.Fatalf("expected silence for empty chunk, got %v", ev.Type)
	}

	// If speech was active, empty chunk should emit SpeechEnd
	speechChunk := audio.Chunk{
		Samples:    []float32{0.5, -0.5},
		SampleRate: 16000,
	}
	detector.Process(speechChunk)
	ev = detector.Process(audio.Chunk{})
	if ev.Type != vad.SpeechEnd {
		t.Fatalf("expected SpeechEnd on empty chunk during active speech, got %v", ev.Type)
	}
}

func TestSimpleVAD_ZeroThreshold(t *testing.T) {
	detector := vad.NewSimple(0.0)
	// Pure silence (rms 0) must still be Silence even with threshold 0
	zeroChunk := audio.Chunk{
		Samples:    []float32{0.0, 0.0, 0.0},
		SampleRate: 16000,
	}
	ev := detector.Process(zeroChunk)
	if ev.Type != vad.Silence {
		t.Fatalf("expected silence for zero RMS chunk with threshold 0, got %v", ev.Type)
	}
}
