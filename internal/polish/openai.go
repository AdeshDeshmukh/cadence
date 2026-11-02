package polish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultOpenAISystemPrompt provides standard instructions for polishing dictation.
const DefaultOpenAISystemPrompt = `You are an expert real-time voice dictation cleanup engine.
Your task is to polish raw speech-to-text transcriptions while preserving the speaker's original meaning and intent.
Rules:
1. Fix grammar, spelling, typos, and formatting.
2. Remove verbal filler words and speech hesitations if any remain.
3. Apply appropriate punctuation, capitalization, and paragraphs.
4. If a specific tone or profile is requested, adapt tone accordingly.
5. NEVER add conversational commentary, explanations, greetings, quotes, or prefaces.
6. Output ONLY the polished text.`

// OpenAIPolisher connects to OpenAI or any OpenAI-compatible API endpoint (Groq, Together, Claude proxy, vLLM, etc.).
type OpenAIPolisher struct {
	endpoint     string
	apiKey       string
	model        string
	systemPrompt string
	temperature  float64
	networkMode  string
	client       *http.Client
}

// OpenAIOption customizes OpenAIPolisher configuration.
type OpenAIOption func(*OpenAIPolisher)

// WithOpenAIEndpoint sets the API base URL (e.g. "https://api.openai.com/v1" or "https://api.groq.com/openai/v1").
func WithOpenAIEndpoint(endpoint string) OpenAIOption {
	return func(o *OpenAIPolisher) {
		if endpoint != "" {
			o.endpoint = strings.TrimRight(endpoint, "/")
		}
	}
}

// WithOpenAIAPIKey sets the authorization bearer key.
func WithOpenAIAPIKey(key string) OpenAIOption {
	return func(o *OpenAIPolisher) {
		if key != "" {
			o.apiKey = key
		}
	}
}

// WithOpenAIModel sets the remote model name (e.g. "gpt-4o-mini", "llama-3.3-70b-versatile").
func WithOpenAIModel(model string) OpenAIOption {
	return func(o *OpenAIPolisher) {
		if model != "" {
			o.model = model
		}
	}
}

// WithOpenAISystemPrompt overrides the base system prompt instructions.
func WithOpenAISystemPrompt(p string) OpenAIOption {
	return func(o *OpenAIPolisher) {
		if p != "" {
			o.systemPrompt = p
		}
	}
}

// WithOpenAITemperature sets generation temperature (default 0.2).
func WithOpenAITemperature(t float64) OpenAIOption {
	return func(o *OpenAIPolisher) {
		o.temperature = t
	}
}

// WithOpenAINetworkMode sets the privacy network mode ("local_only", "hybrid", "cloud").
func WithOpenAINetworkMode(mode string) OpenAIOption {
	return func(o *OpenAIPolisher) {
		if mode != "" {
			o.networkMode = mode
		}
	}
}

// WithOpenAIClient configures a custom HTTP client.
func WithOpenAIClient(client *http.Client) OpenAIOption {
	return func(o *OpenAIPolisher) {
		if client != nil {
			o.client = client
		}
	}
}

// NewOpenAIPolisher creates a new OpenAI-compatible Polisher.
func NewOpenAIPolisher(opts ...OpenAIOption) *OpenAIPolisher {
	o := &OpenAIPolisher{
		endpoint:     "https://api.openai.com/v1",
		model:        "gpt-4o-mini",
		systemPrompt: DefaultOpenAISystemPrompt,
		temperature:  0.2,
		networkMode:  "cloud",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.apiKey == "" {
		if k := os.Getenv("CADENCE_OPENAI_API_KEY"); k != "" {
			o.apiKey = k
		} else if k := os.Getenv("OPENAI_API_KEY"); k != "" {
			o.apiKey = k
		}
	}

	return o
}

func (o *OpenAIPolisher) Name() string {
	return fmt.Sprintf("openai/%s", o.model)
}

func (o *OpenAIPolisher) Endpoint() string {
	return o.endpoint
}

func (o *OpenAIPolisher) Model() string {
	return o.model
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int               `json:"index"`
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

func isLocalEndpoint(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".local")
}

// Polish sends text to the OpenAI-compatible endpoint and returns the cleaned text.
func (o *OpenAIPolisher) Polish(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.Text) == "" {
		return Response{
			Text:  req.Text,
			Model: o.model,
		}, nil
	}

	// Privacy policy check: prevent leaking audio transcripts outside device when local_only
	if o.networkMode == "local_only" && !isLocalEndpoint(o.endpoint) {
		return Response{}, errors.New("openai polisher blocked: network_mode is set to 'local_only'")
	}

	start := time.Now()

	system := o.buildSystemPrompt(req)

	userContent := req.Text
	if req.Selection != "" {
		userContent = fmt.Sprintf("Context/Selection:\n%s\n\nDictated Text to Polish:\n%s", req.Selection, req.Text)
	}

	chatReq := openAIChatRequest{
		Model: o.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: userContent},
		},
		Temperature: o.temperature,
	}

	bodyBytes, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("marshal openai request: %w", err)
	}

	urlStr := o.endpoint
	if !strings.HasSuffix(urlStr, "/chat/completions") {
		urlStr += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return Response{}, fmt.Errorf("create openai http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	httpResp, err := o.client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("call openai api: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read openai response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("openai HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return Response{}, fmt.Errorf("unmarshal openai response: %w", err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return Response{}, fmt.Errorf("openai api error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return Response{
			Text:    req.Text,
			Model:   o.model,
			Latency: time.Since(start),
		}, nil
	}

	cleaned := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if len(cleaned) >= 2 {
		if (strings.HasPrefix(cleaned, "\"") && strings.HasSuffix(cleaned, "\"")) ||
			(strings.HasPrefix(cleaned, "'") && strings.HasSuffix(cleaned, "'")) {
			cleaned = strings.Trim(cleaned, "\"'")
		}
	}

	if cleaned == "" {
		cleaned = req.Text
	}

	modelUsed := chatResp.Model
	if modelUsed == "" {
		modelUsed = o.model
	}

	return Response{
		Text:      cleaned,
		Model:     modelUsed,
		TokensIn:  chatResp.Usage.PromptTokens,
		TokensOut: chatResp.Usage.CompletionTokens,
		Latency:   time.Since(start),
	}, nil
}

func (o *OpenAIPolisher) buildSystemPrompt(req Request) string {
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
