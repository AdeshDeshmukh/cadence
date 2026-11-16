package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/search"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// IPCClient defines the contract needed to communicate with Cadence daemon.
type IPCClient interface {
	Call(ctx context.Context, method string, params any) (any, error)
}

// Provider abstracts the data source (in-process storage or daemon IPC) powering the MCP server.
type Provider interface {
	GetHistory(ctx context.Context, limit int, kind string) ([]storage.Transcript, error)
	GetVocabulary(ctx context.Context, filter string) ([]storage.VocabularyEntry, error)
	AddVocabulary(ctx context.Context, spoken, canonical string) error
	GetSnippets(ctx context.Context, query string) ([]storage.Snippet, error)
	GetContext(ctx context.Context, includeEditor, includeTerminal bool) (map[string]any, error)
	SearchKnowledge(ctx context.Context, query string, semantic bool, limit int) ([]storage.SearchResult, error)
	GetNotes(ctx context.Context, limit int) ([]storage.Note, error)
	GetNote(ctx context.Context, id string) (*storage.Note, error)
}

// DirectProvider accesses local storage, context managers, and search engine directly.
type DirectProvider struct {
	store           storage.Store
	editorContext   *appcontext.EditorContextManager
	terminalContext *appcontext.TerminalContextManager
	appDetector     appcontext.Detector
	semanticEngine  *search.SemanticEngine
}

// NewDirectProvider constructs a Provider backed by local in-process components.
func NewDirectProvider(
	store storage.Store,
	editorCtx *appcontext.EditorContextManager,
	termCtx *appcontext.TerminalContextManager,
	detector appcontext.Detector,
	semantic *search.SemanticEngine,
) *DirectProvider {
	return &DirectProvider{
		store:           store,
		editorContext:   editorCtx,
		terminalContext: termCtx,
		appDetector:     detector,
		semanticEngine:  semantic,
	}
}

