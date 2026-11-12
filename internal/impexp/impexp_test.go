package impexp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/impexp"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestFormats_TimeAndNormalization(t *testing.T) {
	// Normalization
	for _, tc := range []struct {
		input    string
		expected impexp.Format
	}{
		{"markdown", impexp.FormatMarkdown},
		{"MD", impexp.FormatMarkdown},
		{".md", impexp.FormatMarkdown},
		{"srt", impexp.FormatSRT},
		{"vtt", impexp.FormatVTT},
		{"webvtt", impexp.FormatVTT},
		{"json", impexp.FormatJSON},
		{"txt", impexp.FormatTXT},
		{"text", impexp.FormatTXT},
	} {
		f, err := impexp.NormalizeFormat(tc.input)
		if err != nil || f != tc.expected {
			t.Fatalf("NormalizeFormat(%q) = %v, err %v; want %v", tc.input, f, err, tc.expected)
		}
	}

	if _, err := impexp.NormalizeFormat("invalid_xyz"); err == nil {
		t.Fatalf("expected error for invalid format")
	}

	// SRT & VTT time formatting
	ms := int64(1*3600*1000 + 23*60*1000 + 45*1000 + 678) // 01:23:45,678
	if srt := impexp.FormatSRTTime(ms); srt != "01:23:45,678" {
		t.Fatalf("expected 01:23:45,678, got %s", srt)
	}
	if vtt := impexp.FormatVTTTime(ms); vtt != "01:23:45.678" {
		t.Fatalf("expected 01:23:45.678, got %s", vtt)
	}

	// Parsing timestamp
	parsed, err := impexp.ParseTimestamp("01:23:45,678")
	if err != nil || parsed != ms {
		t.Fatalf("ParseTimestamp failed: %v, got: %d, want: %d", err, parsed, ms)
	}

	parsedVTT, err := impexp.ParseTimestamp("01:23:45.678")
	if err != nil || parsedVTT != ms {
		t.Fatalf("ParseTimestamp VTT failed: %v, got: %d", err, parsedVTT)
	}

	shortVTT, err := impexp.ParseTimestamp("02:15.500")
	if err != nil || shortVTT != (2*60+15)*1000+500 {
		t.Fatalf("ParseTimestamp short VTT failed: %v, got: %d", err, shortVTT)
	}

	// Duration formatting
	if dur := impexp.FormatDuration(ms); dur != "1h 23m 45s" {
		t.Fatalf("FormatDuration got: %s", dur)
	}
	if dur := impexp.FormatDuration(45000); dur != "45s" {
		t.Fatalf("FormatDuration short got: %s", dur)
	}
}

func sampleBundle() impexp.MeetingBundle {
	now := time.Now().UTC()
	return impexp.MeetingBundle{
		Meeting: storage.Meeting{
			ID:         "mtg-exp-1",
			Title:      "Sprint Planning",
			DurationMs: 90000,
			CreatedAt:  now,
		},
		Speakers: []storage.MeetingSpeaker{
			{MeetingID: "mtg-exp-1", SpeakerID: "SPEAKER_00", Name: "Alice", UpdatedAt: now},
			{MeetingID: "mtg-exp-1", SpeakerID: "SPEAKER_01", Name: "Bob", UpdatedAt: now},
		},
		Segments: []storage.MeetingSegment{
			{
				ID:           "s-1",
				MeetingID:    "mtg-exp-1",
				SegmentIndex: 0,
				StartMs:      1000,
				EndMs:        4000,
				Speaker:      "SPEAKER_00",
				Text:         "Welcome everyone to sprint planning.",
			},
			{
				ID:           "s-2",
				MeetingID:    "mtg-exp-1",
				SegmentIndex: 1,
				StartMs:      5000,
				EndMs:        9000,
				Speaker:      "SPEAKER_01",
				Text:         "Thanks Alice. We have three main epics to review.",
			},
		},
		Summary: &storage.MeetingSummary{
			MeetingID: "mtg-exp-1",
			Overview:  "Team aligned on sprint goals.",
			Topics:    []string{"Reviewed backlog epics", "Allocated story points"},
			ActionItems: []storage.ActionItem{
				{ID: "act-1", Description: "Deploy staging environment", Assignee: "Bob"},
			},
			Decisions: []string{"Adopt Go for daemon rewrite"},
		},
	}
}

