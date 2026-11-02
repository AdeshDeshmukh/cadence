package polish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestOllamaPolisher_Polish_Success(t *testing.T) {
	var receivedReq ollamaGenerateRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected path /api/generate, got %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)

		resp := ollamaGenerateResponse{
			Model:           "llama3.2",
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
			Response:        "Hello, this is cleanly polished text.",
			Done:            true,
			TotalDuration:   120000000,
			PromptEvalCount: 15,
			EvalCount:       22,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(
		WithOllamaEndpoint(srv.URL),
		WithOllamaModel("llama3.2"),
		WithOllamaTemperature(0.1),
	)

	if polisher.Endpoint() != srv.URL {
		t.Fatalf("expected endpoint %s, got %s", srv.URL, polisher.Endpoint())
	}
	if polisher.Model() != "llama3.2" {
		t.Fatalf("expected model llama3.2, got %s", polisher.Model())
	}
	if polisher.Name() != "ollama/llama3.2" {
		t.Fatalf("expected name ollama/llama3.2, got %s", polisher.Name())
	}

	req := Request{
		Text: "hello this is cleanly polished text",
	}

	ctx := context.Background()
	res, err := polisher.Polish(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Hello, this is cleanly polished text." {
		t.Fatalf("unexpected text: %s", res.Text)
	}
	if res.TokensIn != 15 || res.TokensOut != 22 {
		t.Fatalf("expected tokens (15, 22), got (%d, %d)", res.TokensIn, res.TokensOut)
	}
	if res.Model != "llama3.2" {
		t.Fatalf("expected model llama3.2, got %s", res.Model)
	}
	if res.Latency <= 0 {
		t.Fatalf("expected positive latency")
	}

	if receivedReq.Stream {
		t.Errorf("expected stream=false")
	}
	if receivedReq.Model != "llama3.2" {
		t.Errorf("expected model llama3.2, got %s", receivedReq.Model)
	}
}

func TestOllamaPolisher_Polish_EmptyText(t *testing.T) {
	polisher := NewOllamaPolisher(WithOllamaEndpoint("http://invalid"))
	res, err := polisher.Polish(context.Background(), Request{Text: "  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "  " {
		t.Fatalf("expected unaltered whitespace, got %q", res.Text)
	}
}

func TestOllamaPolisher_Polish_ContextAndFormatting(t *testing.T) {
	var receivedReq ollamaGenerateRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)

		// Return text wrapped in quotes to test quote stripping
		resp := ollamaGenerateResponse{
			Response: `"func main() { fmt.Println(\"hi\") }"`,
			Done:     true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(
		WithOllamaEndpoint(srv.URL),
		WithOllamaModel("qwen2.5"),
		WithOllamaSystemPrompt("Custom base prompt"),
	)

	req := Request{
		Text:        "fmt print line hi",
		Profile:     "code",
		Application: "VS Code",
		Selection:   "package main",
		CustomRules: []string{"Use Go conventions", "No semicolons"},
	}

	res, err := polisher.Polish(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedStripped := `func main() { fmt.Println(\"hi\") }`
	if res.Text != expectedStripped {
		t.Fatalf("expected quotes stripped, got: %s", res.Text)
	}

	if !strings.Contains(receivedReq.System, "Target Profile/Tone: code") {
		t.Errorf("system prompt missing profile: %s", receivedReq.System)
	}
	if !strings.Contains(receivedReq.System, "Target Application: VS Code") {
		t.Errorf("system prompt missing application: %s", receivedReq.System)
	}
	if !strings.Contains(receivedReq.System, "Custom Rules:\n- Use Go conventions") {
		t.Errorf("system prompt missing custom rules: %s", receivedReq.System)
	}
	if !strings.Contains(receivedReq.Prompt, "Context/Selection:\npackage main") {
		t.Errorf("prompt missing selection context: %s", receivedReq.Prompt)
	}
}

func TestOllamaPolisher_Polish_HttpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'nonexistent' not found"}`))
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(
		WithOllamaEndpoint(srv.URL),
		WithOllamaModel("nonexistent"),
	)

	_, err := polisher.Polish(context.Background(), Request{Text: "test input"})
	if err == nil {
		t.Fatalf("expected error on 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("expected HTTP 404 in error message, got: %v", err)
	}
}

func TestOllamaPolisher_Polish_ResponseErrorField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Error: "cuda out of memory",
		})
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(WithOllamaEndpoint(srv.URL))
	_, err := polisher.Polish(context.Background(), Request{Text: "test"})
	if err == nil || !strings.Contains(err.Error(), "cuda out of memory") {
		t.Fatalf("expected cuda out of memory error, got: %v", err)
	}
}

func TestOllamaPolisher_Polish_FallbackOnEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Response: "   ",
			Done:     true,
		})
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(WithOllamaEndpoint(srv.URL))
	res, err := polisher.Polish(context.Background(), Request{Text: "original text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "original text" {
		t.Fatalf("expected fallback to original text, got: %s", res.Text)
	}
}

func TestOllamaPolisher_PingAndListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"0.1.32"}`))
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []struct {
					Name       string `json:"name"`
					ModifiedAt string `json:"modified_at"`
					Size       int64  `json:"size"`
				}{
					{Name: "llama3.2:latest"},
					{Name: "qwen2.5:latest"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(WithOllamaEndpoint(srv.URL))

	ctx := context.Background()
	if err := polisher.Ping(ctx); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	models, err := polisher.ListModels(ctx)
	if err != nil {
		t.Fatalf("list models failed: %v", err)
	}
	if len(models) != 2 || models[0] != "llama3.2:latest" || models[1] != "qwen2.5:latest" {
		t.Fatalf("unexpected models list: %v", models)
	}
}

func TestOllamaPolisher_StepFallbackOnDownServer(t *testing.T) {
	// Point to closed server
	polisher := NewOllamaPolisher(
		WithOllamaEndpoint("http://127.0.0.1:1"),
		WithOllamaClient(&http.Client{Timeout: 50 * time.Millisecond}),
	)

	step := NewStep(polisher, "general", 100*time.Millisecond)
	ctx := postprocess.Context{Mode: "general"}

	input := "raw spoken text"
	out := step.Apply(input, ctx)
	// When Ollama is unreachable, Step must gracefully return raw text without hanging
	if out != input {
		t.Fatalf("expected graceful fallback to raw text %q, got %q", input, out)
	}
}
