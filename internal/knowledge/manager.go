package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/meeting"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// CreateNoteOptions defines the arguments for creating a new note.
type CreateNoteOptions struct {
	ID        string   `json:"id,omitempty"`
	Title     string   `json:"title,omitempty"`
	Content   string   `json:"content"`
	Folder    string   `json:"folder,omitempty"` // folder ID or name
	Tags      []string `json:"tags,omitempty"`
	MeetingID string   `json:"meeting_id,omitempty"`
}

// UpdateNoteOptions defines parameters for updating an existing note.
type UpdateNoteOptions struct {
	Title    *string   `json:"title,omitempty"`
	Content  *string   `json:"content,omitempty"`
	FolderID *string   `json:"folder_id,omitempty"`
	Tags     *[]string `json:"tags,omitempty"`
}

// Manager coordinates operations on the local personal knowledge base.
type Manager struct {
	store storage.Store
}

// NewManager creates a knowledge base Manager backed by the provided Store.
func NewManager(store storage.Store) *Manager {
	return &Manager{store: store}
}

// CreateFolder creates a new folder/notebook with the given name and optional parent.
func (m *Manager) CreateFolder(_ context.Context, name string, parentID string) (*storage.Folder, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("folder name cannot be empty")
	}

	id := fmt.Sprintf("f-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	f := storage.Folder{
		ID:        id,
		Name:      trimmed,
		ParentID:  strings.TrimSpace(parentID),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.store.SaveFolder(f); err != nil {
		return nil, fmt.Errorf("save folder: %w", err)
	}
	return &f, nil
}

// ResolveFolder resolves a folder by its exact ID or by case-insensitive name.
func (m *Manager) ResolveFolder(_ context.Context, idOrName string) (*storage.Folder, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	query := strings.TrimSpace(idOrName)
	if query == "" {
		return nil, nil
	}

	// Try direct lookup by ID
	if f, err := m.store.GetFolder(query); err == nil && f != nil {
		return f, nil
	}

	// Try lookup by name
	folders, err := m.store.ListFolders()
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		if strings.EqualFold(f.Name, query) {
			res := f
			return &res, nil
		}
	}
	return nil, nil
}

// ListFolders lists all folders.
func (m *Manager) ListFolders(_ context.Context) ([]storage.Folder, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	return m.store.ListFolders()
}

// DeleteFolder deletes a folder by ID.
func (m *Manager) DeleteFolder(_ context.Context, id string) error {
	if m.store == nil {
		return errors.New("storage engine is not configured")
	}
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return errors.New("folder id is required")
	}
	return m.store.DeleteFolder(trimmed)
}

// CreateNote creates a new document in the knowledge base.
func (m *Manager) CreateNote(ctx context.Context, opts CreateNoteOptions) (*storage.Note, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}

	noteID := opts.ID
	if strings.TrimSpace(noteID) == "" {
		noteID = fmt.Sprintf("note-%d", time.Now().UnixNano())
	}

	folderID := ""
	if strings.TrimSpace(opts.Folder) != "" {
		resolved, err := m.ResolveFolder(ctx, opts.Folder)
		if err != nil {
			return nil, fmt.Errorf("resolve folder: %w", err)
		}
		if resolved != nil {
			folderID = resolved.ID
		} else {
			// Auto-create folder if named
			created, err := m.CreateFolder(ctx, opts.Folder, "")
			if err != nil {
				return nil, fmt.Errorf("auto-create folder: %w", err)
			}
			folderID = created.ID
		}
	}

	content := opts.Content
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = extractTitleFromContent(content)
	}

	now := time.Now().UTC()
	note := storage.Note{
		ID:        noteID,
		FolderID:  folderID,
		Title:     title,
		Content:   content,
		MeetingID: strings.TrimSpace(opts.MeetingID),
		Tags:      opts.Tags,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.store.SaveNote(note); err != nil {
		return nil, fmt.Errorf("save note: %w", err)
	}

	return m.store.GetNote(noteID)
}

// GetNote retrieves a note by ID.
func (m *Manager) GetNote(_ context.Context, id string) (*storage.Note, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil, errors.New("note id is required")
	}
	return m.store.GetNote(trimmed)
}

// UpdateNote updates fields of an existing note.
func (m *Manager) UpdateNote(ctx context.Context, id string, opts UpdateNoteOptions) (*storage.Note, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	note, err := m.GetNote(ctx, id)
	if err != nil {
		return nil, err
	}
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", id)
	}

	if opts.Title != nil {
		note.Title = strings.TrimSpace(*opts.Title)
	}
	if opts.Content != nil {
		note.Content = *opts.Content
	}
	if opts.FolderID != nil {
		note.FolderID = strings.TrimSpace(*opts.FolderID)
	}
	if opts.Tags != nil {
		note.Tags = *opts.Tags
	}
	note.UpdatedAt = time.Now().UTC()

	if err := m.store.SaveNote(*note); err != nil {
		return nil, fmt.Errorf("save updated note: %w", err)
	}
	return m.store.GetNote(id)
}

