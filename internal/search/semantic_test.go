package search_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/search"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestVector_Math(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	c := []float32{0.0, 1.0, 0.0}
	d := []float32{-1.0, 0.0, 0.0}

	if sim := search.CosineSimilarity(a, b); sim < 0.99 {
		t.Fatalf("expected cosine similarity ~1.0 for identical vectors, got %f", sim)
	}
	if sim := search.CosineSimilarity(a, c); sim > 0.01 {
		t.Fatalf("expected cosine similarity ~0.0 for orthogonal vectors, got %f", sim)
	}
	if sim := search.CosineSimilarity(a, d); sim > -0.99 {
		t.Fatalf("expected cosine similarity ~-1.0 for opposite vectors, got %f", sim)
	}

	// L2Normalize
	unnorm := []float32{3.0, 4.0}
	normed := search.L2Normalize(unnorm)
	if search.L2Norm(normed) < 0.99 || search.L2Norm(normed) > 1.01 {
		t.Fatalf("expected L2 norm 1.0, got %f", search.L2Norm(normed))
	}
}

func TestLocalHashEmbedder_Semantics(t *testing.T) {
	embedder := search.NewLocalHashEmbedder(384)
	if embedder.Dimension() != 384 {
		t.Fatalf("expected dim 384, got %d", embedder.Dimension())
	}
	if embedder.ModelName() != "cadence-hash-v1" {
		t.Fatalf("expected model cadence-hash-v1, got %s", embedder.ModelName())
	}

	ctx := context.Background()
	texts := []string{
		"deploying a kubernetes cluster into production",
		"production kubernetes cluster deployment",
		"homemade strawberry gelato ice cream recipe with fresh milk",
	}

	vecs, err := embedder.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}

	simRelated := search.CosineSimilarity(vecs[0], vecs[1])
	simUnrelated := search.CosineSimilarity(vecs[0], vecs[2])

	t.Logf("Related sim: %f, Unrelated sim: %f", simRelated, simUnrelated)
	if simRelated < 0.35 {
		t.Fatalf("expected high similarity (> 0.35) for related k8s texts, got %f", simRelated)
	}
	if simUnrelated > 0.20 {
		t.Fatalf("expected low similarity (< 0.20) for k8s vs gelato, got %f", simUnrelated)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestOllamaEmbedder_HTTP(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/embed" {
				rec := httptest.NewRecorder()
				rec.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(rec).Encode(map[string]any{
					"embeddings": [][]float32{
						{0.5, 0.5, 0.5, 0.5},
					},
				})
				return rec.Result(), nil
			}
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusNotFound)
			return rec.Result(), nil
		}),
	}

	embedder := search.NewOllamaEmbedder("http://mock-ollama:11434", "test-model", 4)
	embedder.SetHTTPClient(mockClient)

	vecs, err := embedder.Embed(context.Background(), []string{"test sentence"})
	if err != nil {
		t.Fatalf("ollama embed failed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 4 {
		t.Fatalf("unexpected vecs returned: %+v", vecs)
	}
}

func TestOpenAIEmbedder_HTTP(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/embeddings") {
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("expected Authorization Bearer test-key, got %s", r.Header.Get("Authorization"))
				}
				rec := httptest.NewRecorder()
				rec.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(rec).Encode(map[string]any{
					"data": []map[string]any{
						{
							"index":     0,
							"embedding": []float32{1.0, 0.0, 0.0, 0.0},
						},
					},
				})
				return rec.Result(), nil
			}
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusNotFound)
			return rec.Result(), nil
		}),
	}

	embedder := search.NewOpenAIEmbedder("http://mock-openai:8000/v1", "text-embedding-3-small", "test-key", 4)
	embedder.SetHTTPClient(mockClient)

	vecs, err := embedder.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("openai embed failed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 4 {
		t.Fatalf("unexpected vecs: %+v", vecs)
	}
}

