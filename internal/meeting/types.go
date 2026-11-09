package meeting

import (
	"time"
)

// State represents the current lifecycle phase of a meeting capture session.
type State string

const (
	StateIdle      State = "idle"
	StateRecording State = "recording"
	StatePaused    State = "paused"
	StateStopped   State = "stopped"
)

// Session represents a continuous recording meeting capture session.
type Session struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	State      State      `json:"state"`
	StartTime  time.Time  `json:"start_time"`
	EndTime    *time.Time `json:"ended_at,omitempty"`
	DurationMs int64      `json:"duration_ms"`
	SampleRate int        `json:"sample_rate"`
	AudioPath  string     `json:"audio_path,omitempty"`
	Segments   []Segment  `json:"segments,omitempty"`
}

// Segment represents a discrete rolling segment of meeting audio and transcription.
type Segment struct {
	ID           string    `json:"id"`
	MeetingID    string    `json:"meeting_id"`
	SegmentIndex int       `json:"segment_index"`
	StartMs      int64     `json:"start_ms"`
	EndMs        int64     `json:"end_ms"`
	Text         string    `json:"text"`
	Speaker      string    `json:"speaker,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// StatusReport details the active meeting state and elapsed runtime.
type StatusReport struct {
	ActiveSession *Session `json:"active_session,omitempty"`
	IsRecording   bool     `json:"is_recording"`
	IsPaused      bool     `json:"is_paused"`
	ElapsedMs     int64    `json:"elapsed_ms"`
	SegmentCount  int      `json:"segment_count"`
}
