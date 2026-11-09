package meeting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/diarize"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// ManagerOption allows customizing Meeting Manager settings.
type ManagerOption func(*Manager)

// WithAudioDirectory sets the base directory where meeting WAV files are saved.
func WithAudioDirectory(dir string) ManagerOption {
	return func(m *Manager) {
		if dir != "" {
			m.audioDir = dir
		}
	}
}

// WithASREngine configures an optional ASR engine for live transcription of meeting chunks.
func WithASREngine(engine asr.Engine) ManagerOption {
	return func(m *Manager) {
		m.asrEngine = engine
	}
}

// WithSampleRate sets the audio sample rate for meeting capture (default 16000).
func WithSampleRate(sr int) ManagerOption {
	return func(m *Manager) {
		if sr > 0 {
			m.sampleRate = sr
		}
	}
}

// WithSegmentDuration sets the target rolling segment duration (default 15s).
func WithSegmentDuration(d time.Duration) ManagerOption {
	return func(m *Manager) {
		if d >= 2*time.Second {
			m.segmentDuration = d
		}
	}
}

// WithDiarizer sets a custom speaker diarization engine.
func WithDiarizer(d diarize.Diarizer) ManagerOption {
	return func(m *Manager) {
		if d != nil {
			m.diarizer = diarize.NewEngine(d)
		}
	}
}

// Manager coordinates continuous recording sessions, rolling segmentation, and persistence.
type Manager struct {
	mu              sync.RWMutex
	store           storage.Store
	capture         audio.Capture
	asrEngine       asr.Engine
	diarizer        *diarize.Engine
	audioDir        string
	sampleRate      int
	segmentDuration time.Duration

	activeSession *Session
	chunker       *Chunker
	wavWriter     *WAVWriter
	cancelFn      context.CancelFunc
	wg            sync.WaitGroup
	isPaused      bool
	pausedAt      time.Time
	totalPausedMs int64
}

// NewManager creates a fully configured Meeting Manager.
func NewManager(store storage.Store, capture audio.Capture, opts ...ManagerOption) *Manager {
	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".cadence", "meetings")

	m := &Manager{
		store:           store,
		capture:         capture,
		audioDir:        defaultDir,
		sampleRate:      16000,
		segmentDuration: 15 * time.Second,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start initiates a continuous recording meeting capture session.
func (m *Manager) Start(ctx context.Context, title string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeSession != nil && m.activeSession.State == StateRecording {
		return nil, errors.New("a meeting is already actively recording")
	}

	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		trimmedTitle = fmt.Sprintf("Meeting %s", time.Now().Format("2006-01-02 15:04"))
	}

	sessionID := fmt.Sprintf("mtg-%d", time.Now().UnixNano())
	wavPath := filepath.Join(m.audioDir, fmt.Sprintf("%s.wav", sessionID))

	// Initialize WAV writer
	writer, err := NewWAVWriter(wavPath, m.sampleRate)
	if err != nil {
		return nil, fmt.Errorf("init wav writer: %w", err)
	}
	m.wavWriter = writer

	now := time.Now().UTC()
	sess := &Session{
		ID:         sessionID,
		Title:      trimmedTitle,
		State:      StateRecording,
		StartTime:  now,
		SampleRate: m.sampleRate,
		AudioPath:  wavPath,
		Segments:   make([]Segment, 0),
	}
	m.activeSession = sess
	m.isPaused = false
	m.totalPausedMs = 0

	// Create chunker with segment handler
	handler := func(seg Segment, samples []float32) {
		m.onSegmentProduced(seg, samples)
	}

	m.chunker = NewChunker(sessionID, writer, handler, ChunkerOptions{
		SegmentDuration: m.segmentDuration,
		OverlapDuration: 1 * time.Second,
		SampleRate:      m.sampleRate,
	})

	// Start audio capture stream
	streamCtx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel

	var audioChan <-chan audio.Chunk
	if m.capture != nil {
		ch, err := m.capture.Start()
		if err != nil {
			_ = writer.Close()
			m.activeSession = nil
			cancel()
			return nil, fmt.Errorf("start audio capture: %w", err)
		}
		audioChan = ch
	}

	// Persist initial meeting record to database
	if m.store != nil {
		_ = m.store.SaveMeeting(storage.Meeting{
			ID:         sess.ID,
			Title:      sess.Title,
			State:      string(sess.State),
			AudioPath:  sess.AudioPath,
			SampleRate: sess.SampleRate,
			CreatedAt:  sess.StartTime,
			DurationMs: 0,
		})
	}

	// Ingestion loop
	if audioChan != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for {
				select {
				case <-streamCtx.Done():
					return
				case chunk, ok := <-audioChan:
					if !ok {
						return
					}
					m.mu.RLock()
					paused := m.isPaused
					chunker := m.chunker
					m.mu.RUnlock()

					if !paused && chunker != nil && len(chunk.Samples) > 0 {
						_ = chunker.Ingest(chunk.Samples)
					}
				}
			}
		}()
	}

	return sess, nil
}

