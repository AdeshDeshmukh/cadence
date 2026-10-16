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
		postprocess.NewPunctuation(),
	)

	ctx := postprocess.Context{
		Vocabulary: map[string]string{
			"docker compose": "docker-compose",
		},
	}

	input := "um start docker compose comma please"
	expected := "Start docker-compose, please."
	if got := p.Apply(input, ctx); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}
