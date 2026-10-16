package storage_test

import (
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestMemoryStore_Transcripts(t *testing.T) {
	mem := storage.NewMemory()

	now := time.Now()
	t1 := storage.Transcript{
		ID:        "1",
		RawText:   "hello",
		Text:      "Hello.",
		CreatedAt: now.Add(-1 * time.Minute),
	}
	t2 := storage.Transcript{
		ID:        "2",
		RawText:   "world",
		Text:      "World.",
		CreatedAt: now,
	}

	if err := mem.SaveTranscript(t1); err != nil {
		t.Fatalf("failed to save t1: %v", err)
	}
	if err := mem.SaveTranscript(t2); err != nil {
		t.Fatalf("failed to save t2: %v", err)
	}

	// Should list in descending order of CreatedAt (t2 first, then t1)
	list, err := mem.ListTranscripts(10)
	if err != nil {
		t.Fatalf("failed to list transcripts: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 transcripts, got %d", len(list))
	}
	if list[0].ID != "2" || list[1].ID != "1" {
		t.Fatalf("unexpected ordering: got %v then %v", list[0].ID, list[1].ID)
	}

	// Test limit
	listLimited, err := mem.ListTranscripts(1)
	if err != nil {
		t.Fatalf("failed to list with limit: %v", err)
	}
	if len(listLimited) != 1 || listLimited[0].ID != "2" {
		t.Fatalf("expected 1 transcript with ID 2, got %v", listLimited)
	}
}

func TestMemoryStore_Vocabulary(t *testing.T) {
	mem := storage.NewMemory()

	entry := storage.VocabularyEntry{
		Spoken:    "cube control",
		Canonical: "kubectl",
	}

	if err := mem.SetVocabulary(entry); err != nil {
		t.Fatalf("failed to set vocabulary: %v", err)
	}

	vocab, err := mem.Vocabulary()
	if err != nil {
		t.Fatalf("failed to get vocabulary: %v", err)
	}
	if len(vocab) != 1 || vocab[0].Spoken != "cube control" || vocab[0].Canonical != "kubectl" {
		t.Fatalf("unexpected vocabulary: %+v", vocab)
	}
}
