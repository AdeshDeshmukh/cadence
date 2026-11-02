package polish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultOllamaSystemPrompt provides standard instructions for polishing dictation.
const DefaultOllamaSystemPrompt = `You are an expert real-time voice dictation cleanup engine.
Your task is to polish raw speech-to-text transcriptions while preserving the speaker's original meaning and intent.
Rules:
1. Fix grammar, spelling, typos, and formatting.
2. Remove verbal filler words and speech hesitations if any remain.
3. Apply appropriate punctuation, capitalization, and paragraphs.
4. If a specific tone or profile is requested, adapt tone accordingly.
5. NEVER add conversational commentary, explanations, greetings, quotes, or prefaces.
6. Output ONLY the polished text.`

// OllamaPolisher communicates with a local or remote Ollama HTTP server.
type OllamaPolisher struct {
	endpoint     string
	model        string
	systemPrompt string
	temperature  float64
	client       *http.Client
}

// OllamaOption allows customizing the OllamaPolisher.
type OllamaOption func(*OllamaPolisher)

// WithOllamaEndpoint sets the Ollama server URL (e.g. "http://127.0.0.1:11434").
func WithOllamaEndpoint(endpoint string) OllamaOption {
	return func(o *OllamaPolisher) {
		if endpoint != "" {
			o.endpoint = strings.TrimRight(endpoint, "/")
		}
	}
}

// WithOllamaModel sets the model to invoke (e.g. "llama3.2", "mistral", "qwen2.5").
func WithOllamaModel(model string) OllamaOption {
	return func(o *OllamaPolisher) {
		if model != "" {
			o.model = model
		}
	}
}

// WithOllamaSystemPrompt overrides the base system instructions.
func WithOllamaSystemPrompt(p string) OllamaOption {
	return func(o *OllamaPolisher) {
		if p != "" {
			o.systemPrompt = p
		}
	}
}

// WithOllamaTemperature sets generation temperature (default 0.2).
func WithOllamaTemperature(t float64) OllamaOption {
	return func(o *OllamaPolisher) {
		o.temperature = t
	}
}

// WithOllamaClient configures a custom HTTP client.
func WithOllamaClient(client *http.Client) OllamaOption {
	return func(o *OllamaPolisher) {
		if client != nil {
			o.client = client
		}
	}
}

// NewOllamaPolisher creates a new Ollama-backed Polisher.
func NewOllamaPolisher(opts ...OllamaOption) *OllamaPolisher {
	o := &OllamaPolisher{
		endpoint:     "http://127.0.0.1:11434",
		model:        "llama3.2",
		systemPrompt: DefaultOllamaSystemPrompt,
		temperature:  0.2,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *OllamaPolisher) Name() string {
	return fmt.Sprintf("ollama/%s", o.model)
}

func (o *OllamaPolisher) Endpoint() string {
	return o.endpoint
}

func (o *OllamaPolisher) Model() string {
	return o.model
}

type ollamaGenerateRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	System  string                 `json:"system,omitempty"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Model           string `json:"model"`
	CreatedAt       string `json:"created_at"`
	Response        string `json:"response"`
	Done            bool   `json:"done"`
	TotalDuration   int64  `json:"total_duration"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	Error           string `json:"error,omitempty"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name       string `json:"name"`
		ModifiedAt string `json:"modified_at"`
		Size       int64  `json:"size"`
	} `json:"models"`
}

// Polish sends the text to Ollama and returns the polished text.
func (o *OllamaPolisher) Polish(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.Text) == "" {
		return Response{
			Text:  req.Text,
			Model: o.model,
		}, nil
	}

	start := time.Now()

	// Assemble system prompt with contextual profile and rules
	system := o.buildSystemPrompt(req)

	// Assemble prompt
	prompt := req.Text
	if req.Selection != "" {
		prompt = fmt.Sprintf("Context/Selection:\n%s\n\nDictated Text to Polish:\n%s", req.Selection, req.Text)
	}

	genReq := ollamaGenerateRequest{
		Model:  o.model,
		Prompt: prompt,
		System: system,
		Stream: false,
		Options: map[string]interface{}{
			"temperature": o.temperature,
		},
	}

	bodyBytes, err := json.Marshal(genReq)
	if err != nil {
		return Response{}, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return Response{}, fmt.Errorf("create ollama http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := o.client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("call ollama api: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read ollama response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("ollama HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var genResp ollamaGenerateResponse
	if err := json.Unmarshal(respBytes, &genResp); err != nil {
		return Response{}, fmt.Errorf("unmarshal ollama response: %w", err)
	}

	if genResp.Error != "" {
		return Response{}, fmt.Errorf("ollama response error: %s", genResp.Error)
	}

	cleaned := strings.TrimSpace(genResp.Response)
	// Strip enclosing quotation marks if model wrapped response
	if len(cleaned) >= 2 {
		if (strings.HasPrefix(cleaned, "\"") && strings.HasSuffix(cleaned, "\"")) ||
			(strings.HasPrefix(cleaned, "'") && strings.HasSuffix(cleaned, "'")) {
			cleaned = strings.Trim(cleaned, "\"'")
		}
	}

	if cleaned == "" {
		cleaned = req.Text
	}

	return Response{
		Text:      cleaned,
		Model:     o.model,
		TokensIn:  genResp.PromptEvalCount,
		TokensOut: genResp.EvalCount,
		Latency:   time.Since(start),
	}, nil
}

// Ping checks if the Ollama endpoint is reachable and responsive.
func (o *OllamaPolisher) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint+"/api/version", nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	return nil
}

// ListModels retrieves the names of models currently installed in Ollama.
func (o *OllamaPolisher) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama list models failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama list models HTTP %d", resp.StatusCode)
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("decode ollama models: %w", err)
	}

	names := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

func (o *OllamaPolisher) buildSystemPrompt(req Request) string {
	var sb strings.Builder
	sb.WriteString(o.systemPrompt)

	if req.Profile != "" && req.Profile != "general" {
		sb.WriteString(fmt.Sprintf("\nTarget Profile/Tone: %s", req.Profile))
	}

	if req.Application != "" {
		sb.WriteString(fmt.Sprintf("\nTarget Application: %s", req.Application))
	}

	if len(req.CustomRules) > 0 {
		sb.WriteString("\nCustom Rules:\n")
		for _, r := range req.CustomRules {
			sb.WriteString(fmt.Sprintf("- %s\n", r))
		}
	}

	return sb.String()
}
