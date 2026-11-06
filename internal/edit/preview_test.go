package edit

import (
	"errors"
	"strings"
	"testing"
)

func TestPreviewManager_Lifecycle(t *testing.T) {
	mgr := NewPreviewManager(3)

	instr := Instruction{
		Intent: IntentShorten,
		Tone:   "concise",
	}

	// 1. Create session
	sess := mgr.Create("Original text to shorten", "Short text", instr)
	if sess.ID == "" {
		t.Fatal("expected non-empty preview ID")
	}
	if sess.Status != PreviewPending {
		t.Fatalf("expected status %q, got %q", PreviewPending, sess.Status)
	}
	if !strings.Contains(sess.DiffSummary, "- Original text") || !strings.Contains(sess.DiffSummary, "+ Short text") {
		t.Fatalf("unexpected diff summary:\n%s", sess.DiffSummary)
	}

	// 2. Get session
	retrieved, err := mgr.Get(sess.ID)
	if err != nil {
		t.Fatalf("get preview failed: %v", err)
	}
	if retrieved.ID != sess.ID {
		t.Fatalf("expected ID %q, got %q", sess.ID, retrieved.ID)
	}

	// 3. List pending
	pending := mgr.ListPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending session, got %d", len(pending))
	}

	// 4. Apply session
	applied, err := mgr.Apply(sess.ID)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if applied.Status != PreviewApplied {
		t.Fatalf("expected status %q, got %q", PreviewApplied, applied.Status)
	}
	if applied.ResolvedAt == nil {
		t.Fatal("expected non-nil ResolvedAt")
	}

	// 5. Applying already resolved session fails
	if _, err := mgr.Apply(sess.ID); !errors.Is(err, ErrPreviewResolved) {
		t.Fatalf("expected ErrPreviewResolved, got %v", err)
	}
	if _, err := mgr.Reject(sess.ID); !errors.Is(err, ErrPreviewResolved) {
		t.Fatalf("expected ErrPreviewResolved on reject, got %v", err)
	}

	// 6. Pending list should now be empty
	if len(mgr.ListPending()) != 0 {
		t.Fatalf("expected 0 pending sessions, got %d", len(mgr.ListPending()))
	}
}

func TestPreviewManager_RejectAndPruning(t *testing.T) {
	mgr := NewPreviewManager(2)

	s1 := mgr.Create("Orig 1", "Cand 1", Instruction{Intent: IntentRewrite})
	s2 := mgr.Create("Orig 2", "Cand 2", Instruction{Intent: IntentRewrite})

	// Reject s2
	rejected, err := mgr.Reject(s2.ID)
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if rejected.Status != PreviewRejected {
		t.Fatalf("expected status %q, got %q", PreviewRejected, rejected.Status)
	}

	// Adding s3 should prune oldest (s1) because maxKeep is 2
	s3 := mgr.Create("Orig 3", "Cand 3", Instruction{Intent: IntentRewrite})

	if _, err := mgr.Get(s1.ID); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound for pruned session s1, got %v", err)
	}
	if _, err := mgr.Get(s2.ID); err != nil {
		t.Fatalf("expected s2 to exist: %v", err)
	}
	if _, err := mgr.Get(s3.ID); err != nil {
		t.Fatalf("expected s3 to exist: %v", err)
	}
}
