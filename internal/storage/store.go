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
}
