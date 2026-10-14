package storage

import "time"

type Transcript struct {
	ID          string
	RawText     string
	Text        string
	Language    string
	Mode        string
	Application string
	CreatedAt   time.Time
}

type VocabularyEntry struct {
	Spoken    string
	Canonical string
}

type Store interface {
	SaveTranscript(Transcript) error
	ListTranscripts(int) ([]Transcript, error)
	SetVocabulary(VocabularyEntry) error
	Vocabulary() ([]VocabularyEntry, error)
}
