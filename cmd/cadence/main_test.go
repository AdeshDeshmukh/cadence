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
