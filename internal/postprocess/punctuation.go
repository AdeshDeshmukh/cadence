package postprocess

import (
	"regexp"
	"strings"
)

type Punctuation struct{}

func NewPunctuation() *Punctuation { return &Punctuation{} }
func (Punctuation) Name() string   { return "punctuation" }

func (Punctuation) Apply(text string, _ Context) string {
	rules := []struct {
		p *regexp.Regexp
		s string
	}{
		{regexp.MustCompile(`(?i)\s+comma\b`), ","},
		{regexp.MustCompile(`(?i)\s+question mark\b`), "?"},
		{regexp.MustCompile(`(?i)\s+exclamation mark\b`), "!"},
		{regexp.MustCompile(`(?i)\s+(period|full stop)\b`), "."},
	}

	for _, r := range rules {
		text = r.p.ReplaceAllString(text, r.s)
	}

	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return text
	}

	text = strings.ToUpper(text[:1]) + text[1:]
	last := text[len(text)-1]
	if last != '.' && last != '?' && last != '!' {
		text += "."
	}
	return text
}
