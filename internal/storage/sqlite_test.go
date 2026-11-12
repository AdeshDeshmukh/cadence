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

func TestSQLite_Snippets(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initial list should be empty
	list, err := s.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 snippets, got %d", len(list))
	}

	// Add snippets
	s1 := Snippet{
		Trigger:     "my email",
		Template:    "contact@example.com",
		Description: "Primary email address",
	}
	s2 := Snippet{
		Trigger:     "standup report",
		Template:    "Standup {date}:\n- Done: {clipboard}\n- Next: ...",
		Description: "Daily standup template",
	}

	if err := s.SetSnippet(s1); err != nil {
		t.Fatalf("SetSnippet s1: %v", err)
	}
	if err := s.SetSnippet(s2); err != nil {
		t.Fatalf("SetSnippet s2: %v", err)
	}

	// Get single snippet
	got, err := s.GetSnippet("my email")
	if err != nil || got == nil {
		t.Fatalf("GetSnippet(my email): %v, got: %+v", err, got)
	}
	if got.Template != s1.Template {
		t.Errorf("expected template %q, got %q", s1.Template, got.Template)
	}

	// List snippets (should be alphabetical)
	list, err = s.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 snippets, got %d", len(list))
	}
	if list[0].Trigger != "my email" || list[1].Trigger != "standup report" {
		t.Errorf("unexpected list ordering: %+v", list)
	}

	// Delete snippet
	if err := s.DeleteSnippet("my email"); err != nil {
		t.Fatalf("DeleteSnippet: %v", err)
	}
	delCheck, err := s.GetSnippet("my email")
	if err != nil || delCheck != nil {
		t.Fatalf("expected nil after delete, got %+v, err: %v", delCheck, err)
	}
}

