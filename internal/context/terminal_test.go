package appcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTerminalContextManager_SetAndGet(t *testing.T) {
	mgr := NewTerminalContextManager()

	if mgr.IsActive() {
		t.Fatal("expected manager to be inactive initially")
	}
	if summary := mgr.FormatContextSummary(); summary != "No active terminal context" {
		t.Fatalf("unexpected summary: %s", summary)
	}

	// Create a temp git repository to test git branch resolution
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("create temp .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/voice-ops\n"), 0644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	subDir := filepath.Join(tmpDir, "src", "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	ctx := TerminalContext{
		TerminalApp:  "iTerm2",
		Shell:        "zsh",
		PID:          45678,
		WorkingDir:   subDir,
		LastCommand:  "go test -v ./...",
		LastExitCode: 0,
	}

	mgr.Set(ctx)

	if !mgr.IsActive() {
		t.Fatal("expected manager to be active after Set")
	}

	retrieved, ok := mgr.Get()
	if !ok {
		t.Fatal("expected Get() ok=true")
	}
	if retrieved.TerminalApp != "iTerm2" || retrieved.Shell != "zsh" {
		t.Fatalf("unexpected app or shell: %+v", retrieved)
	}
	if retrieved.GitBranch != "feature/voice-ops" {
		t.Fatalf("expected resolved git branch 'feature/voice-ops', got %q", retrieved.GitBranch)
	}
	if retrieved.LastCommand != "go test -v ./..." {
		t.Fatalf("unexpected last command: %s", retrieved.LastCommand)
	}
	if len(retrieved.RecentCommands) != 1 || retrieved.RecentCommands[0] != "go test -v ./..." {
		t.Fatalf("unexpected recent commands: %+v", retrieved.RecentCommands)
	}

	// Record another command
	mgr.RecordCommand("git status", 0, subDir)
	retrieved2, _ := mgr.Get()
	if len(retrieved2.RecentCommands) != 2 || retrieved2.RecentCommands[1] != "git status" {
		t.Fatalf("expected 2 recent commands, got %+v", retrieved2.RecentCommands)
	}
	if retrieved2.LastCommand != "git status" {
		t.Fatalf("expected last command 'git status', got %s", retrieved2.LastCommand)
	}

	summary := mgr.FormatContextSummary()
	if !strings.Contains(summary, "Terminal: iTerm2") ||
		!strings.Contains(summary, "Shell: zsh") ||
		!strings.Contains(summary, "feature/voice-ops") ||
		!strings.Contains(summary, "git status") {
		t.Fatalf("unexpected summary format:\n%s", summary)
	}

	promptBlock := mgr.FormatPromptContext()
	if !strings.Contains(promptBlock, "[Terminal & Shell Context]") ||
		!strings.Contains(promptBlock, "Shell: zsh (PID: 45678)") ||
		!strings.Contains(promptBlock, "Git Branch: feature/voice-ops") ||
		!strings.Contains(promptBlock, "Last Executed Command: git status (Exit Code: 0)") {
		t.Fatalf("unexpected prompt block:\n%s", promptBlock)
	}

	// Clear
	mgr.Clear()
	if mgr.IsActive() {
		t.Fatal("expected manager to be inactive after Clear")
	}
	if _, ok := mgr.Get(); ok {
		t.Fatal("expected Get() ok=false after Clear")
	}
}

func TestTerminalContextManager_TTL(t *testing.T) {
	mgr := &TerminalContextManager{
		ttl: 50 * time.Millisecond,
	}

	mgr.Set(TerminalContext{
		TerminalApp: "Ghostty",
		WorkingDir:  "/tmp",
	})

	if !mgr.IsActive() {
		t.Fatal("expected manager active immediately")
	}

	time.Sleep(70 * time.Millisecond)

	if mgr.IsActive() {
		t.Fatal("expected manager expired after TTL")
	}
}

func TestDetectGitBranch(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. No git repo
	if branch := DetectGitBranch(tmpDir); branch != "" {
		t.Fatalf("expected empty branch for non-git directory, got %q", branch)
	}

	// 2. Standard branch
	gitDir := filepath.Join(tmpDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	if branch := DetectGitBranch(tmpDir); branch != "main" {
		t.Fatalf("expected branch 'main', got %q", branch)
	}

	// 3. Detached HEAD
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("a1b2c3d4e5f6\n"), 0644)
	if branch := DetectGitBranch(tmpDir); branch != "a1b2c3d" {
		t.Fatalf("expected detached short sha 'a1b2c3d', got %q", branch)
	}

	// 4. Git worktree (.git file)
	worktreeDir := t.TempDir()
	commonGitDir := filepath.Join(t.TempDir(), "main-repo", ".git", "worktrees", "wt1")
	_ = os.MkdirAll(commonGitDir, 0755)
	_ = os.WriteFile(filepath.Join(commonGitDir, "HEAD"), []byte("ref: refs/heads/worktree-branch\n"), 0644)
	_ = os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: "+commonGitDir+"\n"), 0644)

	if branch := DetectGitBranch(worktreeDir); branch != "worktree-branch" {
		t.Fatalf("expected worktree branch 'worktree-branch', got %q", branch)
	}
}
