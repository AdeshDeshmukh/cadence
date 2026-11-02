package polish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestOpenAIPolisher_Polish_Success(t *testing.T) {
	var receivedAuth string
	var receivedReq openAIChatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)

		resp := openAIChatResponse{
			ID:    "chatcmpl-test",
			Model: "gpt-4o-mini",
			Choices: []struct {
				Index   int               `json:"index"`
				Message openAIChatMessage `json:"message"`
			}{
				{
					Index: 0,
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: "This is polished text from OpenAI.",
					},
				},
			},
		}
		resp.Usage.PromptTokens = 20
		resp.Usage.CompletionTokens = 12
		resp.Usage.TotalTokens = 32

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	polisher := NewOpenAIPolisher(
		WithOpenAIEndpoint(srv.URL),
		WithOpenAIAPIKey("sk-test-secret-key"),
		WithOpenAIModel("gpt-4o-mini"),
		WithOpenAITemperature(0.1),
		WithOpenAINetworkMode("cloud"),
	)

	if polisher.Endpoint() != srv.URL {
		t.Fatalf("expected endpoint %s, got %s", srv.URL, polisher.Endpoint())
	}
	if polisher.Model() != "gpt-4o-mini" {
		t.Fatalf("expected model gpt-4o-mini, got %s", polisher.Model())
	}
	if polisher.Name() != "openai/gpt-4o-mini" {
		t.Fatalf("expected name openai/gpt-4o-mini, got %s", polisher.Name())
	}

	res, err := polisher.Polish(context.Background(), Request{
		Text: "this is unpolished raw speech",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "This is polished text from OpenAI." {
		t.Fatalf("unexpected result: %s", res.Text)
	}
	if res.TokensIn != 20 || res.TokensOut != 12 {
		t.Fatalf("expected tokens (20, 12), got (%d, %d)", res.TokensIn, res.TokensOut)
	}
	if res.Model != "gpt-4o-mini" {
		t.Fatalf("expected model gpt-4o-mini, got %s", res.Model)
	}
	if res.Latency <= 0 {
		t.Fatalf("expected positive latency")
	}

	if receivedAuth != "Bearer sk-test-secret-key" {
		t.Errorf("expected Bearer sk-test-secret-key, got %s", receivedAuth)
	}
}

func TestOpenAIPolisher_Polish_EmptyText(t *testing.T) {
	polisher := NewOpenAIPolisher(WithOpenAIEndpoint("http://invalid"))
	res, err := polisher.Polish(context.Background(), Request{Text: "  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "  " {
		t.Fatalf("expected unaltered whitespace, got %q", res.Text)
	}
}

func TestOpenAIPolisher_Polish_ContextAndFormatting(t *testing.T) {
	var receivedReq openAIChatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)

		resp := openAIChatResponse{
			Choices: []struct {
				Index   int               `json:"index"`
				Message openAIChatMessage `json:"message"`
			}{
				{
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: `"Polished with quotes"`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	polisher := NewOpenAIPolisher(
		WithOpenAIEndpoint(srv.URL),
		WithOpenAIModel("llama-3.3-70b-versatile"),
	)

	req := Request{
		Text:        "unpolished sentence",
		Profile:     "medical",
		Application: "Epic EMR",
		Selection:   "Patient history notes",
		CustomRules: []string{"Use metric units", "No medical jargon abbreviations"},
	}

	res, err := polisher.Polish(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Polished with quotes" {
		t.Fatalf("expected quotes stripped, got: %s", res.Text)
	}

	if len(receivedReq.Messages) != 2 {
		t.Fatalf("expected 2 messages (system, user), got %d", len(receivedReq.Messages))
	}
	systemMsg := receivedReq.Messages[0].Content
	userMsg := receivedReq.Messages[1].Content

	if !strings.Contains(systemMsg, "Target Profile/Tone: medical") {
		t.Errorf("system missing profile: %s", systemMsg)
	}
	if !strings.Contains(systemMsg, "Target Application: Epic EMR") {
		t.Errorf("system missing app: %s", systemMsg)
	}
	if !strings.Contains(systemMsg, "Custom Rules:\n- Use metric units") {
		t.Errorf("system missing rules: %s", systemMsg)
	}
	if !strings.Contains(userMsg, "Context/Selection:\nPatient history notes") {
		t.Errorf("user message missing selection context: %s", userMsg)
	}
}

func TestOpenAIPolisher_Polish_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	polisher := NewOpenAIPolisher(
		WithOpenAIEndpoint(srv.URL),
		WithOpenAIAPIKey("sk-invalid"),
	)

	_, err := polisher.Polish(context.Background(), Request{Text: "test input"})
	if err == nil {
		t.Fatalf("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected HTTP 401 in error message, got: %v", err)
	}
}

func TestOpenAIPolisher_Polish_PrivacyLocalOnlyBlocked(t *testing.T) {
	polisher := NewOpenAIPolisher(
		WithOpenAIEndpoint("https://api.openai.com/v1"),
		WithOpenAINetworkMode("local_only"),
	)

	_, err := polisher.Polish(context.Background(), Request{Text: "sensitive private text"})
	if err == nil {
		t.Fatalf("expected privacy policy error in local_only mode, got nil")
	}
	if !strings.Contains(err.Error(), "local_only") {
		t.Fatalf("expected 'local_only' in error, got: %v", err)
	}
}

func TestOpenAIPolisher_Polish_LocalEndpointAllowedInLocalOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIChatResponse{
			Choices: []struct {
				Index   int               `json:"index"`
				Message openAIChatMessage `json:"message"`
			}{
				{
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: "Local vLLM response",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// local loopback server should be allowed even if networkMode is local_only
	polisher := NewOpenAIPolisher(
		WithOpenAIEndpoint(srv.URL),
		WithOpenAINetworkMode("local_only"),
	)

	res, err := polisher.Polish(context.Background(), Request{Text: "input"})
	if err != nil {
		t.Fatalf("expected local endpoint to succeed in local_only mode, got: %v", err)
	}
	if res.Text != "Local vLLM response" {
		t.Fatalf("unexpected text: %s", res.Text)
	}
}

func TestOpenAIPolisher_APIKeyFromEnv(t *testing.T) {
	_ = os.Setenv("CADENCE_OPENAI_API_KEY", "sk-env-cadence-key")
	defer os.Unsetenv("CADENCE_OPENAI_API_KEY")

	polisher := NewOpenAIPolisher()
	if polisher.apiKey != "sk-env-cadence-key" {
		t.Fatalf("expected api key from CADENCE_OPENAI_API_KEY, got: %s", polisher.apiKey)
	}

	_ = os.Unsetenv("CADENCE_OPENAI_API_KEY")
	_ = os.Setenv("OPENAI_API_KEY", "sk-env-standard-key")
	defer os.Unsetenv("OPENAI_API_KEY")

	polisher2 := NewOpenAIPolisher()
	if polisher2.apiKey != "sk-env-standard-key" {
		t.Fatalf("expected api key from OPENAI_API_KEY, got: %s", polisher2.apiKey)
	}
}
