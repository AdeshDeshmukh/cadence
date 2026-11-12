package impexp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AdeshDeshmukh/cadence/internal/meeting"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// MeetingBundle encapsulates full meeting data for export or import.
type MeetingBundle struct {
	Meeting  storage.Meeting          `json:"meeting"`
	Speakers []storage.MeetingSpeaker `json:"speakers,omitempty"`
	Segments []storage.MeetingSegment `json:"segments,omitempty"`
	Turns    []meeting.Turn           `json:"turns,omitempty"`
	Summary  *storage.MeetingSummary  `json:"summary,omitempty"`
}

// BuildSpeakerMap maps raw speaker IDs to custom display names.
func BuildSpeakerMap(speakers []storage.MeetingSpeaker) map[string]string {
	m := make(map[string]string, len(speakers))
	for _, s := range speakers {
		if s.Name != "" {
			m[s.SpeakerID] = s.Name
		}
	}
	return m
}

// ResolveSpeaker returns the display name of a speaker if registered, or the raw speaker string.
func ResolveSpeaker(speaker string, speakerMap map[string]string) string {
	if speaker == "" {
		return "Speaker"
	}
	if name, ok := speakerMap[speaker]; ok && name != "" {
		return name
	}
	return speaker
}

// ExportMeeting serializes meeting details, transcript, and summary into the specified format.
func ExportMeeting(bundle MeetingBundle, format Format) ([]byte, error) {
	switch format {
	case FormatMarkdown:
		return exportMeetingMarkdown(bundle)
	case FormatSRT:
		return exportMeetingSRT(bundle)
	case FormatVTT:
		return exportMeetingVTT(bundle)
	case FormatJSON:
		return exportMeetingJSON(bundle)
	case FormatTXT:
		return exportMeetingTXT(bundle)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

func exportMeetingMarkdown(bundle MeetingBundle) ([]byte, error) {
	var b bytes.Buffer
	spkMap := BuildSpeakerMap(bundle.Speakers)

	title := bundle.Meeting.Title
	if title == "" {
		title = "Untitled Meeting"
	}
	dateStr := bundle.Meeting.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	durStr := FormatDuration(bundle.Meeting.DurationMs)

	b.WriteString(fmt.Sprintf("# %s\n\n", title))
	b.WriteString(fmt.Sprintf("> **Date:** %s | **Duration:** %s | **ID:** `%s`\n\n", dateStr, durStr, bundle.Meeting.ID))

	// Speakers Section
	if len(bundle.Speakers) > 0 {
		b.WriteString("## Participants\n")
		for _, spk := range bundle.Speakers {
			name := spk.Name
			if name == "" {
				name = spk.SpeakerID
			}
			b.WriteString(fmt.Sprintf("- **%s** (`%s`)\n", name, spk.SpeakerID))
		}
		b.WriteString("\n")
	}

	// Executive Summary Section
	if bundle.Summary != nil {
		if strings.TrimSpace(bundle.Summary.Overview) != "" {
			b.WriteString("## Summary\n")
			b.WriteString(strings.TrimSpace(bundle.Summary.Overview) + "\n\n")
		}

		if len(bundle.Summary.Topics) > 0 {
			b.WriteString("### Topics Discussed\n")
			for _, top := range bundle.Summary.Topics {
				b.WriteString(fmt.Sprintf("- %s\n", top))
			}
			b.WriteString("\n")
		}

		if len(bundle.Summary.ActionItems) > 0 {
			b.WriteString("### Action Items\n")
			for _, item := range bundle.Summary.ActionItems {
				check := " "
				if item.Completed {
					check = "x"
				}
				line := fmt.Sprintf("- [%s] %s", check, item.Description)
				if item.Assignee != "" {
					line += fmt.Sprintf(" (@%s)", item.Assignee)
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("\n")
		}

		if len(bundle.Summary.Decisions) > 0 {
			b.WriteString("### Decisions Made\n")
			for _, dec := range bundle.Summary.Decisions {
				b.WriteString(fmt.Sprintf("- %s\n", dec))
			}
			b.WriteString("\n")
		}
	}

	// Transcript Section
	b.WriteString("## Transcript\n\n")

	// If turns are provided or segments exist, render dialogue turns
	turns := bundle.Turns
	if len(turns) == 0 && len(bundle.Segments) > 0 {
		turns = meeting.AggregateTurns(bundle.Segments)
	}

	if len(turns) == 0 {
		b.WriteString("*No transcript recorded.*\n")
	} else {
		for _, t := range turns {
			spkName := ResolveSpeaker(t.Speaker, spkMap)
			timeStr := FormatSRTTime(t.StartMs)
			// Truncate milliseconds for cleaner Markdown timestamps: HH:MM:SS
			if idx := strings.Index(timeStr, ","); idx != -1 {
				timeStr = timeStr[:idx]
			}
			b.WriteString(fmt.Sprintf("**[%s] %s:**\n%s\n\n", timeStr, spkName, strings.TrimSpace(t.Text)))
		}
	}

	return b.Bytes(), nil
}

func exportMeetingSRT(bundle MeetingBundle) ([]byte, error) {
	var b bytes.Buffer
	spkMap := BuildSpeakerMap(bundle.Speakers)

	// In subtitles, export segment by segment with timestamps
	idx := 1
	for _, seg := range bundle.Segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		spkName := ResolveSpeaker(seg.Speaker, spkMap)
		startTime := FormatSRTTime(seg.StartMs)
		endTime := FormatSRTTime(seg.EndMs)

		b.WriteString(fmt.Sprintf("%d\n", idx))
		b.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))
		if spkName != "" {
			b.WriteString(fmt.Sprintf("%s: %s\n\n", spkName, text))
		} else {
			b.WriteString(fmt.Sprintf("%s\n\n", text))
		}
		idx++
	}

	return b.Bytes(), nil
}

func exportMeetingVTT(bundle MeetingBundle) ([]byte, error) {
	var b bytes.Buffer
	spkMap := BuildSpeakerMap(bundle.Speakers)

	b.WriteString("WEBVTT\n\n")

	for _, seg := range bundle.Segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		spkName := ResolveSpeaker(seg.Speaker, spkMap)
		startTime := FormatVTTTime(seg.StartMs)
		endTime := FormatVTTTime(seg.EndMs)

		b.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))
		if spkName != "" {
			b.WriteString(fmt.Sprintf("<v %s>%s</v>\n\n", spkName, text))
		} else {
			b.WriteString(fmt.Sprintf("%s\n\n", text))
		}
	}

	return b.Bytes(), nil
}

