//go:build linux

package audio

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLinuxAudioDriver_Lifecycle(t *testing.T) {
	driver := NewLinuxAudioDriver("")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := make(chan []float32, 10)

	err := driver.StartStream(ctx, 16000, outCh)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}

	// Double start should fail with ErrAlreadyCapturing
	if err := driver.StartStream(ctx, 16000, outCh); !errors.Is(err, ErrAlreadyCapturing) {
		t.Errorf("expected ErrAlreadyCapturing on double start, got %v", err)
	}

	// Read at least 2 sample chunks
	select {
	case chunk := <-outCh:
		if len(chunk) == 0 {
			t.Errorf("received empty chunk")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for audio chunk")
	}

	// Stop stream
	if err := driver.StopStream(); err != nil {
		t.Fatalf("StopStream failed: %v", err)
	}

	// Double stop should return ErrNotCapturing
	if err := driver.StopStream(); !errors.Is(err, ErrNotCapturing) {
		t.Errorf("expected ErrNotCapturing on double stop, got %v", err)
	}
}

func TestLinuxAudioDriver_BackendDetection(t *testing.T) {
	backend := DetectLinuxAudioBackend()
	switch backend {
	case BackendPipeWire, BackendPulseAudio, BackendSimulated:
		// Valid backends
	default:
		t.Errorf("unexpected backend detected: %s", backend)
	}
}
