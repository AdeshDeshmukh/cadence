package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// SemanticResult represents a matched item from semantic search or related retrieval.
type SemanticResult struct {
	ID        string                   `json:"id"`
	Type      storage.SearchResultType `json:"type"` // "transcript", "note", "meeting"
	Title     string                   `json:"title"`
	Snippet   string                   `json:"snippet"`
	Score     float64                  `json:"score"` // Cosine similarity (0.0 to 1.0)
	TargetID  string                   `json:"target_id"`
	ParentID  string                   `json:"parent_id,omitempty"`
	CreatedAt time.Time                `json:"created_at"`
}

// SemanticSearchOptions specifies filtering and score constraints.
type SemanticSearchOptions struct {
	Types    []storage.SearchResultType `json:"types,omitempty"`
	Limit    int                        `json:"limit,omitempty"`
	MinScore float64                    `json:"min_score,omitempty"`
}

// SemanticEngine indexes and searches transcripts, notes, and meeting segments using vector embeddings.
type SemanticEngine struct {
	store    storage.Store
	embedder Embedder
	mu       sync.RWMutex
}

// NewSemanticEngine constructs a new SemanticEngine.
func NewSemanticEngine(store storage.Store, embedder Embedder) *SemanticEngine {
	if embedder == nil {
		embedder = NewLocalHashEmbedder(384)
	}
	return &SemanticEngine{
		store:    store,
		embedder: embedder,
	}
}

// Embedder returns the active embedding provider.
func (e *SemanticEngine) Embedder() Embedder {
	return e.embedder
}

// IndexTranscript computes and saves an embedding vector for a transcription record.
func (e *SemanticEngine) IndexTranscript(ctx context.Context, t storage.Transcript) error {
	text := strings.TrimSpace(t.Text)
	if text == "" {
		text = strings.TrimSpace(t.RawText)
	}
	if text == "" {
		return nil
	}

	vecs, err := e.embedder.Embed(ctx, []string{text})
	if err != nil {
		return fmt.Errorf("embed transcript %s: %w", t.ID, err)
	}
	if len(vecs) == 0 {
		return nil
	}

	return e.store.SaveEmbedding(storage.EmbeddingRecord{
		TargetType: string(storage.SearchResultTranscript),
		TargetID:   t.ID,
		Model:      e.embedder.ModelName(),
		Embedding:  vecs[0],
		UpdatedAt:  time.Now(),
	})
}

// IndexNote computes and saves an embedding vector for a knowledge note.
func (e *SemanticEngine) IndexNote(ctx context.Context, n storage.Note) error {
	content := fmt.Sprintf("%s\n%s", n.Title, n.Content)
	if len(n.Tags) > 0 {
		content += "\nTags: " + strings.Join(n.Tags, ", ")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	vecs, err := e.embedder.Embed(ctx, []string{content})
	if err != nil {
		return fmt.Errorf("embed note %s: %w", n.ID, err)
	}
	if len(vecs) == 0 {
		return nil
	}

	return e.store.SaveEmbedding(storage.EmbeddingRecord{
		TargetType: string(storage.SearchResultNote),
		TargetID:   n.ID,
		Model:      e.embedder.ModelName(),
		Embedding:  vecs[0],
		UpdatedAt:  time.Now(),
	})
}

// IndexMeetingSegment computes and saves an embedding vector for a meeting dialogue segment.
func (e *SemanticEngine) IndexMeetingSegment(ctx context.Context, s storage.MeetingSegment) error {
	text := strings.TrimSpace(s.Text)
	if text == "" {
		return nil
	}
	if s.Speaker != "" {
		text = fmt.Sprintf("%s: %s", s.Speaker, text)
	}

	vecs, err := e.embedder.Embed(ctx, []string{text})
	if err != nil {
		return fmt.Errorf("embed meeting segment %s: %w", s.ID, err)
	}
	if len(vecs) == 0 {
		return nil
	}

	return e.store.SaveEmbedding(storage.EmbeddingRecord{
		TargetType: string(storage.SearchResultMeeting),
		TargetID:   s.ID,
		Model:      e.embedder.ModelName(),
		Embedding:  vecs[0],
		UpdatedAt:  time.Now(),
	})
}

// IndexAll scans all transcripts, notes, and meeting segments in the store,
// generating embeddings for any that have not yet been embedded with the current model.
func (e *SemanticEngine) IndexAll(ctx context.Context) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	model := e.embedder.ModelName()
	indexed := 0

	// 1. Existing embeddings lookup set
	existing, err := e.store.ListEmbeddings("", model)
	if err != nil {
		return 0, fmt.Errorf("list existing embeddings: %w", err)
	}
	seen := make(map[string]bool, len(existing))
	for _, rec := range existing {
		seen[rec.TargetType+":"+rec.TargetID] = true
	}

	// 2. Transcripts
	transcripts, err := e.store.ListTranscripts(0)
	if err == nil {
		for _, t := range transcripts {
			if !seen[string(storage.SearchResultTranscript)+":"+t.ID] {
				if err := e.IndexTranscript(ctx, t); err == nil {
					indexed++
				}
			}
		}
	}

	// 3. Notes
	notes, err := e.store.ListNotes(storage.NoteFilter{})
	if err == nil {
		for _, n := range notes {
			if !seen[string(storage.SearchResultNote)+":"+n.ID] {
				if err := e.IndexNote(ctx, n); err == nil {
					indexed++
				}
			}
		}
	}

	// 4. Meeting segments
	meetings, err := e.store.ListMeetings(0)
	if err == nil {
		for _, m := range meetings {
			segs, err := e.store.ListMeetingSegments(m.ID)
			if err != nil {
				continue
			}
			for _, s := range segs {
				if !seen[string(storage.SearchResultMeeting)+":"+s.ID] {
					if err := e.IndexMeetingSegment(ctx, s); err == nil {
						indexed++
					}
				}
			}
		}
	}

	return indexed, nil
}

