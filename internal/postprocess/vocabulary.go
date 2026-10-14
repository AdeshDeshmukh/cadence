package postprocess

import (
	"regexp"
)

type Vocabulary struct{}

func NewVocabulary() *Vocabulary { return &Vocabulary{} }
func (Vocabulary) Name() string  { return "vocabulary" }

func (Vocabulary) Apply(text string, ctx Context) string {
	for spoken, canonical := range ctx.Vocabulary {
		if spoken == "" {
			continue
		}
		p := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(spoken) + `\b`)
		text = p.ReplaceAllString(text, canonical)
	}
	return text
}
