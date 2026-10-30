package postprocess_test

import (
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestFillerRemoval(t *testing.T) {
	step := postprocess.NewFillerRemoval()
	ctx := postprocess.Context{}

	input := "um hello uh world er hmm you know this is a test"
	expected := "hello world this is a test"
	if got := step.Apply(input, ctx); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestPunctuation(t *testing.T) {
	step := postprocess.NewPunctuation()
	ctx := postprocess.Context{}

	tests := []struct {
		input    string
		expected string
	}{
		{"hello comma world", "Hello, world."},
		{"is this working question mark", "Is this working?"},
		{"amazing exclamation mark", "Amazing!"},
		{"finish the sentence period", "Finish the sentence."},
		{"already capitalized.", "Already capitalized."},
		{"first part semicolon second part", "First part; second part."},
		{"note colon important info", "Note: important info."},
		{"open quote hello world close quote", `"Hello world"`},
		{"an open paren aside close paren here", "An (aside) here."},
		{"first line new line second line", "First line\nSecond line."},
		{"first para new paragraph second para", "First para\n\nSecond para."},
	}

	for _, tc := range tests {
		if got := step.Apply(tc.input, ctx); got != tc.expected {
			t.Fatalf("input %q: expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

func TestVocabulary(t *testing.T) {
	step := postprocess.NewVocabulary()
	ctx := postprocess.Context{
		Vocabulary: map[string]string{
			"cube control": "kubectl",
			"golang":       "Go",
		},
	}

	input := "run cube control for golang"
	expected := "run kubectl for Go"
	if got := step.Apply(input, ctx); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestPipeline(t *testing.T) {
	p := postprocess.New(
		postprocess.NewFillerRemoval(),
		postprocess.NewVocabulary(),
		postprocess.NewITN(),
		postprocess.NewPunctuation(),
	)

	ctx := postprocess.Context{
		Vocabulary: map[string]string{
			"docker compose": "docker-compose",
		},
	}

	input := "um start docker compose comma please cost is twenty dollars"
	expected := "Start docker-compose, please cost is $20."
	if got := p.Apply(input, ctx); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}

	multilineInput := "hello world period new paragraph this costs fifty percent more new line see you tomorrow period"
	expectedMultiline := "Hello world.\n\nThis costs 50% more\nSee you tomorrow."
	if got := p.Apply(multilineInput, ctx); got != expectedMultiline {
		t.Fatalf("expected %q, got %q", expectedMultiline, got)
	}
}
