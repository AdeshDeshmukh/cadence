package polish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestBuiltins(t *testing.T) {
	builtins := Builtins()
	if len(builtins) != 5 {
		t.Fatalf("expected 5 builtins, got %d", len(builtins))
	}

	expected := map[string]bool{
		"general":  true,
		"code":     true,
		"medical":  true,
		"legal":    true,
		"academic": true,
	}

	for _, p := range builtins {
		if !expected[p.Name] {
			t.Errorf("unexpected profile: %s", p.Name)
		}
		if !p.IsBuiltin {
			t.Errorf("profile %s should have IsBuiltin=true", p.Name)
		}
		if p.SystemPrompt == "" {
			t.Errorf("profile %s missing system prompt", p.Name)
		}
	}
}

func TestProfileRegistry_ResolveAndList(t *testing.T) {
	store := storage.NewMemory()
	reg := NewProfileRegistry(store)

	// Resolve built-in
	codeProf := reg.Resolve("code")
	if codeProf.Name != "code" || !strings.Contains(codeProf.SystemPrompt, "technical coding") {
		t.Fatalf("unexpected code profile: %+v", codeProf)
	}

	// Resolve default fallback for unknown profile
	fallback := reg.Resolve("nonexistent")
	if fallback.Name != "general" {
		t.Fatalf("expected general fallback, got: %s", fallback.Name)
	}

	// List initially contains 5 builtins
	list := reg.List()
	if len(list) != 5 {
		t.Fatalf("expected 5 initial profiles, got %d", len(list))
	}

	// Add custom profile
	custom := Profile{
		Name:         "blog_post",
		Description:  "Casual first-person conversational blog style",
		SystemPrompt: "Write engaging casual blog posts.",
		Temperature:  0.4,
		CustomRules:  []string{"Use lively metaphors", "Short paragraphs"},
	}

	if err := reg.Set(custom); err != nil {
		t.Fatalf("Set custom profile: %v", err)
	}

	// Retrieve custom profile
	gotCustom, ok := reg.Get("blog_post")
	if !ok || gotCustom.Description != custom.Description || len(gotCustom.CustomRules) != 2 {
		t.Fatalf("Get custom profile failed: %+v", gotCustom)
	}

	// List now contains 6 profiles
	listAfter := reg.List()
	if len(listAfter) != 6 {
		t.Fatalf("expected 6 profiles after adding custom, got %d", len(listAfter))
	}

	// Delete custom profile
	if err := reg.Delete("blog_post"); err != nil {
		t.Fatalf("Delete custom profile: %v", err)
	}

	_, okAfterDel := reg.Get("blog_post")
	if okAfterDel {
		t.Fatalf("expected custom profile to be deleted")
	}
}

func TestProfileRegistry_OllamaIntegration(t *testing.T) {
	store := storage.NewMemory()
	reg := NewProfileRegistry(store)

	_ = reg.Set(Profile{
		Name:         "slack",
		Description:  "Friendly chat messages",
		SystemPrompt: "Format as concise Slack message with emojis.",
		CustomRules:  []string{"Use bullet points for lists"},
	})

	var receivedReq ollamaGenerateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)

		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{
			Response: "Polished message",
			Done:     true,
		})
	}))
	defer srv.Close()

	polisher := NewOllamaPolisher(
		WithOllamaEndpoint(srv.URL),
		WithOllamaProfiles(reg),
	)

	_, err := polisher.Polish(context.Background(), Request{
		Text:    "meeting tomorrow at 10",
		Profile: "slack",
	})
	if err != nil {
		t.Fatalf("Polish: %v", err)
	}

	if !strings.Contains(receivedReq.System, "Format as concise Slack message with emojis.") {
		t.Errorf("system prompt missing profile prompt: %s", receivedReq.System)
	}
	if !strings.Contains(receivedReq.System, "Use bullet points for lists") {
		t.Errorf("system prompt missing custom rule: %s", receivedReq.System)
	}
}

func TestProfileRegistry_OpenAIIntegration(t *testing.T) {
	store := storage.NewMemory()
	reg := NewProfileRegistry(store)

	var receivedReq openAIChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)

		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			Choices: []struct {
				Index   int               `json:"index"`
				Message openAIChatMessage `json:"message"`
			}{
				{Message: openAIChatMessage{Role: "assistant", Content: "Polished medical note"}},
			},
		})
	}))
	defer srv.Close()

	polisher := NewOpenAIPolisher(
		WithOpenAIEndpoint(srv.URL),
		WithOpenAIProfiles(reg),
	)

	_, err := polisher.Polish(context.Background(), Request{
		Text:    "patient has fever and cough",
		Profile: "medical",
	})
	if err != nil {
		t.Fatalf("Polish: %v", err)
	}

	systemPrompt := receivedReq.Messages[0].Content
	if !strings.Contains(systemPrompt, "clinical documentation cleanup engine") {
		t.Errorf("expected medical prompt, got: %s", systemPrompt)
	}
}
