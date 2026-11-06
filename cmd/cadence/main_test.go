package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

type mockDaemonHandler struct{}

func (m mockDaemonHandler) Handle(_ context.Context, req ipc.Request) (any, error) {
	switch req.Method {
	case "ping":
		return "pong", nil
	case "status":
		return map[string]any{
			"status":    "running",
			"version":   version.Version,
			"recording": false,
			"session":   "test-session",
			"shortcut":  "Cmd+Shift+Space",
			"mode":      "push_to_talk",
		}, nil
	case "dictate_start":
		return map[string]any{"ok": true, "action": "started"}, nil
	case "dictate_stop":
		return map[string]any{"ok": true, "action": "stopped"}, nil
	case "history":
		return []storage.Transcript{
			{
				ID:          "tx-1",
				RawText:     "hello world",
				Text:        "Hello world.",
				Language:    "en",
				Mode:        "push_to_talk",
				Application: "VS Code",
				CreatedAt:   time.Now().UTC(),
			},
		}, nil
	case "recover":
		return storage.SessionRecovery{
			ID:          "rec-mock-1",
			State:       "cancelled",
			PartialText: "mock recovered text",
			RawText:     "mock recovered text raw",
			Text:        "Mock recovered text.",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}, nil
	case "recoveries":
		return []storage.SessionRecovery{
			{
				ID:        "rec-mock-1",
				State:     "cancelled",
				Text:      "Mock recovered text.",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		}, nil
	case "vocab_list":
		return []storage.VocabularyEntry{
			{Spoken: "grpc", Canonical: "gRPC"},
		}, nil
	case "vocab_add":
		return map[string]any{"ok": true}, nil
	case "vocab_remove":
		return map[string]any{"ok": true}, nil
	case "vocab_import":
		return map[string]any{"ok": true, "count": float64(2)}, nil
	case "vocab_export":
		return map[string]any{"ok": true, "content": "grpc = gRPC\n", "format": "txt"}, nil
	case "snippet_list":
		return []storage.Snippet{
			{
				Trigger:     "signoff",
				Template:    "Best regards,\nAlice",
				Description: "Email signoff",
			},
		}, nil
	case "snippet_set":
		return map[string]any{"ok": true}, nil
	case "snippet_delete":
		return map[string]any{"ok": true}, nil
	case "snippet_expand":
		return map[string]any{
			"ok":       true,
			"template": "Best regards,\nAlice",
			"expanded": "Best regards,\nAlice",
		}, nil
	case "polish":
		params, _ := req.Params.(map[string]any)
		text, _ := params["text"].(string)
		return map[string]any{
			"text":       "Polished: " + text,
			"model":      "llama3.2",
			"tokens_in":  10,
			"tokens_out": 12,
			"latency_ms": 42,
		}, nil
	case "polish_profile_list":
		return []map[string]any{
			{"name": "general", "is_builtin": true, "description": "General prose"},
			{"name": "code", "is_builtin": true, "description": "Code formatting"},
		}, nil
	case "polish_profile_get":
		return map[string]any{
			"name":          "code",
			"description":   "Code formatting",
			"system_prompt": "Format code.",
			"temperature":   0.1,
			"is_builtin":    true,
			"custom_rules":  []any{"rule 1"},
		}, nil
	case "polish_profile_set":
		return map[string]any{"ok": true}, nil
	case "polish_profile_delete":
		return map[string]any{"ok": true}, nil
	case "active_app":
		return map[string]any{
			"name":         "Visual Studio Code",
			"bundle_id":    "com.microsoft.VSCode",
			"window_title": "cadence - main.go",
			"pid":          float64(4567),
			"category":     "code",
		}, nil
	case "polish_route_list":
		return map[string]any{"slack": "friendly_chat"}, nil
	case "polish_route_set":
		return map[string]any{"ok": true}, nil
	case "polish_route_delete":
		return map[string]any{"ok": true}, nil
	case "polish_route_test":
		params, _ := req.Params.(map[string]any)
		app, _ := params["app"].(string)
		return map[string]any{
			"app":      app,
			"category": "code",
			"profile":  "code",
		}, nil
	case "undo":
		params, _ := req.Params.(map[string]any)
		target, _ := params["target"].(string)
		if target == "raw" {
			return map[string]any{
				"session_id":    "sess-99",
				"reverted_from": "AI Polished Text",
				"reverted_to":   "raw asr text",
				"target":        "raw",
				"remaining":     1,
			}, nil
		}
		return map[string]any{
			"session_id":    "sess-99",
			"reverted_from": "AI Polished Text",
			"reverted_to":   "unpolished post-processed text",
			"target":        "unpolished",
			"remaining":     1,
		}, nil
	case "undo_list":
		return []map[string]any{
			{
				"session_id":      "sess-99",
				"raw_text":        "raw asr text",
				"unpolished_text": "unpolished post-processed text",
				"polished_text":   "AI Polished Text",
				"is_polished":     true,
				"pasted_at":       time.Now(),
				"undone":          false,
			},
		}, nil
	case "correct":
		params, _ := req.Params.(map[string]any)
		text, _ := params["text"].(string)
		app, _ := params["app"].(string)
		return map[string]any{
			"original":            text,
			"corrected":           "for loop in pytest",
			"app":                 app,
			"category":            "code",
			"corrections_applied": 2,
		}, nil
	case "selection_get":
		return map[string]any{
			"text":        "func calculateSum(a, b int) int",
			"method":      "accessibility_ax",
			"application": "Visual Studio Code",
		}, nil
	case "edit_parse":
		params, _ := req.Params.(map[string]any)
		text, _ := params["text"].(string)
		parser := edit.NewParser(nil)
		return parser.Parse(context.Background(), text), nil
	case "edit_execute":
		params, _ := req.Params.(map[string]any)
		ins, _ := params["instruction"].(string)
		txt, _ := params["text"].(string)
		dryRun, _ := params["dry_run"].(bool)
		if txt == "" {
			txt = "func calculateSum(a, b int) int"
		}
		parser := edit.NewParser(nil)
		instr := parser.Parse(context.Background(), ins)
		return map[string]any{
			"original_text": txt,
			"edited_text":   "// rewritten function\nfunc calculateSum(a, b int) int",
			"instruction":   instr,
			"dry_run":       dryRun,
			"pasted":        !dryRun,
			"model":         "mock-daemon",
			"latency_ms":    12,
		}, nil
	case "edit_preview":
		params, _ := req.Params.(map[string]any)
		ins, _ := params["instruction"].(string)
		txt, _ := params["text"].(string)
		if txt == "" {
			txt = "func calculateSum(a, b int) int"
		}
		parser := edit.NewParser(nil)
		instr := parser.Parse(context.Background(), ins)
		return &edit.PreviewSession{
			ID:            "prev-mock-123",
			OriginalText:  txt,
			CandidateText: "// preview candidate\n" + txt,
			Instruction:   instr,
			DiffSummary:   "- " + txt + "\n+ // preview candidate\n+ " + txt,
			Status:        edit.PreviewPending,
			CreatedAt:     time.Now(),
		}, nil
	case "edit_apply":
		params, _ := req.Params.(map[string]any)
		id, _ := params["id"].(string)
		now := time.Now()
		return &edit.PreviewSession{
			ID:            id,
			OriginalText:  "Original text",
			CandidateText: "Applied text",
			Status:        edit.PreviewApplied,
			CreatedAt:     now,
			ResolvedAt:    &now,
		}, nil
	case "edit_reject":
		params, _ := req.Params.(map[string]any)
		id, _ := params["id"].(string)
		now := time.Now()
		return &edit.PreviewSession{
			ID:            id,
			OriginalText:  "Original text",
			CandidateText: "Candidate text",
			Status:        edit.PreviewRejected,
			CreatedAt:     now,
			ResolvedAt:    &now,
		}, nil
	case "edit_preview_list":
		return []*edit.PreviewSession{
			{
				ID:            "prev-mock-123",
				OriginalText:  "Original",
				CandidateText: "Candidate",
				Instruction:   edit.Instruction{Intent: edit.IntentRewrite},
				Status:        edit.PreviewPending,
				CreatedAt:     time.Now(),
			},
		}, nil
	default:
		return nil, os.ErrInvalid
	}
}

func startMockDaemon(t *testing.T, socketPath string) func() {
	t.Helper()
	_ = os.Remove(socketPath)
	_ = os.MkdirAll(filepath.Dir(socketPath), 0755)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen mock socket: %v", err)
	}

	server := ipc.NewServer(listener, mockDaemonHandler{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = server.Serve(ctx)
	}()

	return func() {
		cancel()
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}
}

func TestCLI_UsageAndVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected usage text, got: %s", stdout.String())
	}

	stdout.Reset()
	code = run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), version.Version) {
		t.Fatalf("expected version in stdout, got: %s", stdout.String())
	}
}

