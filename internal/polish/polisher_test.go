package polish_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestNoopPolisher(t *testing.T) {
	p := polish.NewNoopPolisher()
	if p.Name() != "noop" {
		t.Fatalf("expected name noop, got %s", p.Name())
	}

	req := polish.Request{Text: "raw input text", Profile: "code"}
	resp, err := p.Polish(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != req.Text {
		t.Fatalf("expected text %q, got %q", req.Text, resp.Text)
	}
	if resp.Model != "noop" {
		t.Fatalf("expected model noop, got %s", resp.Model)
	}
}

func TestMockPolisher(t *testing.T) {
	mock := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:     strings.ToUpper(req.Text),
			Model:    "test-model",
			TokensIn: 5, TokensOut: 5,
		}, nil
	})

	resp, err := mock.Polish(context.Background(), polish.Request{Text: "hello"})
	if err != nil {
		t.Fatalf("mock error: %v", err)
	}
	if resp.Text != "HELLO" {
		t.Fatalf("expected HELLO, got %s", resp.Text)
	}
}

func TestStep_Apply_Success(t *testing.T) {
	mock := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:  "Polished: " + req.Text,
			Model: "mock-llm",
		}, nil
	})

	step := polish.NewStep(mock, "general", 500*time.Millisecond)
	if step.Name() != "polish" {
		t.Errorf("expected step name 'polish', got %s", step.Name())
	}

	got := step.Apply("raw text", postprocess.Context{Mode: "general"})
	expected := "Polished: raw text"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestStep_Apply_TimeoutFallback(t *testing.T) {
	mockHanging := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		time.Sleep(100 * time.Millisecond)
		return polish.Response{Text: "too late"}, nil
	})

	step := polish.NewStep(mockHanging, "general", 20*time.Millisecond)
	got := step.Apply("original speech", postprocess.Context{Mode: "general"})

	// Must fall back to original speech
	if got != "original speech" {
		t.Fatalf("expected fallback to original speech on timeout, got %q", got)
	}
}

func TestStep_Apply_ErrorFallback(t *testing.T) {
	mockError := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{}, errors.New("backend connection refused")
	})

	step := polish.NewStep(mockError, "general", 500*time.Millisecond)
	got := step.Apply("original speech", postprocess.Context{Mode: "general"})

	if got != "original speech" {
		t.Fatalf("expected fallback to original speech on error, got %q", got)
	}
}

func TestStep_Apply_Empty(t *testing.T) {
	step := polish.NewStep(polish.NewNoopPolisher(), "general", 500*time.Millisecond)
	if got := step.Apply("", postprocess.Context{}); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
