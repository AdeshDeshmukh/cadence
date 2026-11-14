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

// PolishProfile defines a saved prompt profile for LLM text polishing.
type PolishProfile struct {
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	SystemPrompt string    `json:"system_prompt"`
	Temperature  float64   `json:"temperature"`
	CustomRules  []string  `json:"custom_rules,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Meeting represents a continuous recording meeting capture session.
type Meeting struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	State      string     `json:"state"` // "recording", "paused", "stopped"
	AudioPath  string     `json:"audio_path,omitempty"`
	SampleRate int        `json:"sample_rate"`
	CreatedAt  time.Time  `json:"created_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	DurationMs int64      `json:"duration_ms"`
}

// MeetingSegment represents an audio/transcript chunk within a meeting session.
type MeetingSegment struct {
	ID           string    `json:"id"`
	MeetingID    string    `json:"meeting_id"`
	SegmentIndex int       `json:"segment_index"`
	StartMs      int64     `json:"start_ms"`
	EndMs        int64     `json:"end_ms"`
	Text         string    `json:"text"`
	Speaker      string    `json:"speaker,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// MeetingSpeaker records an assigned human label for a speaker in a meeting.
type MeetingSpeaker struct {
	MeetingID string    `json:"meeting_id"`
	SpeakerID string    `json:"speaker_id"` // e.g. "SPEAKER_00"
	Name      string    `json:"name"`       // e.g. "Adesh Deshmukh"
	UpdatedAt time.Time `json:"updated_at"`
}

// ActionItem represents a task or deliverable extracted from a meeting.
type ActionItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Assignee    string `json:"assignee,omitempty"`
	Completed   bool   `json:"completed"`
}

// MeetingSummary captures structured intelligence extracted from a meeting.
type MeetingSummary struct {
	MeetingID   string       `json:"meeting_id"`
	Overview    string       `json:"overview"`
	Decisions   []string     `json:"decisions"`
	ActionItems []ActionItem `json:"action_items"`
	Topics      []string     `json:"topics"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Folder represents an organizational folder or notebook for notes.
type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NoteFilter allows filtering notes by folder, tag, or linked meeting.
type NoteFilter struct {
	FolderID  string `json:"folder_id,omitempty"`
	Tag       string `json:"tag,omitempty"`
	MeetingID string `json:"meeting_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// Note represents a markdown note or document in the personal knowledge base.
type Note struct {
	ID        string    `json:"id"`
	FolderID  string    `json:"folder_id,omitempty"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	MeetingID string    `json:"meeting_id,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tag represents a categorization label with count of tagged notes.
type Tag struct {
	Name      string    `json:"name"`
	NoteCount int       `json:"note_count,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SearchResultType represents the kind of entity found in full-text search.
type SearchResultType string

const (
	SearchResultNote       SearchResultType = "note"
	SearchResultTranscript SearchResultType = "transcript"
	SearchResultMeeting    SearchResultType = "meeting"
)

// SearchResult represents a matching hit from full-text search.
type SearchResult struct {
	ID        string           `json:"id"`
	Type      SearchResultType `json:"type"`
	Title     string           `json:"title"`
	Snippet   string           `json:"snippet"`
	Rank      float64          `json:"rank,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

// SearchFilter configures query scope and result limits.
type SearchFilter struct {
	Types []SearchResultType `json:"types,omitempty"`
	Limit int                `json:"limit,omitempty"`
}

// EmbeddingRecord represents a stored vector embedding for a target entity.
type EmbeddingRecord struct {
	TargetType string    `json:"target_type"` // "transcript", "note", "meeting_segment"
	TargetID   string    `json:"target_id"`
	Model      string    `json:"model"`
	Embedding  []float32 `json:"embedding"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PluginRecord represents the persistent state and location of an installed plugin.
type PluginRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	InstallDir   string    `json:"install_dir"`
	Enabled      bool      `json:"enabled"`
	ManifestJSON string    `json:"manifest_json"`
	InstalledAt  time.Time `json:"installed_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Store interface {
	SaveTranscript(Transcript) error
	GetTranscript(id string) (*Transcript, error)
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
	SetPolishProfile(PolishProfile) error
	GetPolishProfile(string) (*PolishProfile, error)
	ListPolishProfiles() ([]PolishProfile, error)
	DeletePolishProfile(string) error
	SaveMeeting(Meeting) error
	GetMeeting(string) (*Meeting, error)
	ListMeetings(int) ([]Meeting, error)
	DeleteMeeting(string) error
	SaveMeetingSegment(MeetingSegment) error
	ListMeetingSegments(string) ([]MeetingSegment, error)
	SetMeetingSpeaker(MeetingSpeaker) error
	ListMeetingSpeakers(string) ([]MeetingSpeaker, error)
	RenameMeetingSpeaker(meetingID string, fromSpeaker string, toName string) error
	SaveMeetingSummary(MeetingSummary) error
	GetMeetingSummary(string) (*MeetingSummary, error)
	DeleteMeetingSummary(string) error

	// Knowledge base: Folders
	SaveFolder(Folder) error
	GetFolder(id string) (*Folder, error)
	ListFolders() ([]Folder, error)
	DeleteFolder(id string) error

	// Knowledge base: Notes
	SaveNote(Note) error
	GetNote(id string) (*Note, error)
	ListNotes(filter NoteFilter) ([]Note, error)
	DeleteNote(id string) error

	// Knowledge base: Tags
	ListTags() ([]Tag, error)
	SetNoteTags(noteID string, tags []string) error
	GetNoteTags(noteID string) ([]string, error)
	DeleteTag(name string) error

	// Full-text search
	Search(query string, filter SearchFilter) ([]SearchResult, error)

	// Embeddings & Semantic Search
	SaveEmbedding(EmbeddingRecord) error
	GetEmbedding(targetType, targetID, model string) (*EmbeddingRecord, error)
	ListEmbeddings(targetType, model string) ([]EmbeddingRecord, error)
	DeleteEmbedding(targetType, targetID, model string) error

	// Plugins
	SavePlugin(PluginRecord) error
	GetPlugin(id string) (*PluginRecord, error)
	ListPlugins() ([]PluginRecord, error)
	DeletePlugin(id string) error
	SetPluginEnabled(id string, enabled bool) error
}
