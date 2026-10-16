package storage

import (
	"sort"
	"sync"
)

type Memory struct {
	mu          sync.RWMutex
	transcripts []Transcript
	vocabulary  map[string]string
}

func NewMemory() *Memory {
	return &Memory{vocabulary: map[string]string{}}
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