// Search executes semantic similarity search across stored entity embeddings.
func (e *SemanticEngine) Search(ctx context.Context, query string, opts SemanticSearchOptions) ([]SemanticResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SemanticResult{}, nil
	}

	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.MinScore <= 0 {
		opts.MinScore = 0.15
	}

	vecs, err := e.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed search query: %w", err)
	}
	if len(vecs) == 0 {
		return []SemanticResult{}, nil
	}
	queryVec := vecs[0]

	return e.searchWithVector(ctx, queryVec, "", "", opts)
}

// FindRelated locates transcripts, notes, or segments most semantically similar to a given entity.
func (e *SemanticEngine) FindRelated(ctx context.Context, targetType storage.SearchResultType, targetID string, limit int, minScore float64) ([]SemanticResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if minScore <= 0 {
		minScore = 0.20
	}

	model := e.embedder.ModelName()
	targetRec, err := e.store.GetEmbedding(string(targetType), targetID, model)
	if err != nil || targetRec == nil {
		// Attempt on-the-fly embedding of the target entity
		if err := e.ensureTargetEmbedded(ctx, targetType, targetID); err != nil {
			return nil, fmt.Errorf("embed target %s/%s: %w", targetType, targetID, err)
		}
		targetRec, err = e.store.GetEmbedding(string(targetType), targetID, model)
		if err != nil {
			return nil, fmt.Errorf("target embedding not found after index: %w", err)
		}
	}

	opts := SemanticSearchOptions{
		Limit:    limit,
		MinScore: minScore,
	}
	// Pass targetType and targetID to exclude self from related results
	return e.searchWithVector(ctx, targetRec.Embedding, string(targetType), targetID, opts)
}

func (e *SemanticEngine) ensureTargetEmbedded(ctx context.Context, targetType storage.SearchResultType, targetID string) error {
	switch targetType {
	case storage.SearchResultTranscript:
		tr, err := e.store.GetTranscript(targetID)
		if err != nil {
			return err
		}
		return e.IndexTranscript(ctx, *tr)
	case storage.SearchResultNote:
		n, err := e.store.GetNote(targetID)
		if err != nil {
			return err
		}
		return e.IndexNote(ctx, *n)
	case storage.SearchResultMeeting:
		// Target is a meeting segment
		// Search across meeting segments to find the segment
		meetings, err := e.store.ListMeetings(0)
		if err != nil {
			return err
		}
		for _, m := range meetings {
			segs, err := e.store.ListMeetingSegments(m.ID)
			if err != nil {
				continue
			}
			for _, s := range segs {
				if s.ID == targetID {
					return e.IndexMeetingSegment(ctx, s)
				}
			}
		}
		return fmt.Errorf("meeting segment %s not found", targetID)
	default:
		return fmt.Errorf("unsupported target type: %s", targetType)
	}
}

