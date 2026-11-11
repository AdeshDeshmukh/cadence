package meeting

import (
	"context"
	"errors"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestExtractiveSummarizer_EmptySegments(t *testing.T) {
	summarizer := NewExtractiveSummarizer()
	mtg := storage.Meeting{
		ID:    "mtg-1",
		Title: "Sync Meeting",
	}

	summary, err := summarizer.Summarize(context.Background(), mtg, nil)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary.MeetingID != "mtg-1" {
		t.Errorf("expected meeting ID 'mtg-1', got %q", summary.MeetingID)
	}
	if len(summary.Decisions) != 0 || len(summary.ActionItems) != 0 {
		t.Errorf("expected empty decisions and action items")
	}
	if summary.Overview == "" {
		t.Errorf("expected non-empty overview")
	}
}

func TestExtractiveSummarizer_Extraction(t *testing.T) {
	summarizer := NewExtractiveSummarizer()
	mtg := storage.Meeting{
		ID:    "mtg-prod-sync",
		Title: "Production Launch Review",
	}

	segments := []storage.MeetingSegment{
		{
			ID:           "seg-1",
			MeetingID:    mtg.ID,
			SegmentIndex: 0,
			StartMs:      0,
			EndMs:        5000,
			Speaker:      "Alice",
			Text:         "Good morning. We are here regarding deployment pipeline setup.",
		},
		{
			ID:           "seg-2",
			MeetingID:    mtg.ID,
			SegmentIndex: 1,
			StartMs:      5100,
			EndMs:        12000,
			Speaker:      "Bob",
			Text:         "We agreed to migrate the database on Saturday night.",
		},
		{
			ID:           "seg-3",
			MeetingID:    mtg.ID,
			SegmentIndex: 2,
			StartMs:      12100,
			EndMs:        20000,
			Speaker:      "Charlie",
			Text:         "Action item: Dave will update the monitoring dashboards. Also please make sure to assign to @Eve for load testing.",
		},
		{
			ID:           "seg-4",
			MeetingID:    mtg.ID,
			SegmentIndex: 3,
			StartMs:      20100,
			EndMs:        28000,
			Speaker:      "Alice",
			Text:         "I will review the pull requests before Friday.",
		},
	}

	summary, err := summarizer.Summarize(context.Background(), mtg, segments)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if len(summary.Decisions) == 0 {
		t.Errorf("expected at least 1 decision, got 0")
	}
	if len(summary.ActionItems) < 2 {
		t.Errorf("expected at least 2 action items, got %d", len(summary.ActionItems))
	}

	// Verify action item assignment
	var foundEve, foundDave, foundAlice bool
	for _, act := range summary.ActionItems {
		if act.Assignee == "Eve" {
			foundEve = true
		}
		if act.Assignee == "Dave" {
			foundDave = true
		}
		if act.Assignee == "Alice" {
			foundAlice = true
		}
	}
	if !foundEve && !foundDave && !foundAlice {
		t.Errorf("expected assignees Eve, Dave, or Alice, got %+v", summary.ActionItems)
	}

	if summary.Overview == "" {
		t.Errorf("expected generated overview, got empty")
	}
}

func TestLLMSummarizer_Success(t *testing.T) {
	mockPolisher := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		jsonResp := `
` + "```json" + `
{
  "overview": "The team aligned on the Q4 roadmap and agreed to ship continuous recording.",
  "decisions": [
    "Ship continuous recording mode in Milestone 49",
    "Adopt SQLite for local meeting storage"
  ],
  "action_items": [
    {
      "description": "Finalize CLI commands",
      "assignee": "Adesh"
    },
    {
      "description": "Write integration test suite",
      "assignee": "Alice"
    }
  ],
  "topics": [
    "Roadmap",
    "Storage",
    "Continuous Recording"
  ]
}
` + "```"
		return polish.Response{
			Text:  jsonResp,
			Model: "mock-llm",
		}, nil
	})

	summarizer := NewLLMSummarizer(mockPolisher)
	mtg := storage.Meeting{
		ID:    "mtg-q4",
		Title: "Q4 Planning",
	}
	segments := []storage.MeetingSegment{
		{
			ID:           "seg-1",
			MeetingID:    mtg.ID,
			SegmentIndex: 0,
			StartMs:      0,
			EndMs:        4000,
			Speaker:      "Adesh",
			Text:         "Let's discuss the Q4 roadmap.",
		},
	}

	summary, err := summarizer.Summarize(context.Background(), mtg, segments)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if summary.Overview != "The team aligned on the Q4 roadmap and agreed to ship continuous recording." {
		t.Errorf("unexpected overview: %q", summary.Overview)
	}
	if len(summary.Decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(summary.Decisions))
	}
	if len(summary.ActionItems) != 2 {
		t.Errorf("expected 2 action items, got %d", len(summary.ActionItems))
	}
	if summary.ActionItems[0].Assignee != "Adesh" || summary.ActionItems[1].Assignee != "Alice" {
		t.Errorf("unexpected action item assignees: %+v", summary.ActionItems)
	}
	if len(summary.Topics) != 3 {
		t.Errorf("expected 3 topics, got %d", len(summary.Topics))
	}
}

func TestLLMSummarizer_FallbackOnError(t *testing.T) {
	mockPolisher := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{}, errors.New("network failure to LLM endpoint")
	})

	summarizer := NewLLMSummarizer(mockPolisher)
	mtg := storage.Meeting{
		ID:    "mtg-fallback",
		Title: "Fallback Sync",
	}
	segments := []storage.MeetingSegment{
		{
			ID:           "seg-1",
			MeetingID:    mtg.ID,
			SegmentIndex: 0,
			StartMs:      0,
			EndMs:        5000,
			Speaker:      "Alice",
			Text:         "We decided to use extractive fallback when offline.",
		},
	}

	summary, err := summarizer.Summarize(context.Background(), mtg, segments)
	if err != nil {
		t.Fatalf("expected fallback without error, got: %v", err)
	}
	if summary == nil {
		t.Fatal("expected summary from fallback, got nil")
	}
	if len(summary.Decisions) != 1 {
		t.Errorf("expected fallback to extract decision, got %+v", summary.Decisions)
	}
}
