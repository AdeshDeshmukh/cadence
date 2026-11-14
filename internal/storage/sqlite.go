package storage

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
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
	{
		version: 2,
		name:    "create_session_recoveries",
		up: `
CREATE TABLE IF NOT EXISTS session_recoveries (
	id TEXT PRIMARY KEY,
	state TEXT NOT NULL,
	partial_text TEXT NOT NULL DEFAULT '',
	raw_text TEXT NOT NULL DEFAULT '',
	text TEXT NOT NULL DEFAULT '',
	audio_samples INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_recoveries_updated_at ON session_recoveries (updated_at DESC);
`,
	},
	{
		version: 3,
		name:    "create_snippets",
		up: `
CREATE TABLE IF NOT EXISTS snippets (
	trigger TEXT PRIMARY KEY,
	template TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snippets_trigger ON snippets (trigger);
`,
	},
	{
		version: 4,
		name:    "create_polish_profiles",
		up: `
CREATE TABLE IF NOT EXISTS polish_profiles (
	name TEXT PRIMARY KEY,
	description TEXT NOT NULL DEFAULT '',
	system_prompt TEXT NOT NULL,
	temperature REAL NOT NULL DEFAULT 0.2,
	custom_rules TEXT NOT NULL DEFAULT '[]',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_polish_profiles_name ON polish_profiles (name);
`,
	},
	{
		version: 5,
		name:    "create_meetings",
		up: `
CREATE TABLE IF NOT EXISTS meetings (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	state TEXT NOT NULL,
	audio_path TEXT NOT NULL DEFAULT '',
	sample_rate INTEGER NOT NULL DEFAULT 16000,
	created_at TIMESTAMP NOT NULL,
	ended_at TIMESTAMP,
	duration_ms INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_meetings_created_at ON meetings (created_at DESC);

CREATE TABLE IF NOT EXISTS meeting_segments (
	id TEXT PRIMARY KEY,
	meeting_id TEXT NOT NULL,
	segment_index INTEGER NOT NULL,
	start_ms INTEGER NOT NULL,
	end_ms INTEGER NOT NULL,
	text TEXT NOT NULL DEFAULT '',
	speaker TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	FOREIGN KEY(meeting_id) REFERENCES meetings(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_meeting_segments_meeting_id ON meeting_segments (meeting_id, segment_index ASC);
`,
	},
	{
		version: 6,
		name:    "create_meeting_speakers",
		up: `
CREATE TABLE IF NOT EXISTS meeting_speakers (
	meeting_id TEXT NOT NULL,
	speaker_id TEXT NOT NULL,
	name TEXT NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	PRIMARY KEY (meeting_id, speaker_id),
	FOREIGN KEY(meeting_id) REFERENCES meetings(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_meeting_speakers_meeting_id ON meeting_speakers (meeting_id);
`,
	},
	{
		version: 7,
		name:    "create_meeting_summaries",
		up: `
CREATE TABLE IF NOT EXISTS meeting_summaries (
	meeting_id TEXT PRIMARY KEY,
	overview TEXT NOT NULL DEFAULT '',
	decisions TEXT NOT NULL DEFAULT '[]',
	action_items TEXT NOT NULL DEFAULT '[]',
	topics TEXT NOT NULL DEFAULT '[]',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	FOREIGN KEY(meeting_id) REFERENCES meetings(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_meeting_summaries_meeting_id ON meeting_summaries (meeting_id);
`,
	},
	{
		version: 8,
		name:    "create_knowledge_base",
		up: `
CREATE TABLE IF NOT EXISTS folders (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	parent_id TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON folders (parent_id);
CREATE INDEX IF NOT EXISTS idx_folders_name ON folders (name);

CREATE TABLE IF NOT EXISTS notes (
	id TEXT PRIMARY KEY,
	folder_id TEXT,
	title TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL DEFAULT '',
	meeting_id TEXT,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	FOREIGN KEY(folder_id) REFERENCES folders(id) ON DELETE SET NULL,
	FOREIGN KEY(meeting_id) REFERENCES meetings(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_folder_id ON notes (folder_id);
CREATE INDEX IF NOT EXISTS idx_notes_meeting_id ON notes (meeting_id);
CREATE INDEX IF NOT EXISTS idx_notes_updated_at ON notes (updated_at DESC);

CREATE TABLE IF NOT EXISTS tags (
	name TEXT PRIMARY KEY,
	created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS note_tags (
	note_id TEXT NOT NULL,
	tag_name TEXT NOT NULL,
	PRIMARY KEY (note_id, tag_name),
	FOREIGN KEY(note_id) REFERENCES notes(id) ON DELETE CASCADE,
	FOREIGN KEY(tag_name) REFERENCES tags(name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_note_tags_note_id ON note_tags (note_id);
CREATE INDEX IF NOT EXISTS idx_note_tags_tag_name ON note_tags (tag_name);
`,
	},
	{
		version: 9,
		name:    "create_fts5_indexes",
		up: `
CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
	id UNINDEXED,
	title,
	content
);

CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
	INSERT INTO notes_fts(id, title, content) VALUES (new.id, new.title, new.content);
END;
CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
	DELETE FROM notes_fts WHERE id = old.id;
END;
CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
	DELETE FROM notes_fts WHERE id = old.id;
	INSERT INTO notes_fts(id, title, content) VALUES (new.id, new.title, new.content);
END;

CREATE VIRTUAL TABLE IF NOT EXISTS transcripts_fts USING fts5(
	id UNINDEXED,
	text,
	raw_text
);

CREATE TRIGGER IF NOT EXISTS transcripts_ai AFTER INSERT ON transcripts BEGIN
	INSERT INTO transcripts_fts(id, text, raw_text) VALUES (new.id, new.text, new.raw_text);
END;
CREATE TRIGGER IF NOT EXISTS transcripts_ad AFTER DELETE ON transcripts BEGIN
	DELETE FROM transcripts_fts WHERE id = old.id;
END;
CREATE TRIGGER IF NOT EXISTS transcripts_au AFTER UPDATE ON transcripts BEGIN
	DELETE FROM transcripts_fts WHERE id = old.id;
	INSERT INTO transcripts_fts(id, text, raw_text) VALUES (new.id, new.text, new.raw_text);
END;

CREATE VIRTUAL TABLE IF NOT EXISTS meeting_segments_fts USING fts5(
	id UNINDEXED,
	meeting_id UNINDEXED,
	speaker,
	text
);

CREATE TRIGGER IF NOT EXISTS meeting_segments_ai AFTER INSERT ON meeting_segments BEGIN
	INSERT INTO meeting_segments_fts(id, meeting_id, speaker, text) VALUES (new.id, new.meeting_id, new.speaker, new.text);
END;
CREATE TRIGGER IF NOT EXISTS meeting_segments_ad AFTER DELETE ON meeting_segments BEGIN
	DELETE FROM meeting_segments_fts WHERE id = old.id;
END;
CREATE TRIGGER IF NOT EXISTS meeting_segments_au AFTER UPDATE ON meeting_segments BEGIN
	DELETE FROM meeting_segments_fts WHERE id = old.id;
	INSERT INTO meeting_segments_fts(id, meeting_id, speaker, text) VALUES (new.id, new.meeting_id, new.speaker, new.text);
END;

INSERT INTO notes_fts(id, title, content) SELECT id, title, content FROM notes;
INSERT INTO transcripts_fts(id, text, raw_text) SELECT id, text, raw_text FROM transcripts;
INSERT INTO meeting_segments_fts(id, meeting_id, speaker, text) SELECT id, meeting_id, speaker, text FROM meeting_segments;
`,
	},
	{
		version: 10,
		name:    "create_embeddings_table",
		up: `
CREATE TABLE IF NOT EXISTS embeddings (
	target_type TEXT NOT NULL,
	target_id TEXT NOT NULL,
	model TEXT NOT NULL,
	embedding BLOB NOT NULL,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (target_type, target_id, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_type_model ON embeddings(target_type, model);

CREATE TRIGGER IF NOT EXISTS notes_embeddings_ad AFTER DELETE ON notes BEGIN
	DELETE FROM embeddings WHERE target_type = 'note' AND target_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS transcripts_embeddings_ad AFTER DELETE ON transcripts BEGIN
	DELETE FROM embeddings WHERE target_type = 'transcript' AND target_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS meeting_segments_embeddings_ad AFTER DELETE ON meeting_segments BEGIN
	DELETE FROM embeddings WHERE target_type = 'meeting_segment' AND target_id = old.id;
END;
`,
	},
	{
		version: 11,
		name:    "create_plugins_table",
		up: `
CREATE TABLE IF NOT EXISTS plugins (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	install_dir TEXT NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT 1,
	manifest_json TEXT NOT NULL,
	installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plugins_enabled ON plugins(enabled);
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
	if path != ":memory:" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		dsn = fmt.Sprintf("%s%s_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON&_txlock=immediate", path, sep)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if path == ":memory:" {
		if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable foreign keys: %w", err)
		}
	}

	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return s, nil
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "busy") || strings.Contains(msg, "locked")
}

func (s *SQLite) migrate() error {
	const initMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TIMESTAMP NOT NULL
);
`
	var lastErr error
	const maxRetries = 50
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := s.tryMigrate(initMigrationsTable)
		if err == nil {
			return nil
		}
		lastErr = err
		if isBusy(err) && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10+attempt*10) * time.Millisecond)
			continue
		}
		return err
	}
	return lastErr
}

