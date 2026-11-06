package appcontext

import (
	"strings"
	"testing"
	"time"
)

func TestEditorContextManager_SetAndGet(t *testing.T) {
	mgr := NewEditorContextManager()

	// Initial state should be empty/inactive
	if mgr.IsActive() {
		t.Fatal("expected manager to be inactive initially")
	}
	if summary := mgr.FormatContextSummary(); summary != "No active editor context" {
		t.Fatalf("unexpected summary: %s", summary)
	}

	// Set context
	ctx := EditorContext{
		EditorName:    "vscode",
		WorkspaceRoot: "/workspace/project",
		FilePath:      "/workspace/project/cmd/main.go",
		Line:          42,
		Column:        10,
		SelectionText: "fmt.Println(\"hello world\")",
		SelectionRange: &SelectionRange{
			StartLine: 42,
			StartCol:  1,
			EndLine:   42,
			EndCol:    28,
		},
		SurroundingContext: "package main\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}",
	}

	mgr.Set(ctx)

	if !mgr.IsActive() {
		t.Fatal("expected manager to be active after Set")
	}

	retrieved, ok := mgr.Get()
	if !ok {
		t.Fatal("expected Get() to return ok=true")
	}
	if retrieved.FileName != "main.go" {
		t.Errorf("expected auto-populated FileName 'main.go', got '%s'", retrieved.FileName)
	}
	if retrieved.LanguageID != "go" {
		t.Errorf("expected auto-inferred LanguageID 'go', got '%s'", retrieved.LanguageID)
	}
	if retrieved.SelectionText != ctx.SelectionText {
		t.Errorf("expected selection text '%s', got '%s'", ctx.SelectionText, retrieved.SelectionText)
	}

	summary := mgr.FormatContextSummary()
	if !strings.Contains(summary, "Editor: vscode") || !strings.Contains(summary, "File: main.go") || !strings.Contains(summary, "Lang: go") {
		t.Errorf("unexpected summary format: %s", summary)
	}

	promptCtx := mgr.FormatPromptContext()
	if !strings.Contains(promptCtx, "[Code Editor Context]") ||
		!strings.Contains(promptCtx, "Language: go") ||
		!strings.Contains(promptCtx, "Selected Text:\n```go\nfmt.Println(\"hello world\")") {
		t.Errorf("unexpected prompt context:\n%s", promptCtx)
	}

	// Clear
	mgr.Clear()
	if mgr.IsActive() {
		t.Fatal("expected manager to be inactive after Clear")
	}
	if _, ok := mgr.Get(); ok {
		t.Fatal("expected Get() to return ok=false after Clear")
	}
}

func TestEditorContextManager_TTL(t *testing.T) {
	mgr := &EditorContextManager{
		ttl: 50 * time.Millisecond,
	}

	mgr.Set(EditorContext{
		EditorName: "cursor",
		FilePath:   "app.py",
	})

	if !mgr.IsActive() {
		t.Fatal("expected manager to be active immediately")
	}

	time.Sleep(70 * time.Millisecond)

	if mgr.IsActive() {
		t.Fatal("expected manager context to expire after TTL")
	}
	if _, ok := mgr.Get(); ok {
		t.Fatal("expected Get() to return false after TTL expiration")
	}
}

func TestGuessLanguage(t *testing.T) {
	cases := map[string]string{
		"main.go":           "go",
		"script.py":         "python",
		"component.tsx":     "typescriptreact",
		"index.ts":          "typescript",
		"app.js":            "javascript",
		"style.css":         "css",
		"config.yaml":       "yaml",
		"data.json":         "json",
		"lib.rs":            "rust",
		"run.sh":            "shellscript",
		"unknown.xyz123":    "",
	}

	for path, expected := range cases {
		got := GuessLanguage(path)
		if got != expected {
			t.Errorf("GuessLanguage(%q) = %q, expected %q", path, got, expected)
		}
	}
}