func TestExport_Meeting(t *testing.T) {
	bundle := sampleBundle()

	// 1. Markdown
	mdBytes, err := impexp.ExportMeeting(bundle, impexp.FormatMarkdown)
	if err != nil {
		t.Fatalf("ExportMeeting Markdown failed: %v", err)
	}
	mdStr := string(mdBytes)
	for _, expected := range []string{
		"# Sprint Planning",
		"Alice",
		"Bob",
		"Team aligned on sprint goals.",
		"Deploy staging environment",
		"Adopt Go for daemon rewrite",
		"Welcome everyone to sprint planning.",
	} {
		if !strings.Contains(mdStr, expected) {
			t.Fatalf("Markdown export missing %q:\n%s", expected, mdStr)
		}
	}

	// 2. SRT
	srtBytes, err := impexp.ExportMeeting(bundle, impexp.FormatSRT)
	if err != nil {
		t.Fatalf("ExportMeeting SRT failed: %v", err)
	}
	srtStr := string(srtBytes)
	if !strings.Contains(srtStr, "00:00:01,000 --> 00:00:04,000") ||
		!strings.Contains(srtStr, "Alice: Welcome everyone to sprint planning.") ||
		!strings.Contains(srtStr, "00:00:05,000 --> 00:00:09,000") ||
		!strings.Contains(srtStr, "Bob: Thanks Alice.") {
		t.Fatalf("SRT export content unexpected:\n%s", srtStr)
	}

	// 3. WebVTT
	vttBytes, err := impexp.ExportMeeting(bundle, impexp.FormatVTT)
	if err != nil {
		t.Fatalf("ExportMeeting VTT failed: %v", err)
	}
	vttStr := string(vttBytes)
	if !strings.Contains(vttStr, "WEBVTT") ||
		!strings.Contains(vttStr, "00:00:01.000 --> 00:00:04.000") ||
		!strings.Contains(vttStr, "<v Alice>Welcome everyone to sprint planning.</v>") {
		t.Fatalf("VTT export content unexpected:\n%s", vttStr)
	}

	// 4. JSON
	jsonBytes, err := impexp.ExportMeeting(bundle, impexp.FormatJSON)
	if err != nil {
		t.Fatalf("ExportMeeting JSON failed: %v", err)
	}
	parsed, err := impexp.ParseJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		t.Fatalf("ParseJSON failed on exported JSON: %v", err)
	}
	if parsed.Meeting.ID != "mtg-exp-1" || len(parsed.Segments) != 2 {
		t.Fatalf("roundtrip JSON mismatch: %+v", parsed)
	}

	// 5. TXT
	txtBytes, err := impexp.ExportMeeting(bundle, impexp.FormatTXT)
	if err != nil {
		t.Fatalf("ExportMeeting TXT failed: %v", err)
	}
	txtStr := string(txtBytes)
	if !strings.Contains(txtStr, "Alice: Welcome everyone") || !strings.Contains(txtStr, "Bob: Thanks Alice") {
		t.Fatalf("TXT export mismatch: %s", txtStr)
	}
}

func TestExport_TranscriptAndNote(t *testing.T) {
	tr := storage.Transcript{
		ID:          "tr-001",
		Text:        "This is a transcribed sentence.",
		RawText:     "this is a transcribed sentence",
		Mode:        "dictation",
		Application: "Notes",
		CreatedAt:   time.Now(),
	}

	md, err := impexp.ExportTranscript(tr, impexp.FormatMarkdown)
	if err != nil || !strings.Contains(string(md), "This is a transcribed sentence.") {
		t.Fatalf("ExportTranscript Markdown failed: %v, got %s", err, string(md))
	}

	srt, err := impexp.ExportTranscript(tr, impexp.FormatSRT)
	if err != nil || !strings.Contains(string(srt), "-->") {
		t.Fatalf("ExportTranscript SRT failed: %v", err)
	}

	vtt, err := impexp.ExportTranscript(tr, impexp.FormatVTT)
	if err != nil || !strings.Contains(string(vtt), "WEBVTT") {
		t.Fatalf("ExportTranscript VTT failed: %v", err)
	}

	note := storage.Note{
		ID:        "n-001",
		Title:     "Architecture Specs",
		Content:   "Microservices layout and gRPC communication.",
		Tags:      []string{"arch", "spec"},
		CreatedAt: time.Now(),
	}
	noteMd, err := impexp.ExportNote(note, impexp.FormatMarkdown)
	if err != nil || !strings.Contains(string(noteMd), "# Architecture Specs") {
		t.Fatalf("ExportNote failed: %v", err)
	}
}

