package paste_test

import (
	"errors"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
)

func TestMockReverter(t *testing.T) {
	rev := paste.NewMockReverter()
	if rev.Count() != 0 {
		t.Fatalf("expected count 0, got %d", rev.Count())
	}

	if err := rev.Revert(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rev.Count() != 1 {
		t.Fatalf("expected count 1, got %d", rev.Count())
	}

	rev.RevertFunc = func() error {
		return errors.New("simulated error")
	}
	if err := rev.Revert(); err == nil {
		t.Fatal("expected error, got nil")
	}
	if rev.Count() != 2 {
		t.Fatalf("expected count 2, got %d", rev.Count())
	}
}
