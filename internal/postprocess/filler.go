package postprocess

import (
	"regexp"
	"strings"
)

type FillerRemoval struct {
	patterns []*regexp.Regexp
}

func NewFillerRemoval() *FillerRemoval {
	return &FillerRemoval{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(um+|uh+|er+|hmm+)\b[,\s]*`),
			regexp.MustCompile(`(?i)\b(you know)\b[,\s]*`),
		},
	}
}

func (f *FillerRemoval) Name() string { return "filler-removal" }

func (f *FillerRemoval) Apply(
	text string,
	_ Context,
) string {
	for _, p := range f.patterns {
		text = p.ReplaceAllString(text, "")
	}
	return strings.Join(strings.Fields(text), " ")
}
