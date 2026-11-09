package meeting

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestWAVWriter(t *testing.T) {
	tmpDir := t.TempDir()
	wavPath := filepath.Join(tmpDir, "test.wav")

	writer, err := NewWAVWriter(wavPath, 16000)
	if err != nil {
		t.Fatalf("NewWAVWriter failed: %v", err)
	}

	// 1 second of 16kHz tone/silence
	samples := make([]float32, 16000)
	for i := range samples {
		samples[i] = 0.5
	}

	if err := writer.WriteSamples(samples); err != nil {
		t.Fatalf("WriteSamples failed: %v", err)
	}

	if writer.NumSamples() != 16000 {
		t.Fatalf("expected 16000 samples, got %d", writer.NumSamples())
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Check file size: 44 bytes header + 32000 bytes PCM = 32044 bytes
	info, err := os.Stat(wavPath)
	if err != nil {
		t.Fatalf("stat wav file: %v", err)
	}
	if info.Size() != 32044 {
		t.Fatalf("expected 32044 bytes, got %d", info.Size())
	}
}

func TestChunker(t *testing.T) {
	var received []Segment

	handler := func(seg Segment, samples []float32) {
		received = append(received, seg)
		if len(samples) != 32000 { // 2s * 16000
			t.Errorf("expected 32000 samples per chunk, got %d", len(samples))
		}
	}

	// 2s segments with 500ms overlap at 16kHz
	opts := ChunkerOptions{
		SegmentDuration: 2 * time.Second,
		OverlapDuration: 500 * time.Millisecond,
		SampleRate:      16000,
	}

	chunker := NewChunker("test-mtg", nil, handler, opts)

	// Ingest 5 seconds of audio (5 * 16000 = 80000 samples) in small chunks
	stream := make([]float32, 800) // 50ms chunk
	for i := range stream {
		stream[i] = 0.1
	}

	for i := 0; i < 100; i++ {
		_ = chunker.Ingest(stream)
	}

	chunker.Flush()

	if len(received) < 2 {
		t.Fatalf("expected at least 2 segments, got %d", len(received))
	}
	if received[0].SegmentIndex != 0 || received[1].SegmentIndex != 1 {
		t.Fatalf("unexpected segment indices: %+v", received)
	}
	if received[0].StartMs != 0 {
		t.Fatalf("expected first segment StartMs 0, got %d", received[0].StartMs)
	}
}

func TestMeetingManager_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	memStore := storage.NewMemory()

	mgr := NewManager(memStore, nil,
		WithAudioDirectory(tmpDir),
		WithSegmentDuration(2*time.Second),
		WithSampleRate(16000),
		WithASREngine(asr.MockEngine{Text: "Transcribed meeting discussion."}),
	)

	ctx := context.Background()

	// 1. Initial status is idle
	st := mgr.Status()
	if st.IsRecording || st.IsPaused || st.ActiveSession != nil {
		t.Fatalf("expected idle status, got %+v", st)
	}

	// 2. Start meeting
	sess, err := mgr.Start(ctx, "Sprint Planning")
	if err != nil {
		t.Fatalf("Start meeting failed: %v", err)
	}
	if sess.Title != "Sprint Planning" || sess.State != StateRecording {
		t.Fatalf("unexpected session state: %+v", sess)
	}

	// 3. Double start must fail
	if _, err := mgr.Start(ctx, "Another Meeting"); err == nil {
		t.Fatal("expected error on starting second meeting while one is active")
	}

	// 4. Ingest 2.5s of audio to trigger segment emission
	samples := make([]float32, 40000)
	for i := range samples {
		samples[i] = 0.2
	}
	if err := mgr.IngestSamples(samples); err != nil {
		t.Fatalf("IngestSamples failed: %v", err)
	}

	// Check status reflects recording
	st = mgr.Status()
	if !st.IsRecording || st.IsPaused || st.ActiveSession == nil {
		t.Fatalf("expected active recording status, got %+v", st)
	}

	// 5. Pause meeting
	if err := mgr.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	st = mgr.Status()
	if !st.IsPaused || st.IsRecording {
		t.Fatalf("expected paused status, got %+v", st)
	}

	// Pause again must fail
	if err := mgr.Pause(); err == nil {
		t.Fatal("expected error on pausing already paused meeting")
	}

	// 6. Resume meeting
	time.Sleep(50 * time.Millisecond)
	if err := mgr.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	st = mgr.Status()
	if st.IsPaused || !st.IsRecording {
		t.Fatalf("expected recording status after resume, got %+v", st)
	}

	// 7. Stop meeting
	stopped, err := mgr.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if stopped.State != StateStopped || stopped.EndTime == nil {
		t.Fatalf("expected stopped session, got %+v", stopped)
	}

	// 8. Verify persistence in storage
	list, err := mgr.List(10)
	if err != nil {
		t.Fatalf("List meetings failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != sess.ID {
		t.Fatalf("expected 1 meeting in list, got %+v", list)
	}

	mtg, segs, err := mgr.Get(sess.ID)
	if err != nil || mtg == nil {
		t.Fatalf("Get meeting failed: %v", err)
	}
	if len(segs) == 0 {
		t.Fatalf("expected at least 1 segment persisted, got %d", len(segs))
	}
	if segs[0].Text != "Transcribed meeting discussion." {
		t.Fatalf("expected mocked ASR transcription, got %q", segs[0].Text)
	}

	// 9. Delete meeting
	if err := mgr.Delete(sess.ID); err != nil {
		t.Fatalf("Delete meeting failed: %v", err)
	}
	afterDel, _, _ := mgr.Get(sess.ID)
	if afterDel != nil {
		t.Fatalf("expected nil after delete, got %+v", afterDel)
	}
}
