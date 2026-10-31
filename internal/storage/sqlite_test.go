package storage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSQLite_Memory(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer s.Close()

	testStoreContracts(t, s)
}

func TestSQLite_FilePersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "test_cadence.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}

	tr := Transcript{
		ID:          "sess-001",
		RawText:     "hello world raw",
		Text:        "Hello world.",
		Language:    "en",
		Mode:        "dictation",
		Application: "Notes",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := s1.SaveTranscript(tr); err != nil {
		t.Fatalf("SaveTranscript failed: %v", err)
	}

	if err := s1.SetVocabulary(VocabularyEntry{Spoken: "grpc", Canonical: "gRPC"}); err != nil {
		t.Fatalf("SetVocabulary failed: %v", err)
	}

	if err := s1.Close(); err != nil {
		t.Fatalf("close s1: %v", err)
	}

	// Reopen same database file to test persistence
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer s2.Close()

	list, err := s2.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts on reopened DB failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 transcript, got %d", len(list))
	}
	if list[0].ID != tr.ID || list[0].Text != tr.Text {
		t.Errorf("expected %+v, got %+v", tr, list[0])
	}

	vocab, err := s2.Vocabulary()
	if err != nil {
		t.Fatalf("Vocabulary on reopened DB failed: %v", err)
	}
	if len(vocab) != 1 || vocab[0].Spoken != "grpc" || vocab[0].Canonical != "gRPC" {
		t.Errorf("unexpected vocabulary: %+v", vocab)
	}
}

func TestSQLite_SearchAndLookup(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	t1 := Transcript{
		ID:          "id-1",
		RawText:     "the quick brown fox",
		Text:        "The quick brown fox.",
		Language:    "en",
		Mode:        "dictation",
		Application: "Browser",
		CreatedAt:   now.Add(-2 * time.Minute),
	}
	t2 := Transcript{
		ID:          "id-2",
		RawText:     "jumps over lazy dog",
		Text:        "Jumps over lazy dog.",
		Language:    "en",
		Mode:        "dictation",
		Application: "Slack",
		CreatedAt:   now.Add(-1 * time.Minute),
	}

	if err := s.SaveTranscript(t1); err != nil {
		t.Fatalf("SaveTranscript t1 failed: %v", err)
	}
	if err := s.SaveTranscript(t2); err != nil {
		t.Fatalf("SaveTranscript t2 failed: %v", err)
	}

	found, err := s.GetTranscript("id-1")
	if err != nil || found == nil {
		t.Fatalf("GetTranscript(id-1) failed: %v, found: %+v", err, found)
	}
	if found.Text != t1.Text {
		t.Errorf("expected %q, got %q", t1.Text, found.Text)
	}

	none, err := s.GetTranscript("non-existent")
	if err != nil || none != nil {
		t.Errorf("expected nil for non-existent transcript, got %v, %v", none, err)
	}

	res, err := s.SearchTranscripts("lazy", 10)
	if err != nil {
		t.Fatalf("SearchTranscripts failed: %v", err)
	}
	if len(res) != 1 || res[0].ID != "id-2" {
		t.Fatalf("expected 1 result with id-2, got: %+v", res)
	}

	if err := s.DeleteTranscript("id-1"); err != nil {
		t.Fatalf("DeleteTranscript failed: %v", err)
	}
	resAfterDel, err := s.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(resAfterDel) != 1 || resAfterDel[0].ID != "id-2" {
		t.Errorf("unexpected transcripts after delete: %+v", resAfterDel)
	}
}

func TestSQLite_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "concurrent.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	var wg sync.WaitGroup
	workers := 10
	itemsPerWorker := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerWorker; j++ {
				tr := Transcript{
					ID:        time.Now().Format("150405.000000000") + string(rune('a'+workerID)) + string(rune('0'+j%10)),
					RawText:   "concurrent text",
					Text:      "Concurrent text.",
					Language:  "en",
					CreatedAt: time.Now().UTC(),
				}
				_ = s.SaveTranscript(tr)
				_ = s.SetVocabulary(VocabularyEntry{
					Spoken:    tr.ID,
					Canonical: "canonical",
				})
			}
		}(i)
	}

	wg.Wait()

	list, err := s.ListTranscripts(1000)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(list) != workers*itemsPerWorker {
		t.Fatalf("expected %d transcripts, got %d", workers*itemsPerWorker, len(list))
	}
}

func TestSQLite_ConcurrentMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "concurrent_migrate.db")

	var wg sync.WaitGroup
	instances := 8
	errs := make(chan error, instances)

	for i := 0; i < instances; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(dbPath)
			if err != nil {
				errs <- err
				return
			}
			_ = s.Close()
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Open failed: %v", err)
		}
	}
}