// ListNotes lists notes matching the given filter.
func (m *Manager) ListNotes(_ context.Context, filter storage.NoteFilter) ([]storage.Note, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	return m.store.ListNotes(filter)
}

// DeleteNote deletes a note by ID.
func (m *Manager) DeleteNote(_ context.Context, id string) error {
	if m.store == nil {
		return errors.New("storage engine is not configured")
	}
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return errors.New("note id is required")
	}
	return m.store.DeleteNote(trimmed)
}

// ListTags lists all tags in the knowledge base.
func (m *Manager) ListTags(_ context.Context) ([]storage.Tag, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	return m.store.ListTags()
}

// SetNoteTags sets the tags for a specific note.
func (m *Manager) SetNoteTags(_ context.Context, noteID string, tags []string) error {
	if m.store == nil {
		return errors.New("storage engine is not configured")
	}
	return m.store.SetNoteTags(noteID, tags)
}

// DeleteTag removes a tag entirely.
func (m *Manager) DeleteTag(_ context.Context, name string) error {
	if m.store == nil {
		return errors.New("storage engine is not configured")
	}
	return m.store.DeleteTag(name)
}

// CreateNoteFromMeeting synthesizes a comprehensive markdown document from a meeting and stores it as a linked note.
func (m *Manager) CreateNoteFromMeeting(ctx context.Context, meetingID string, folderID string) (*storage.Note, error) {
	if m.store == nil {
		return nil, errors.New("storage engine is not configured")
	}
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return nil, errors.New("meeting id is required")
	}

	mtg, err := m.store.GetMeeting(meetingID)
	if err != nil {
		return nil, fmt.Errorf("get meeting: %w", err)
	}
	if mtg == nil {
		return nil, fmt.Errorf("meeting not found: %s", meetingID)
	}

	segments, err := m.store.ListMeetingSegments(meetingID)
	if err != nil {
		return nil, fmt.Errorf("list meeting segments: %w", err)
	}

	summary, _ := m.store.GetMeetingSummary(meetingID)

	var sb strings.Builder
	title := mtg.Title
	if title == "" {
		title = fmt.Sprintf("Meeting %s", mtg.ID)
	}
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("- **Date:** %s\n", mtg.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	if mtg.DurationMs > 0 {
		duration := time.Duration(mtg.DurationMs) * time.Millisecond
		sb.WriteString(fmt.Sprintf("- **Duration:** %s\n", duration.Round(time.Second)))
	}
	sb.WriteString(fmt.Sprintf("- **Meeting ID:** `%s`\n\n", mtg.ID))

	if summary != nil {
		if summary.Overview != "" {
			sb.WriteString("## Overview\n\n")
			sb.WriteString(summary.Overview)
			sb.WriteString("\n\n")
		}

		if len(summary.Decisions) > 0 {
			sb.WriteString("## Key Decisions\n\n")
			for _, d := range summary.Decisions {
				sb.WriteString(fmt.Sprintf("- %s\n", d))
			}
			sb.WriteString("\n")
		}

		if len(summary.ActionItems) > 0 {
			sb.WriteString("## Action Items\n\n")
			for _, a := range summary.ActionItems {
				status := "[ ]"
				if a.Completed {
					status = "[x]"
				}
				if a.Assignee != "" {
					sb.WriteString(fmt.Sprintf("- %s %s (@%s)\n", status, a.Description, a.Assignee))
				} else {
					sb.WriteString(fmt.Sprintf("- %s %s\n", status, a.Description))
				}
			}
			sb.WriteString("\n")
		}
	}

	turns := meeting.AggregateTurns(segments)
	if len(turns) > 0 {
		sb.WriteString("## Dialogue Transcript\n\n")
		for _, t := range turns {
			banner := meeting.FormatTurnTimestamp(t.StartMs, t.EndMs)
			sb.WriteString(fmt.Sprintf("**%s** `[%s]`\n\n> %s\n\n", t.Speaker, banner, t.Text))
		}
	}

	tags := []string{"meeting"}
	if summary != nil {
		for _, top := range summary.Topics {
			clean := strings.ToLower(strings.TrimSpace(top))
			if clean != "" && !containsTag(tags, clean) {
				tags = append(tags, clean)
			}
		}
	}

	return m.CreateNote(ctx, CreateNoteOptions{
		Title:     title,
		Content:   sb.String(),
		Folder:    folderID,
		Tags:      tags,
		MeetingID: mtg.ID,
	})
}

func containsTag(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}

func extractTitleFromContent(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			clean := strings.TrimLeft(trimmed, "#")
			clean = strings.TrimSpace(clean)
			if clean != "" {
				return clean
			}
		}
		if trimmed != "" {
			if len(trimmed) > 50 {
				return trimmed[:50] + "..."
			}
			return trimmed
		}
	}
	return fmt.Sprintf("Note %s", time.Now().Format("2006-01-02 15:04"))
}
