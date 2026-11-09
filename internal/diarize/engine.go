package diarize

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// Engine orchestrates speaker diarization, segment alignment, and temporal smoothing.
type Engine struct {
	diarizer Diarizer
}

// NewEngine constructs a diarization engine with the given Diarizer implementation.
func NewEngine(d Diarizer) *Engine {
	return &Engine{
		diarizer: d,
	}
}

// Diarize delegates to the underlying diarizer and returns smoothed segments.
func (e *Engine) Diarize(ctx context.Context, audioPath string, opts Options) ([]Segment, error) {
	if e.diarizer == nil {
		return nil, errors.New("diarizer is not configured")
	}

	segs, err := e.diarizer.Diarize(ctx, audioPath, opts)
	if err != nil {
		return nil, err
	}

	return e.MergeContiguous(segs, 500), nil
}

// MergeContiguous consolidates consecutive segments from the same speaker if the gap between them <= maxGapMs.
func (e *Engine) MergeContiguous(segments []Segment, maxGapMs int64) []Segment {
	if len(segments) <= 1 {
		return segments
	}

	var merged []Segment
	current := segments[0]

	for i := 1; i < len(segments); i++ {
		next := segments[i]
		if next.Speaker == current.Speaker && (next.StartMs-current.EndMs) <= maxGapMs {
			// Extend current segment
			if next.EndMs > current.EndMs {
				current.EndMs = next.EndMs
			}
			if next.Confidence > current.Confidence {
				current.Confidence = next.Confidence
			}
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	return merged
}

// UniqueSpeakers returns a sorted list of unique speaker identifiers from a set of segments.
func (e *Engine) UniqueSpeakers(segments []Segment) []string {
	seen := make(map[string]bool)
	var list []string
	for _, s := range segments {
		if s.Speaker != "" && !seen[s.Speaker] {
			seen[s.Speaker] = true
			list = append(list, s.Speaker)
		}
	}
	sort.Strings(list)
	return list
}

// AlignMeetingSegments maps diarized speaker intervals to transcript segments by maximum temporal overlap.
func (e *Engine) AlignMeetingSegments(meetingSegments []storage.MeetingSegment, diarized []Segment) []storage.MeetingSegment {
	if len(meetingSegments) == 0 {
		return meetingSegments
	}

	if len(diarized) == 0 {
		// Default to SPEAKER_00 if no diarization output available
		out := make([]storage.MeetingSegment, len(meetingSegments))
		for i, s := range meetingSegments {
			out[i] = s
			if out[i].Speaker == "" {
				out[i].Speaker = "SPEAKER_00"
			}
		}
		return out
	}

	out := make([]storage.MeetingSegment, len(meetingSegments))

	for i, seg := range meetingSegments {
		out[i] = seg

		// Calculate overlap with each diarized segment
		speakerOverlap := make(map[string]int64)
		for _, d := range diarized {
			overlapStart := max(seg.StartMs, d.StartMs)
			overlapEnd := min(seg.EndMs, d.EndMs)
			if overlapEnd > overlapStart {
				speakerOverlap[d.Speaker] += (overlapEnd - overlapStart)
			}
		}

		bestSpeaker := ""
		var maxOverlap int64
		for spk, overlap := range speakerOverlap {
			if overlap > maxOverlap {
				maxOverlap = overlap
				bestSpeaker = spk
			}
		}

		if bestSpeaker != "" {
			out[i].Speaker = bestSpeaker
		} else {
			// Fallback: pick the diarized segment closest in time
			var minDistance int64 = math.MaxInt64
			closestSpeaker := "SPEAKER_00"

			for _, d := range diarized {
				dist := abs(seg.StartMs - d.StartMs)
				if dist < minDistance {
					minDistance = dist
					closestSpeaker = d.Speaker
				}
			}
			out[i].Speaker = closestSpeaker
		}
	}

	return out
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