func TestCLI_Config(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"config", "path"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Database:") || !strings.Contains(stdout.String(), "Socket:") {
		t.Fatalf("expected path output, got: %s", stdout.String())
	}

	stdout.Reset()
	code = run([]string{"config", "show"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	var m map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &m); err != nil {
		t.Fatalf("config show is not valid JSON: %v", err)
	}
}

func TestCLI_Status_DaemonOffline(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"status"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1 when daemon offline, got %d", code)
	}
	if !strings.Contains(stderr.String(), "is not running") {
		t.Fatalf("expected offline error message, got: %s", stderr.String())
	}
}

func TestCLI_CommandsWithMockDaemon(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_%d.sock", time.Now().UnixNano()))

	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()

	// 1. Status command
	var out bytes.Buffer
	if err := cmdStatus(ctx, client, &out); err != nil {
		t.Fatalf("cmdStatus failed: %v", err)
	}
	if !strings.Contains(out.String(), "Cadence Daemon Status:") || !strings.Contains(out.String(), "push_to_talk") {
		t.Fatalf("unexpected status output: %s", out.String())
	}

	// 2. Dictate start / stop
	out.Reset()
	if err := cmdDictate(ctx, client, []string{"start"}, &out); err != nil {
		t.Fatalf("cmdDictate start failed: %v", err)
	}
	if !strings.Contains(out.String(), "Dictation started.") {
		t.Fatalf("unexpected dictate start output: %s", out.String())
	}

	out.Reset()
	if err := cmdDictate(ctx, client, []string{"stop"}, &out); err != nil {
		t.Fatalf("cmdDictate stop failed: %v", err)
	}
	if !strings.Contains(out.String(), "Dictation stopped.") {
		t.Fatalf("unexpected dictate stop output: %s", out.String())
	}

	// 3. History
	out.Reset()
	if err := cmdHistory(ctx, client, []string{"5"}, &out); err != nil {
		t.Fatalf("cmdHistory failed: %v", err)
	}
	if !strings.Contains(out.String(), "Recent Transcripts") || !strings.Contains(out.String(), "Hello world.") {
		t.Fatalf("unexpected history output: %s", out.String())
	}

	// 4. Doctor
	out.Reset()
	cfg := config.Default()
	cfg.IPC.SocketPath = sockPath
	if err := cmdDoctor(ctx, cfg, client, &out); err != nil {
		t.Fatalf("cmdDoctor failed: %v", err)
	}
	if !strings.Contains(out.String(), "running and healthy") {
		t.Fatalf("expected doctor to find healthy daemon, got: %s", out.String())
	}

	// 5. Recover (last)
	out.Reset()
	if err := cmdRecover(ctx, cfg, client, []string{}, &out); err != nil {
		t.Fatalf("cmdRecover failed: %v", err)
	}
	if !strings.Contains(out.String(), "Recovered Session:") || !strings.Contains(out.String(), "Mock recovered text.") {
		t.Fatalf("unexpected recover output: %s", out.String())
	}

	// 6. Recover list
	out.Reset()
	if err := cmdRecover(ctx, cfg, client, []string{"--list"}, &out); err != nil {
		t.Fatalf("cmdRecover --list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Recent Recoverable Sessions") || !strings.Contains(out.String(), "rec-mock-1") {
		t.Fatalf("unexpected recover list output: %s", out.String())
	}
}

func TestCLI_Recover_OfflineSQLiteFallback(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fallback.db")

	s, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC()
	if err := s.SaveRecovery(storage.SessionRecovery{
		ID:          "rec-offline-1",
		State:       "failed",
		PartialText: "offline recovery draft",
		Error:       "power loss",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("save recovery: %v", err)
	}
	_ = s.Close()

	cfg := config.Default()
	cfg.Storage.DBPath = dbPath
	cfg.IPC.SocketPath = filepath.Join(dir, "nonexistent.sock")
	client := ipc.Client{
		Network: "unix",
		Address: cfg.IPC.SocketPath,
	}

	var out bytes.Buffer
	if err := cmdRecover(context.Background(), cfg, client, []string{}, &out); err != nil {
		t.Fatalf("cmdRecover with offline fallback failed: %v", err)
	}
	if !strings.Contains(out.String(), "rec-offline-1") || !strings.Contains(out.String(), "offline recovery draft") {
		t.Fatalf("expected offline recovery text in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "power loss") {
		t.Fatalf("expected error message in output, got: %s", out.String())
	}
}

func TestCLI_Vocab(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_vocab_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	cfg := config.Default()
	cfg.IPC.SocketPath = sockPath

	var out bytes.Buffer

	// 1. Vocab list
	if err := cmdVocab(ctx, cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdVocab list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Custom Vocabulary") || !strings.Contains(out.String(), "grpc") {
		t.Fatalf("unexpected vocab list output: %s", out.String())
	}

	// 2. Vocab add
	out.Reset()
	if err := cmdVocab(ctx, cfg, client, []string{"add", "k8s", "Kubernetes"}, &out); err != nil {
		t.Fatalf("cmdVocab add failed: %v", err)
	}
	if !strings.Contains(out.String(), "Added vocabulary:") {
		t.Fatalf("unexpected vocab add output: %s", out.String())
	}

	// 3. Vocab remove
	out.Reset()
	if err := cmdVocab(ctx, cfg, client, []string{"remove", "grpc"}, &out); err != nil {
		t.Fatalf("cmdVocab remove failed: %v", err)
	}
	if !strings.Contains(out.String(), "Removed vocabulary:") {
		t.Fatalf("unexpected vocab remove output: %s", out.String())
	}

	// 4. Vocab import
	tmpFile := filepath.Join(t.TempDir(), "vocab.csv")
	if err := os.WriteFile(tmpFile, []byte("spoken,canonical\na,b\nc,d\n"), 0644); err != nil {
		t.Fatalf("write tmp vocab: %v", err)
	}
	out.Reset()
	if err := cmdVocab(ctx, cfg, client, []string{"import", tmpFile}, &out); err != nil {
		t.Fatalf("cmdVocab import failed: %v", err)
	}
	if !strings.Contains(out.String(), "Successfully imported 2 vocabulary entries") {
		t.Fatalf("unexpected vocab import output: %s", out.String())
	}

	// 5. Vocab export
	exportFile := filepath.Join(t.TempDir(), "export.txt")
	out.Reset()
	if err := cmdVocab(ctx, cfg, client, []string{"export", exportFile, "--format", "txt"}, &out); err != nil {
		t.Fatalf("cmdVocab export failed: %v", err)
	}
	if !strings.Contains(out.String(), "Exported vocabulary to") {
		t.Fatalf("unexpected vocab export output: %s", out.String())
	}
}

func TestCLI_Vocab_OfflineFallback(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fallback_vocab.db")

	s, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := s.SetVocabulary(storage.VocabularyEntry{
		Spoken:    "offline term",
		Canonical: "Offline Canonical",
	}); err != nil {
		t.Fatalf("set vocab: %v", err)
	}
	_ = s.Close()

	cfg := config.Default()
	cfg.Storage.DBPath = dbPath
	cfg.IPC.SocketPath = filepath.Join(dir, "nonexistent.sock")
	client := ipc.Client{
		Network: "unix",
		Address: cfg.IPC.SocketPath,
	}

	var out bytes.Buffer
	if err := cmdVocab(context.Background(), cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdVocab list offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "offline term") || !strings.Contains(out.String(), "Offline Canonical") {
		t.Fatalf("expected offline vocabulary in output, got: %s", out.String())
	}

	// Test offline add
	out.Reset()
	if err := cmdVocab(context.Background(), cfg, client, []string{"add", "new term", "New Canonical"}, &out); err != nil {
		t.Fatalf("cmdVocab add offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "Added vocabulary:") {
		t.Fatalf("unexpected vocab add output: %s", out.String())
	}

	// Reopen database and verify both entries exist
	s2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	defer s2.Close()
	all, err := s2.Vocabulary()
	if err != nil || len(all) != 2 {
		t.Fatalf("expected 2 vocabulary entries in database, got: %+v, err: %v", all, err)
	}
}

func TestCLI_Snippet(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_snip_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	cfg := config.Default()
	cfg.IPC.SocketPath = sockPath

	var out bytes.Buffer

	// 1. Snippet list
	if err := cmdSnippet(ctx, cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdSnippet list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Custom Snippets") || !strings.Contains(out.String(), "signoff") {
		t.Fatalf("unexpected snippet list output: %s", out.String())
	}

	// 2. Snippet add
	out.Reset()
	if err := cmdSnippet(ctx, cfg, client, []string{"add", "standup", "Standup {date}"}, &out); err != nil {
		t.Fatalf("cmdSnippet add failed: %v", err)
	}
	if !strings.Contains(out.String(), "Saved snippet:") {
		t.Fatalf("unexpected snippet add output: %s", out.String())
	}

	// 3. Snippet test
	out.Reset()
	if err := cmdSnippet(ctx, cfg, client, []string{"test", "signoff"}, &out); err != nil {
		t.Fatalf("cmdSnippet test failed: %v", err)
	}
	if !strings.Contains(out.String(), "Snippet Preview:") || !strings.Contains(out.String(), "Expanded:") {
		t.Fatalf("unexpected snippet test output: %s", out.String())
	}

	// 4. Snippet remove
	out.Reset()
	if err := cmdSnippet(ctx, cfg, client, []string{"remove", "signoff"}, &out); err != nil {
		t.Fatalf("cmdSnippet remove failed: %v", err)
	}
	if !strings.Contains(out.String(), "Removed snippet:") {
		t.Fatalf("unexpected snippet remove output: %s", out.String())
	}
}

func TestCLI_Snippet_OfflineFallback(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fallback_snip.db")

	s, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := s.SetSnippet(storage.Snippet{
		Trigger:     "offline snip",
		Template:    "Offline template {date}",
		Description: "Offline desc",
	}); err != nil {
		t.Fatalf("set snippet: %v", err)
	}
	_ = s.Close()

	cfg := config.Default()
	cfg.Storage.DBPath = dbPath
	cfg.IPC.SocketPath = filepath.Join(dir, "nonexistent.sock")
	client := ipc.Client{
		Network: "unix",
		Address: cfg.IPC.SocketPath,
	}

	var out bytes.Buffer
	if err := cmdSnippet(context.Background(), cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdSnippet list offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "offline snip") {
		t.Fatalf("expected offline snippet in list, got: %s", out.String())
	}

	// Test offline test
	out.Reset()
	if err := cmdSnippet(context.Background(), cfg, client, []string{"test", "offline snip"}, &out); err != nil {
		t.Fatalf("cmdSnippet test offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "Offline template") {
		t.Fatalf("expected expanded offline snippet in preview, got: %s", out.String())
	}
}

func TestCLI_Polish(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_polish_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}

	var out bytes.Buffer
	if err := cmdPolish(context.Background(), client, []string{"hello", "world", "--profile", "general"}, &out); err != nil {
		t.Fatalf("cmdPolish failed: %v", err)
	}

	if !strings.Contains(out.String(), "Polished: hello world") {
		t.Fatalf("expected polished output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "model: llama3.2") {
		t.Fatalf("expected model info, got: %s", out.String())
	}

	// Test empty text
	if err := cmdPolish(context.Background(), client, []string{}, &out); err == nil {
		t.Fatalf("expected error for empty args")
	}
}

func TestCLI_Profile(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_profile_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	cfg := config.Default()

	// 1. List
	var out bytes.Buffer
	if err := cmdProfile(context.Background(), cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdProfile list failed: %v", err)
	}
	if !strings.Contains(out.String(), "general") || !strings.Contains(out.String(), "code") {
		t.Fatalf("expected general and code in list, got: %s", out.String())
	}

	// 2. Show
	out.Reset()
	if err := cmdProfile(context.Background(), cfg, client, []string{"show", "code"}, &out); err != nil {
		t.Fatalf("cmdProfile show failed: %v", err)
	}
	if !strings.Contains(out.String(), "Profile: code") || !strings.Contains(out.String(), "Format code.") {
		t.Fatalf("unexpected profile show output: %s", out.String())
	}

	// 3. Set
	out.Reset()
	if err := cmdProfile(context.Background(), cfg, client, []string{"set", "custom", "--desc", "Custom desc", "--prompt", "Custom prompt"}, &out); err != nil {
		t.Fatalf("cmdProfile set failed: %v", err)
	}
	if !strings.Contains(out.String(), "configured successfully") {
		t.Fatalf("unexpected set output: %s", out.String())
	}

	// 4. Delete
	out.Reset()
	if err := cmdProfile(context.Background(), cfg, client, []string{"delete", "custom"}, &out); err != nil {
		t.Fatalf("cmdProfile delete failed: %v", err)
	}
	if !strings.Contains(out.String(), "removed successfully") {
		t.Fatalf("unexpected delete output: %s", out.String())
	}
}

func TestCLI_App(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_app_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}

	var out bytes.Buffer
	if err := cmdApp(context.Background(), client, &out); err != nil {
		t.Fatalf("cmdApp failed: %v", err)
	}

	if !strings.Contains(out.String(), "Visual Studio Code") {
		t.Fatalf("expected Visual Studio Code, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Category:     code") {
		t.Fatalf("expected Category: code, got: %s", out.String())
	}
}

func TestCLI_Route(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_route_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	cfg := config.Default()

	// 1. List
	var out bytes.Buffer
	if err := cmdRoute(context.Background(), cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdRoute list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Application Polish Profile Routes:") || !strings.Contains(out.String(), "slack") {
		t.Fatalf("unexpected route list: %s", out.String())
	}

	// 2. Set
	out.Reset()
	if err := cmdRoute(context.Background(), cfg, client, []string{"set", "obsidian", "academic"}, &out); err != nil {
		t.Fatalf("cmdRoute set failed: %v", err)
	}
	if !strings.Contains(out.String(), "Route configured: \"obsidian\" -> \"academic\"") {
		t.Fatalf("unexpected route set: %s", out.String())
	}

	// 3. Test
	out.Reset()
	if err := cmdRoute(context.Background(), cfg, client, []string{"test", "Visual Studio Code"}, &out); err != nil {
		t.Fatalf("cmdRoute test failed: %v", err)
	}
	if !strings.Contains(out.String(), "Category: code") || !strings.Contains(out.String(), "Profile:  code") {
		t.Fatalf("unexpected route test: %s", out.String())
	}

	// 4. Delete
	out.Reset()
	if err := cmdRoute(context.Background(), cfg, client, []string{"delete", "slack"}, &out); err != nil {
		t.Fatalf("cmdRoute delete failed: %v", err)
	}
	if !strings.Contains(out.String(), "Route for \"slack\" removed.") {
		t.Fatalf("unexpected route delete: %s", out.String())
	}
}

func TestCLI_Undo(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_undo_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}

	// 1. cadence undo (defaults to unpolished)
	var out bytes.Buffer
	if err := cmdUndo(context.Background(), client, []string{}, &out); err != nil {
		t.Fatalf("cmdUndo failed: %v", err)
	}
	if !strings.Contains(out.String(), "Reverted AI polish transformation") || !strings.Contains(out.String(), "unpolished post-processed text") {
		t.Fatalf("unexpected undo output: %s", out.String())
	}

	// 2. cadence undo --raw
	out.Reset()
	if err := cmdUndo(context.Background(), client, []string{"--raw"}, &out); err != nil {
		t.Fatalf("cmdUndo --raw failed: %v", err)
	}
	if !strings.Contains(out.String(), "Reverted to raw ASR transcript") || !strings.Contains(out.String(), "raw asr text") {
		t.Fatalf("unexpected undo --raw output: %s", out.String())
	}

	// 3. cadence undo --help
	out.Reset()
	if err := cmdUndo(context.Background(), client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdUndo --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence undo") {
		t.Fatalf("unexpected undo help output: %s", out.String())
	}

	// 4. cadence undo --list
	out.Reset()
	if err := cmdUndo(context.Background(), client, []string{"--list"}, &out); err != nil {
		t.Fatalf("cmdUndo --list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Cadence Reversible Transformation Stack:") || !strings.Contains(out.String(), "sess-99") {
		t.Fatalf("unexpected undo --list output: %s", out.String())
	}

	// 5. cadence undo --depth 2
	out.Reset()
	if err := cmdUndo(context.Background(), client, []string{"--depth", "2"}, &out); err != nil {
		t.Fatalf("cmdUndo --depth failed: %v", err)
	}
	if !strings.Contains(out.String(), "Remaining reversible transformations in stack:") {
		t.Fatalf("unexpected undo depth output: %s", out.String())
	}
}

func TestCLI_Correct(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_correct_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	cfg := config.Default()

	// 1. cadence correct with app
	var out bytes.Buffer
	if err := cmdCorrect(context.Background(), cfg, client, []string{"four loop in pie test", "--app", "VS Code"}, &out); err != nil {
		t.Fatalf("cmdCorrect failed: %v", err)
	}
	if !strings.Contains(out.String(), "ASR Self-Correction Resolution") ||
		!strings.Contains(out.String(), "for loop in pytest") ||
		!strings.Contains(out.String(), "2 correction(s) applied") {
		t.Fatalf("unexpected correct output: %s", out.String())
	}

	// 2. Offline fallback
	offlineClient := ipc.Client{Network: "unix", Address: "/tmp/nonexistent_sock_offline.sock"}
	out.Reset()
	if err := cmdCorrect(context.Background(), cfg, offlineClient, []string{"four loop in pie test", "--app", "VS Code"}, &out); err != nil {
		t.Fatalf("cmdCorrect offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "for loop in pytest") {
		t.Fatalf("unexpected offline correct output: %s", out.String())
	}

	// 3. Help
	out.Reset()
	if err := cmdCorrect(context.Background(), cfg, client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdCorrect --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence correct") {
		t.Fatalf("unexpected help output: %s", out.String())
	}

	// 4. Empty text error
	if err := cmdCorrect(context.Background(), cfg, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty text")
	}
}

func TestCLI_Selection(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_sel_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}

	// 1. Standard selection display
	var out bytes.Buffer
	if err := cmdSelection(context.Background(), client, []string{}, &out); err != nil {
		t.Fatalf("cmdSelection failed: %v", err)
	}
	if !strings.Contains(out.String(), "Captured Selection from Visual Studio Code") ||
		!strings.Contains(out.String(), "func calculateSum") {
		t.Fatalf("unexpected selection output: %s", out.String())
	}

	// 2. JSON output
	out.Reset()
	if err := cmdSelection(context.Background(), client, []string{"--json"}, &out); err != nil {
		t.Fatalf("cmdSelection --json failed: %v", err)
	}
	if !strings.Contains(out.String(), `"text":"func calculateSum(a, b int) int"`) {
		t.Fatalf("unexpected JSON output: %s", out.String())
	}

	// 3. Copy to clipboard
	out.Reset()
	if err := cmdSelection(context.Background(), client, []string{"--copy"}, &out); err != nil {
		t.Fatalf("cmdSelection --copy failed: %v", err)
	}
	if !strings.Contains(out.String(), "Copied selection to clipboard.") {
		t.Fatalf("unexpected copy output: %s", out.String())
	}

	// 4. Help flag
	out.Reset()
	if err := cmdSelection(context.Background(), client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdSelection --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence selection") {
		t.Fatalf("unexpected help output: %s", out.String())
	}
}

func TestCLI_Parse(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_parse_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	cfg := config.Default()

	// 1. Parse translate command via daemon IPC
	var out bytes.Buffer
	if err := cmdParse(context.Background(), cfg, client, []string{"translate", "this", "to", "French"}, &out); err != nil {
		t.Fatalf("cmdParse failed: %v", err)
	}
	if !strings.Contains(out.String(), "Intent:      translate") ||
		!strings.Contains(out.String(), "Target Lang: French") ||
		!strings.Contains(out.String(), "Is Edit:     true") {
		t.Fatalf("unexpected parse output: %s", out.String())
	}

	// 2. Parse shorten command as JSON
	out.Reset()
	if err := cmdParse(context.Background(), cfg, client, []string{"make", "this", "shorter", "--json"}, &out); err != nil {
		t.Fatalf("cmdParse --json failed: %v", err)
	}
	if !strings.Contains(out.String(), `"intent": "shorten"`) ||
		!strings.Contains(out.String(), `"tone": "concise"`) {
		t.Fatalf("unexpected JSON output: %s", out.String())
	}

	// 3. Offline fallback (disconnected client)
	out.Reset()
	badClient := ipc.Client{Network: "unix", Address: "/tmp/nonexistent-parse.sock"}
	if err := cmdParse(context.Background(), cfg, badClient, []string{"summarize", "this"}, &out); err != nil {
		t.Fatalf("cmdParse offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "Intent:      summarize") {
		t.Fatalf("unexpected offline output: %s", out.String())
	}

	// 4. Empty text error
	if err := cmdParse(context.Background(), cfg, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty text")
	}

	// 5. Help flag
	out.Reset()
	if err := cmdParse(context.Background(), cfg, client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdParse --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence parse") {
		t.Fatalf("unexpected help output: %s", out.String())
	}
}

func TestCLI_Edit(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_edit_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	cfg := config.Default()

	// 1. Edit via mock daemon IPC
	var out bytes.Buffer
	if err := cmdEdit(context.Background(), cfg, client, []string{"rewrite", "this", "to", "sound", "professional"}, &out); err != nil {
		t.Fatalf("cmdEdit failed: %v", err)
	}
	if !strings.Contains(out.String(), "Voice Text Edit Result:") ||
		!strings.Contains(out.String(), "Intent:      rewrite") ||
		!strings.Contains(out.String(), "// rewritten function") ||
		!strings.Contains(out.String(), "Applied & pasted") {
		t.Fatalf("unexpected edit output: %s", out.String())
	}

	// 2. Edit with explicit text and dry-run
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"make", "this", "shorter", "-t", "A very long sentence here", "--dry-run"}, &out); err != nil {
		t.Fatalf("cmdEdit dry run failed: %v", err)
	}
	if !strings.Contains(out.String(), "Original:    A very long sentence here") ||
		!strings.Contains(out.String(), "Status:      Dry run") {
		t.Fatalf("unexpected dry run output: %s", out.String())
	}

	// 3. Edit with JSON output
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"translate", "to", "French", "-t", "Hello", "--json"}, &out); err != nil {
		t.Fatalf("cmdEdit json failed: %v", err)
	}
	if !strings.Contains(out.String(), `"original_text": "Hello"`) ||
		!strings.Contains(out.String(), `"intent": "translate"`) {
		t.Fatalf("unexpected json output: %s", out.String())
	}

	// 4. Offline fallback with explicit text and dry run
	out.Reset()
	badClient := ipc.Client{Network: "unix", Address: "/tmp/nonexistent-edit.sock"}
	if err := cmdEdit(context.Background(), cfg, badClient, []string{"shorten", "this", "-t", "Some input text", "--dry-run"}, &out); err != nil {
		t.Fatalf("cmdEdit offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "Intent:      shorten") {
		t.Fatalf("unexpected offline output: %s", out.String())
	}

	// 5. Missing instruction error
	if err := cmdEdit(context.Background(), cfg, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty instruction")
	}

	// 6. Help flag
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdEdit --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence edit") {
		t.Fatalf("unexpected help output: %s", out.String())
	}

	// 7. Preview mode
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"--preview", "make", "this", "shorter"}, &out); err != nil {
		t.Fatalf("cmdEdit --preview failed: %v", err)
	}
	if !strings.Contains(out.String(), "Voice Text Edit Proposal") || !strings.Contains(out.String(), "cadence edit apply") {
		t.Fatalf("unexpected preview output: %s", out.String())
	}

	// 8. Pending list
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdEdit list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Pending Edit Proposals") || !strings.Contains(out.String(), "prev-mock-123") {
		t.Fatalf("unexpected pending list: %s", out.String())
	}

	// 9. Apply preview
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"apply", "prev-mock-123"}, &out); err != nil {
		t.Fatalf("cmdEdit apply failed: %v", err)
	}
	if !strings.Contains(out.String(), "Applied and pasted edit proposal [prev-mock-123]") {
		t.Fatalf("unexpected apply output: %s", out.String())
	}

	// 10. Reject preview
	out.Reset()
	if err := cmdEdit(context.Background(), cfg, client, []string{"reject", "prev-mock-123"}, &out); err != nil {
		t.Fatalf("cmdEdit reject failed: %v", err)
	}
	if !strings.Contains(out.String(), "Rejected edit proposal [prev-mock-123]") {
		t.Fatalf("unexpected reject output: %s", out.String())
	}
}