func TestSQLite_PolishProfiles(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initial list empty
	list, err := s.ListPolishProfiles()
	if err != nil {
		t.Fatalf("ListPolishProfiles failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(list))
	}

	p1 := PolishProfile{
		Name:         "custom_code",
		Description:  "Special coding profile",
		SystemPrompt: "Write idiomatic Go code.",
		Temperature:  0.1,
		CustomRules:  []string{"No semicolons", "Use camelCase"},
	}
	p2 := PolishProfile{
		Name:         "academic_review",
		Description:  "Peer review format",
		SystemPrompt: "Use formal academic tone.",
		Temperature:  0.3,
		CustomRules:  []string{"Include APA citation style"},
	}

	if err := s.SetPolishProfile(p1); err != nil {
		t.Fatalf("SetPolishProfile p1: %v", err)
	}
	if err := s.SetPolishProfile(p2); err != nil {
		t.Fatalf("SetPolishProfile p2: %v", err)
	}

	// Get single profile
	got, err := s.GetPolishProfile("custom_code")
	if err != nil || got == nil {
		t.Fatalf("GetPolishProfile failed: %v, got: %+v", err, got)
	}
	if got.SystemPrompt != p1.SystemPrompt || len(got.CustomRules) != 2 || got.CustomRules[0] != "No semicolons" {
		t.Fatalf("unexpected profile data: %+v", got)
	}

	// List ordered by name
	list, err = s.ListPolishProfiles()
	if err != nil {
		t.Fatalf("ListPolishProfiles failed: %v", err)
	}
	if len(list) != 2 || list[0].Name != "academic_review" || list[1].Name != "custom_code" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Update existing profile
	p1.Description = "Updated description"
	if err := s.SetPolishProfile(p1); err != nil {
		t.Fatalf("SetPolishProfile update: %v", err)
	}
	gotUpdated, err := s.GetPolishProfile("custom_code")
	if err != nil || gotUpdated == nil || gotUpdated.Description != "Updated description" {
		t.Fatalf("expected updated description, got: %+v", gotUpdated)
	}

	// Delete profile
	if err := s.DeletePolishProfile("academic_review"); err != nil {
		t.Fatalf("DeletePolishProfile: %v", err)
	}
	delCheck, err := s.GetPolishProfile("academic_review")
	if err != nil || delCheck != nil {
		t.Fatalf("expected nil after delete, got: %+v", delCheck)
	}
}

func TestSQLite_Meetings(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Initial list empty
	list, err := s.ListMeetings(10)
	if err != nil {
		t.Fatalf("ListMeetings: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 meetings initially, got %d", len(list))
	}

	// 2. Save meeting
	m1 := Meeting{
		ID:         "mtg-1",
		Title:      "Q4 Architecture Sync",
		State:      "recording",
		AudioPath:  "/tmp/mtg-1.wav",
		SampleRate: 16000,
		CreatedAt:  now,
		DurationMs: 0,
	}
	if err := s.SaveMeeting(m1); err != nil {
		t.Fatalf("SaveMeeting: %v", err)
	}

	// 3. Get meeting
	got, err := s.GetMeeting("mtg-1")
	if err != nil || got == nil {
		t.Fatalf("GetMeeting failed: %v, got: %+v", err, got)
	}
	if got.Title != "Q4 Architecture Sync" || got.State != "recording" {
		t.Fatalf("unexpected meeting data: %+v", got)
	}

	// 4. Save segments
	seg1 := MeetingSegment{
		ID:           "seg-1",
		MeetingID:    "mtg-1",
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        15000,
		Text:         "Good morning team, let's start the sync.",
		Speaker:      "Speaker 1",
		CreatedAt:    now,
	}
	seg2 := MeetingSegment{
		ID:           "seg-2",
		MeetingID:    "mtg-1",
		SegmentIndex: 1,
		StartMs:      15000,
		EndMs:        30000,
		Text:         "We are discussing the new audio pipeline today.",
		Speaker:      "Speaker 2",
		CreatedAt:    now.Add(15 * time.Second),
	}
	if err := s.SaveMeetingSegment(seg1); err != nil {
		t.Fatalf("SaveMeetingSegment 1: %v", err)
	}
	if err := s.SaveMeetingSegment(seg2); err != nil {
		t.Fatalf("SaveMeetingSegment 2: %v", err)
	}

	// 5. List segments
	segs, err := s.ListMeetingSegments("mtg-1")
	if err != nil || len(segs) != 2 {
		t.Fatalf("ListMeetingSegments failed: %v, len: %d", err, len(segs))
	}
	if segs[0].SegmentIndex != 0 || segs[1].SegmentIndex != 1 {
		t.Fatalf("unexpected segment order: %+v", segs)
	}

	// 6. Update meeting to stopped with ended_at and duration
	endTime := now.Add(30 * time.Second)
	m1.State = "stopped"
	m1.EndedAt = &endTime
	m1.DurationMs = 30000
	if err := s.SaveMeeting(m1); err != nil {
		t.Fatalf("Update meeting: %v", err)
	}
	updated, err := s.GetMeeting("mtg-1")
	if err != nil || updated == nil || updated.State != "stopped" || updated.DurationMs != 30000 {
		t.Fatalf("expected updated meeting state, got: %+v", updated)
	}

	// 7. Delete meeting
	if err := s.DeleteMeeting("mtg-1"); err != nil {
		t.Fatalf("DeleteMeeting: %v", err)
	}
	delMtg, err := s.GetMeeting("mtg-1")
	if err != nil || delMtg != nil {
		t.Fatalf("expected nil after deletion, got: %+v", delMtg)
	}
	delSegs, err := s.ListMeetingSegments("mtg-1")
	if err != nil || len(delSegs) != 0 {
		t.Fatalf("expected 0 segments after meeting deletion, got: %d", len(delSegs))
	}
}

func TestSQLite_MeetingSpeakers(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)

	// 1. Create meeting
	m := Meeting{
		ID:        "mtg-speakers-test",
		Title:     "Daily Standup",
		State:     "stopped",
		CreatedAt: now,
	}
	if err := s.SaveMeeting(m); err != nil {
		t.Fatalf("SaveMeeting: %v", err)
	}

	// 2. Add segments with SPEAKER_00 and SPEAKER_01
	seg1 := MeetingSegment{
		ID:           "seg-spk-1",
		MeetingID:    m.ID,
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        5000,
		Text:         "Good morning everyone.",
		Speaker:      "SPEAKER_00",
		CreatedAt:    now,
	}
	seg2 := MeetingSegment{
		ID:           "seg-spk-2",
		MeetingID:    m.ID,
		SegmentIndex: 1,
		StartMs:      5000,
		EndMs:        10000,
		Text:         "Good morning! I have updates on backend.",
		Speaker:      "SPEAKER_01",
		CreatedAt:    now.Add(5 * time.Second),
	}
	_ = s.SaveMeetingSegment(seg1)
	_ = s.SaveMeetingSegment(seg2)

	// 3. SetMeetingSpeaker
	spk := MeetingSpeaker{
		MeetingID: m.ID,
		SpeakerID: "SPEAKER_00",
		Name:      "Alice",
		UpdatedAt: now,
	}
	if err := s.SetMeetingSpeaker(spk); err != nil {
		t.Fatalf("SetMeetingSpeaker failed: %v", err)
	}

	// 4. ListMeetingSpeakers
	speakers, err := s.ListMeetingSpeakers(m.ID)
	if err != nil {
		t.Fatalf("ListMeetingSpeakers failed: %v", err)
	}
	if len(speakers) != 1 || speakers[0].Name != "Alice" {
		t.Fatalf("unexpected speakers list: %+v", speakers)
	}

	// 5. RenameMeetingSpeaker (SPEAKER_00 -> "Alice Smith")
	if err := s.RenameMeetingSpeaker(m.ID, "SPEAKER_00", "Alice Smith"); err != nil {
		t.Fatalf("RenameMeetingSpeaker failed: %v", err)
	}

	// Verify segment 1 speaker was renamed to "Alice Smith"
	segs, err := s.ListMeetingSegments(m.ID)
	if err != nil {
		t.Fatalf("ListMeetingSegments: %v", err)
	}
	if segs[0].Speaker != "Alice Smith" {
		t.Fatalf("expected seg1 speaker to be 'Alice Smith', got %q", segs[0].Speaker)
	}
	if segs[1].Speaker != "SPEAKER_01" {
		t.Fatalf("expected seg2 speaker to remain 'SPEAKER_01', got %q", segs[1].Speaker)
	}

	// 6. Cascading deletion when meeting is deleted
	if err := s.DeleteMeeting(m.ID); err != nil {
		t.Fatalf("DeleteMeeting failed: %v", err)
	}
	speakersAfterDel, err := s.ListMeetingSpeakers(m.ID)
	if err != nil {
		t.Fatalf("ListMeetingSpeakers after delete: %v", err)
	}
	if len(speakersAfterDel) != 0 {
		t.Fatalf("expected 0 speakers after meeting delete, got %d", len(speakersAfterDel))
	}
}

