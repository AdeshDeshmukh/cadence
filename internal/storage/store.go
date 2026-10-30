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

type Store interface {
	SaveTranscript(Transcript) error
	ListTranscripts(int) ([]Transcript, error)
	SetVocabulary(VocabularyEntry) error
	Vocabulary() ([]VocabularyEntry, error)
}
