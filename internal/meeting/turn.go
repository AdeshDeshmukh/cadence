package meeting

import (
	"fmt"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// Turn represents a contiguous spoken dialogue turn by a single speaker.
type Turn struct {
	Speaker    string   `json:"speaker"`
	StartMs    int64    `json:"start_ms"`
	EndMs      int64    `json:"end_ms"`
	Text       string   `json:"text"`
	SegmentIDs []string `json:"segment_ids"`
}

// AggregateTurns consolidates contiguous meeting segments from the same speaker into distinct conversational turns.
func AggregateTurns(segments []storage.MeetingSegment) []Turn {
	if len(segments) == 0 {
		return []Turn{}
	}

	var turns []Turn
	var current *Turn

	for _, seg := range segments {
		speaker := seg.Speaker
		if speaker == "" {
			speaker = "SPEAKER_00"
		}
		text := strings.TrimSpace(seg.Text)

		if current == nil {
			current = &Turn{
				Speaker:    speaker,
				StartMs:    seg.StartMs,
				EndMs:      seg.EndMs,
				Text:       text,
				SegmentIDs: []string{seg.ID},
			}
		} else if current.Speaker == speaker {
			// Extend existing turn
			if seg.EndMs > current.EndMs {
				current.EndMs = seg.EndMs
			}
			if text != "" {
				if current.Text != "" {
					current.Text += " " + text
				} else {
					current.Text = text
				}
			}
			current.SegmentIDs = append(current.SegmentIDs, seg.ID)
		} else {
			// Speaker changed: flush current turn and start new turn
			turns = append(turns, *current)
			current = &Turn{
				Speaker:    speaker,
				StartMs:    seg.StartMs,
				EndMs:      seg.EndMs,
				Text:       text,
				SegmentIDs: []string{seg.ID},
			}
		}
	}

	if current != nil {
		turns = append(turns, *current)
	}

	return turns
}

// FormatTurnTimestamp formats milliseconds into [MM:SS - MM:SS] timestamp banner.
func FormatTurnTimestamp(startMs, endMs int64) string {
	start := time.Duration(startMs) * time.Millisecond
	end := time.Duration(endMs) * time.Millisecond
	return fmt.Sprintf("[%02d:%02d - %02d:%02d]",
		int(start.Minutes()), int(start.Seconds())%60,
		int(end.Minutes()), int(end.Seconds())%60,
	)
}

// FormatTurns produces human-readable dialogue transcript lines with timestamps and speaker labels.
func FormatTurns(turns []Turn) string {
	var b strings.Builder
	for i, turn := range turns {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("%s %s: %s",
			FormatTurnTimestamp(turn.StartMs, turn.EndMs),
			turn.Speaker,
			turn.Text,
		))
	}
	return b.String()
}
