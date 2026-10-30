package postprocess_test

import (
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestITN_Currencies(t *testing.T) {
	step := postprocess.NewITN()
	ctx := postprocess.Context{}

	tests := []struct {
		input    string
		expected string
	}{
		{"five dollars", "$5"},
		{"twenty bucks", "$20"},
		{"one hundred dollars", "$100"},
		{"ten dollars and fifty cents", "$10.50"},
		{"fifty cents", "50¢"},
		{"twenty euros", "€20"},
		{"fifteen pounds", "£15"},
	}

	for _, tc := range tests {
		if got := step.Apply(tc.input, ctx); got != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

func TestITN_Percentages(t *testing.T) {
	step := postprocess.NewITN()
	ctx := postprocess.Context{}

	tests := []struct {
		input    string
		expected string
	}{
		{"twenty percent", "20%"},
		{"five percent", "5%"},
		{"one hundred percent", "100%"},
	}

	for _, tc := range tests {
		if got := step.Apply(tc.input, ctx); got != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

func TestITN_Time(t *testing.T) {
	step := postprocess.NewITN()
	ctx := postprocess.Context{}

	tests := []struct {
		input    string
		expected string
	}{
		{"ten thirty am", "10:30 AM"},
		{"five pm", "5 PM"},
		{"two fifteen pm", "2:15 PM"},
	}

	for _, tc := range tests {
		if got := step.Apply(tc.input, ctx); got != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

func TestITN_Ordinals(t *testing.T) {
	step := postprocess.NewITN()
	ctx := postprocess.Context{}

	tests := []struct {
		input    string
		expected string
	}{
		{"first place", "1st place"},
		{"second round", "2nd round"},
		{"twenty-first century", "21st century"},
		{"thirty first", "31st"},
	}

	for _, tc := range tests {
		if got := step.Apply(tc.input, ctx); got != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

func TestITN_CardinalNumbers(t *testing.T) {
	step := postprocess.NewITN()
	ctx := postprocess.Context{}

	tests := []struct {
		input    string
		expected string
	}{
		{"forty two", "42"},
		{"forty-two", "42"},
		{"one hundred twenty", "120"},
		{"two thousand twenty six", "2026"},
		{"there are forty two items in the box", "there are 42 items in the box"},
	}

	for _, tc := range tests {
		if got := step.Apply(tc.input, ctx); got != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}
