package selection_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/selection"
)

func TestMockCapturer_Success(t *testing.T) {
	mock := selection.NewMockCapturer("function calculateTotal()", "accessibility_ax")
	if mock.CallCount() != 0 {
		t.Fatalf("expected 0 calls, got %d", mock.CallCount())
	}

	res, err := mock.Capture(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "function calculateTotal()" {
		t.Fatalf("expected 'function calculateTotal()', got %q", res.Text)
	}
	if res.Method != "accessibility_ax" {
		t.Fatalf("expected method 'accessibility_ax', got %q", res.Method)
	}
	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.CallCount())
	}
}

func TestMockCapturer_Empty(t *testing.T) {
	mock := selection.NewMockCapturer("", "mock")
	_, err := mock.Capture(context.Background())
	if !errors.Is(err, selection.ErrNoSelection) {
		t.Fatalf("expected ErrNoSelection, got %v", err)
	}
}

func TestMockCapturer_Error(t *testing.T) {
	mock := selection.NewMockCapturer("some text", "mock")
	simErr := errors.New("accessibility permission denied")
	mock.SetError(simErr)

	_, err := mock.Capture(context.Background())
	if !errors.Is(err, simErr) {
		t.Fatalf("expected simErr, got %v", err)
	}
}

func TestNewDefaultCapturer(t *testing.T) {
	c := selection.NewDefaultCapturer()
	if c == nil {
		t.Fatal("expected non-nil default capturer")
	}
}
