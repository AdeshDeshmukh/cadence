package meeting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestAggregateTurns(t *testing.T) {
	// 1. Empty input
	if len(AggregateTurns(nil)) != 0 {
		t.Fatal("expected empty turns for nil segments")
	}

	// 2. Sequential segments with speaker transitions
	segs := []storage.MeetingSegment{
		{
			ID:           "seg-1",
			MeetingID:    "mtg-1",
			SegmentIndex: 0,
			StartMs:      0,
			EndMs:        10000,
			Speaker:      "Alice",
			Text:         "Good morning everyone.",
		},
		{
			ID:           "seg-2",
			MeetingID:    "mtg-1",
			SegmentIndex: 1,
			StartMs:      10000,
			EndMs:        20000,
			Speaker:      "Alice",
			Text:         "Let's review the sprint goals.",
		},
		{
			ID:           "seg-3",
			MeetingID:    "mtg-1",
			SegmentIndex: 2,
			StartMs:      20000,
			EndMs:        30000,
			Speaker:      "Bob",
			Text:         "Phase 7 is on track.",
		},
		{
			ID:           "seg-4",
			MeetingID:    "mtg-1",
			SegmentIndex: 3,
			StartMs:      30000,
			EndMs:        40000,
			Speaker:      "Alice",
			Text:         "Excellent.",
		},
	}

	turns := AggregateTurns(segs)
	if len(turns) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(turns))
	}

	// Turn 1: Alice [0 - 20000]
	if turns[0].Speaker != "Alice" || turns[0].StartMs != 0 || turns[0].EndMs != 20000 {
		t.Fatalf("unexpected turn 0: %+v", turns[0])
	}
	if turns[0].Text != "Good morning everyone. Let's review the sprint goals." {
		t.Fatalf("unexpected turn 0 text: %q", turns[0].Text)
	}
	if len(turns[0].SegmentIDs) != 2 {
		t.Fatalf("expected 2 segment IDs for turn 0, got %d", len(turns[0].SegmentIDs))
	}

	// Turn 2: Bob [20000 - 30000]
	if turns[1].Speaker != "Bob" || turns[1].StartMs != 20000 || turns[1].EndMs != 30000 {
		t.Fatalf("unexpected turn 1: %+v", turns[1])
	}
	if turns[1].Text != "Phase 7 is on track." {
		t.Fatalf("unexpected turn 1 text: %q", turns[1].Text)
	}

	// Turn 3: Alice [30000 - 40000]
	if turns[2].Speaker != "Alice" || turns[2].StartMs != 30000 || turns[2].EndMs != 40000 {
		t.Fatalf("unexpected turn 2: %+v", turns[2])
	}

	// 3. FormatTurns
	formatted := FormatTurns(turns)
	if !strings.Contains(formatted, "[00:00 - 00:20] Alice: Good morning everyone.") ||
		!strings.Contains(formatted, "[00:20 - 00:30] Bob: Phase 7 is on track.") {
		t.Fatalf("unexpected formatted output:\n%s", formatted)
	}
}

func TestMeetingManager_SpeakerManagementAndTurns(t *testing.T) {
	tmpDir := t.TempDir()
	store := storage.NewMemory()

	mtgID := "mtg-speakers-test"
	_ = store.SaveMeeting(storage.Meeting{
		ID:        mtgID,
		Title:     "Architecture Review",
		State:     "stopped",
		CreatedAt: time.Now(),
	})

	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "s-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        10000,
		Text:         "Welcome to the review.",
		Speaker:      "SPEAKER_00",
		CreatedAt:    time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "s-2",
		MeetingID:    mtgID,
		SegmentIndex: 1,
		StartMs:      10000,
		EndMs:        20000,
		Text:         "Let's check the schema.",
		Speaker:      "SPEAKER_00",
		CreatedAt:    time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "s-3",
		MeetingID:    mtgID,
		SegmentIndex: 2,
		StartMs:      20000,
		EndMs:        30000,
		Text:         "Schema looks good.",
		Speaker:      "SPEAKER_01",
		CreatedAt:    time.Now(),
	})

	mgr := NewManager(store, nil, WithAudioDirectory(tmpDir))
	ctx := context.Background()

	// 1. RenameSpeaker: SPEAKER_00 -> Adesh Deshmukh
	if err := mgr.RenameSpeaker(ctx, mtgID, "SPEAKER_00", "Adesh Deshmukh"); err != nil {
		t.Fatalf("RenameSpeaker failed: %v", err)
	}

	// 2. ListSpeakers
	speakers, err := mgr.ListSpeakers(ctx, mtgID)
	if err != nil {
		t.Fatalf("ListSpeakers failed: %v", err)
	}
	if len(speakers) != 1 || speakers[0].Name != "Adesh Deshmukh" {
		t.Fatalf("unexpected speakers list: %+v", speakers)
	}

	// 3. GetTranscriptTurns should show 2 turns (Adesh [0 - 20000], SPEAKER_01 [20000 - 30000])
	turns, err := mgr.GetTranscriptTurns(ctx, mtgID)
	if err != nil {
		t.Fatalf("GetTranscriptTurns failed: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d: %+v", len(turns), turns)
	}
	if turns[0].Speaker != "Adesh Deshmukh" || turns[0].EndMs != 20000 {
		t.Fatalf("expected aggregated turn 0 by Adesh Deshmukh, got: %+v", turns[0])
	}
	if turns[1].Speaker != "SPEAKER_01" {
		t.Fatalf("expected turn 1 by SPEAKER_01, got: %+v", turns[1])
	}

	// 4. Error validations
	if err := mgr.RenameSpeaker(ctx, "", "A", "B"); err == nil {
		t.Fatal("expected error on empty meetingID")
	}
	if err := mgr.RenameSpeaker(ctx, mtgID, "", "B"); err == nil {
		t.Fatal("expected error on empty fromSpeaker")
	}
	if _, err := mgr.ListSpeakers(ctx, ""); err == nil {
		t.Fatal("expected error listing speakers with empty meetingID")
	}
	if _, err := mgr.GetTranscriptTurns(ctx, ""); err == nil {
		t.Fatal("expected error getting turns with empty meetingID")
	}
}
