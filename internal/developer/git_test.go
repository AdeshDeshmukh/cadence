package developer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v (output: %s)", strings.Join(args, " "), err, string(out))
		}
	}

	runGit("init")
	runGit("checkout", "-B", "main")
	runGit("config", "user.name", "Cadence Test")
	runGit("config", "user.email", "test@cadence.local")

	// Create initial commit
	initFile := filepath.Join(dir, "README.md")
	_ = os.WriteFile(initFile, []byte("# Test Repo\n"), 0644)
	runGit("add", "README.md")
	runGit("commit", "-m", "chore: initial commit")

	return dir
}

func TestGitAssistant_StagedDiff(t *testing.T) {
	dir := setupTestGitRepo(t)
	ctx := context.Background()
	assistant := NewGitAssistant(nil)

	// 1. Initially no staged changes
	summary, err := assistant.GetStagedDiff(ctx, dir)
	if err != nil {
		t.Fatalf("staged diff error: %v", err)
	}
	if summary.FilesChanged != 0 || len(summary.Files) != 0 {
		t.Fatalf("expected 0 staged files, got %d", summary.FilesChanged)
	}

	// 2. Stage a file
	testFile := filepath.Join(dir, "main.go")
	_ = os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	cmd := exec.Command("git", "add", "main.go")
	cmd.Dir = dir
	_ = cmd.Run()

	summary, err = assistant.GetStagedDiff(ctx, dir)
	if err != nil {
		t.Fatalf("staged diff error after add: %v", err)
	}
	if summary.FilesChanged != 1 || summary.Files[0] != "main.go" {
		t.Fatalf("expected main.go staged, got %+v", summary)
	}
	if !strings.Contains(summary.RawDiff, "package main") {
		t.Fatalf("expected raw diff to contain package main, got %s", summary.RawDiff)
	}
}

func TestGitAssistant_GenerateCommit_NoStagedChanges(t *testing.T) {
	dir := setupTestGitRepo(t)
	ctx := context.Background()
	assistant := NewGitAssistant(nil)

	_, err := assistant.GenerateCommit(ctx, CommitGenRequest{WorkingDir: dir})
	if err == nil {
		t.Fatal("expected error on empty staged changes")
	}
	if !strings.Contains(err.Error(), "no staged changes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitAssistant_GenerateCommit_WithSpokenHint(t *testing.T) {
	dir := setupTestGitRepo(t)
	ctx := context.Background()
	assistant := NewGitAssistant(nil)

	// Stage a fix in auth
	authDir := filepath.Join(dir, "auth")
	_ = os.MkdirAll(authDir, 0755)
	_ = os.WriteFile(filepath.Join(authDir, "jwt.go"), []byte("package auth\n\n// Token verification\n"), 0644)

	cmd := exec.Command("git", "add", "auth/jwt.go")
	cmd.Dir = dir
	_ = cmd.Run()

	commitMsg, err := assistant.GenerateCommit(ctx, CommitGenRequest{
		WorkingDir: dir,
		SpokenHint: "fix jwt token expiration check",
	})
	if err != nil {
		t.Fatalf("generate commit failed: %v", err)
	}

	if !strings.HasPrefix(commitMsg.Title, "fix") {
		t.Fatalf("expected title to start with fix, got %s", commitMsg.Title)
	}
	if !strings.Contains(commitMsg.Title, "jwt token expiration check") {
		t.Fatalf("expected hint in title, got %s", commitMsg.Title)
	}
	if commitMsg.Scope != "auth" {
		t.Fatalf("expected scope auth, got %s", commitMsg.Scope)
	}
	if !strings.Contains(commitMsg.Body, "auth/jwt.go") {
		t.Fatalf("expected body to mention auth/jwt.go, got %s", commitMsg.Body)
	}
}

func TestGitAssistant_GenerateCommit_LLMSynthesis(t *testing.T) {
	dir := setupTestGitRepo(t)
	ctx := context.Background()

	_ = os.WriteFile(filepath.Join(dir, "service.go"), []byte("package main\n"), 0644)
	cmd := exec.Command("git", "add", "service.go")
	cmd.Dir = dir
	_ = cmd.Run()

	mockLLM := mockPolisher{
		output: "feat(service): add background health check worker\n\nImplements periodic pinging of external services with exponential backoff.",
	}
	assistant := NewGitAssistant(mockLLM)

	commitMsg, err := assistant.GenerateCommit(ctx, CommitGenRequest{
		WorkingDir: dir,
		SpokenHint: "add health check worker",
	})
	if err != nil {
		t.Fatalf("generate commit with LLM failed: %v", err)
	}

	if commitMsg.Title != "feat(service): add background health check worker" {
		t.Fatalf("unexpected title: %s", commitMsg.Title)
	}
	if commitMsg.Type != "feat" || commitMsg.Scope != "service" {
		t.Fatalf("unexpected type/scope: %s/%s", commitMsg.Type, commitMsg.Scope)
	}
	if !strings.Contains(commitMsg.Body, "exponential backoff") {
		t.Fatalf("unexpected body: %s", commitMsg.Body)
	}
}

func TestGitAssistant_GeneratePR(t *testing.T) {
	dir := setupTestGitRepo(t)
	ctx := context.Background()

	// Create feature branch
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		_ = cmd.Run()
	}

	runGit("checkout", "-b", "feat/voice-editor")
	_ = os.WriteFile(filepath.Join(dir, "editor.go"), []byte("package editor\n"), 0644)
	runGit("add", "editor.go")
	runGit("commit", "-m", "feat(editor): implement voice editing primitives")

	assistant := NewGitAssistant(nil)
	prDesc, err := assistant.GeneratePR(ctx, PRGenRequest{
		WorkingDir:  dir,
		BaseBranch:  "main",
		SpokenNotes: "Adds initial voice editing primitives and token matching.",
	})
	if err != nil {
		t.Fatalf("generate PR failed: %v", err)
	}

	if !strings.Contains(prDesc.Title, "voice") && !strings.Contains(prDesc.Title, "feat") {
		t.Fatalf("unexpected PR title: %s", prDesc.Title)
	}
	if !strings.Contains(prDesc.Body, "Overview") || !strings.Contains(prDesc.Body, "Key Changes") {
		t.Fatalf("unexpected PR markdown body: %s", prDesc.Body)
	}
	if !strings.Contains(prDesc.Body, "voice editing primitives") {
		t.Fatalf("expected spoken notes in PR body, got %s", prDesc.Body)
	}
	if prDesc.CommitCount != 1 {
		t.Fatalf("expected 1 commit count, got %d", prDesc.CommitCount)
	}
}

func TestGitAssistant_HelperFunctions(t *testing.T) {
	// 1. Commit type & scope extraction
	cType, scope := extractCommitTypeAndScope("feat(api): add new endpoints")
	if cType != "feat" || scope != "api" {
		t.Fatalf("expected feat/api, got %s/%s", cType, scope)
	}

	cType, scope = extractCommitTypeAndScope("fix: resolve nil pointer crash")
	if cType != "fix" || scope != "" {
		t.Fatalf("expected fix/'', got %s/%s", cType, scope)
	}

	// 2. Stat counts
	ins, del := parseStatCounts(" 3 files changed, 45 insertions(+), 12 deletions(-)")
	if ins != 45 || del != 12 {
		t.Fatalf("expected 45/12, got %d/%d", ins, del)
	}
}