// Pause temporarily suspends incoming audio ingestion.
func (m *Manager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeSession == nil || m.activeSession.State != StateRecording {
		return errors.New("no active recording session to pause")
	}

	m.activeSession.State = StatePaused
	m.isPaused = true
	m.pausedAt = time.Now()

	if m.store != nil {
		_ = m.store.SaveMeeting(storage.Meeting{
			ID:         m.activeSession.ID,
			Title:      m.activeSession.Title,
			State:      string(StatePaused),
			AudioPath:  m.activeSession.AudioPath,
			SampleRate: m.activeSession.SampleRate,
			CreatedAt:  m.activeSession.StartTime,
			DurationMs: m.activeSession.DurationMs,
		})
	}
	return nil
}

// Resume continues a paused meeting recording session.
func (m *Manager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeSession == nil || m.activeSession.State != StatePaused {
		return errors.New("meeting is not currently paused")
	}

	if !m.pausedAt.IsZero() {
		m.totalPausedMs += time.Since(m.pausedAt).Milliseconds()
		m.pausedAt = time.Time{}
	}

	m.activeSession.State = StateRecording
	m.isPaused = false

	if m.store != nil {
		_ = m.store.SaveMeeting(storage.Meeting{
			ID:         m.activeSession.ID,
			Title:      m.activeSession.Title,
			State:      string(StateRecording),
			AudioPath:  m.activeSession.AudioPath,
			SampleRate: m.activeSession.SampleRate,
			CreatedAt:  m.activeSession.StartTime,
			DurationMs: m.activeSession.DurationMs,
		})
	}
	return nil
}

// Stop finalizes and persists the active meeting recording session.
func (m *Manager) Stop() (*Session, error) {
	m.mu.Lock()
	if m.activeSession == nil {
		m.mu.Unlock()
		return nil, errors.New("no active meeting session to stop")
	}

	sess := m.activeSession
	cancel := m.cancelFn
	cap := m.capture
	chunker := m.chunker
	writer := m.wavWriter
	m.mu.Unlock()

	// 1. Cancel ingestion loop
	if cancel != nil {
		cancel()
	}

	// 2. Stop capture hardware
	if cap != nil {
		_ = cap.Stop()
	}

	m.wg.Wait()

	// 3. Flush residual chunker buffer
	if chunker != nil {
		chunker.Flush()
	}

	// 4. Close WAV file
	if writer != nil {
		_ = writer.Close()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	endTime := time.Now().UTC()
	sess.EndTime = &endTime
	rawDuration := endTime.Sub(sess.StartTime).Milliseconds() - m.totalPausedMs
	if rawDuration < 0 {
		rawDuration = 0
	}
	sess.DurationMs = rawDuration
	sess.State = StateStopped

	// 5. Persist final record to storage
	if m.store != nil {
		_ = m.store.SaveMeeting(storage.Meeting{
			ID:         sess.ID,
			Title:      sess.Title,
			State:      string(StateStopped),
			AudioPath:  sess.AudioPath,
			SampleRate: sess.SampleRate,
			CreatedAt:  sess.StartTime,
			EndedAt:    sess.EndTime,
			DurationMs: sess.DurationMs,
		})
	}

	m.activeSession = nil
	m.chunker = nil
	m.wavWriter = nil
	m.isPaused = false

	return sess, nil
}

// IngestSamples pushes audio samples manually (useful in headless tests or mock drivers).
func (m *Manager) IngestSamples(samples []float32) error {
	m.mu.RLock()
	paused := m.isPaused
	chunker := m.chunker
	m.mu.RUnlock()

	if paused || chunker == nil {
		return nil
	}
	return chunker.Ingest(samples)
}

// Status returns a point-in-time status report of the meeting recorder.
func (m *Manager) Status() StatusReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeSession == nil {
		return StatusReport{
			IsRecording:  false,
			IsPaused:     false,
			ElapsedMs:    0,
			SegmentCount: 0,
		}
	}

	elapsed := time.Since(m.activeSession.StartTime).Milliseconds() - m.totalPausedMs
	if m.isPaused && !m.pausedAt.IsZero() {
		elapsed -= time.Since(m.pausedAt).Milliseconds()
	}
	if elapsed < 0 {
		elapsed = 0
	}

	sessCopy := *m.activeSession
	sessCopy.DurationMs = elapsed

	return StatusReport{
		ActiveSession: &sessCopy,
		IsRecording:   m.activeSession.State == StateRecording,
		IsPaused:      m.activeSession.State == StatePaused,
		ElapsedMs:     elapsed,
		SegmentCount:  len(m.activeSession.Segments),
	}
}

// List returns historical meetings from the database.
func (m *Manager) List(limit int) ([]storage.Meeting, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	return m.store.ListMeetings(limit)
}