func TestSemanticEngine_SearchAndRelated(t *testing.T) {
	mem := storage.NewMemory()
	embedder := search.NewLocalHashEmbedder(384)
	engine := search.NewSemanticEngine(mem, embedder)

	now := time.Now()

	// 1. Save transcripts
	t1 := storage.Transcript{
		ID:        "tr-k8s-1",
		Text:      "We configured Kubernetes Helm charts and ingress controllers for production.",
		RawText:   "we configured kubernetes helm charts",
		CreatedAt: now.Add(-10 * time.Minute),
	}
	t2 := storage.Transcript{
		ID:        "tr-k8s-2",
		Text:      "Scaling Kubernetes deployment replicas and checking cluster pods health.",
		RawText:   "scaling kubernetes deployment replicas",
		CreatedAt: now.Add(-5 * time.Minute),
	}
	t3 := storage.Transcript{
		ID:        "tr-food-1",
		Text:      "Baking homemade sourdough bread with organic flour and wild yeast starter.",
		RawText:   "baking homemade sourdough bread",
		CreatedAt: now,
	}

	_ = mem.SaveTranscript(t1)
	_ = mem.SaveTranscript(t2)
	_ = mem.SaveTranscript(t3)

	// 2. Save a knowledge note
	note1 := storage.Note{
		ID:        "note-k8s",
		Title:     "Kubernetes Production Runbook",
		Content:   "Steps to upgrade Kubernetes cluster worker nodes and manage Helm releases safely.",
		Tags:      []string{"devops", "kubernetes"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = mem.SaveNote(note1)

	// 3. IndexAll
	ctx := context.Background()
	indexedCount, err := engine.IndexAll(ctx)
	if err != nil {
		t.Fatalf("IndexAll failed: %v", err)
	}
	if indexedCount != 4 { // 3 transcripts + 1 note
		t.Fatalf("expected 4 items indexed, got %d", indexedCount)
	}

	// 4. Semantic search for "Kubernetes deployment and pods"
	results, err := engine.Search(ctx, "Kubernetes deployment and pods", search.SemanticSearchOptions{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search hits for kubernetes query")
	}

	// Ensure t2 and note1 or t1 are in top results and sourdough is NOT top
	t.Logf("Search top hit: %s (%s, score=%.4f): %s", results[0].ID, results[0].Type, results[0].Score, results[0].Snippet)
	if results[0].ID == "tr-food-1" {
		t.Fatalf("sourdough should not be the top hit for kubernetes query")
	}

	// 5. Test FindRelated for transcript tr-k8s-1
	related, err := engine.FindRelated(ctx, storage.SearchResultTranscript, "tr-k8s-1", 5, 0.15)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}
	if len(related) == 0 {
		t.Fatalf("expected related items for tr-k8s-1")
	}

	// Ensure self is excluded
	for _, r := range related {
		if r.ID == "tr-k8s-1" {
			t.Fatalf("self should not be returned in FindRelated: %+v", r)
		}
	}

	// Top related should be tr-k8s-2 or note-k8s
	foundRelatedK8s := false
	for _, r := range related {
		if r.ID == "tr-k8s-2" || r.ID == "note-k8s" {
			foundRelatedK8s = true
			break
		}
	}
	if !foundRelatedK8s {
		t.Fatalf("expected tr-k8s-2 or note-k8s in related results, got: %+v", related)
	}

	// 6. Test FindRelated on an unindexed item (auto-indexing on the fly)
	tUnindexed := storage.Transcript{
		ID:        "tr-new-unindexed",
		Text:      "Monitoring Kubernetes CPU metrics and cluster alerts with Prometheus.",
		CreatedAt: now,
	}
	_ = mem.SaveTranscript(tUnindexed)

	autoRelated, err := engine.FindRelated(ctx, storage.SearchResultTranscript, "tr-new-unindexed", 5, 0.10)
	if err != nil {
		t.Fatalf("FindRelated on unindexed item failed: %v", err)
	}
	if len(autoRelated) == 0 {
		t.Fatalf("expected related hits after auto-indexing")
	}
}