func TestImport_Subtitles(t *testing.T) {
	srtData := `1
00:00:02,500 --> 00:00:06,000
Alice: Good afternoon team.

2
00:00:07,000 --> 00:00:11,500
Bob: Hi Alice, let's get started.
`
	segments, err := impexp.ParseSRT(strings.NewReader(srtData))
	if err != nil {
		t.Fatalf("ParseSRT failed: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments from SRT, got %d", len(segments))
	}
	if segments[0].Speaker != "Alice" || segments[0].Text != "Good afternoon team." || segments[0].StartMs != 2500 || segments[0].EndMs != 6000 {
		t.Fatalf("segment 0 mismatch: %+v", segments[0])
	}
	if segments[1].Speaker != "Bob" || segments[1].Text != "Hi Alice, let's get started." || segments[1].StartMs != 7000 || segments[1].EndMs != 11500 {
		t.Fatalf("segment 1 mismatch: %+v", segments[1])
	}

	vttData := `WEBVTT

00:00:01.000 --> 00:00:04.000
<v Charlie>Testing WebVTT speaker tags.</v>

00:00:05.000 --> 00:00:08.500
David: And prefix speaker format too.
`
	vttSegments, err := impexp.ParseWebVTT(strings.NewReader(vttData))
	if err != nil {
		t.Fatalf("ParseWebVTT failed: %v", err)
	}
	if len(vttSegments) != 2 {
		t.Fatalf("expected 2 segments from VTT, got %d", len(vttSegments))
	}
	if vttSegments[0].Speaker != "Charlie" || vttSegments[0].Text != "Testing WebVTT speaker tags." {
		t.Fatalf("vtt segment 0 mismatch: %+v", vttSegments[0])
	}
	if vttSegments[1].Speaker != "David" || vttSegments[1].Text != "And prefix speaker format too." {
		t.Fatalf("vtt segment 1 mismatch: %+v", vttSegments[1])
	}
}

func TestImportEngine_SubtitlesAndStore(t *testing.T) {
	store := storage.NewMemory()
	engine := impexp.NewImportEngine(store, nil, nil)

	srtData := `1
00:00:01,000 --> 00:00:05,000
Alice: Reviewing architecture designs.
`
	bundle, err := engine.ImportSubtitles(context.Background(), "Arch Review", impexp.FormatSRT, strings.NewReader(srtData))
	if err != nil {
		t.Fatalf("ImportSubtitles failed: %v", err)
	}
	if bundle.Meeting.Title != "Arch Review" {
		t.Fatalf("expected title Arch Review, got %s", bundle.Meeting.Title)
	}

	// Verify in store
	mtg, err := store.GetMeeting(bundle.Meeting.ID)
	if err != nil || mtg.Title != "Arch Review" {
		t.Fatalf("store.GetMeeting failed: %v", err)
	}
	segs, err := store.ListMeetingSegments(bundle.Meeting.ID)
	if err != nil || len(segs) != 1 {
		t.Fatalf("expected 1 segment in store, got %d (err: %v)", len(segs), err)
	}
	if segs[0].Speaker != "Alice" || segs[0].Text != "Reviewing architecture designs." {
		t.Fatalf("unexpected segment: %+v", segs[0])
	}
}

func TestImportEngine_Media(t *testing.T) {
	tempDir := t.TempDir()
	store := storage.NewMemory()
	mockASR := asr.MockEngine{Text: "Imported transcribed audio"}

	// Mock extractor creating a valid dummy WAV
	mockExtractor := impexp.NewSmartExtractor(func(ctx context.Context, inputPath, outputPath string) error {
		// Write dummy WAV header
		f, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = f.Write([]byte("RIFF____WAVEfmt "))
		return nil
	})

	engine := impexp.NewImportEngine(store, mockASR, mockExtractor)

	// Create dummy media file
	dummyVideo := filepath.Join(tempDir, "interview.mp4")
	_ = os.WriteFile(dummyVideo, []byte("fake mp4 video bytes"), 0644)

	bundle, err := engine.ImportMedia(context.Background(), dummyVideo, "Product Interview", tempDir, true)
	if err != nil {
		t.Fatalf("ImportMedia failed: %v", err)
	}

	if bundle.Meeting.Title != "Product Interview" {
		t.Fatalf("expected title Product Interview, got %s", bundle.Meeting.Title)
	}
	if len(bundle.Segments) != 1 || bundle.Segments[0].Text != "Imported transcribed audio" {
		t.Fatalf("expected transcribed segment, got: %+v", bundle.Segments)
	}
}
