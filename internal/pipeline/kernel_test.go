package pipeline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/pipeline"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

type failingASR struct{}

func (failingASR) Name() string { return "failing-asr" }

func (failingASR) Transcribe(context.Context, []audio.Chunk) (asr.Result, error) {
	return asr.Result{}, errors.New("simulated asr crash")
}

func (failingASR) Stream(context.Context, <-chan audio.Chunk) (<-chan asr.PartialResult, error) {
	return nil, nil
}

type hangingCapture struct {
	ch   chan audio.Chunk
	stop chan struct{}
}

func newHangingCapture() *hangingCapture {
	return &hangingCapture{
		ch:   make(chan audio.Chunk),
		stop: make(chan struct{}),
	}
}

func (h *hangingCapture) Devices() ([]audio.Device, error) {
	return nil, nil
}

func (h *hangingCapture) Start() (<-chan audio.Chunk, error) {
	return h.ch, nil
}

func (h *hangingCapture) Stop() error {
	select {
	case <-h.stop:
	default:
		close(h.stop)
	}
	return nil
}

func TestKernel_CancellationRecovery(t *testing.T) {
	cap := newHangingCapture()
	store := storage.NewMemory()
	paster := paste.NewClipboard()
	proc := postprocess.New()
	mockASR := asr.MockEngine{Text: "recovered partial text"}

	kernel := pipeline.NewKernel(cap, mockASR, paster, proc, store)
	session := pipeline.NewSession("sess-cancel")

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- kernel.Run(ctx, session, postprocess.Context{Mode: "dictation"})
	}()

	// Wait briefly then cancel
	time.Sleep(20 * time.Millisecond)
	session.UpdatePartial("hello live speech")
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	if session.GetState() != pipeline.Cancelled {
		t.Fatalf("expected state Cancelled, got: %s", session.GetState())
	}

	rec, err := store.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery: %v", err)
	}
	if rec == nil {
		t.Fatalf("expected non-nil recovery record")
	}
	if rec.ID != "sess-cancel" {
		t.Errorf("expected ID sess-cancel, got %s", rec.ID)
	}
	if rec.State != "cancelled" {
		t.Errorf("expected state 'cancelled', got %s", rec.State)
	}
	if rec.BestText() != "hello live speech" {
		t.Errorf("expected BestText 'hello live speech', got %q", rec.BestText())
	}
}

func TestKernel_FailureRecovery(t *testing.T) {
	cap := audio.NewMockCapture([]audio.Chunk{{
		Samples:    make([]float32, 1600),
		SampleRate: 16000,
	}})
	store := storage.NewMemory()
	paster := paste.NewClipboard()
	proc := postprocess.New()

	kernel := pipeline.NewKernel(cap, failingASR{}, paster, proc, store)
	session := pipeline.NewSession("sess-fail")

	err := kernel.Run(context.Background(), session, postprocess.Context{Mode: "dictation"})
	if err == nil {
		t.Fatalf("expected error from failing ASR")
	}

	if session.GetState() != pipeline.Failed {
		t.Fatalf("expected state Failed, got: %s", session.GetState())
	}

	rec, err := store.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery: %v", err)
	}
	if rec == nil {
		t.Fatalf("expected non-nil recovery record")
	}
	if rec.ID != "sess-fail" {
		t.Errorf("expected ID sess-fail, got %s", rec.ID)
	}
	if rec.State != "failed" {
		t.Errorf("expected state 'failed', got %s", rec.State)
	}
	if rec.Error != "simulated asr crash" {
		t.Errorf("expected error 'simulated asr crash', got %q", rec.Error)
	}
}
