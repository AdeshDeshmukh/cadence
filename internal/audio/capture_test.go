package audio

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEngine_Lifecycle(t *testing.T) {
	simDriver := NewSimulatedDriver(440, 256, 10*time.Millisecond)
	engine := NewEngine(
		WithDriver(simDriver),
		WithSampleRate(16000),
		WithPreRollDuration(100*time.Millisecond),
	)

	if engine.IsCapturing() {
		t.Fatalf("expected engine not capturing initially")
	}

	// Double stop should return ErrNotCapturing
	if err := engine.Stop(); !errors.Is(err, ErrNotCapturing) {
		t.Fatalf("expected ErrNotCapturing on initial stop, got %v", err)
	}

	// Start capture
	ch, err := engine.Start()
	if err != nil {
		t.Fatalf("unexpected error starting capture: %v", err)
	}
	if !engine.IsCapturing() {
		t.Fatalf("expected engine to be capturing after Start()")
	}

	// Double start should return ErrAlreadyCapturing
	if _, err := engine.Start(); !errors.Is(err, ErrAlreadyCapturing) {
		t.Fatalf("expected ErrAlreadyCapturing, got %v", err)
	}

	// Read at least 2 chunks
	chunksReceived := 0
	timeout := time.After(500 * time.Millisecond)

readLoop:
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				break readLoop
			}
			if len(chunk.Samples) > 0 {
				chunksReceived++
			}
			if chunksReceived >= 3 {
				break readLoop
			}
		case <-timeout:
			t.Fatalf("timed out waiting for audio chunks, received %d", chunksReceived)
		}
	}

	if chunksReceived < 3 {
		t.Fatalf("expected >= 3 chunks, got %d", chunksReceived)
	}

	// Stop capture
	if err := engine.Stop(); err != nil {
		t.Fatalf("unexpected error stopping capture: %v", err)
	}

	if engine.IsCapturing() {
		t.Fatalf("expected engine not capturing after Stop()")
	}

	// Output channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			// In case buffered items were drained, wait for close
			for range ch {
			}
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected channel to close after Stop()")
	}
}

func TestEngine_PreRollInjection(t *testing.T) {
	simDriver := NewSimulatedDriver(0, 256, 10*time.Millisecond)
	engine := NewEngine(
		WithDriver(simDriver),
		WithSampleRate(16000),
		WithPreRollDuration(100*time.Millisecond), // 1600 samples
	)

	// Pre-fill ring buffer with recognizable samples [100.0, 101.0, ...]
	preRollSamples := make([]float32, 1600)
	for i := range preRollSamples {
		preRollSamples[i] = float32(100 + i)
	}
	engine.RingBuffer().Write(preRollSamples)

	ch, err := engine.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer engine.Stop()

	// The very first chunk MUST contain the pre-roll samples!
	select {
	case firstChunk, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed prematurely")
		}
		if len(firstChunk.Samples) != 1600 {
			t.Fatalf("expected 1600 pre-roll samples in first chunk, got %d", len(firstChunk.Samples))
		}
		if firstChunk.Samples[0] != 100 || firstChunk.Samples[1599] != 1699 {
			t.Fatalf("unexpected first chunk boundaries: start=%f, end=%f", firstChunk.Samples[0], firstChunk.Samples[1599])
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("timed out waiting for first pre-roll chunk")
	}
}

func TestEngine_Devices(t *testing.T) {
	mockDevs := []Device{
		{ID: "mic-1", Name: "Microphone 1", Input: true, Default: true},
	}
	mgr := NewManager(&mockEnumerator{devices: mockDevs})
	engine := NewEngine(
		WithDeviceManager(mgr),
		WithDriver(NewSimulatedDriver(0, 256, 10*time.Millisecond)),
	)

	devs, err := engine.Devices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devs) != 1 || devs[0].ID != "mic-1" {
		t.Fatalf("unexpected devices: %+v", devs)
	}

	if engine.DeviceManager() != mgr {
		t.Fatalf("expected DeviceManager to match provided manager")
	}
}

func TestSimulatedDriver_SilenceAndTone(t *testing.T) {
	// Silence (freq = 0)
	silenceDriver := NewSimulatedDriver(0, 100, 5*time.Millisecond)
	ch := make(chan []float32, 10)
	ctx, cancel := contextWithTimeout(100 * time.Millisecond)
	defer cancel()

	if err := silenceDriver.StartStream(ctx, 16000, ch); err != nil {
		t.Fatalf("failed to start silence driver: %v", err)
	}

	select {
	case frame := <-ch:
		for _, s := range frame {
			if s != 0.0 {
				t.Fatalf("expected 0.0 silence sample, got %f", s)
			}
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for silence frame")
	}
	silenceDriver.StopStream()

	// Tone (freq = 440)
	toneDriver := NewSimulatedDriver(440, 100, 5*time.Millisecond)
	chTone := make(chan []float32, 10)
	ctxTone, cancelTone := contextWithTimeout(100 * time.Millisecond)
	defer cancelTone()

	if err := toneDriver.StartStream(ctxTone, 16000, chTone); err != nil {
		t.Fatalf("failed to start tone driver: %v", err)
	}

	select {
	case frame := <-chTone:
		nonZero := false
		for _, s := range frame {
			if s != 0.0 {
				nonZero = true
				break
			}
		}
		if !nonZero {
			t.Fatalf("expected non-zero samples for 440Hz tone")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for tone frame")
	}
	toneDriver.StopStream()
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