type scoredCandidate struct {
	rec   storage.EmbeddingRecord
	score float64
}

func (e *SemanticEngine) searchWithVector(
	ctx context.Context,
	queryVec []float32,
	excludeType string,
	excludeID string,
	opts SemanticSearchOptions,
) ([]SemanticResult, error) {
	model := e.embedder.ModelName()

	// Check allowed types
	typeAllowed := make(map[string]bool)
	if len(opts.Types) > 0 {
		for _, t := range opts.Types {
			typeAllowed[string(t)] = true
		}
	}

	records, err := e.store.ListEmbeddings("", model)
	if err != nil {
		return nil, fmt.Errorf("list embeddings: %w", err)
	}

	var candidates []scoredCandidate
	for _, rec := range records {
		if excludeType != "" && excludeID != "" {
			if rec.TargetType == excludeType && rec.TargetID == excludeID {
				continue
			}
		}
		if len(typeAllowed) > 0 && !typeAllowed[rec.TargetType] {
			continue
		}

		sim := CosineSimilarity(queryVec, rec.Embedding)
		if sim >= opts.MinScore {
			candidates = append(candidates, scoredCandidate{
				rec:   rec,
				score: sim,
			})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if opts.Limit > 0 && len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}

	// Hydrate result details
	results := make([]SemanticResult, 0, len(candidates))
	for _, cand := range candidates {
		res, err := e.hydrateResult(cand.rec, cand.score)
		if err == nil {
			results = append(results, res)
		}
	}

	return results, nil
}

func (e *SemanticEngine) hydrateResult(rec storage.EmbeddingRecord, score float64) (SemanticResult, error) {
	res := SemanticResult{
		ID:        rec.TargetID,
		Score:     score,
		TargetID:  rec.TargetID,
		CreatedAt: rec.UpdatedAt,
	}

	switch storage.SearchResultType(rec.TargetType) {
	case storage.SearchResultTranscript:
		res.Type = storage.SearchResultTranscript
		tr, err := e.store.GetTranscript(rec.TargetID)
		if err != nil {
			res.Title = fmt.Sprintf("Transcript %s", rec.TargetID)
			res.Snippet = ""
			return res, nil
		}
		res.CreatedAt = tr.CreatedAt
		text := tr.Text
		if text == "" {
			text = tr.RawText
		}
		res.Title = fmt.Sprintf("Transcript (%s)", tr.CreatedAt.Format("2006-01-02 15:04"))
		if len(text) > 160 {
			res.Snippet = text[:160] + "..."
		} else {
			res.Snippet = text
		}
		return res, nil

	case storage.SearchResultNote:
		res.Type = storage.SearchResultNote
		note, err := e.store.GetNote(rec.TargetID)
		if err != nil {
			res.Title = fmt.Sprintf("Note %s", rec.TargetID)
			return res, nil
		}
		res.CreatedAt = note.CreatedAt
		res.Title = note.Title
		snippet := note.Content
		if len(snippet) > 160 {
			snippet = snippet[:160] + "..."
		}
		res.Snippet = snippet
		return res, nil

	case storage.SearchResultMeeting:
		res.Type = storage.SearchResultMeeting
		// Find meeting segment
		meetings, err := e.store.ListMeetings(0)
		if err == nil {
			for _, m := range meetings {
				segs, err := e.store.ListMeetingSegments(m.ID)
				if err != nil {
					continue
				}
				for _, s := range segs {
					if s.ID == rec.TargetID {
						res.ParentID = m.ID
						res.CreatedAt = s.CreatedAt
						speaker := s.Speaker
						if speaker == "" {
							speaker = "Unknown"
						}
						res.Title = fmt.Sprintf("Meeting: %s (%s)", m.Title, speaker)
						snippet := s.Text
						if len(snippet) > 160 {
							snippet = snippet[:160] + "..."
						}
						res.Snippet = snippet
						return res, nil
					}
				}
			}
		}
		res.Title = fmt.Sprintf("Meeting Segment %s", rec.TargetID)
		return res, nil

	default:
		res.Type = storage.SearchResultType(rec.TargetType)
		res.Title = fmt.Sprintf("Entity %s", rec.TargetID)
		return res, nil
	}
}