func (s *SQLite) tryMigrate(initMigrationsTable string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(initMigrationsTable); err != nil {
		return fmt.Errorf("init schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var count int
		err := tx.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", m.version).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		if _, err := tx.Exec(m.up); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			m.version, m.name, time.Now().UTC()); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
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

// SaveRecovery inserts or updates a session recovery record.
func (s *SQLite) SaveRecovery(r SessionRecovery) error {
	if r.ID == "" {
		return errors.New("recovery id cannot be empty")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now().UTC()
	}

	const query = `
INSERT INTO session_recoveries (id, state, partial_text, raw_text, text, audio_samples, error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	state = excluded.state,
	partial_text = excluded.partial_text,
	raw_text = excluded.raw_text,
	text = excluded.text,
	audio_samples = excluded.audio_samples,
	error = excluded.error,
	updated_at = excluded.updated_at;
`
	_, err := s.db.Exec(
		query,
		r.ID,
		r.State,
		r.PartialText,
		r.RawText,
		r.Text,
		r.AudioSamples,
		r.Error,
		r.CreatedAt.UTC(),
		r.UpdatedAt.UTC(),
	)
	return err
}

// GetLastRecovery retrieves the most recent recoverable session (cancelled or failed) with text.
func (s *SQLite) GetLastRecovery() (*SessionRecovery, error) {
	const query = `
SELECT id, state, partial_text, raw_text, text, audio_samples, error, created_at, updated_at
FROM session_recoveries
WHERE state != 'completed' AND (text != '' OR raw_text != '' OR partial_text != '' OR error != '')
ORDER BY updated_at DESC
LIMIT 1;
`
	var r SessionRecovery
	err := s.db.QueryRow(query).Scan(
		&r.ID,
		&r.State,
		&r.PartialText,
		&r.RawText,
		&r.Text,
		&r.AudioSamples,
		&r.Error,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRecoveries returns recent recoverable sessions ordered newest first.
func (s *SQLite) ListRecoveries(limit int) ([]SessionRecovery, error) {
	if limit <= 0 {
		limit = 20
	}

	const query = `
SELECT id, state, partial_text, raw_text, text, audio_samples, error, created_at, updated_at
FROM session_recoveries
WHERE state != 'completed' AND (text != '' OR raw_text != '' OR partial_text != '' OR error != '')
ORDER BY updated_at DESC
LIMIT ?;
`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionRecovery
	for rows.Next() {
		var r SessionRecovery
		if err := rows.Scan(
			&r.ID,
			&r.State,
			&r.PartialText,
			&r.RawText,
			&r.Text,
			&r.AudioSamples,
			&r.Error,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}

	return out, rows.Err()
}

// DeleteRecovery deletes a recovery entry by ID.
func (s *SQLite) DeleteRecovery(id string) error {
	_, err := s.db.Exec("DELETE FROM session_recoveries WHERE id = ?", id)
	return err
}

// ClearRecoveries purges all recovery records from the store.
func (s *SQLite) ClearRecoveries() error {
	_, err := s.db.Exec("DELETE FROM session_recoveries")
	return err
}

// SetSnippet inserts or updates a voice snippet template.
func (s *SQLite) SetSnippet(snip Snippet) error {
	if strings.TrimSpace(snip.Trigger) == "" {
		return errors.New("snippet trigger cannot be empty")
	}
	if snip.CreatedAt.IsZero() {
		snip.CreatedAt = time.Now().UTC()
	}

	const query = `
INSERT INTO snippets (trigger, template, description, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(trigger) DO UPDATE SET
	template = excluded.template,
	description = excluded.description,
	created_at = excluded.created_at;
`
	_, err := s.db.Exec(query, snip.Trigger, snip.Template, snip.Description, snip.CreatedAt.UTC())
	return err
}

// GetSnippet retrieves a snippet by its voice trigger.
func (s *SQLite) GetSnippet(trigger string) (*Snippet, error) {
	const query = `
SELECT trigger, template, description, created_at
FROM snippets
WHERE trigger = ?;
`
	var snip Snippet
	err := s.db.QueryRow(query, trigger).Scan(&snip.Trigger, &snip.Template, &snip.Description, &snip.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snip, nil
}

// ListSnippets returns all snippets ordered alphabetically by trigger.
func (s *SQLite) ListSnippets() ([]Snippet, error) {
	const query = `
SELECT trigger, template, description, created_at
FROM snippets
ORDER BY trigger ASC;
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Snippet
	for rows.Next() {
		var snip Snippet
		if err := rows.Scan(&snip.Trigger, &snip.Template, &snip.Description, &snip.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, snip)
	}

	return out, rows.Err()
}

// DeleteSnippet removes a snippet by its trigger.
func (s *SQLite) DeleteSnippet(trigger string) error {
	_, err := s.db.Exec("DELETE FROM snippets WHERE trigger = ?", trigger)
	return err
}

// SetPolishProfile creates or updates a custom polish profile.
func (s *SQLite) SetPolishProfile(p PolishProfile) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return errors.New("profile name cannot be empty")
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	p.Name = name

	rulesJSON, err := json.Marshal(p.CustomRules)
	if err != nil {
		rulesJSON = []byte("[]")
	}

	const query = `
INSERT INTO polish_profiles (name, description, system_prompt, temperature, custom_rules, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
	description = excluded.description,
	system_prompt = excluded.system_prompt,
	temperature = excluded.temperature,
	custom_rules = excluded.custom_rules,
	updated_at = excluded.updated_at;
`
	_, err = s.db.Exec(query, p.Name, p.Description, p.SystemPrompt, p.Temperature, string(rulesJSON), p.CreatedAt, p.UpdatedAt)
	return err
}

// GetPolishProfile retrieves a single polish profile by name.
func (s *SQLite) GetPolishProfile(name string) (*PolishProfile, error) {
	const query = `
SELECT name, description, system_prompt, temperature, custom_rules, created_at, updated_at
FROM polish_profiles
WHERE name = ?;
`
	var p PolishProfile
	var rulesRaw string
	err := s.db.QueryRow(query, name).Scan(
		&p.Name, &p.Description, &p.SystemPrompt, &p.Temperature, &rulesRaw, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(rulesRaw), &p.CustomRules)
	return &p, nil
}

// ListPolishProfiles returns all stored custom polish profiles ordered alphabetically.
func (s *SQLite) ListPolishProfiles() ([]PolishProfile, error) {
	const query = `
SELECT name, description, system_prompt, temperature, custom_rules, created_at, updated_at
FROM polish_profiles
ORDER BY name ASC;
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PolishProfile
	for rows.Next() {
		var p PolishProfile
		var rulesRaw string
		if err := rows.Scan(
			&p.Name, &p.Description, &p.SystemPrompt, &p.Temperature, &rulesRaw, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rulesRaw), &p.CustomRules)
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePolishProfile deletes a polish profile by name.
func (s *SQLite) DeletePolishProfile(name string) error {
	_, err := s.db.Exec("DELETE FROM polish_profiles WHERE name = ?", name)
	return err
}

// SaveMeeting inserts or updates a meeting record.
func (s *SQLite) SaveMeeting(m Meeting) error {
	const query = `
INSERT INTO meetings (id, title, state, audio_path, sample_rate, created_at, ended_at, duration_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	title = excluded.title,
	state = excluded.state,
	audio_path = excluded.audio_path,
	sample_rate = excluded.sample_rate,
	ended_at = excluded.ended_at,
	duration_ms = excluded.duration_ms;
`
	_, err := s.db.Exec(query, m.ID, m.Title, m.State, m.AudioPath, m.SampleRate, m.CreatedAt, m.EndedAt, m.DurationMs)
	return err
}

// GetMeeting retrieves a meeting by ID.
func (s *SQLite) GetMeeting(id string) (*Meeting, error) {
	const query = `
SELECT id, title, state, audio_path, sample_rate, created_at, ended_at, duration_ms
FROM meetings
WHERE id = ?;
`
	var m Meeting
	var endedAt sql.NullTime
	err := s.db.QueryRow(query, id).Scan(
		&m.ID, &m.Title, &m.State, &m.AudioPath, &m.SampleRate, &m.CreatedAt, &endedAt, &m.DurationMs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		m.EndedAt = &endedAt.Time
	}
	return &m, nil
}

// ListMeetings returns recent meetings ordered by created_at DESC.
func (s *SQLite) ListMeetings(limit int) ([]Meeting, error) {
	if limit <= 0 {
		limit = 50
	}
	const query = `
SELECT id, title, state, audio_path, sample_rate, created_at, ended_at, duration_ms
FROM meetings
ORDER BY created_at DESC
LIMIT ?;
`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Meeting
	for rows.Next() {
		var m Meeting
		var endedAt sql.NullTime
		if err := rows.Scan(
			&m.ID, &m.Title, &m.State, &m.AudioPath, &m.SampleRate, &m.CreatedAt, &endedAt, &m.DurationMs,
		); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			m.EndedAt = &endedAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMeeting removes a meeting and cascades to its segments.
func (s *SQLite) DeleteMeeting(id string) error {
	_, err := s.db.Exec("DELETE FROM meetings WHERE id = ?", id)
	return err
}

// SaveMeetingSegment inserts a new segment for a meeting.
func (s *SQLite) SaveMeetingSegment(seg MeetingSegment) error {
	const query = `
INSERT INTO meeting_segments (id, meeting_id, segment_index, start_ms, end_ms, text, speaker, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	text = excluded.text,
	speaker = excluded.speaker,
	start_ms = excluded.start_ms,
	end_ms = excluded.end_ms;
`
	_, err := s.db.Exec(query, seg.ID, seg.MeetingID, seg.SegmentIndex, seg.StartMs, seg.EndMs, seg.Text, seg.Speaker, seg.CreatedAt)
	return err
}

// ListMeetingSegments returns all segments for a meeting ordered by segment_index ASC.
func (s *SQLite) ListMeetingSegments(meetingID string) ([]MeetingSegment, error) {
	const query = `
SELECT id, meeting_id, segment_index, start_ms, end_ms, text, speaker, created_at
FROM meeting_segments
WHERE meeting_id = ?
ORDER BY segment_index ASC;
`
	rows, err := s.db.Query(query, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MeetingSegment
	for rows.Next() {
		var seg MeetingSegment
		if err := rows.Scan(
			&seg.ID, &seg.MeetingID, &seg.SegmentIndex, &seg.StartMs, &seg.EndMs, &seg.Text, &seg.Speaker, &seg.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

// SetMeetingSpeaker inserts or updates a speaker assignment for a meeting.
func (s *SQLite) SetMeetingSpeaker(spk MeetingSpeaker) error {
	if spk.MeetingID == "" || spk.SpeakerID == "" {
		return errors.New("meeting_id and speaker_id are required")
	}
	if spk.UpdatedAt.IsZero() {
		spk.UpdatedAt = time.Now().UTC()
	}

	const query = `
INSERT INTO meeting_speakers (meeting_id, speaker_id, name, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(meeting_id, speaker_id) DO UPDATE SET
	name = excluded.name,
	updated_at = excluded.updated_at;
`
	_, err := s.db.Exec(query, spk.MeetingID, spk.SpeakerID, spk.Name, spk.UpdatedAt)
	return err
}

// ListMeetingSpeakers returns all known speaker mappings for a meeting.
func (s *SQLite) ListMeetingSpeakers(meetingID string) ([]MeetingSpeaker, error) {
	const query = `
SELECT meeting_id, speaker_id, name, updated_at
FROM meeting_speakers
WHERE meeting_id = ?
ORDER BY speaker_id ASC;
`
	rows, err := s.db.Query(query, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MeetingSpeaker
	for rows.Next() {
		var spk MeetingSpeaker
		if err := rows.Scan(&spk.MeetingID, &spk.SpeakerID, &spk.Name, &spk.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, spk)
	}
	return out, rows.Err()
}

// RenameMeetingSpeaker renames a speaker for a meeting and updates all corresponding segments in a single transaction.
func (s *SQLite) RenameMeetingSpeaker(meetingID string, fromSpeaker string, toName string) error {
	if meetingID == "" || fromSpeaker == "" || toName == "" {
		return errors.New("meeting_id, fromSpeaker, and toName are required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const spkQuery = `
INSERT INTO meeting_speakers (meeting_id, speaker_id, name, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(meeting_id, speaker_id) DO UPDATE SET
	name = excluded.name,
	updated_at = excluded.updated_at;
`
	if _, err := tx.Exec(spkQuery, meetingID, fromSpeaker, toName, time.Now().UTC()); err != nil {
		return err
	}

	const segQuery = `
UPDATE meeting_segments
SET speaker = ?
WHERE meeting_id = ? AND speaker = ?;
`
	if _, err := tx.Exec(segQuery, toName, meetingID, fromSpeaker); err != nil {
		return err
	}

	return tx.Commit()
}

// SaveMeetingSummary persists or updates the structured summary for a meeting.
func (s *SQLite) SaveMeetingSummary(ms MeetingSummary) error {
	if ms.MeetingID == "" {
		return errors.New("meeting_id is required")
	}
	now := time.Now().UTC()
	if ms.CreatedAt.IsZero() {
		ms.CreatedAt = now
	}
	ms.UpdatedAt = now

	decisionsJSON, err := json.Marshal(ms.Decisions)
	if err != nil {
		return fmt.Errorf("marshal decisions: %w", err)
	}
	actionItemsJSON, err := json.Marshal(ms.ActionItems)
	if err != nil {
		return fmt.Errorf("marshal action items: %w", err)
	}
	topicsJSON, err := json.Marshal(ms.Topics)
	if err != nil {
		return fmt.Errorf("marshal topics: %w", err)
	}

	const query = `
INSERT INTO meeting_summaries (meeting_id, overview, decisions, action_items, topics, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(meeting_id) DO UPDATE SET
	overview = excluded.overview,
	decisions = excluded.decisions,
	action_items = excluded.action_items,
	topics = excluded.topics,
	updated_at = excluded.updated_at;
`
	_, err = s.db.Exec(query, ms.MeetingID, ms.Overview, string(decisionsJSON), string(actionItemsJSON), string(topicsJSON), ms.CreatedAt, ms.UpdatedAt)
	return err
}

// GetMeetingSummary retrieves the structured summary for a meeting, if present.
func (s *SQLite) GetMeetingSummary(meetingID string) (*MeetingSummary, error) {
	const query = `
SELECT meeting_id, overview, decisions, action_items, topics, created_at, updated_at
FROM meeting_summaries
WHERE meeting_id = ?;
`
	var (
		ms                                         MeetingSummary
		decisionsJSON, actionItemsJSON, topicsJSON string
	)
	err := s.db.QueryRow(query, meetingID).Scan(
		&ms.MeetingID,
		&ms.Overview,
		&decisionsJSON,
		&actionItemsJSON,
		&topicsJSON,
		&ms.CreatedAt,
		&ms.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if decisionsJSON != "" {
		_ = json.Unmarshal([]byte(decisionsJSON), &ms.Decisions)
	}
	if ms.Decisions == nil {
		ms.Decisions = []string{}
	}
	if actionItemsJSON != "" {
		_ = json.Unmarshal([]byte(actionItemsJSON), &ms.ActionItems)
	}
	if ms.ActionItems == nil {
		ms.ActionItems = []ActionItem{}
	}
	if topicsJSON != "" {
		_ = json.Unmarshal([]byte(topicsJSON), &ms.Topics)
	}
	if ms.Topics == nil {
		ms.Topics = []string{}
	}

	return &ms, nil
}

// DeleteMeetingSummary removes the structured summary for a meeting.
func (s *SQLite) DeleteMeetingSummary(meetingID string) error {
	const query = `DELETE FROM meeting_summaries WHERE meeting_id = ?;`
	_, err := s.db.Exec(query, meetingID)
	return err
}

// SaveFolder creates or updates a folder in SQLite.
func (s *SQLite) SaveFolder(f Folder) error {
	if strings.TrimSpace(f.ID) == "" {
		return errors.New("folder id is required")
	}
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("folder name is required")
	}
	now := time.Now().UTC()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now

	const query = `
INSERT INTO folders (id, name, parent_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	parent_id = excluded.parent_id,
	updated_at = excluded.updated_at;
`
	_, err := s.db.Exec(query, f.ID, f.Name, f.ParentID, f.CreatedAt, f.UpdatedAt)
	return err
}

// GetFolder retrieves a single folder by ID.
func (s *SQLite) GetFolder(id string) (*Folder, error) {
	const query = `
SELECT id, name, parent_id, created_at, updated_at
FROM folders
WHERE id = ?;
`
	var f Folder
	err := s.db.QueryRow(query, id).Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ListFolders returns all folders sorted by name.
func (s *SQLite) ListFolders() ([]Folder, error) {
	const query = `
SELECT id, name, parent_id, created_at, updated_at
FROM folders
ORDER BY name ASC;
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFolder deletes a folder. Associated notes have their folder_id set to NULL via foreign key.
func (s *SQLite) DeleteFolder(id string) error {
	const query = `DELETE FROM folders WHERE id = ?;`
	_, err := s.db.Exec(query, id)
	return err
}

// SaveNote creates or updates a note and synchronizes its tags.
func (s *SQLite) SaveNote(n Note) error {
	if strings.TrimSpace(n.ID) == "" {
		return errors.New("note id is required")
	}
	now := time.Now().UTC()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var folderIDVal any
	if strings.TrimSpace(n.FolderID) != "" {
		folderIDVal = n.FolderID
	}
	var meetingIDVal any
	if strings.TrimSpace(n.MeetingID) != "" {
		meetingIDVal = n.MeetingID
	}

	const query = `
INSERT INTO notes (id, folder_id, title, content, meeting_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	folder_id = excluded.folder_id,
	title = excluded.title,
	content = excluded.content,
	meeting_id = excluded.meeting_id,
	updated_at = excluded.updated_at;
`
	if _, err := tx.Exec(query, n.ID, folderIDVal, n.Title, n.Content, meetingIDVal, n.CreatedAt, n.UpdatedAt); err != nil {
		return err
	}

	if n.Tags != nil {
		if _, err := tx.Exec(`DELETE FROM note_tags WHERE note_id = ?;`, n.ID); err != nil {
			return err
		}
		for _, t := range n.Tags {
			clean := strings.ToLower(strings.TrimSpace(t))
			if clean == "" {
				continue
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (name, created_at) VALUES (?, ?);`, clean, now); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO note_tags (note_id, tag_name) VALUES (?, ?);`, n.ID, clean); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// GetNote retrieves a single note by ID and includes its tags.
func (s *SQLite) GetNote(id string) (*Note, error) {
	const query = `
SELECT id, COALESCE(folder_id, ''), title, content, COALESCE(meeting_id, ''), created_at, updated_at
FROM notes
WHERE id = ?;
`
	var n Note
	err := s.db.QueryRow(query, id).Scan(
		&n.ID,
		&n.FolderID,
		&n.Title,
		&n.Content,
		&n.MeetingID,
		&n.CreatedAt,
		&n.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	tags, err := s.getNoteTagsTx(s.db, n.ID)
	if err != nil {
		return nil, err
	}
	n.Tags = tags
	return &n, nil
}

type queryable interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func (s *SQLite) getNoteTagsTx(q queryable, noteID string) ([]string, error) {
	const tagQuery = `SELECT tag_name FROM note_tags WHERE note_id = ? ORDER BY tag_name ASC;`
	rows, err := q.Query(tagQuery, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

// ListNotes retrieves notes matching the specified filter criteria.
func (s *SQLite) ListNotes(filter NoteFilter) ([]Note, error) {
	var (
		clauses []string
		args    []any
	)

	targetTag := strings.ToLower(strings.TrimSpace(filter.Tag))
	query := `
SELECT DISTINCT n.id, COALESCE(n.folder_id, ''), n.title, n.content, COALESCE(n.meeting_id, ''), n.created_at, n.updated_at
FROM notes n
`
	if targetTag != "" {
		query += ` JOIN note_tags nt ON n.id = nt.note_id`
		clauses = append(clauses, "nt.tag_name = ?")
		args = append(args, targetTag)
	}

	if filter.FolderID != "" {
		clauses = append(clauses, "n.folder_id = ?")
		args = append(args, filter.FolderID)
	}
	if filter.MeetingID != "" {
		clauses = append(clauses, "n.meeting_id = ?")
		args = append(args, filter.MeetingID)
	}

	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY n.updated_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(
			&n.ID,
			&n.FolderID,
			&n.Title,
			&n.Content,
			&n.MeetingID,
			&n.CreatedAt,
			&n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range notes {
		tags, err := s.getNoteTagsTx(s.db, notes[i].ID)
		if err != nil {
			return nil, err
		}
		notes[i].Tags = tags
	}

	return notes, nil
}

// DeleteNote removes a note and all its tag associations.
func (s *SQLite) DeleteNote(id string) error {
	const query = `DELETE FROM notes WHERE id = ?;`
	_, err := s.db.Exec(query, id)
	return err
}

// ListTags returns all known tags along with their count of associated notes.
func (s *SQLite) ListTags() ([]Tag, error) {
	const query = `
SELECT t.name, t.created_at, COUNT(nt.note_id) as note_count
FROM tags t
LEFT JOIN note_tags nt ON t.name = nt.tag_name
GROUP BY t.name
ORDER BY t.name ASC;
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Name, &t.CreatedAt, &t.NoteCount); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// SetNoteTags sets the tag list for a note.
func (s *SQLite) SetNoteTags(noteID string, tags []string) error {
	if strings.TrimSpace(noteID) == "" {
		return errors.New("note id is required")
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM note_tags WHERE note_id = ?;`, noteID); err != nil {
		return err
	}

	for _, t := range tags {
		clean := strings.ToLower(strings.TrimSpace(t))
		if clean == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (name, created_at) VALUES (?, ?);`, clean, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO note_tags (note_id, tag_name) VALUES (?, ?);`, noteID, clean); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`UPDATE notes SET updated_at = ? WHERE id = ?;`, now, noteID); err != nil {
		return err
	}

	return tx.Commit()
}

// GetNoteTags returns all tags attached to a given note.
func (s *SQLite) GetNoteTags(noteID string) ([]string, error) {
	return s.getNoteTagsTx(s.db, noteID)
}

// DeleteTag deletes a tag definition and removes it from all notes.
func (s *SQLite) DeleteTag(name string) error {
	clean := strings.ToLower(strings.TrimSpace(name))
	const query = `DELETE FROM tags WHERE name = ?;`
	_, err := s.db.Exec(query, clean)
	return err
}

// Search executes full-text search across notes, transcripts, and meetings using SQLite FTS5.
func (s *SQLite) Search(query string, filter SearchFilter) ([]SearchResult, error) {
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return []SearchResult{}, nil
	}

	searchAll := len(filter.Types) == 0
	typesSet := make(map[SearchResultType]bool)
	for _, t := range filter.Types {
		typesSet[t] = true
	}

	limit := 50
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	ftsQuery := buildFTSQuery(cleanQuery)
	var results []SearchResult

	// 1. Notes FTS
	if searchAll || typesSet[SearchResultNote] {
		const notesSQL = `
SELECT n.id, n.title, snippet(notes_fts, 2, '', '', '...', 25) AS snip, n.created_at, bm25(notes_fts) AS rank
FROM notes_fts
JOIN notes n ON notes_fts.id = n.id
WHERE notes_fts MATCH ?
ORDER BY rank ASC
LIMIT ?;
`
		rows, err := s.db.Query(notesSQL, ftsQuery, limit)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var r SearchResult
				r.Type = SearchResultNote
				if err := rows.Scan(&r.ID, &r.Title, &r.Snippet, &r.CreatedAt, &r.Rank); err == nil {
					results = append(results, r)
				}
			}
		} else {
			likeSQL := `
SELECT id, title, content, created_at
FROM notes
WHERE title LIKE ? OR content LIKE ?
ORDER BY updated_at DESC
LIMIT ?;
`
			likePattern := "%" + cleanQuery + "%"
			if lRows, lErr := s.db.Query(likeSQL, likePattern, likePattern, limit); lErr == nil {
				defer lRows.Close()
				for lRows.Next() {
					var r SearchResult
					var content string
					r.Type = SearchResultNote
					if err := lRows.Scan(&r.ID, &r.Title, &content, &r.CreatedAt); err == nil {
						r.Snippet = extractSnippet(content, cleanQuery, 120)
						results = append(results, r)
					}
				}
			}
		}
	}

	// 2. Transcripts FTS
	if searchAll || typesSet[SearchResultTranscript] {
		const transcriptsSQL = `
SELECT t.id, t.created_at, snippet(transcripts_fts, 1, '', '', '...', 25) AS snip, bm25(transcripts_fts) AS rank
FROM transcripts_fts
JOIN transcripts t ON transcripts_fts.id = t.id
WHERE transcripts_fts MATCH ?
ORDER BY rank ASC
LIMIT ?;
`
		rows, err := s.db.Query(transcriptsSQL, ftsQuery, limit)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var r SearchResult
				r.Type = SearchResultTranscript
				if err := rows.Scan(&r.ID, &r.CreatedAt, &r.Snippet, &r.Rank); err == nil {
					r.Title = fmt.Sprintf("Dictation %s", r.CreatedAt.Format("2006-01-02 15:04"))
					results = append(results, r)
				}
			}
		} else {
			likeSQL := `
SELECT id, text, created_at
FROM transcripts
WHERE text LIKE ? OR raw_text LIKE ?
ORDER BY created_at DESC
LIMIT ?;
`
			likePattern := "%" + cleanQuery + "%"
			if lRows, lErr := s.db.Query(likeSQL, likePattern, likePattern, limit); lErr == nil {
				defer lRows.Close()
				for lRows.Next() {
					var r SearchResult
					var text string
					r.Type = SearchResultTranscript
					if err := lRows.Scan(&r.ID, &text, &r.CreatedAt); err == nil {
						r.Title = fmt.Sprintf("Dictation %s", r.CreatedAt.Format("2006-01-02 15:04"))
						r.Snippet = extractSnippet(text, cleanQuery, 120)
						results = append(results, r)
					}
				}
			}
		}
	}

	// 3. Meeting segments FTS
	if searchAll || typesSet[SearchResultMeeting] {
		const mtgSQL = `
SELECT m.id, m.title, snippet(meeting_segments_fts, 3, '', '', '...', 25) AS snip, m.created_at, MIN(bm25(meeting_segments_fts)) AS rank
FROM meeting_segments_fts
JOIN meetings m ON meeting_segments_fts.meeting_id = m.id
WHERE meeting_segments_fts MATCH ?
GROUP BY m.id
ORDER BY rank ASC
LIMIT ?;
`
		rows, err := s.db.Query(mtgSQL, ftsQuery, limit)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var r SearchResult
				r.Type = SearchResultMeeting
				if err := rows.Scan(&r.ID, &r.Title, &r.Snippet, &r.CreatedAt, &r.Rank); err == nil {
					results = append(results, r)
				}
			}
		} else {
			likeSQL := `
SELECT m.id, m.title, m.created_at, ms.text
FROM meetings m
JOIN meeting_segments ms ON m.id = ms.meeting_id
WHERE m.title LIKE ? OR ms.text LIKE ?
GROUP BY m.id
ORDER BY m.created_at DESC
LIMIT ?;
`
			likePattern := "%" + cleanQuery + "%"
			if lRows, lErr := s.db.Query(likeSQL, likePattern, likePattern, limit); lErr == nil {
				defer lRows.Close()
				for lRows.Next() {
					var r SearchResult
					var text string
					r.Type = SearchResultMeeting
					if err := lRows.Scan(&r.ID, &r.Title, &r.CreatedAt, &text); err == nil {
						r.Snippet = extractSnippet(text, cleanQuery, 120)
						results = append(results, r)
					}
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Rank != 0 && results[j].Rank != 0 {
			return results[i].Rank < results[j].Rank
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

func buildFTSQuery(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, `"`) || strings.Contains(raw, " OR ") || strings.Contains(raw, " AND ") || strings.Contains(raw, " NOT ") {
		return raw
	}
	words := strings.Fields(raw)
	if len(words) == 0 {
		return raw
	}
	var tokens []string
	for _, w := range words {
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return -1
		}, w)
		if clean != "" {
			tokens = append(tokens, clean+"*")
		}
	}
	if len(tokens) == 0 {
		return `"` + raw + `"`
	}
	return strings.Join(tokens, " ")
}

// Float32SliceToBytes serializes a slice of float32 into little-endian binary bytes.
func Float32SliceToBytes(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// BytesToFloat32Slice deserializes little-endian binary bytes into a slice of float32.
func BytesToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(b)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return vec
}

// SaveEmbedding stores or updates an embedding vector for a target entity.
func (s *SQLite) SaveEmbedding(rec EmbeddingRecord) error {
	blob := Float32SliceToBytes(rec.Embedding)
	query := `
INSERT INTO embeddings(target_type, target_id, model, embedding, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(target_type, target_id, model) DO UPDATE SET
	embedding = excluded.embedding,
	updated_at = CURRENT_TIMESTAMP;
`
	_, err := s.db.Exec(query, rec.TargetType, rec.TargetID, rec.Model, blob)
	return err
}

// GetEmbedding fetches a single embedding record by target type, ID, and model.
func (s *SQLite) GetEmbedding(targetType, targetID, model string) (*EmbeddingRecord, error) {
	query := `SELECT target_type, target_id, model, embedding, updated_at FROM embeddings WHERE target_type = ? AND target_id = ? AND model = ?`
	row := s.db.QueryRow(query, targetType, targetID, model)

	var rec EmbeddingRecord
	var blob []byte
	var updatedAt time.Time
	if err := row.Scan(&rec.TargetType, &rec.TargetID, &rec.Model, &blob, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("embedding not found")
		}
		return nil, err
	}
	rec.Embedding = BytesToFloat32Slice(blob)
	rec.UpdatedAt = updatedAt
	return &rec, nil
}

// ListEmbeddings lists stored embedding records, optionally filtered by target type and model.
func (s *SQLite) ListEmbeddings(targetType, model string) ([]EmbeddingRecord, error) {
	var query string
	var args []any

	if targetType != "" && model != "" {
		query = `SELECT target_type, target_id, model, embedding, updated_at FROM embeddings WHERE target_type = ? AND model = ? ORDER BY updated_at DESC`
		args = []any{targetType, model}
	} else if targetType != "" {
		query = `SELECT target_type, target_id, model, embedding, updated_at FROM embeddings WHERE target_type = ? ORDER BY updated_at DESC`
		args = []any{targetType}
	} else if model != "" {
		query = `SELECT target_type, target_id, model, embedding, updated_at FROM embeddings WHERE model = ? ORDER BY updated_at DESC`
		args = []any{model}
	} else {
		query = `SELECT target_type, target_id, model, embedding, updated_at FROM embeddings ORDER BY updated_at DESC`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EmbeddingRecord
	for rows.Next() {
		var rec EmbeddingRecord
		var blob []byte
		var updatedAt time.Time
		if err := rows.Scan(&rec.TargetType, &rec.TargetID, &rec.Model, &blob, &updatedAt); err != nil {
			return nil, err
		}
		rec.Embedding = BytesToFloat32Slice(blob)
		rec.UpdatedAt = updatedAt
		results = append(results, rec)
	}
	return results, rows.Err()
}

// DeleteEmbedding removes an embedding record. If model is empty, removes all models for the target.
func (s *SQLite) DeleteEmbedding(targetType, targetID, model string) error {
	if model != "" {
		_, err := s.db.Exec(`DELETE FROM embeddings WHERE target_type = ? AND target_id = ? AND model = ?`, targetType, targetID, model)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM embeddings WHERE target_type = ? AND target_id = ?`, targetType, targetID)
	return err
}

// SavePlugin inserts or updates a plugin record.
func (s *SQLite) SavePlugin(rec PluginRecord) error {
	if rec.ID == "" {
		return errors.New("plugin id cannot be empty")
	}
	if rec.InstalledAt.IsZero() {
		rec.InstalledAt = time.Now().UTC()
	}
	rec.UpdatedAt = time.Now().UTC()

	const query = `
INSERT INTO plugins (id, name, version, install_dir, enabled, manifest_json, installed_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	version = excluded.version,
	install_dir = excluded.install_dir,
	enabled = excluded.enabled,
	manifest_json = excluded.manifest_json,
	updated_at = excluded.updated_at;
`
	_, err := s.db.Exec(query, rec.ID, rec.Name, rec.Version, rec.InstallDir, rec.Enabled, rec.ManifestJSON, rec.InstalledAt, rec.UpdatedAt)
	return err
}

// GetPlugin retrieves a plugin record by ID.
func (s *SQLite) GetPlugin(id string) (*PluginRecord, error) {
	const query = `
SELECT id, name, version, install_dir, enabled, manifest_json, installed_at, updated_at
FROM plugins
WHERE id = ?;
`
	var rec PluginRecord
	err := s.db.QueryRow(query, id).Scan(
		&rec.ID,
		&rec.Name,
		&rec.Version,
		&rec.InstallDir,
		&rec.Enabled,
		&rec.ManifestJSON,
		&rec.InstalledAt,
		&rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("plugin not found: " + id)
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListPlugins returns all installed plugins ordered by installed_at ASC.
func (s *SQLite) ListPlugins() ([]PluginRecord, error) {
	const query = `
SELECT id, name, version, install_dir, enabled, manifest_json, installed_at, updated_at
FROM plugins
ORDER BY installed_at ASC;
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PluginRecord
	for rows.Next() {
		var rec PluginRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.Name,
			&rec.Version,
			&rec.InstallDir,
			&rec.Enabled,
			&rec.ManifestJSON,
			&rec.InstalledAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, rows.Err()
}

// DeletePlugin deletes a plugin record by ID.
func (s *SQLite) DeletePlugin(id string) error {
	res, err := s.db.Exec(`DELETE FROM plugins WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("plugin not found: " + id)
	}
	return nil
}

// SetPluginEnabled toggles a plugin's enabled status.
func (s *SQLite) SetPluginEnabled(id string, enabled bool) error {
	now := time.Now().UTC()
	res, err := s.db.Exec(`UPDATE plugins SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("plugin not found: " + id)
	}
	return nil
}

// Close releases the underlying SQLite database handle.
func (s *SQLite) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
