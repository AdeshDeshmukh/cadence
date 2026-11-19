//go:build windows

package audio

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWindowsWASAPIDriver_Lifecycle(t *testing.T) {
	driver := NewWindowsWASAPIDriver("")

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

	// Read at least 1 sample chunk
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

func TestWindowsWASAPIDriver_NamedDevice(t *testing.T) {
	driver := NewWindowsWASAPIDriver("Realtek High Definition Audio")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := make(chan []float32, 10)
	if err := driver.StartStream(ctx, 16000, outCh); err != nil {
		t.Fatalf("StartStream with named device failed: %v", err)
	}
	defer driver.StopStream()
}
