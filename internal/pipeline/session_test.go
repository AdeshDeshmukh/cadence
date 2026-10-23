package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/pipeline"
)

func TestSessionState(t *testing.T) {
	s := pipeline.NewSession("sess-1")
	if s.GetState() != pipeline.Idle {
		t.Fatalf("expected Idle, got %s", s.GetState())
	}

	s.SetState(pipeline.Recording)
	if s.GetState() != pipeline.Recording {
		t.Fatalf("expected Recording, got %s", s.GetState())
	}

	s.SetState(pipeline.Complete)
	if s.GetState() != pipeline.Complete {
		t.Fatalf("expected Complete, got %s", s.GetState())
	}
	if s.EndedAt.IsZero() {
		t.Fatalf("expected EndedAt to be set on terminal state")
	}
}

func TestTerminalStates(t *testing.T) {
	if !pipeline.Complete.Terminal() || !pipeline.Failed.Terminal() || !pipeline.Cancelled.Terminal() {
		t.Fatalf("expected Complete, Failed, and Cancelled to be terminal")
	}
	if pipeline.Recording.Terminal() || pipeline.Transcribing.Terminal() || pipeline.Processing.Terminal() || pipeline.Pasting.Terminal() || pipeline.Idle.Terminal() {
		t.Fatalf("non-terminal states reported as terminal")
	}
}

func TestContextCancellation(t *testing.T) {
	// Simple test verifying Cancelled state handling
	s := pipeline.NewSession("cancel-test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	select {
	case <-ctx.Done():
		s.SetState(pipeline.Cancelled)
	default:
		t.Fatal("expected context to be cancelled")
	}

	if s.GetState() != pipeline.Cancelled {
		t.Fatalf("expected Cancelled, got %s", s.GetState())
	}
	if time.Since(s.StartedAt) < 0 {
		t.Fatal("invalid start time")
	}
}
