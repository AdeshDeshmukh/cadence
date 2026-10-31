package postprocess

import (
	"regexp"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/paste"
)

// ExpandVariables replaces dynamic variable placeholders in template.
func ExpandVariables(template string, clipboard string, t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}

	result := template
	result = strings.ReplaceAll(result, "{date}", t.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "{time}", t.Format("15:04:05"))
	result = strings.ReplaceAll(result, "{datetime}", t.Format("2006-01-02 15:04:05"))
	result = strings.ReplaceAll(result, "{year}", t.Format("2006"))
	result = strings.ReplaceAll(result, "{month}", t.Format("01"))
	result = strings.ReplaceAll(result, "{day}", t.Format("02"))
	result = strings.ReplaceAll(result, "{clipboard}", clipboard)
	return result
}

// Snippets expands voice-triggered snippet templates into expanded text.
type Snippets struct {
	clipboardFn func() string
	nowFn       func() time.Time
}

// NewSnippets creates a new snippets post-processing step.
func NewSnippets() *Snippets {
	return &Snippets{
		clipboardFn: func() string {
			text, _ := paste.GetSystemClipboard()
			return text
		},
		nowFn: time.Now,
	}
}

// SetClipboardFn overrides clipboard reader for testing.
func (s *Snippets) SetClipboardFn(fn func() string) {
	s.clipboardFn = fn
}

// SetNowFn overrides clock reader for testing.
func (s *Snippets) SetNowFn(fn func() time.Time) {
	s.nowFn = fn
}

func (s *Snippets) Name() string {
	return "snippets"
}

func (s *Snippets) Apply(text string, ctx Context) string {
	if len(ctx.Snippets) == 0 || strings.TrimSpace(text) == "" {
		return text
	}

	now := time.Now()
	if s.nowFn != nil {
		now = s.nowFn()
	}
	var clip string
	if s.clipboardFn != nil {
		clip = s.clipboardFn()
	}

	cleanText := strings.TrimRight(strings.TrimSpace(text), ".!?")

	for trigger, template := range ctx.Snippets {
		trig := strings.TrimSpace(trigger)
		if trig == "" {
			continue
		}

		expanded := ExpandVariables(template, clip, now)

		// Check whole sentence match first (ignoring trailing punctuation)
		cleanTrig := strings.TrimRight(trig, ".!?")
		if strings.EqualFold(cleanText, cleanTrig) {
			return expanded
		}

		// Check word boundary substring match
		p := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(trig) + `\b`)
		if p.MatchString(text) {
			text = p.ReplaceAllString(text, expanded)
		}
	}

	return text
}
