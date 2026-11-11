package meeting

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// Summarizer defines the interface for generating structured summaries from meeting records.
type Summarizer interface {
	Summarize(ctx context.Context, mtg storage.Meeting, segments []storage.MeetingSegment) (*storage.MeetingSummary, error)
}

// ExtractiveSummarizer provides offline, heuristic rule-based extraction of
// meeting overviews, key decisions, and action items with speaker attribution.
type ExtractiveSummarizer struct{}

// NewExtractiveSummarizer creates a rule-based summarizer requiring no external model.
func NewExtractiveSummarizer() *ExtractiveSummarizer {
	return &ExtractiveSummarizer{}
}

var (
	decisionRegex = regexp.MustCompile(`(?i)\b(?:we agreed|we decided|agreed that|decided that|decided to|consensus is|the decision is|decision:|agreed to|let's go with|we will go with|we have approved|approved)\b[:\s]*(.*)`)
	actionRegex   = regexp.MustCompile(`(?i)\b(?:action item|action items|to-do|todo|follow-up|follow up|will handle|will take care of|will work on|will create|will implement|will send|will review|will update|assigned to|let's make sure to|please make sure to|make sure to)\b[:\s]*(.*)`)
	assigneeRegex = regexp.MustCompile(`(?i)(?:assigned to|assign to|@)\s*([A-Za-z0-9_\-]+)`)
	willRegex     = regexp.MustCompile(`(?i)\b([A-Z][a-z]+)\s+will\b`)
	sentenceRegex = regexp.MustCompile(`[^.!?]+[.!?]?`)
)

func (e *ExtractiveSummarizer) Summarize(_ context.Context, mtg storage.Meeting, segments []storage.MeetingSegment) (*storage.MeetingSummary, error) {
	now := time.Now().UTC()
	summary := &storage.MeetingSummary{
		MeetingID:   mtg.ID,
		Decisions:   []string{},
		ActionItems: []storage.ActionItem{},
		Topics:      []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if len(segments) == 0 {
		title := mtg.Title
		if title == "" {
			title = mtg.ID
		}
		summary.Overview = fmt.Sprintf("Meeting %q had no recorded transcripts.", title)
		return summary, nil
	}

	turns := AggregateTurns(segments)
	uniqueSpeakers := make(map[string]struct{})
	topicSet := make(map[string]struct{})
	actionIDCounter := 1

	if mtg.Title != "" {
		topicSet[mtg.Title] = struct{}{}
	}

	for _, turn := range turns {
		if turn.Speaker != "" {
			uniqueSpeakers[turn.Speaker] = struct{}{}
		}

		sentences := sentenceRegex.FindAllString(turn.Text, -1)
		for _, rawSentence := range sentences {
			sentence := strings.TrimSpace(rawSentence)
			if sentence == "" {
				continue
			}

			// Check for decision
			if decisionRegex.MatchString(sentence) {
				clean := cleanSentence(sentence)
				if clean != "" && !containsString(summary.Decisions, clean) {
					summary.Decisions = append(summary.Decisions, clean)
				}
			}

			// Check for action item
			if actionRegex.MatchString(sentence) {
				clean := cleanSentence(sentence)
				if clean != "" {
					assignee := turn.Speaker
					if match := assigneeRegex.FindStringSubmatch(sentence); len(match) > 1 {
						assignee = match[1]
					} else if match := willRegex.FindStringSubmatch(sentence); len(match) > 1 {
						assignee = match[1]
					}

					item := storage.ActionItem{
						ID:          fmt.Sprintf("act-%d", actionIDCounter),
						Description: clean,
						Assignee:    assignee,
						Completed:   false,
					}
					actionIDCounter++
					summary.ActionItems = append(summary.ActionItems, item)
				}
			}

			// Extract potential topics (mentions of "regarding", "discuss", "topic:")
			if topic := extractTopic(sentence); topic != "" {
				topicSet[topic] = struct{}{}
			}
		}
	}

	for t := range topicSet {
		summary.Topics = append(summary.Topics, t)
	}

	// Synthesize overview paragraph
	title := mtg.Title
	if title == "" {
		title = "Cadence Meeting"
	}
	speakerCount := len(uniqueSpeakers)
	turnCount := len(turns)
	var topicsDesc string
	if len(summary.Topics) > 0 {
		topicsDesc = fmt.Sprintf(" Key topics covered include %s.", strings.Join(summary.Topics, ", "))
	}
	summary.Overview = fmt.Sprintf("%s concluded with %d participant(s) over %d spoken turn(s).%s Recorded %d key decision(s) and %d action item(s).",
		title, speakerCount, turnCount, topicsDesc, len(summary.Decisions), len(summary.ActionItems))

	return summary, nil
}

func cleanSentence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "- ")
	s = strings.TrimPrefix(s, "* ")
	s = strings.Trim(s, `"'`)
	return s
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}