func exportMeetingJSON(bundle MeetingBundle) ([]byte, error) {
	if len(bundle.Turns) == 0 && len(bundle.Segments) > 0 {
		bundle.Turns = meeting.AggregateTurns(bundle.Segments)
	}
	return json.MarshalIndent(bundle, "", "  ")
}

func exportMeetingTXT(bundle MeetingBundle) ([]byte, error) {
	var b bytes.Buffer
	spkMap := BuildSpeakerMap(bundle.Speakers)

	turns := bundle.Turns
	if len(turns) == 0 && len(bundle.Segments) > 0 {
		turns = meeting.AggregateTurns(bundle.Segments)
	}

	for _, t := range turns {
		spkName := ResolveSpeaker(t.Speaker, spkMap)
		timeStr := FormatSRTTime(t.StartMs)
		if idx := strings.Index(timeStr, ","); idx != -1 {
			timeStr = timeStr[:idx]
		}
		b.WriteString(fmt.Sprintf("[%s] %s: %s\n", timeStr, spkName, strings.TrimSpace(t.Text)))
	}

	return b.Bytes(), nil
}

// ExportTranscript serializes a dictation transcript into the requested format.
func ExportTranscript(t storage.Transcript, format Format) ([]byte, error) {
	switch format {
	case FormatMarkdown:
		var b bytes.Buffer
		title := fmt.Sprintf("# Transcript: %s", t.ID)
		b.WriteString(title + "\n\n")
		b.WriteString(fmt.Sprintf("> **Date:** %s | **Mode:** `%s` | **App:** `%s`\n\n",
			t.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), t.Mode, t.Application))
		b.WriteString("## Text\n\n" + t.Text + "\n")
		if t.RawText != "" && t.RawText != t.Text {
			b.WriteString("\n## Raw Speech\n\n" + t.RawText + "\n")
		}
		return b.Bytes(), nil

	case FormatSRT:
		var b bytes.Buffer
		b.WriteString("1\n00:00:00,000 --> 00:00:05,000\n" + t.Text + "\n\n")
		return b.Bytes(), nil

	case FormatVTT:
		var b bytes.Buffer
		b.WriteString("WEBVTT\n\n00:00:00.000 --> 00:00:05.000\n" + t.Text + "\n\n")
		return b.Bytes(), nil

	case FormatJSON:
		return json.MarshalIndent(t, "", "  ")

	case FormatTXT:
		return []byte(t.Text + "\n"), nil

	default:
		return nil, fmt.Errorf("unsupported transcript format: %s", format)
	}
}

// ExportNote serializes a knowledge note into the requested format.
func ExportNote(n storage.Note, format Format) ([]byte, error) {
	switch format {
	case FormatMarkdown:
		var b bytes.Buffer
		b.WriteString(fmt.Sprintf("# %s\n\n", n.Title))
		if len(n.Tags) > 0 {
			b.WriteString(fmt.Sprintf("> **Tags:** %s\n\n", strings.Join(n.Tags, ", ")))
		}
		b.WriteString(n.Content + "\n")
		return b.Bytes(), nil

	case FormatJSON:
		return json.MarshalIndent(n, "", "  ")

	case FormatTXT:
		var b bytes.Buffer
		b.WriteString(n.Title + "\n\n" + n.Content + "\n")
		return b.Bytes(), nil

	default:
		return nil, fmt.Errorf("unsupported note format: %s", format)
	}
}
