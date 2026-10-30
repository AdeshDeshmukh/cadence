package postprocess

import (
	"regexp"
	"strings"
	"unicode"
)

type Punctuation struct{}

// NewPunctuation returns a new spoken punctuation processor.
func NewPunctuation() *Punctuation { return &Punctuation{} }

// Name returns the processor name.
func (Punctuation) Name() string { return "punctuation" }

// Apply transforms spoken punctuation commands into punctuation marks and normalizes spacing.
func (Punctuation) Apply(text string, _ Context) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	rules := []struct {
		p *regexp.Regexp
		s string
	}{
		// Multi-line breaks first
		{regexp.MustCompile(`(?i)(?:^|\s+)(new paragraph)\b`), "\n\n"},
		{regexp.MustCompile(`(?i)(?:^|\s+)(new line)\b`), "\n"},

		// Punctuation phrases
		{regexp.MustCompile(`(?i)(?:^|\s+)comma\b`), ","},
		{regexp.MustCompile(`(?i)(?:^|\s+)question mark\b`), "?"},
		{regexp.MustCompile(`(?i)(?:^|\s+)(exclamation mark|exclamation point)\b`), "!"},
		{regexp.MustCompile(`(?i)(?:^|\s+)(period|full stop)\b`), "."},
		{regexp.MustCompile(`(?i)(?:^|\s+)(semicolon|semi-colon)\b`), ";"},
		{regexp.MustCompile(`(?i)(?:^|\s+)colon\b`), ":"},
		{regexp.MustCompile(`(?i)(?:^|\s+)(ellipsis|dot dot dot)\b`), "..."},
		{regexp.MustCompile(`(?i)(?:^|\s+)(hyphen|dash)\b`), "-"},

		// Parentheses
		{regexp.MustCompile(`(?i)(?:^|\s+)(open parenthesis|open paren)\b`), " ("},
		{regexp.MustCompile(`(?i)(?:^|\s+)(close parenthesis|close paren)\b`), ")"},

		// Quotes
		{regexp.MustCompile(`(?i)(?:^|\s+)(open quote|begin quote)\b`), ` "`},
		{regexp.MustCompile(`(?i)(?:^|\s+)(close quote|end quote)\b`), `"`},
		{regexp.MustCompile(`(?i)(?:^|\s+)quote\b`), `"`},
	}

	for _, r := range rules {
		text = r.p.ReplaceAllString(text, r.s)
	}

	// Normalize spacing around punctuation
	text = cleanSpacing(text)

	// Capitalize sentences and line starts
	text = capitalizeSentences(text)

	// Ensure trailing sentence terminal punctuation if not empty
	trimmed := strings.TrimRight(text, " \t")
	if len(trimmed) > 0 {
		last := trimmed[len(trimmed)-1]
		if last != '.' && last != '?' && last != '!' && last != '\n' && last != '"' && last != '\'' {
			text = trimmed + "."
		}
	}

	return text
}

func cleanSpacing(text string) string {
	// Remove space before punctuation marks: , . ? ! : ; ) ] % ¢
	reBeforePunct := regexp.MustCompile(`\s+([,.\?!:;\)\]%¢])`)
	text = reBeforePunct.ReplaceAllString(text, "$1")

	// Remove space after opening brackets / currencies / quotes: ( [ $ € £ "
	reAfterOpen := regexp.MustCompile(`([(\[\$€£"])\s+`)
	text = reAfterOpen.ReplaceAllString(text, "$1")

	// Remove space before closing quote
	reBeforeCloseQuote := regexp.MustCompile(`\s+"`)
	text = reBeforeCloseQuote.ReplaceAllString(text, `"`)

	// Ensure single space after punctuation: , : ;
	reAfterPunct := regexp.MustCompile(`([,:;])([^\s\n\d"'\)\]])`)
	text = reAfterPunct.ReplaceAllString(text, "$1 $2")

	// Clean intra-line spaces while preserving newlines
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(strings.Join(strings.Fields(line), " "))
	}
	return strings.Join(lines, "\n")
}

func capitalizeSentences(text string) string {
	runes := []rune(text)
	capNext := true

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if capNext && unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
			capNext = false
		} else if r == '.' || r == '?' || r == '!' || r == '\n' {
			capNext = true
		}
	}

	return string(runes)
}
