package edit

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PreviewStatus represents the state of a proposed edit in the review workflow.
type PreviewStatus string

const (
	PreviewPending  PreviewStatus = "pending_review"
	PreviewApplied  PreviewStatus = "applied"
	PreviewRejected PreviewStatus = "rejected"
)

// PreviewSession holds the proposal state for an interactive edit review.
type PreviewSession struct {
	ID            string        `json:"id"`
	OriginalText  string        `json:"original_text"`
	CandidateText string        `json:"candidate_text"`
	Instruction   Instruction   `json:"instruction"`
	DiffSummary   string        `json:"diff_summary"`
	Status        PreviewStatus `json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
	ResolvedAt    *time.Time    `json:"resolved_at,omitempty"`
}

var (
	ErrPreviewNotFound = errors.New("preview session not found")
	ErrPreviewResolved = errors.New("preview session has already been resolved")
)

// PreviewManager manages preview sessions for voice text editing.
type PreviewManager struct {
	mu       sync.RWMutex
	seq      uint64
	sessions map[string]*PreviewSession
	order    []string
	maxKeep  int
}

// NewPreviewManager creates a new preview session manager.
func NewPreviewManager(maxKeep int) *PreviewManager {
	if maxKeep <= 0 {
		maxKeep = 50
	}
	return &PreviewManager{
		sessions: make(map[string]*PreviewSession),
		maxKeep:  maxKeep,
	}
}

// Create registers a new candidate transformation for user preview and review.
func (m *PreviewManager) Create(original, candidate string, instr Instruction) *PreviewSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.seq++
	id := fmt.Sprintf("prev-%d-%d", time.Now().UnixNano(), m.seq)
	sess := &PreviewSession{
		ID:            id,
		OriginalText:  original,
		CandidateText: candidate,
		Instruction:   instr,
		DiffSummary:   GenerateDiffSummary(original, candidate),
		Status:        PreviewPending,
		CreatedAt:     time.Now(),
	}

	m.sessions[id] = sess
	m.order = append(m.order, id)

	// Prune old sessions if exceeding max
	for len(m.order) > m.maxKeep {
		oldestID := m.order[0]
		m.order = m.order[1:]
		delete(m.sessions, oldestID)
	}

	return sess
}

// Get returns the preview session with the given ID.
func (m *PreviewManager) Get(id string) (*PreviewSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, ErrPreviewNotFound
	}
	cpy := *sess
	return &cpy, nil
}

// Apply marks the preview session as applied.
func (m *PreviewManager) Apply(id string) (*PreviewSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, ErrPreviewNotFound
	}
	if sess.Status != PreviewPending {
		return nil, fmt.Errorf("%w: current status is %s", ErrPreviewResolved, sess.Status)
	}

	now := time.Now()
	sess.Status = PreviewApplied
	sess.ResolvedAt = &now

	cpy := *sess
	return &cpy, nil
}

// Reject marks the preview session as rejected.
func (m *PreviewManager) Reject(id string) (*PreviewSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, ErrPreviewNotFound
	}
	if sess.Status != PreviewPending {
		return nil, fmt.Errorf("%w: current status is %s", ErrPreviewResolved, sess.Status)
	}

	now := time.Now()
	sess.Status = PreviewRejected
	sess.ResolvedAt = &now

	cpy := *sess
	return &cpy, nil
}

// ListPending returns all preview sessions waiting for user review.
func (m *PreviewManager) ListPending() []*PreviewSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []*PreviewSession
	for _, id := range m.order {
		if sess, ok := m.sessions[id]; ok && sess.Status == PreviewPending {
			cpy := *sess
			pending = append(pending, &cpy)
		}
	}
	return pending
}

// GenerateDiffSummary builds a line-by-line diff comparing original and candidate text.
func GenerateDiffSummary(original, candidate string) string {
	origLines := strings.Split(original, "\n")
	candLines := strings.Split(candidate, "\n")

	var sb strings.Builder

	// Quick check if identical
	if original == candidate {
		return "  (no changes)"
	}

	for _, l := range origLines {
		sb.WriteString("- " + l + "\n")
	}
	for _, l := range candLines {
		sb.WriteString("+ " + l + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
