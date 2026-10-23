package paste_test

import (
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
)

func TestClipboardPaste(t *testing.T) {
	cb := paste.NewClipboard()

	res, err := cb.Paste("test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.Method != "memory" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got := cb.Text(); got != "test text" {
		t.Fatalf("expected 'test text', got %q", got)
	}
}

func TestUnsupportedPaster(t *testing.T) {
	u := paste.Unsupported{}
	_, err := u.Paste("hello")
	if err == nil {
		t.Fatalf("expected error from unsupported paster, got nil")
	}
}
