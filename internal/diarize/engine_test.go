package diarize

import (
	"context"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestEngine_MergeContiguous(t *testing.T) {
	eng := NewEngine(NewMockDiarizer())

	// 1. Empty and single segment
	if len(eng.MergeContiguous(nil, 500)) != 0 {
		t.Fatal("expected empty merge result for nil input")
	}
	single := []Segment{{StartMs: 0, EndMs: 1000, Speaker: "SPEAKER_00", Confidence: 0.9}}
	mergedSingle := eng.MergeContiguous(single, 500)
	if len(mergedSingle) != 1 || mergedSingle[0].EndMs != 1000 {
		t.Fatalf("unexpected single merge result: %+v", mergedSingle)
	}

	// 2. Multiple adjacent segments from same speaker with gap <= 500ms
	input := []Segment{
		{StartMs: 0, EndMs: 4000, Speaker: "SPEAKER_00", Confidence: 0.8},
		{StartMs: 4200, EndMs: 8000, Speaker: "SPEAKER_00", Confidence: 0.95},
		{StartMs: 8500, EndMs: 12000, Speaker: "SPEAKER_01", Confidence: 0.85},
		{StartMs: 12100, EndMs: 15000, Speaker: "SPEAKER_01", Confidence: 0.9},
	}

	res := eng.MergeContiguous(input, 500)
	if len(res) != 2 {
		t.Fatalf("expected 2 merged segments, got %d: %+v", len(res), res)
	}

	if res[0].Speaker != "SPEAKER_00" || res[0].StartMs != 0 || res[0].EndMs != 8000 {
		t.Fatalf("unexpected segment 0: %+v", res[0])
	}
	if res[1].Speaker != "SPEAKER_01" || res[1].StartMs != 8500 || res[1].EndMs != 15000 {
		t.Fatalf("unexpected segment 1: %+v", res[1])
	}
}

func TestEngine_UniqueSpeakers(t *testing.T) {
	eng := NewEngine(NewMockDiarizer())
	input := []Segment{
		{Speaker: "SPEAKER_02"},
		{Speaker: "SPEAKER_00"},
		{Speaker: "SPEAKER_01"},
		{Speaker: "SPEAKER_00"},
	}

	speakers := eng.UniqueSpeakers(input)
	if len(speakers) != 3 {
		t.Fatalf("expected 3 unique speakers, got %d", len(speakers))
	}
	if speakers[0] != "SPEAKER_00" || speakers[1] != "SPEAKER_01" || speakers[2] != "SPEAKER_02" {
		t.Fatalf("unexpected sorted speaker order: %v", speakers)
	}
}

func TestEngine_AlignMeetingSegments(t *testing.T) {
	eng := NewEngine(NewMockDiarizer())

	meetingSegs := []storage.MeetingSegment{
		{
			ID:           "seg-1",
			MeetingID:    "mtg-1",
			SegmentIndex: 0,
			StartMs:      0,
			EndMs:        15000,
			Text:         "Good morning everyone.",
			CreatedAt:    time.Now(),
		},
		{
			ID:           "seg-2",
			MeetingID:    "mtg-1",
			SegmentIndex: 1,
			StartMs:      15000,
			EndMs:        30000,
			Text:         "Yes, let's start the review.",
			CreatedAt:    time.Now(),
		},
	}

	diarized := []Segment{
		{StartMs: 0, EndMs: 14000, Speaker: "SPEAKER_ALICE", Confidence: 0.9},
		{StartMs: 15500, EndMs: 29000, Speaker: "SPEAKER_BOB", Confidence: 0.9},
	}

	aligned := eng.AlignMeetingSegments(meetingSegs, diarized)
	if len(aligned) != 2 {
		t.Fatalf("expected 2 aligned segments, got %d", len(aligned))
	}

	if aligned[0].Speaker != "SPEAKER_ALICE" {
		t.Fatalf("expected SPEAKER_ALICE, got %s", aligned[0].Speaker)
	}
	if aligned[1].Speaker != "SPEAKER_BOB" {
		t.Fatalf("expected SPEAKER_BOB, got %s", aligned[1].Speaker)
	}

	// Fallback when diarized is empty
	alignedEmpty := eng.AlignMeetingSegments(meetingSegs, nil)
	if alignedEmpty[0].Speaker != "SPEAKER_00" {
		t.Fatalf("expected default SPEAKER_00 on empty diarization, got %s", alignedEmpty[0].Speaker)
	}
}

func TestMockDiarizer(t *testing.T) {
	ctx := context.Background()
	mock := NewMockDiarizer()

	segs, err := mock.Diarize(ctx, "/path/to/meeting.wav", Options{NumSpeakers: 3})
	if err != nil {
		t.Fatalf("mock diarize failed: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	if segs[0].Speaker != "SPEAKER_00" || segs[1].Speaker != "SPEAKER_01" || segs[2].Speaker != "SPEAKER_02" {
		t.Fatalf("unexpected mock speakers: %+v", segs)
	}
}