func testStoreContracts(t *testing.T, s Store) {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Second)
	t1 := Transcript{
		ID:        "sess-1",
		RawText:   "apple",
		Text:      "Apple",
		Language:  "en",
		CreatedAt: now.Add(-10 * time.Second),
	}
	t2 := Transcript{
		ID:        "sess-2",
		RawText:   "banana",
		Text:      "Banana",
		Language:  "en",
		CreatedAt: now,
	}

	if err := s.SaveTranscript(t1); err != nil {
		t.Fatalf("save t1: %v", err)
	}
	if err := s.SaveTranscript(t2); err != nil {
		t.Fatalf("save t2: %v", err)
	}

	// Should list newest first
	list, err := s.ListTranscripts(10)
	if err != nil {
		t.Fatalf("list transcripts: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 transcripts, got %d", len(list))
	}
	if list[0].ID != "sess-2" || list[1].ID != "sess-1" {
		t.Errorf("expected newest first order, got %s then %s", list[0].ID, list[1].ID)
	}

	// Limit test
	limited, err := s.ListTranscripts(1)
	if err != nil {
		t.Fatalf("list transcripts limit 1: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "sess-2" {
		t.Errorf("expected only newest transcript, got: %+v", limited)
	}

	// Vocabulary tests
	if err := s.SetVocabulary(VocabularyEntry{Spoken: "cadence", Canonical: "Cadence"}); err != nil {
		t.Fatalf("set vocab 1: %v", err)
	}
	if err := s.SetVocabulary(VocabularyEntry{Spoken: "ai", Canonical: "AI"}); err != nil {
		t.Fatalf("set vocab 2: %v", err)
	}

	vocab, err := s.Vocabulary()
	if err != nil {
		t.Fatalf("get vocab: %v", err)
	}
	if len(vocab) != 2 {
		t.Fatalf("expected 2 vocab entries, got %d", len(vocab))
	}
	// Alphabetical order: "ai" then "cadence"
	if vocab[0].Spoken != "ai" || vocab[1].Spoken != "cadence" {
		t.Errorf("expected alphabetical vocab order, got: %+v", vocab)
	}
}

func TestSQLite_SessionRecovery(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initial query should return nil
	last, err := s.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery failed: %v", err)
	}
	if last != nil {
		t.Fatalf("expected nil recovery, got: %+v", last)
	}

	now := time.Now().UTC()

	// Save uncommitted session with partial text
	r1 := SessionRecovery{
		ID:           "sess-rec-1",
		State:        "cancelled",
		PartialText:  "streaming partial text here",
		RawText:      "streaming partial text here raw",
		Text:         "Streaming partial text here.",
		AudioSamples: 16000,
		CreatedAt:    now.Add(-2 * time.Minute),
		UpdatedAt:    now.Add(-2 * time.Minute),
	}
	if err := s.SaveRecovery(r1); err != nil {
		t.Fatalf("SaveRecovery r1: %v", err)
	}

	// Save newer failed session
	r2 := SessionRecovery{
		ID:           "sess-rec-2",
		State:        "failed",
		PartialText:  "interrupted dictation",
		RawText:      "",
		Text:         "",
		AudioSamples: 8000,
		Error:        "asr worker timeout",
		CreatedAt:    now.Add(-1 * time.Minute),
		UpdatedAt:    now.Add(-1 * time.Minute),
	}
	if err := s.SaveRecovery(r2); err != nil {
		t.Fatalf("SaveRecovery r2: %v", err)
	}

	// GetLastRecovery should return newest (r2)
	last, err = s.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery: %v", err)
	}
	if last == nil || last.ID != "sess-rec-2" {
		t.Fatalf("expected r2 as last recovery, got: %+v", last)
	}
	if last.BestText() != "interrupted dictation" {
		t.Errorf("expected BestText to be %q, got %q", "interrupted dictation", last.BestText())
	}

	// List recoveries
	list, err := s.ListRecoveries(10)
	if err != nil {
		t.Fatalf("ListRecoveries: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 recoveries, got %d", len(list))
	}
	if list[0].ID != "sess-rec-2" || list[1].ID != "sess-rec-1" {
		t.Errorf("expected newest first order, got: %+v", list)
	}

	// Save completed session for r2 (should no longer appear in GetLastRecovery or ListRecoveries)
	r2.State = "completed"
	r2.UpdatedAt = now
	if err := s.SaveRecovery(r2); err != nil {
		t.Fatalf("SaveRecovery update r2: %v", err)
	}

	lastAfterComp, err := s.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery after completed: %v", err)
	}
	if lastAfterComp == nil || lastAfterComp.ID != "sess-rec-1" {
		t.Fatalf("expected r1 to be returned since r2 is completed, got: %+v", lastAfterComp)
	}

	// Delete recovery r1
	if err := s.DeleteRecovery("sess-rec-1"); err != nil {
		t.Fatalf("DeleteRecovery: %v", err)
	}
	lastAfterDel, err := s.GetLastRecovery()
	if err != nil {
		t.Fatalf("GetLastRecovery after delete: %v", err)
	}
	if lastAfterDel != nil {
		t.Fatalf("expected nil after delete, got: %+v", lastAfterDel)
	}
}