var topicMarkerRegex = regexp.MustCompile(`(?i)\b(?:regarding|discussing|topic|agenda|focus on)\s+([A-Za-z0-9_\- ]{3,30})`)

func extractTopic(sentence string) string {
	m := topicMarkerRegex.FindStringSubmatch(sentence)
	if len(m) > 1 {
		t := strings.TrimSpace(m[1])
		t = strings.TrimRight(t, ".,;:")
		if len(t) >= 3 && !strings.EqualFold(t, "the") && !strings.EqualFold(t, "this") {
			return capitalize(t)
		}
	}
	return ""
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// LLMSummarizer leverages an LLM polisher to perform structured extraction,
// with automatic fallback to ExtractiveSummarizer.
type LLMSummarizer struct {
	polisher   polish.Polisher
	extractive *ExtractiveSummarizer
}

// NewLLMSummarizer returns a summarizer that uses an LLM polisher with extractive fallback.
func NewLLMSummarizer(polisher polish.Polisher) *LLMSummarizer {
	return &LLMSummarizer{
		polisher:   polisher,
		extractive: NewExtractiveSummarizer(),
	}
}

type llmSummaryPayload struct {
	Overview    string   `json:"overview"`
	Decisions   []string `json:"decisions"`
	ActionItems []struct {
		Description string `json:"description"`
		Assignee    string `json:"assignee"`
	} `json:"action_items"`
	Topics []string `json:"topics"`
}

func (l *LLMSummarizer) Summarize(ctx context.Context, mtg storage.Meeting, segments []storage.MeetingSegment) (*storage.MeetingSummary, error) {
	if l.polisher == nil || len(segments) == 0 {
		return l.extractive.Summarize(ctx, mtg, segments)
	}

	turns := AggregateTurns(segments)
	var sb strings.Builder
	for _, t := range turns {
		sb.WriteString(fmt.Sprintf("%s [%s]: %s\n", t.Speaker, FormatTurnTimestamp(t.StartMs, t.EndMs), t.Text))
	}

	systemPrompt := `You are an AI meeting assistant. Analyze the meeting transcript turns and output a structured JSON summary.
Your output MUST be strictly valid JSON matching this schema:
{
  "overview": "A comprehensive 2-4 sentence summary of the meeting",
  "decisions": ["string"],
  "action_items": [{"description": "string", "assignee": "string"}],
  "topics": ["string"]
}
Do not include any prose outside the JSON.`

	userPrompt := fmt.Sprintf("Meeting Title: %s\n\nTranscript:\n%s", mtg.Title, sb.String())

	fullText := fmt.Sprintf("%s\n\n%s", systemPrompt, userPrompt)
	req := polish.Request{
		Text: fullText,
	}

	resp, err := l.polisher.Polish(ctx, req)
	if err != nil || strings.TrimSpace(resp.Text) == "" {
		// Fallback to extractive
		return l.extractive.Summarize(ctx, mtg, segments)
	}

	rawJSON := resp.Text
	// Strip markdown code fences if LLM wrapped output
	if idx := strings.Index(rawJSON, "```json"); idx != -1 {
		rawJSON = rawJSON[idx+7:]
		if end := strings.Index(rawJSON, "```"); end != -1 {
			rawJSON = rawJSON[:end]
		}
	} else if idx := strings.Index(rawJSON, "```"); idx != -1 {
		rawJSON = rawJSON[idx+3:]
		if end := strings.Index(rawJSON, "```"); end != -1 {
			rawJSON = rawJSON[:end]
		}
	}
	rawJSON = strings.TrimSpace(rawJSON)

	var payload llmSummaryPayload
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		// Parsing failed, fallback to extractive
		return l.extractive.Summarize(ctx, mtg, segments)
	}

	now := time.Now().UTC()
	summary := &storage.MeetingSummary{
		MeetingID:   mtg.ID,
		Overview:    strings.TrimSpace(payload.Overview),
		Decisions:   payload.Decisions,
		ActionItems: make([]storage.ActionItem, 0, len(payload.ActionItems)),
		Topics:      payload.Topics,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if summary.Decisions == nil {
		summary.Decisions = []string{}
	}
	if summary.Topics == nil {
		summary.Topics = []string{}
	}

	for i, item := range payload.ActionItems {
		summary.ActionItems = append(summary.ActionItems, storage.ActionItem{
			ID:          fmt.Sprintf("act-%d", i+1),
			Description: strings.TrimSpace(item.Description),
			Assignee:    strings.TrimSpace(item.Assignee),
			Completed:   false,
		})
	}

	if summary.Overview == "" {
		ext, _ := l.extractive.Summarize(ctx, mtg, segments)
		summary.Overview = ext.Overview
	}

	return summary, nil
}