func (p *DirectProvider) GetHistory(ctx context.Context, limit int, kind string) ([]storage.Transcript, error) {
	if p.store == nil {
		return nil, errors.New("store not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return p.store.ListTranscripts(limit)
}

func (p *DirectProvider) GetVocabulary(ctx context.Context, filter string) ([]storage.VocabularyEntry, error) {
	if p.store == nil {
		return nil, errors.New("store not configured")
	}
	entries, err := p.store.Vocabulary()
	if err != nil {
		return nil, err
	}
	if filter == "" {
		return entries, nil
	}

	filterLower := strings.ToLower(strings.TrimSpace(filter))
	filtered := make([]storage.VocabularyEntry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Spoken), filterLower) ||
			strings.Contains(strings.ToLower(e.Canonical), filterLower) {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}

func (p *DirectProvider) AddVocabulary(ctx context.Context, spoken, canonical string) error {
	if p.store == nil {
		return errors.New("store not configured")
	}
	spoken = strings.TrimSpace(spoken)
	canonical = strings.TrimSpace(canonical)
	if spoken == "" || canonical == "" {
		return errors.New("spoken and canonical terms must not be empty")
	}
	return p.store.SetVocabulary(storage.VocabularyEntry{
		Spoken:    spoken,
		Canonical: canonical,
	})
}

func (p *DirectProvider) GetSnippets(ctx context.Context, query string) ([]storage.Snippet, error) {
	if p.store == nil {
		return nil, errors.New("store not configured")
	}
	snippets, err := p.store.ListSnippets()
	if err != nil {
		return nil, err
	}
	if query == "" {
		return snippets, nil
	}

	qLower := strings.ToLower(strings.TrimSpace(query))
	filtered := make([]storage.Snippet, 0, len(snippets))
	for _, s := range snippets {
		if strings.Contains(strings.ToLower(s.Trigger), qLower) ||
			strings.Contains(strings.ToLower(s.Description), qLower) ||
			strings.Contains(strings.ToLower(s.Template), qLower) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

func (p *DirectProvider) GetContext(ctx context.Context, includeEditor, includeTerminal bool) (map[string]any, error) {
	res := make(map[string]any)

	if p.appDetector != nil {
		if info, err := p.appDetector.Detect(ctx); err == nil {
			res["active_app"] = info
		}
	}

	if includeEditor && p.editorContext != nil {
		if ed, ok := p.editorContext.Get(); ok {
			res["editor"] = ed
		}
	}

	if includeTerminal && p.terminalContext != nil {
		if term, ok := p.terminalContext.Get(); ok {
			res["terminal"] = term
		}
	}

	res["timestamp"] = time.Now().UTC()
	return res, nil
}

func (p *DirectProvider) SearchKnowledge(ctx context.Context, query string, semantic bool, limit int) ([]storage.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	if semantic && p.semanticEngine != nil {
		semHits, err := p.semanticEngine.Search(ctx, query, search.SemanticSearchOptions{
			Limit: limit,
		})
		if err == nil && len(semHits) > 0 {
			results := make([]storage.SearchResult, 0, len(semHits))
			for _, hit := range semHits {
				results = append(results, storage.SearchResult{
					ID:        hit.ID,
					Type:      hit.Type,
					Title:     hit.Title,
					Snippet:   hit.Snippet,
					Rank:      hit.Score,
					CreatedAt: hit.CreatedAt,
				})
			}
			return results, nil
		}
	}

	if p.store == nil {
		return nil, errors.New("store not configured")
	}

	return p.store.Search(query, storage.SearchFilter{
		Limit: limit,
	})
}

func (p *DirectProvider) GetNotes(ctx context.Context, limit int) ([]storage.Note, error) {
	if p.store == nil {
		return nil, errors.New("store not configured")
	}
	notes, err := p.store.ListNotes(storage.NoteFilter{})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(notes) > limit {
		return notes[:limit], nil
	}
	return notes, nil
}

func (p *DirectProvider) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	if p.store == nil {
		return nil, errors.New("store not configured")
	}
	return p.store.GetNote(id)
}

// IPCProvider retrieves data from a running Cadence daemon over UNIX socket IPC.
type IPCProvider struct {
	client IPCClient
}

// NewIPCProvider wraps an existing daemon IPC client.
func NewIPCProvider(client IPCClient) *IPCProvider {
	return &IPCProvider{client: client}
}

func (p *IPCProvider) GetHistory(ctx context.Context, limit int, kind string) ([]storage.Transcript, error) {
	resp, err := p.client.Call(ctx, "history", map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	var out []storage.Transcript
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *IPCProvider) GetVocabulary(ctx context.Context, filter string) ([]storage.VocabularyEntry, error) {
	resp, err := p.client.Call(ctx, "vocabulary", nil)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	var entries []storage.VocabularyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if filter == "" {
		return entries, nil
	}
	filterLower := strings.ToLower(strings.TrimSpace(filter))
	filtered := make([]storage.VocabularyEntry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Spoken), filterLower) ||
			strings.Contains(strings.ToLower(e.Canonical), filterLower) {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}

func (p *IPCProvider) AddVocabulary(ctx context.Context, spoken, canonical string) error {
	_, err := p.client.Call(ctx, "vocab_add", map[string]any{
		"spoken":    spoken,
		"canonical": canonical,
	})
	return err
}

func (p *IPCProvider) GetSnippets(ctx context.Context, query string) ([]storage.Snippet, error) {
	resp, err := p.client.Call(ctx, "snippet_list", nil)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	var snippets []storage.Snippet
	if err := json.Unmarshal(data, &snippets); err != nil {
		return nil, err
	}
	if query == "" {
		return snippets, nil
	}
	qLower := strings.ToLower(strings.TrimSpace(query))
	filtered := make([]storage.Snippet, 0, len(snippets))
	for _, s := range snippets {
		if strings.Contains(strings.ToLower(s.Trigger), qLower) ||
			strings.Contains(strings.ToLower(s.Description), qLower) ||
			strings.Contains(strings.ToLower(s.Template), qLower) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

func (p *IPCProvider) GetContext(ctx context.Context, includeEditor, includeTerminal bool) (map[string]any, error) {
	res := make(map[string]any)

	if appInfo, err := p.client.Call(ctx, "active_app", nil); err == nil {
		res["active_app"] = appInfo
	}

	if includeEditor {
		if edCtx, err := p.client.Call(ctx, "editor_context_get", nil); err == nil {
			res["editor"] = edCtx
		}
	}

	if includeTerminal {
		if termCtx, err := p.client.Call(ctx, "terminal_context_get", nil); err == nil {
			res["terminal"] = termCtx
		}
	}

	res["timestamp"] = time.Now().UTC()
	return res, nil
}

func (p *IPCProvider) SearchKnowledge(ctx context.Context, query string, semantic bool, limit int) ([]storage.SearchResult, error) {
	params := map[string]any{
		"query":    query,
		"semantic": semantic,
		"limit":    limit,
	}
	resp, err := p.client.Call(ctx, "search", params)
	if err != nil {
		return nil, err
	}
	resMap, ok := resp.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected search response format: %v", resp)
	}
	resultsRaw := resMap["results"]
	data, err := json.Marshal(resultsRaw)
	if err != nil {
		return nil, err
	}
	var results []storage.SearchResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (p *IPCProvider) GetNotes(ctx context.Context, limit int) ([]storage.Note, error) {
	resp, err := p.client.Call(ctx, "note_list", map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	var notes []storage.Note
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

func (p *IPCProvider) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	resp, err := p.client.Call(ctx, "note_get", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	var note storage.Note
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, err
	}
	return &note, nil
}

// AutoProvider checks daemon connectivity and falls back to DirectProvider if offline.
type AutoProvider struct {
	ipc    *IPCProvider
	direct *DirectProvider
}

// NewAutoProvider creates a hybrid provider that prefers IPC when daemon is available.
func NewAutoProvider(ipcProv *IPCProvider, directProv *DirectProvider) *AutoProvider {
	return &AutoProvider{
		ipc:    ipcProv,
		direct: directProv,
	}
}

func (a *AutoProvider) active(ctx context.Context) Provider {
	if a.ipc != nil && a.ipc.client != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		if _, err := a.ipc.client.Call(pingCtx, "ping", nil); err == nil {
			return a.ipc
		}
	}
	return a.direct
}

func (a *AutoProvider) GetHistory(ctx context.Context, limit int, kind string) ([]storage.Transcript, error) {
	return a.active(ctx).GetHistory(ctx, limit, kind)
}

func (a *AutoProvider) GetVocabulary(ctx context.Context, filter string) ([]storage.VocabularyEntry, error) {
	return a.active(ctx).GetVocabulary(ctx, filter)
}

func (a *AutoProvider) AddVocabulary(ctx context.Context, spoken, canonical string) error {
	return a.active(ctx).AddVocabulary(ctx, spoken, canonical)
}

func (a *AutoProvider) GetSnippets(ctx context.Context, query string) ([]storage.Snippet, error) {
	return a.active(ctx).GetSnippets(ctx, query)
}

func (a *AutoProvider) GetContext(ctx context.Context, includeEditor, includeTerminal bool) (map[string]any, error) {
	return a.active(ctx).GetContext(ctx, includeEditor, includeTerminal)
}

func (a *AutoProvider) SearchKnowledge(ctx context.Context, query string, semantic bool, limit int) ([]storage.SearchResult, error) {
	return a.active(ctx).SearchKnowledge(ctx, query, semantic, limit)
}

func (a *AutoProvider) GetNotes(ctx context.Context, limit int) ([]storage.Note, error) {
	return a.active(ctx).GetNotes(ctx, limit)
}

func (a *AutoProvider) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	return a.active(ctx).GetNote(ctx, id)
}
