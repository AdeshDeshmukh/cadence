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

func TestMemoryStore_Embeddings(t *testing.T) {
	mem := storage.NewMemory()

	// Get non-existent transcript
	_, err := mem.GetTranscript("missing")
	if err == nil {
		t.Fatalf("expected error for missing transcript")
	}

	tr := storage.Transcript{
		ID:        "tr-mem-get",
		Text:      "Found text.",
		CreatedAt: time.Now(),
	}
	_ = mem.SaveTranscript(tr)
	found, err := mem.GetTranscript("tr-mem-get")
	if err != nil || found.Text != "Found text." {
		t.Fatalf("GetTranscript failed: %v, got: %+v", err, found)
	}

	// Embeddings Save, Get, List, Delete
	rec1 := storage.EmbeddingRecord{
		TargetType: "transcript",
		TargetID:   "tr-mem-get",
		Model:      "hash-v1",
		Embedding:  []float32{0.1, 0.2, 0.3},
	}
	if err := mem.SaveEmbedding(rec1); err != nil {
		t.Fatalf("SaveEmbedding failed: %v", err)
	}

	got, err := mem.GetEmbedding("transcript", "tr-mem-get", "hash-v1")
	if err != nil || len(got.Embedding) != 3 {
		t.Fatalf("GetEmbedding failed: %v", err)
	}

	list, err := mem.ListEmbeddings("transcript", "hash-v1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListEmbeddings failed: %v, got %d", err, len(list))
	}

	if err := mem.DeleteEmbedding("transcript", "tr-mem-get", "hash-v1"); err != nil {
		t.Fatalf("DeleteEmbedding failed: %v", err)
	}
	_, err = mem.GetEmbedding("transcript", "tr-mem-get", "hash-v1")
	if err == nil {
		t.Fatalf("expected embedding to be deleted")
	}
}

func TestMemoryStore_Plugins(t *testing.T) {
	mem := storage.NewMemory()

	p1 := storage.PluginRecord{
		ID:           "test-plugin-1",
		Name:         "Test Plugin 1",
		Version:      "1.0.0",
		InstallDir:   "/tmp/plugins/test1",
		Enabled:      true,
		ManifestJSON: `{"id":"test-plugin-1"}`,
		InstalledAt:  time.Now().Add(-time.Hour),
	}
	p2 := storage.PluginRecord{
		ID:           "test-plugin-2",
		Name:         "Test Plugin 2",
		Version:      "0.1.0",
		InstallDir:   "/tmp/plugins/test2",
		Enabled:      false,
		ManifestJSON: `{"id":"test-plugin-2"}`,
		InstalledAt:  time.Now(),
	}

	if err := mem.SavePlugin(p1); err != nil {
		t.Fatalf("SavePlugin p1 failed: %v", err)
	}
	if err := mem.SavePlugin(p2); err != nil {
		t.Fatalf("SavePlugin p2 failed: %v", err)
	}

	got, err := mem.GetPlugin("test-plugin-1")
	if err != nil {
		t.Fatalf("GetPlugin failed: %v", err)
	}
	if got.Name != "Test Plugin 1" || !got.Enabled {
		t.Fatalf("unexpected plugin got: %+v", got)
	}

	list, err := mem.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(list))
	}
	if list[0].ID != "test-plugin-1" || list[1].ID != "test-plugin-2" {
		t.Fatalf("expected sorted by installed_at ASC, got %s then %s", list[0].ID, list[1].ID)
	}

	if err := mem.SetPluginEnabled("test-plugin-2", true); err != nil {
		t.Fatalf("SetPluginEnabled failed: %v", err)
	}
	p2Got, err := mem.GetPlugin("test-plugin-2")
	if err != nil || !p2Got.Enabled {
		t.Fatalf("expected p2 to be enabled: %+v", p2Got)
	}

	if err := mem.DeletePlugin("test-plugin-1"); err != nil {
		t.Fatalf("DeletePlugin failed: %v", err)
	}
	if _, err := mem.GetPlugin("test-plugin-1"); err == nil {
		t.Fatalf("expected plugin 1 to be deleted")
	}
}
