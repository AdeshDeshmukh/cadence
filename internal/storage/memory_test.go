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

func TestMemoryStore_SessionRecovery(t *testing.T) {
	mem := storage.NewMemory()

	last, err := mem.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery failed: %v", err)
	}
	if last != nil {
		t.Fatalf("expected nil recovery initially, got: %+v", last)
	}

	now := time.Now()
	r1 := storage.SessionRecovery{
		ID:          "rec-1",
		State:       "cancelled",
		PartialText: "hello world",
		CreatedAt:   now.Add(-2 * time.Minute),
		UpdatedAt:   now.Add(-2 * time.Minute),
	}
	r2 := storage.SessionRecovery{
		ID:        "rec-2",
		State:     "failed",
		Text:      "Final failed text",
		CreatedAt: now.Add(-1 * time.Minute),
		UpdatedAt: now.Add(-1 * time.Minute),
	}

	if err := mem.SaveRecovery(r1); err != nil {
		t.Fatalf("save r1: %v", err)
	}
	if err := mem.SaveRecovery(r2); err != nil {
		t.Fatalf("save r2: %v", err)
	}

	last, err = mem.GetLastRecovery()
	if err != nil || last == nil || last.ID != "rec-2" {
		t.Fatalf("expected r2 as last recovery, got: %+v, err: %v", last, err)
	}

	list, err := mem.ListRecoveries(5)
	if err != nil || len(list) != 2 {
		t.Fatalf("expected 2 recoveries, got: %+v, err: %v", list, err)
	}

	if err := mem.DeleteRecovery("rec-2"); err != nil {
		t.Fatalf("delete r2: %v", err)
	}

	last, err = mem.GetLastRecovery()
	if err != nil || last == nil || last.ID != "rec-1" {
		t.Fatalf("expected r1 after deleting r2, got: %+v, err: %v", last, err)
	}

	if err := mem.ClearRecoveries(); err != nil {
		t.Fatalf("clear recoveries: %v", err)
	}
	last, err = mem.GetLastRecovery()
	if err != nil || last != nil {
		t.Fatalf("expected nil after clear, got: %+v", last)
	}
}

func TestMemoryStore_Snippets(t *testing.T) {
	mem := storage.NewMemory()

	snip := storage.Snippet{
		Trigger:     "signoff",
		Template:    "Best regards,\nAlice",
		Description: "Email sign-off",
	}

	if err := mem.SetSnippet(snip); err != nil {
		t.Fatalf("SetSnippet: %v", err)
	}

	got, err := mem.GetSnippet("signoff")
	if err != nil || got == nil {
		t.Fatalf("GetSnippet: %v", err)
	}
	if got.Template != snip.Template {
		t.Errorf("expected %q, got %q", snip.Template, got.Template)
	}

	list, err := mem.ListSnippets()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 snippet, got %v", list)
	}

	if err := mem.DeleteSnippet("signoff"); err != nil {
		t.Fatalf("DeleteSnippet: %v", err)
	}

	gotAfter, err := mem.GetSnippet("signoff")
	if err != nil || gotAfter != nil {
		t.Fatalf("expected nil after delete, got %v", gotAfter)
	}
}

func TestMemoryStore_KnowledgeBase(t *testing.T) {
	mem := storage.NewMemory()
	now := time.Now()

	// 1. Folders
	f := storage.Folder{
		ID:        "folder-1",
		Name:      "Engineering",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := mem.SaveFolder(f); err != nil {
		t.Fatalf("SaveFolder: %v", err)
	}
	gotF, err := mem.GetFolder("folder-1")
	if err != nil || gotF == nil || gotF.Name != "Engineering" {
		t.Fatalf("GetFolder failed: %v, got %+v", err, gotF)
	}

	// 2. Notes
	n := storage.Note{
		ID:        "note-1",
		FolderID:  "folder-1",
		Title:     "Standup Notes",
		Content:   "Discussed sprint goals",
		Tags:      []string{"daily", "standup"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := mem.SaveNote(n); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}

	gotN, err := mem.GetNote("note-1")
	if err != nil || gotN == nil || len(gotN.Tags) != 2 {
		t.Fatalf("GetNote failed: %v, got %+v", err, gotN)
	}

	// 3. Filter notes
	filtered, err := mem.ListNotes(storage.NoteFilter{Tag: "standup"})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("ListNotes by tag: %v, got %+v", err, filtered)
	}

	// 4. Tags
	tags, err := mem.ListTags()
	if err != nil || len(tags) != 2 {
		t.Fatalf("ListTags failed: %v, got %+v", err, tags)
	}

	// 5. Delete folder unlinks
	if err := mem.DeleteFolder("folder-1"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	afterFolderDel, err := mem.GetNote("note-1")
	if err != nil || afterFolderDel.FolderID != "" {
		t.Fatalf("expected folder_id unlinked, got %q", afterFolderDel.FolderID)
	}

	// 6. Delete note
	if err := mem.DeleteNote("note-1"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	afterNoteDel, err := mem.GetNote("note-1")
	if err != nil || afterNoteDel != nil {
		t.Fatalf("expected nil note after delete, got %+v", afterNoteDel)
	}
}

func TestMemoryStore_Search(t *testing.T) {
	mem := storage.NewMemory()
	now := time.Now()

	_ = mem.SaveNote(storage.Note{
		ID:        "note-mem-1",
		Title:     "Docker Setup",
		Content:   "Containerizing the application with multi-stage Dockerfile builds.",
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = mem.SaveTranscript(storage.Transcript{
		ID:        "tr-mem-1",
		Text:      "Docker compose up was executed.",
		RawText:   "docker compose up was executed",
		CreatedAt: now.Add(1 * time.Second),
	})
	_ = mem.SaveMeeting(storage.Meeting{
		ID:        "mtg-mem-1",
		Title:     "DevOps Review",
		CreatedAt: now.Add(2 * time.Second),
	})
	_ = mem.SaveMeetingSegment(storage.MeetingSegment{
		ID:        "seg-mem-1",
		MeetingID: "mtg-mem-1",
		Text:      "Let's migrate to Docker swarm or k8s.",
		CreatedAt: now.Add(2 * time.Second),
	})

	// 1. Search all
	res, err := mem.Search("Docker", storage.SearchFilter{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(res))
	}

	// 2. Search notes only
	notesOnly, err := mem.Search("Docker", storage.SearchFilter{
		Types: []storage.SearchResultType{storage.SearchResultNote},
	})
	if err != nil || len(notesOnly) != 1 {
		t.Fatalf("expected 1 note hit, got %d", len(notesOnly))
	}
}
