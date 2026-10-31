package storage

import (
	"errors"
	"sort"
	"sync"
	"time"
)

type Memory struct {
	mu          sync.RWMutex
	transcripts []Transcript
	vocabulary  map[string]string
	recoveries  map[string]SessionRecovery
}

func NewMemory() *Memory {
	return &Memory{
		vocabulary: map[string]string{},
		recoveries: map[string]SessionRecovery{},
	}
}

func (m *Memory) SaveTranscript(t Transcript) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transcripts = append(m.transcripts, t)
	return nil
}

func (m *Memory) ListTranscripts(limit int) ([]Transcript, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := append([]Transcript(nil), m.transcripts...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	if limit <= 0 || limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (m *Memory) SetVocabulary(e VocabularyEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vocabulary[e.Spoken] = e.Canonical
	return nil
}

func (m *Memory) Vocabulary() ([]VocabularyEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]VocabularyEntry, 0, len(m.vocabulary))
	for spoken, canonical := range m.vocabulary {
		out = append(out, VocabularyEntry{
			Spoken: spoken, Canonical: canonical,
		})
	}
	return out, nil
}

func (m *Memory) SaveRecovery(r SessionRecovery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == "" {
		return errors.New("recovery id cannot be empty")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now()
	}
	m.recoveries[r.ID] = r
	return nil
}

func (m *Memory) GetLastRecovery() (*SessionRecovery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []SessionRecovery
	for _, r := range m.recoveries {
		if r.State != "completed" && (r.Text != "" || r.RawText != "" || r.PartialText != "" || r.Error != "") {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
	})

	res := matches[0]
	return &res, nil
}

func (m *Memory) ListRecoveries(limit int) ([]SessionRecovery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []SessionRecovery
	for _, r := range m.recoveries {
		if r.State != "completed" && (r.Text != "" || r.RawText != "" || r.PartialText != "" || r.Error != "") {
			matches = append(matches, r)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
	})

	if limit <= 0 || limit > len(matches) {
		limit = len(matches)
	}
	return matches[:limit], nil
}

func (m *Memory) DeleteRecovery(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.recoveries, id)
	return nil
}

func (m *Memory) ClearRecoveries() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveries = make(map[string]SessionRecovery)
	return nil
}

