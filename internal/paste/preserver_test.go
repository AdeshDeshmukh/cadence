package paste_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
)

type mockClipboardStore struct {
	mu   sync.Mutex
	text string
}

func (m *mockClipboardStore) Get() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text, nil
}

func (m *mockClipboardStore) Set(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text = text
	return nil
}

type mockInnerPaster struct {
	mu     sync.Mutex
	pasted string
	err    error
}

func (m *mockInnerPaster) Paste(text string) (paste.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pasted = text
	if m.err != nil {
		return paste.Result{}, m.err
	}
	return paste.Result{Method: "mock", Success: true}, nil
}

func TestPreservingPaster_PreserveEnabled(t *testing.T) {
	clipStore := &mockClipboardStore{text: "original user clipboard"}
	inner := &mockInnerPaster{}

	preserver := paste.NewPreservingPaster(inner, true, 20*time.Millisecond)
	preserver.SetClipboardAccessors(clipStore.Get, clipStore.Set)

	// Simulate inner paste that overwrites the system clipboard
	innerPasterWithClip := &mockInnerPasterWithClip{
		clipStore: clipStore,
	}
	preserver = paste.NewPreservingPaster(innerPasterWithClip, true, 20*time.Millisecond)
	preserver.SetClipboardAccessors(clipStore.Get, clipStore.Set)

	res, err := preserver.Paste("dictated speech text")
	if err != nil {
		t.Fatalf("Paste failed: %v", err)
	}
	if !res.Success {
		t.Fatal("expected paste success")
	}

	// Immediately after paste, the inner paster set clipboard to dictated text
	if innerPasterWithClip.pasted != "dictated speech text" {
		t.Fatalf("expected inner paster to receive dictated speech text")
	}

	// Wait for restore
	preserver.WaitForRestore()

	// Clipboard must now be restored to "original user clipboard"
	got, _ := clipStore.Get()
	if got != "original user clipboard" {
		t.Fatalf("expected restored clipboard %q, got %q", "original user clipboard", got)
	}
}

type mockInnerPasterWithClip struct {
	clipStore *mockClipboardStore
	pasted    string
}

func (m *mockInnerPasterWithClip) Paste(text string) (paste.Result, error) {
	m.pasted = text
	_ = m.clipStore.Set(text)
	return paste.Result{Method: "mock", Success: true}, nil
}

func TestPreservingPaster_PreserveDisabled(t *testing.T) {
	clipStore := &mockClipboardStore{text: "original user clipboard"}
	inner := &mockInnerPasterWithClip{clipStore: clipStore}

	preserver := paste.NewPreservingPaster(inner, false, 20*time.Millisecond)
	preserver.SetClipboardAccessors(clipStore.Get, clipStore.Set)

	_, err := preserver.Paste("dictated text without restore")
	if err != nil {
		t.Fatalf("Paste failed: %v", err)
	}

	preserver.WaitForRestore()

	// Since preserve=false, clipboard should remain whatever inner set ("dictated text without restore")
	got, _ := clipStore.Get()
	if got != "dictated text without restore" {
		t.Fatalf("expected clipboard to remain %q, got %q", "dictated text without restore", got)
	}
}

func TestPreservingPaster_InnerError(t *testing.T) {
	clipStore := &mockClipboardStore{text: "original"}
	inner := &mockInnerPaster{err: errors.New("paste hardware failure")}

	preserver := paste.NewPreservingPaster(inner, true, 20*time.Millisecond)
	preserver.SetClipboardAccessors(clipStore.Get, clipStore.Set)

	_, err := preserver.Paste("text")
	if err == nil || err.Error() != "paste hardware failure" {
		t.Fatalf("expected inner paste error, got %v", err)
	}
}
