package postprocess_test

import (
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestSnippets_ExpandVariables(t *testing.T) {
	fixedTime := time.Date(2026, 11, 1, 14, 30, 45, 0, time.UTC)
	tmpl := "Date: {date}, Time: {time}, DateTime: {datetime}, Year: {year}, Month: {month}, Day: {day}, Clip: {clipboard}"
	clipboard := "copied token"

	got := postprocess.ExpandVariables(tmpl, clipboard, fixedTime)
	expected := "Date: 2026-11-01, Time: 14:30:45, DateTime: 2026-11-01 14:30:45, Year: 2026, Month: 11, Day: 01, Clip: copied token"

	if got != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func TestSnippets_Apply_ExactMatch(t *testing.T) {
	step := postprocess.NewSnippets()
	fixedTime := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	step.SetNowFn(func() time.Time { return fixedTime })
	step.SetClipboardFn(func() string { return "https://example.com" })

	ctx := postprocess.Context{
		Snippets: map[string]string{
			"standup note": "Standup {date}: reviewed {clipboard}",
		},
	}

	// 1. Exact match with trailing punctuation
	got := step.Apply("Standup note.", ctx)
	expected := "Standup 2026-11-01: reviewed https://example.com"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	// 2. Substring match
	gotSub := step.Apply("Here is my standup note for today.", ctx)
	expectedSub := "Here is my Standup 2026-11-01: reviewed https://example.com for today."
	if gotSub != expectedSub {
		t.Errorf("expected %q, got %q", expectedSub, gotSub)
	}

	// 3. No match
	gotNoMatch := step.Apply("Just normal speech", ctx)
	if gotNoMatch != "Just normal speech" {
		t.Errorf("expected original text, got %q", gotNoMatch)
	}
}
