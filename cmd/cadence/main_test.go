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
}
