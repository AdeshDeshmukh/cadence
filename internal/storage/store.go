package storage

import "time"

type Transcript struct {
	ID          string    `json:"id"`
	RawText     string    `json:"raw_text"`
	Text        string    `json:"text"`
	Language    string    `json:"language"`
	Mode        string    `json:"mode"`
	Application string    `json:"application"`
	CreatedAt   time.Time `json:"created_at"`
}

type VocabularyEntry struct {
	Spoken    string `json:"spoken"`
	Canonical string `json:"canonical"`
}

// Snippet defines a voice-triggered template with dynamic variables.
type Snippet struct {
	Trigger     string    `json:"trigger"`
	Template    string    `json:"template"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionRecovery holds state and interim text from cancelled or failed dictation sessions.
type SessionRecovery struct {
	ID           string    `json:"id"`
	State        string    `json:"state"` // "cancelled", "failed", "uncommitted", "completed"
	PartialText  string    `json:"partial_text"`
	RawText      string    `json:"raw_text"`
	Text         string    `json:"text"`
	AudioSamples int       `json:"audio_samples"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BestText returns the most complete text available in the recovery record.
func (r SessionRecovery) BestText() string {
	if r.Text != "" {
		return r.Text
	}
	if r.RawText != "" {
		return r.RawText
	}
	return r.PartialText
}

type Store interface {
	SaveTranscript(Transcript) error
	ListTranscripts(int) ([]Transcript, error)
	SetVocabulary(VocabularyEntry) error
	Vocabulary() ([]VocabularyEntry, error)
	DeleteVocabulary(string) error
	SetSnippet(Snippet) error
	GetSnippet(string) (*Snippet, error)
	ListSnippets() ([]Snippet, error)
	DeleteSnippet(string) error
	SaveRecovery(SessionRecovery) error
	GetLastRecovery() (*SessionRecovery, error)
	ListRecoveries(int) ([]SessionRecovery, error)
	DeleteRecovery(string) error
	ClearRecoveries() error
}
