package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestKnowledgeManager_Folders(t *testing.T) {
	store := storage.NewMemory()
	mgr := NewManager(store)
	ctx := context.Background()

	// 1. Create folders
	f1, err := mgr.CreateFolder(ctx, "Engineering", "")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if f1.Name != "Engineering" {
		t.Errorf("expected name Engineering, got %s", f1.Name)
	}

	f2, err := mgr.CreateFolder(ctx, "Personal", "")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	// 2. Resolve by exact ID and by name (case-insensitive)
	resolvedByID, err := mgr.ResolveFolder(ctx, f1.ID)
	if err != nil || resolvedByID == nil || resolvedByID.Name != "Engineering" {
		t.Fatalf("ResolveFolder by ID failed: %v, got %+v", err, resolvedByID)
	}

	resolvedByName, err := mgr.ResolveFolder(ctx, "engineering")
	if err != nil || resolvedByName == nil || resolvedByName.ID != f1.ID {
		t.Fatalf("ResolveFolder by Name failed: %v, got %+v", err, resolvedByName)
	}

	// 3. List folders
	folders, err := mgr.ListFolders(ctx)
	if err != nil || len(folders) != 2 {
		t.Fatalf("ListFolders failed: %v, got %d folders", err, len(folders))
	}

	// 4. Delete folder
	if err := mgr.DeleteFolder(ctx, f2.ID); err != nil {
		t.Fatalf("DeleteFolder failed: %v", err)
	}
	foldersAfter, err := mgr.ListFolders(ctx)
	if err != nil || len(foldersAfter) != 1 {
		t.Fatalf("expected 1 folder after delete, got %d", len(foldersAfter))
	}
}

func TestKnowledgeManager_NotesAndTags(t *testing.T) {
	store := storage.NewMemory()
	mgr := NewManager(store)
	ctx := context.Background()

	// 1. Create note with implicit title and auto-folder creation
	note1, err := mgr.CreateNote(ctx, CreateNoteOptions{
		Content: "# System Architecture\n\nDetailed specifications for the engine.",
		Folder:  "Specs",
		Tags:    []string{"architecture", "v2"},
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	if note1.Title != "System Architecture" {
		t.Errorf("expected title 'System Architecture', got %q", note1.Title)
	}
	if len(note1.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(note1.Tags))
	}

	// Folder "Specs" should have been auto-created
	folderSpecs, err := mgr.ResolveFolder(ctx, "Specs")
	if err != nil || folderSpecs == nil {
		t.Fatalf("expected folder Specs to be auto-created")
	}
	if note1.FolderID != folderSpecs.ID {
		t.Errorf("expected note to be in folder %s, got %s", folderSpecs.ID, note1.FolderID)
	}

	// 2. Get note
	got, err := mgr.GetNote(ctx, note1.ID)
	if err != nil || got == nil {
		t.Fatalf("GetNote failed: %v", err)
	}
	if got.Content != note1.Content {
		t.Errorf("unexpected content: %q", got.Content)
	}

	// 3. Update note
	newContent := "# System Architecture v2\n\nUpdated specifications."
	updated, err := mgr.UpdateNote(ctx, note1.ID, UpdateNoteOptions{
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}
	if updated.Content != newContent {
		t.Errorf("content not updated: %q", updated.Content)
	}

	// 4. List notes with filter
	list, err := mgr.ListNotes(ctx, storage.NoteFilter{Tag: "architecture"})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListNotes by tag failed: %v, got %d", err, len(list))
	}

	// 5. Tags list
	tags, err := mgr.ListTags(ctx)
	if err != nil || len(tags) != 2 {
		t.Fatalf("ListTags failed: %v, got %+v", err, tags)
	}

	// 6. Delete tag
	if err := mgr.DeleteTag(ctx, "v2"); err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}
	noteAfterTagDel, err := mgr.GetNote(ctx, note1.ID)
	if err != nil || len(noteAfterTagDel.Tags) != 1 || noteAfterTagDel.Tags[0] != "architecture" {
		t.Fatalf("expected only architecture tag remaining, got %+v", noteAfterTagDel.Tags)
	}

	// 7. Delete note
	if err := mgr.DeleteNote(ctx, note1.ID); err != nil {
		t.Fatalf("DeleteNote failed: %v", err)
	}
	deleted, err := mgr.GetNote(ctx, note1.ID)
	if err != nil || deleted != nil {
		t.Fatalf("expected nil note after delete, got %+v", deleted)
	}
}

func TestKnowledgeManager_CreateNoteFromMeeting(t *testing.T) {
	store := storage.NewMemory()
	mgr := NewManager(store)
	ctx := context.Background()

	mtgID := "mtg-note-test"
	now := time.Now()

	_ = store.SaveMeeting(storage.Meeting{
		ID:         mtgID,
		Title:      "Q3 Kickoff Sync",
		State:      "stopped",
		DurationMs: 65000,
		CreatedAt:  now,
	})

	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        15000,
		Speaker:      "Alice",
		Text:         "Welcome to the kickoff.",
		CreatedAt:    now,
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-2",
		MeetingID:    mtgID,
		SegmentIndex: 1,
		StartMs:      15000,
		EndMs:        30000,
		Speaker:      "Bob",
		Text:         "We agreed to launch on October 1st.",
		CreatedAt:    now,
	})

	_ = store.SaveMeetingSummary(storage.MeetingSummary{
		MeetingID: mtgID,
		Overview:  "Kickoff meeting aligning on launch dates and scope.",
		Decisions: []string{"Launch on October 1st"},
		ActionItems: []storage.ActionItem{
			{
				ID:          "act-1",
				Description: "Prepare release notes",
				Assignee:    "Bob",
				Completed:   false,
			},
		},
		Topics:    []string{"Launch", "Timeline"},
		CreatedAt: now,
		UpdatedAt: now,
	})

	note, err := mgr.CreateNoteFromMeeting(ctx, mtgID, "Meetings")
	if err != nil {
		t.Fatalf("CreateNoteFromMeeting failed: %v", err)
	}

	if note.MeetingID != mtgID {
		t.Errorf("expected meeting_id %s, got %s", mtgID, note.MeetingID)
	}
	if !strings.Contains(note.Content, "# Q3 Kickoff Sync") {
		t.Errorf("expected title heading in content")
	}
	if !strings.Contains(note.Content, "## Key Decisions") || !strings.Contains(note.Content, "Launch on October 1st") {
		t.Errorf("expected decisions section in content")
	}
	if !strings.Contains(note.Content, "## Action Items") || !strings.Contains(note.Content, "[ ] Prepare release notes (@Bob)") {
		t.Errorf("expected action items section in content")
	}
	if !strings.Contains(note.Content, "## Dialogue Transcript") || !strings.Contains(note.Content, "Alice") {
		t.Errorf("expected transcript section in content")
	}
}
