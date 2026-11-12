package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"time"
)

// Embedder generates dense vector representations from input texts.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
	ModelName() string
}

// LocalHashEmbedder is a fast, deterministic, zero-dependency on-device embedder
// based on signed feature hashing across words, n-grams, and subwords.
type LocalHashEmbedder struct {
	dimension int
	modelName string
}

// NewLocalHashEmbedder returns a new LocalHashEmbedder with the specified dimension (e.g. 256 or 384).
func NewLocalHashEmbedder(dimension int) *LocalHashEmbedder {
	if dimension <= 0 {
		dimension = 384
	}
	return &LocalHashEmbedder{
		dimension: dimension,
		modelName: "cadence-hash-v1",
	}
}

func (e *LocalHashEmbedder) Dimension() int {
	return e.dimension
}

func (e *LocalHashEmbedder) ModelName() string {
	return e.modelName
}

func (e *LocalHashEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embeddings[i] = e.embedSingle(text)
	}
	return embeddings, nil
}

func (e *LocalHashEmbedder) embedSingle(text string) []float32 {
	vec := make([]float32, e.dimension)
	cleaned := strings.ToLower(strings.TrimSpace(text))
	if cleaned == "" {
		return vec
	}

	words := strings.FieldsFunc(cleaned, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	})
	if len(words) == 0 {
		return vec
	}

	// 1. Single word features
	for _, w := range words {
		e.addHashFeature(vec, "w:"+w, 1.0)
		// Character 3-grams for subword matching
		if len(w) >= 3 {
			for j := 0; j <= len(w)-3; j++ {
				e.addHashFeature(vec, "c3:"+w[j:j+3], 0.4)
			}
		}
	}

	// 2. Word bigram features for phrase semantics
	for i := 0; i < len(words)-1; i++ {
		bigram := words[i] + "_" + words[i+1]
		e.addHashFeature(vec, "bg:"+bigram, 1.4)
	}

	return L2Normalize(vec)
}

func (e *LocalHashEmbedder) addHashFeature(vec []float32, token string, weight float32) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(token))
	sum := h.Sum64()

	idx := int((sum >> 1) % uint64(e.dimension))
	sign := float32(1.0)
	if sum&1 != 0 {
		sign = -1.0
	}
	vec[idx] += sign * weight
}

// OllamaEmbedder integrates with a local Ollama daemon for neural embeddings.
type OllamaEmbedder struct {
	client    *http.Client
	endpoint  string
	model     string
	dimension int
}

// NewOllamaEmbedder creates an embedder using Ollama's HTTP API.
func NewOllamaEmbedder(endpoint, model string, dimension int) *OllamaEmbedder {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if model == "" {
		model = "nomic-embed-text"
	}
	if dimension <= 0 {
		dimension = 768
	}
	return &OllamaEmbedder{
		client:    &http.Client{Timeout: 15 * time.Second},
		endpoint:  endpoint,
		model:     model,
		dimension: dimension,
	}
}

func (o *OllamaEmbedder) SetHTTPClient(c *http.Client) {
	if c != nil {
		o.client = c
	}
}

func (o *OllamaEmbedder) Dimension() int {
	return o.dimension
}

func (o *OllamaEmbedder) ModelName() string {
	return "ollama:" + o.model
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type ollamaLegacyResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Try Ollama /api/embed batch endpoint first
	url := fmt.Sprintf("%s/api/embed", o.endpoint)
	reqBody, _ := json.Marshal(ollamaEmbedRequest{
		Model: o.model,
		Input: texts,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var res ollamaEmbedResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && len(res.Embeddings) == len(texts) {
			for i := range res.Embeddings {
				res.Embeddings[i] = L2Normalize(res.Embeddings[i])
			}
			return res.Embeddings, nil
		}
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Fallback to legacy single /api/embeddings per text
	legacyURL := fmt.Sprintf("%s/api/embeddings", o.endpoint)
	results := make([][]float32, len(texts))
	for i, t := range texts {
		legacyBody, _ := json.Marshal(map[string]string{
			"model":  o.model,
			"prompt": t,
		})
		lReq, err := http.NewRequestWithContext(ctx, http.MethodPost, legacyURL, bytes.NewReader(legacyBody))
		if err != nil {
			return nil, err
		}
		lReq.Header.Set("Content-Type", "application/json")
		lResp, err := o.client.Do(lReq)
		if err != nil {
			return nil, fmt.Errorf("ollama embedding request failed: %w", err)
		}
		if lResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(lResp.Body)
			_ = lResp.Body.Close()
			return nil, fmt.Errorf("ollama returned status %d: %s", lResp.StatusCode, string(body))
		}
		var lOut ollamaLegacyResponse
		err = json.NewDecoder(lResp.Body).Decode(&lOut)
		_ = lResp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode ollama response: %w", err)
		}
		results[i] = L2Normalize(lOut.Embedding)
	}

	return results, nil
}

// OpenAIEmbedder connects to OpenAI or any OpenAI-compatible API (e.g. LocalAI, vLLM, LM Studio).
type OpenAIEmbedder struct {
	client    *http.Client
	endpoint  string
	model     string
	apiKey    string
	dimension int
}

// NewOpenAIEmbedder creates an embedder using OpenAI-compatible /v1/embeddings API.
func NewOpenAIEmbedder(endpoint, model, apiKey string, dimension int) *OpenAIEmbedder {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8000/v1"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if model == "" {
		model = "text-embedding-3-small"
	}
	if dimension <= 0 {
		dimension = 1536
	}
	return &OpenAIEmbedder{
		client:    &http.Client{Timeout: 15 * time.Second},
		endpoint:  endpoint,
		model:     model,
		apiKey:    apiKey,
		dimension: dimension,
	}
}

func (e *OpenAIEmbedder) SetHTTPClient(c *http.Client) {
	if c != nil {
		e.client = c
	}
}

func (e *OpenAIEmbedder) Dimension() int {
	return e.dimension
}

func (e *OpenAIEmbedder) ModelName() string {
	return "openai:" + e.model
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	url := fmt.Sprintf("%s/embeddings", e.endpoint)
	body, err := json.Marshal(openAIEmbedRequest{
		Model: e.model,
		Input: texts,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai embeddings request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var res openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}

	if len(res.Data) != len(texts) {
		return nil, errors.New("openai response data length mismatch")
	}

	results := make([][]float32, len(texts))
	for _, d := range res.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			continue
		}
		results[d.Index] = L2Normalize(d.Embedding)
	}
	return results, nil
}

// MockEmbedder provides fixed or configurable embeddings for deterministic unit testing.
type MockEmbedder struct {
	DimensionValue int
	ModelNameValue string
	EmbedFunc      func(ctx context.Context, texts []string) ([][]float32, error)
}

func (m *MockEmbedder) Dimension() int {
	if m.DimensionValue <= 0 {
		return 4
	}
	return m.DimensionValue
}

func (m *MockEmbedder) ModelName() string {
	if m.ModelNameValue == "" {
		return "mock-embedder"
	}
	return m.ModelNameValue
}

func (m *MockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, texts)
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.Dimension())
		for j := range vec {
			vec[j] = float32(i+1) * 0.1
		}
		out[i] = L2Normalize(vec)
	}
	return out, nil
}