// Get retrieves meeting metadata and transcribed segments by meeting ID.
func (m *Manager) Get(id string) (*storage.Meeting, []storage.MeetingSegment, error) {
	if m.store == nil {
		return nil, nil, errors.New("storage engine is not configured")
	}
	mtg, err := m.store.GetMeeting(id)
	if err != nil || mtg == nil {
		return nil, nil, err
	}
	segs, err := m.store.ListMeetingSegments(id)
	if err != nil {
		return mtg, nil, err
	}
	return mtg, segs, nil
}

// Delete removes meeting records and associated segments.
func (m *Manager) Delete(id string) error {
	if m.store == nil {
		return errors.New("storage engine is not configured")
	}
	return m.store.DeleteMeeting(id)
}

// Diarizer returns the active speaker diarization engine.
func (m *Manager) Diarizer() *diarize.Engine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.diarizer
}

// DiarizeMeeting executes speaker diarization on a recorded meeting audio file,
// aligns speaker identities to transcript segments, and updates them in persistent storage.
func (m *Manager) DiarizeMeeting(ctx context.Context, meetingID string, opts diarize.Options) ([]storage.MeetingSegment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	if m.diarizer == nil {
		return nil, errors.New("speaker diarizer is not configured")
	}
	if strings.TrimSpace(meetingID) == "" {
		return nil, errors.New("missing meeting id")
	}

	mtg, err := m.store.GetMeeting(meetingID)
	if err != nil || mtg == nil {
		return nil, fmt.Errorf("meeting not found: %s", meetingID)
	}

	if mtg.AudioPath == "" {
		return nil, errors.New("meeting audio file path is empty")
	}

	segs, err := m.store.ListMeetingSegments(meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list meeting segments: %w", err)
	}

	diarized, err := m.diarizer.Diarize(ctx, mtg.AudioPath, opts)
	if err != nil {
		return nil, fmt.Errorf("diarization failed: %w", err)
	}

	aligned := m.diarizer.AlignMeetingSegments(segs, diarized)
	for _, seg := range aligned {
		_ = m.store.SaveMeetingSegment(seg)
	}

	return aligned, nil
}

// RenameSpeaker assigns a custom human-readable name to a speaker across all segments of a meeting.
func (m *Manager) RenameSpeaker(ctx context.Context, meetingID, fromSpeaker, toName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.store == nil {
		return errors.New("storage engine is not configured")
	}
	if strings.TrimSpace(meetingID) == "" {
		return errors.New("missing meeting id")
	}
	if strings.TrimSpace(fromSpeaker) == "" || strings.TrimSpace(toName) == "" {
		return errors.New("fromSpeaker and toName cannot be empty")
	}

	return m.store.RenameMeetingSpeaker(meetingID, fromSpeaker, toName)
}

// ListSpeakers returns known speaker aliases and names associated with a meeting.
func (m *Manager) ListSpeakers(ctx context.Context, meetingID string) ([]storage.MeetingSpeaker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	if strings.TrimSpace(meetingID) == "" {
		return nil, errors.New("missing meeting id")
	}

	return m.store.ListMeetingSpeakers(meetingID)
}

// GetTranscriptTurns returns the meeting's conversation consolidated into dialogue turns.
func (m *Manager) GetTranscriptTurns(ctx context.Context, meetingID string) ([]Turn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	if strings.TrimSpace(meetingID) == "" {
		return nil, errors.New("missing meeting id")
	}

	segs, err := m.store.ListMeetingSegments(meetingID)
	if err != nil {
		return nil, err
	}

	return AggregateTurns(segs), nil
}

// onSegmentProduced handles newly partitioned segments, invokes ASR if configured, and saves to database.
func (m *Manager) onSegmentProduced(seg Segment, samples []float32) {
	// Live transcription if ASR engine is present
	if m.asrEngine != nil && len(samples) > 0 {
		chunks := []audio.Chunk{
			{
				Samples:    samples,
				SampleRate: m.sampleRate,
				Timestamp:  time.Now(),
			},
		}
		if res, err := m.asrEngine.Transcribe(context.Background(), chunks); err == nil {
			seg.Text = strings.TrimSpace(res.Text)
		}
	}

	m.mu.Lock()
	if m.activeSession != nil && m.activeSession.ID == seg.MeetingID {
		m.activeSession.Segments = append(m.activeSession.Segments, seg)
	}
	m.mu.Unlock()

	// Persist to store
	if m.store != nil {
		_ = m.store.SaveMeetingSegment(storage.MeetingSegment{
			ID:           seg.ID,
			MeetingID:    seg.MeetingID,
			SegmentIndex: seg.SegmentIndex,
			StartMs:      seg.StartMs,
			EndMs:        seg.EndMs,
			Text:         seg.Text,
			Speaker:      seg.Speaker,
			CreatedAt:    seg.CreatedAt,
		})
	}
}