func TestSQLite_MeetingSummaries(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)
	m := Meeting{
		ID:        "mtg-sum-1",
		Title:     "Sprint Planning",
		State:     "stopped",
		CreatedAt: now,
	}
	if err := s.SaveMeeting(m); err != nil {
		t.Fatalf("SaveMeeting failed: %v", err)
	}

	// 1. Initial Get should return nil
	sum, err := s.GetMeetingSummary(m.ID)
	if err != nil {
		t.Fatalf("GetMeetingSummary initial error: %v", err)
	}
	if sum != nil {
		t.Fatalf("expected nil summary, got %+v", sum)
	}

	// 2. SaveMeetingSummary
	orig := MeetingSummary{
		MeetingID: m.ID,
		Overview:  "Discussed Q3 deliverables and agreed on architecture.",
		Decisions: []string{"Use SQLite for storage", "Support continuous mode"},
		ActionItems: []ActionItem{
			{ID: "act-1", Description: "Implement summarizer", Assignee: "Alice", Completed: false},
			{ID: "act-2", Description: "Add CLI flags", Assignee: "Bob", Completed: true},
		},
		Topics:    []string{"Storage", "CLI", "Architecture"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveMeetingSummary(orig); err != nil {
		t.Fatalf("SaveMeetingSummary failed: %v", err)
	}

	// 3. Retrieve and verify
	got, err := s.GetMeetingSummary(m.ID)
	if err != nil {
		t.Fatalf("GetMeetingSummary failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil summary")
	}
	if got.Overview != orig.Overview {
		t.Errorf("expected overview %q, got %q", orig.Overview, got.Overview)
	}
	if len(got.Decisions) != 2 || got.Decisions[0] != "Use SQLite for storage" {
		t.Errorf("unexpected decisions: %+v", got.Decisions)
	}
	if len(got.ActionItems) != 2 || got.ActionItems[0].Assignee != "Alice" || !got.ActionItems[1].Completed {
		t.Errorf("unexpected action items: %+v", got.ActionItems)
	}
	if len(got.Topics) != 3 || got.Topics[1] != "CLI" {
		t.Errorf("unexpected topics: %+v", got.Topics)
	}

	// 4. Update summary
	got.Overview = "Updated summary after review."
	got.Decisions = append(got.Decisions, "Deploy to staging first")
	if err := s.SaveMeetingSummary(*got); err != nil {
		t.Fatalf("SaveMeetingSummary update failed: %v", err)
	}
	updated, err := s.GetMeetingSummary(m.ID)
	if err != nil {
		t.Fatalf("GetMeetingSummary after update: %v", err)
	}
	if len(updated.Decisions) != 3 {
		t.Errorf("expected 3 decisions, got %d", len(updated.Decisions))
	}
	if updated.Overview != "Updated summary after review." {
		t.Errorf("expected updated overview, got %q", updated.Overview)
	}

	// 5. DeleteMeetingSummary
	if err := s.DeleteMeetingSummary(m.ID); err != nil {
		t.Fatalf("DeleteMeetingSummary failed: %v", err)
	}
	afterDel, err := s.GetMeetingSummary(m.ID)
	if err != nil {
		t.Fatalf("GetMeetingSummary after delete: %v", err)
	}
	if afterDel != nil {
		t.Fatalf("expected nil after delete, got %+v", afterDel)
	}

	// 6. Test cascading delete with meeting
	if err := s.SaveMeetingSummary(orig); err != nil {
		t.Fatalf("SaveMeetingSummary re-save failed: %v", err)
	}
	if err := s.DeleteMeeting(m.ID); err != nil {
		t.Fatalf("DeleteMeeting failed: %v", err)
	}
	cascaded, err := s.GetMeetingSummary(m.ID)
	if err != nil {
		t.Fatalf("GetMeetingSummary after DeleteMeeting: %v", err)
	}
	if cascaded != nil {
		t.Fatalf("expected nil summary after DeleteMeeting cascade, got %+v", cascaded)
	}
}

func TestSQLite_KnowledgeBase(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Folders CRUD
	f1 := Folder{
		ID:        "f-work",
		Name:      "Work Projects",
		CreatedAt: now,
		UpdatedAt: now,
	}
	f2 := Folder{
		ID:        "f-personal",
		Name:      "Personal Ideas",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveFolder(f1); err != nil {
		t.Fatalf("SaveFolder f1: %v", err)
	}
	if err := s.SaveFolder(f2); err != nil {
		t.Fatalf("SaveFolder f2: %v", err)
	}

	gotF1, err := s.GetFolder("f-work")
	if err != nil || gotF1 == nil || gotF1.Name != "Work Projects" {
		t.Fatalf("GetFolder failed: %v, got %+v", err, gotF1)
	}

	folders, err := s.ListFolders()
	if err != nil || len(folders) != 2 {
		t.Fatalf("ListFolders failed: %v, got %+v", err, folders)
	}

	// 2. Notes CRUD & Tagging
	n1 := Note{
		ID:        "note-1",
		FolderID:  "f-work",
		Title:     "Architecture Review",
		Content:   "Drafting the v2 architecture specifications.",
		Tags:      []string{"golang", "architecture"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	n2 := Note{
		ID:        "note-2",
		FolderID:  "f-personal",
		Title:     "Grocery List",
		Content:   "- Apples\n- Milk",
		Tags:      []string{"errands"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveNote(n1); err != nil {
		t.Fatalf("SaveNote n1: %v", err)
	}
	if err := s.SaveNote(n2); err != nil {
		t.Fatalf("SaveNote n2: %v", err)
	}

	gotN1, err := s.GetNote("note-1")
	if err != nil || gotN1 == nil {
		t.Fatalf("GetNote n1: %v", err)
	}
	if gotN1.Title != "Architecture Review" || len(gotN1.Tags) != 2 {
		t.Errorf("unexpected note 1: %+v", gotN1)
	}

	// 3. List notes with filters
	workNotes, err := s.ListNotes(NoteFilter{FolderID: "f-work"})
	if err != nil || len(workNotes) != 1 || workNotes[0].ID != "note-1" {
		t.Fatalf("ListNotes by folder failed: %v, got %+v", err, workNotes)
	}

	taggedNotes, err := s.ListNotes(NoteFilter{Tag: "golang"})
	if err != nil || len(taggedNotes) != 1 || taggedNotes[0].ID != "note-1" {
		t.Fatalf("ListNotes by tag failed: %v, got %+v", err, taggedNotes)
	}

	// 4. Tags inspection & counts
	tags, err := s.ListTags()
	if err != nil || len(tags) != 3 {
		t.Fatalf("ListTags failed: %v, got %+v", err, tags)
	}
	for _, tag := range tags {
		if tag.Name == "golang" && tag.NoteCount != 1 {
			t.Errorf("expected golang note count 1, got %d", tag.NoteCount)
		}
	}

	// 5. Update note tags
	if err := s.SetNoteTags("note-1", []string{"golang", "distributed-systems"}); err != nil {
		t.Fatalf("SetNoteTags: %v", err)
	}
	updatedN1, err := s.GetNote("note-1")
	if err != nil || len(updatedN1.Tags) != 2 || updatedN1.Tags[0] != "distributed-systems" {
		t.Fatalf("unexpected updated tags: %+v", updatedN1.Tags)
	}

	// 6. Delete folder detaches note
	if err := s.DeleteFolder("f-work"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	detachedN1, err := s.GetNote("note-1")
	if err != nil || detachedN1.FolderID != "" {
		t.Fatalf("expected folder_id to be cleared after folder deletion, got %q", detachedN1.FolderID)
	}

	// 7. Delete note
	if err := s.DeleteNote("note-2"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	delN2, err := s.GetNote("note-2")
	if err != nil || delN2 != nil {
		t.Fatalf("expected nil note-2, got %+v", delN2)
	}

	// 8. Delete tag
	if err := s.DeleteTag("distributed-systems"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	n1AfterTagDel, err := s.GetNote("note-1")
	if err != nil || len(n1AfterTagDel.Tags) != 1 || n1AfterTagDel.Tags[0] != "golang" {
		t.Fatalf("expected only golang tag remaining, got %+v", n1AfterTagDel.Tags)
	}
}
