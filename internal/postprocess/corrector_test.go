package postprocess_test

import (
	"testing"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestCorrector_General(t *testing.T) {
	c := postprocess.NewCorrector(nil)
	if c.Name() != "corrector" {
		t.Fatalf("expected step name 'corrector', got %q", c.Name())
	}

	input := "I could of done that, never the less it was fine"
	ctx := postprocess.Context{Application: "TextEdit"}
	got := c.Apply(input, ctx)
	expected := "I could have done that, nevertheless it was fine"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestCorrector_Code(t *testing.T) {
	c := postprocess.NewCorrector(nil)

	input := "write a four loop with sink await in pie test"
	report := c.Correct(input, "Visual Studio Code")

	if report.Category != appcontext.CategoryCode {
		t.Fatalf("expected category code, got %s", report.Category)
	}
	expected := "write a for loop with async await in pytest"
	if report.Corrected != expected {
		t.Fatalf("expected %q, got %q", expected, report.Corrected)
	}
	if report.CorrectionsApplied < 3 {
		t.Fatalf("expected at least 3 corrections, got %d", report.CorrectionsApplied)
	}

	// Terminal context
	termInput := "git check out main and cube cuddle get pods"
	termReport := c.Correct(termInput, "iTerm2")
	if termReport.Category != appcontext.CategoryTerminal {
		t.Fatalf("expected category terminal, got %s", termReport.Category)
	}
	expectedTerm := "git checkout main and kubectl get pods"
	if termReport.Corrected != expectedTerm {
		t.Fatalf("expected %q, got %q", expectedTerm, termReport.Corrected)
	}
}

func TestCorrector_Medical(t *testing.T) {
	c := postprocess.NewCorrector(nil)

	input := "patient percent with hyper tension and tachy cardia"
	report := c.Correct(input, "Epic Hyperspace")

	if report.Category != appcontext.CategoryMedical {
		t.Fatalf("expected category medical, got %s", report.Category)
	}
	expected := "patient presented with hypertension and tachycardia"
	if report.Corrected != expected {
		t.Fatalf("expected %q, got %q", expected, report.Corrected)
	}
}

func TestCorrector_Legal(t *testing.T) {
	c := postprocess.NewCorrector(nil)

	input := "issue a sub pina per say"
	report := c.Correct(input, "Clio")

	if report.Category != appcontext.CategoryLegal {
		t.Fatalf("expected category legal, got %s", report.Category)
	}
	expected := "issue a subpoena per se"
	if report.Corrected != expected {
		t.Fatalf("expected %q, got %q", expected, report.Corrected)
	}
}

func TestCorrector_ContextIsolation(t *testing.T) {
	c := postprocess.NewCorrector(nil)

	// "four loop" in a regular chat or general app should NOT be modified to "for loop"
	input := "I bought four loop earrings"
	report := c.Correct(input, "Slack")

	if report.Category != appcontext.CategoryChat {
		t.Fatalf("expected category chat, got %s", report.Category)
	}
	if report.Corrected != input {
		t.Fatalf("expected no code modification in chat, got %q", report.Corrected)
	}
}

func TestCorrector_CustomRules(t *testing.T) {
	custom := map[string]string{
		"my test app": "MyTestApp",
		"four loop":   "4-loop", // Custom override
	}
	c := postprocess.NewCorrector(custom)

	input := "running my test app with a four loop"
	report := c.Correct(input, "VS Code")

	if report.Corrected != "running MyTestApp with a 4-loop" {
		t.Fatalf("expected custom rules applied, got %q", report.Corrected)
	}
}

func TestCorrector_Empty(t *testing.T) {
	c := postprocess.NewCorrector(nil)
	report := c.Correct("", "")
	if report.Corrected != "" {
		t.Fatalf("expected empty result, got %q", report.Corrected)
	}
}
