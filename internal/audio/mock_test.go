package audio_test

import (
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

func TestChunkDuration(t *testing.T) {
	c := audio.Chunk{
		Samples:    make([]float32, 16000),
		SampleRate: 16000,
	}
	if d := c.Duration(); d != time.Second {
		t.Fatalf("expected 1s, got %v", d)
	}

	cZero := audio.Chunk{
		Samples:    make([]float32, 16000),
		SampleRate: 0,
	}
	if d := cZero.Duration(); d != 0 {
		t.Fatalf("expected 0s for zero sample rate, got %v", d)
	}
}

func TestMockCapture(t *testing.T) {
	chunk := audio.Chunk{
		Samples:    make([]float32, 160),
		SampleRate: 16000,
	}
	mock := audio.NewMockCapture([]audio.Chunk{chunk, chunk})

	devs, err := mock.Devices()
	if err != nil {
		t.Fatalf("unexpected error getting devices: %v", err)
	}
	if len(devs) == 0 || !devs[0].Default {
		t.Fatalf("expected default device, got %v", devs)
	}

	ch, err := mock.Start()
	if err != nil {
		t.Fatalf("unexpected error starting capture: %v", err)
	}

	count := 0
	for range ch {
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 chunks, got %d", count)
	}

	if err := mock.Stop(); err != nil {
		t.Fatalf("unexpected error stopping capture: %v", err)
	}
}
