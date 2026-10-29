package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type migration struct {
	version int
	name    string
	up      string
}

var migrations = []migration{
	{
		version: 1,
		name:    "create_initial_schema",
		up: `
CREATE TABLE IF NOT EXISTS transcripts (
	id TEXT PRIMARY KEY,
	raw_text TEXT NOT NULL,
	text TEXT NOT NULL,
	language TEXT NOT NULL,
	mode TEXT NOT NULL,
	application TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transcripts_created_at ON transcripts (created_at DESC);

CREATE TABLE IF NOT EXISTS vocabulary (
	spoken TEXT PRIMARY KEY,
	canonical TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL
);
`,
	},
}

// SQLite implements persistent Store backed by an SQLite database.
type SQLite struct {
	db *sql.DB
}

// Open opens an existing SQLite database or creates a new one at the specified path.
// It applies any pending schema migrations and configures write-ahead logging.
func Open(path string) (*SQLite, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create storage directory: %w", err)
		}
	}

	dsn := path
	if path != ":memory:" && !strings.Contains(path, "?") {
		dsn = path + "?_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable wal mode: %w", err)
		}
		if _, err := db.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set synchronous normal: %w", err)
		}
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return s, nil
}

func (s *SQLite) migrate() error {
	const initMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TIMESTAMP NOT NULL
);
`
	if _, err := s.db.Exec(initMigrationsTable); err != nil {
		return err
	}

	for _, m := range migrations {
		var count int
		err := s.db.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", m.version).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(m.up); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			m.version, m.name, time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// SaveTranscript inserts or updates a transcript record.
func (s *SQLite) SaveTranscript(t Transcript) error {
	if t.ID == "" {
		return errors.New("transcript id cannot be empty")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}

	const query = `
INSERT INTO transcripts (id, raw_text, text, language, mode, application, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	raw_text = excluded.raw_text,
	text = excluded.text,
	language = excluded.language,
	mode = excluded.mode,
	application = excluded.application,
	created_at = excluded.created_at;
`
	_, err := s.db.Exec(query, t.ID, t.RawText, t.Text, t.Language, t.Mode, t.Application, t.CreatedAt.UTC())
	return err
}

// ListTranscripts returns the most recent transcripts ordered newest first.
func (s *SQLite) ListTranscripts(limit int) ([]Transcript, error) {
	if limit <= 0 {
		limit = -1
	}

	const query = `
SELECT id, raw_text, text, language, mode, application, created_at
FROM transcripts
ORDER BY created_at DESC
LIMIT ?;
`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Transcript
	for rows.Next() {
		var t Transcript
		if err := rows.Scan(&t.ID, &t.RawText, &t.Text, &t.Language, &t.Mode, &t.Application, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// GetTranscript retrieves a single transcript by session ID.
func (s *SQLite) GetTranscript(id string) (*Transcript, error) {
	const query = `
SELECT id, raw_text, text, language, mode, application, created_at
FROM transcripts
WHERE id = ?;
`
	var t Transcript
	err := s.db.QueryRow(query, id).Scan(&t.ID, &t.RawText, &t.Text, &t.Language, &t.Mode, &t.Application, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SearchTranscripts searches transcripts by substring in text or raw text.
func (s *SQLite) SearchTranscripts(queryStr string, limit int) ([]Transcript, error) {
	if limit <= 0 {
		limit = 50
	}

	pattern := "%" + queryStr + "%"
	const query = `
SELECT id, raw_text, text, language, mode, application, created_at
FROM transcripts
WHERE text LIKE ? OR raw_text LIKE ?
ORDER BY created_at DESC
LIMIT ?;
`
	rows, err := s.db.Query(query, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Transcript
	for rows.Next() {
		var t Transcript
		if err := rows.Scan(&t.ID, &t.RawText, &t.Text, &t.Language, &t.Mode, &t.Application, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}

	return out, rows.Err()
}

// DeleteTranscript deletes a transcript record by its ID.
func (s *SQLite) DeleteTranscript(id string) error {
	_, err := s.db.Exec("DELETE FROM transcripts WHERE id = ?", id)
	return err
}

// SetVocabulary adds or updates a spoken-to-canonical replacement mapping.
func (s *SQLite) SetVocabulary(e VocabularyEntry) error {
	if e.Spoken == "" {
		return errors.New("spoken phrase cannot be empty")
	}

	const query = `
INSERT INTO vocabulary (spoken, canonical, created_at)
VALUES (?, ?, ?)
ON CONFLICT(spoken) DO UPDATE SET
	canonical = excluded.canonical,
	created_at = excluded.created_at;
`
	_, err := s.db.Exec(query, e.Spoken, e.Canonical, time.Now().UTC())
	return err
}

// DeleteVocabulary removes a spoken phrase from the vocabulary table.
func (s *SQLite) DeleteVocabulary(spoken string) error {
	_, err := s.db.Exec("DELETE FROM vocabulary WHERE spoken = ?", spoken)
	return err
}

// Vocabulary returns all vocabulary mappings ordered alphabetically by spoken term.
func (s *SQLite) Vocabulary() ([]VocabularyEntry, error) {
	const query = `
SELECT spoken, canonical
FROM vocabulary
ORDER BY spoken ASC;
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VocabularyEntry
	for rows.Next() {
		var e VocabularyEntry
		if err := rows.Scan(&e.Spoken, &e.Canonical); err != nil {
			return nil, err
		}
		out = append(out, e)
	}

	return out, rows.Err()
}

// Close releases the underlying SQLite database handle.
func (s *SQLite) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
