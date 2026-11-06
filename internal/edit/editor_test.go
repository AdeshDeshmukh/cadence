package edit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
)

type mockPolisher struct {
	lastReq polish.Request
	output  string
	err     error
}

func (m *mockPolisher) Name() string { return "mock" }
func (m *mockPolisher) Polish(_ context.Context, req polish.Request) (polish.Response, error) {
	m.lastReq = req
	if m.err != nil {
		return polish.Response{}, m.err
	}
	return polish.Response{
		Text:    m.output,
		Model:   "mock-llm",
		Latency: 5 * time.Millisecond,
	}, nil
}

func TestEditor_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("Empty text returns error", func(t *testing.T) {
		ed := NewEditor(nil)
		_, err := ed.Execute(ctx, "   ", Instruction{Intent: IntentRewrite})
		if err == nil {
			t.Fatal("expected error on empty text")
		}
	})

	t.Run("Translate instruction", func(t *testing.T) {
		mock := &mockPolisher{output: "Bonjour le monde"}
		ed := NewEditor(mock)

		instr := Instruction{
			Intent:         IntentTranslate,
			TargetLanguage: "French",
		}

		res, err := ed.Execute(ctx, "Hello world", instr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.EditedText != "Bonjour le monde" {
			t.Errorf("got %q, want %q", res.EditedText, "Bonjour le monde")
		}
		if !strings.Contains(mock.lastReq.Text, "Translate the text into French") {
			t.Errorf("expected prompt to contain target language, got %q", mock.lastReq.Text)
		}
		if res.Model != "mock-llm" {
			t.Errorf("expected model mock-llm, got %q", res.Model)
		}
	})

	t.Run("Shorten with surrounding whitespace preservation", func(t *testing.T) {
		mock := &mockPolisher{output: `Here is the shortened text: "We beat our quarterly revenue targets."`}
		ed := NewEditor(mock)

		instr := Instruction{
			Intent: IntentShorten,
			Tone:   "concise",
		}

		original := "\n  In the recent financial quarter, we have achieved results that substantially exceeded our revenue targets.  \n"
		res, err := ed.Execute(ctx, original, instr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "\n  We beat our quarterly revenue targets.  \n"
		if res.EditedText != expected {
			t.Errorf("got %q, want %q", res.EditedText, expected)
		}
		if !strings.Contains(mock.lastReq.Text, "Condense and shorten") {
			t.Errorf("expected prompt to contain condense instruction, got %q", mock.lastReq.Text)
		}
	})

	t.Run("Rewrite with tone", func(t *testing.T) {
		mock := &mockPolisher{output: "Please find attached the financial review."}
		ed := NewEditor(mock)

		instr := Instruction{
			Intent: IntentRewrite,
			Tone:   "professional",
		}

		res, err := ed.Execute(ctx, "here is the numbers check them out", instr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.EditedText != "Please find attached the financial review." {
			t.Errorf("got %q", res.EditedText)
		}
		if !strings.Contains(mock.lastReq.Text, "sound professional") {
			t.Errorf("expected prompt to mention tone professional, got %q", mock.lastReq.Text)
		}
	})

	t.Run("Summarize into bullet points", func(t *testing.T) {
		mock := &mockPolisher{output: "- Point 1\n- Point 2"}
		ed := NewEditor(mock)

		instr := Instruction{
			Intent: IntentSummarize,
			Format: "bullet_list",
		}

		res, err := ed.Execute(ctx, "Long text with multiple ideas...", instr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.EditedText != "- Point 1\n- Point 2" {
			t.Errorf("got %q", res.EditedText)
		}
		if !strings.Contains(mock.lastReq.Text, "bulleted list") {
			t.Errorf("expected prompt to request bulleted list, got %q", mock.lastReq.Text)
		}
	})

	t.Run("Expand instruction", func(t *testing.T) {
		mock := &mockPolisher{output: "This project will revolutionize text editing by using voice directly."}
		ed := NewEditor(mock)

		instr := Instruction{
			Intent: IntentExpand,
		}

		res, err := ed.Execute(ctx, "This project helps editing.", instr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(mock.lastReq.Text, "Elaborate and expand") {
			t.Errorf("expected prompt to request expand, got %q", mock.lastReq.Text)
		}
		if res.EditedText != "This project will revolutionize text editing by using voice directly." {
			t.Errorf("got %q", res.EditedText)
		}
	})

	t.Run("Format as Markdown Table", func(t *testing.T) {
		mock := &mockPolisher{output: "| Name | Age |\n|---|---|\n| Alice | 30 |"}
		ed := NewEditor(mock)

		instr := Instruction{
			Intent: IntentFormatTable,
		}

		res, err := ed.Execute(ctx, "Alice is 30 years old", instr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(mock.lastReq.Text, "Markdown table") {
			t.Errorf("expected prompt to request Markdown table, got %q", mock.lastReq.Text)
		}
		if !strings.Contains(res.EditedText, "| Name | Age |") {
			t.Errorf("got %q", res.EditedText)
		}
	})
}
