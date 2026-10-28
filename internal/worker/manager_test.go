package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createTestWorkerScript(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test_worker.py")

	script := `#!/usr/bin/env python3
import sys, json

for line in sys.stdin:
    if not line.strip():
        continue
    req = json.loads(line)
    method = req.get("method")
    req_id = req.get("id")
    if method == "ping":
        print(json.dumps({"id": req_id, "ok": True, "result": "pong"}), flush=True)
    elif method == "echo":
        print(json.dumps({"id": req_id, "ok": True, "echo": req.get("payload")}), flush=True)
    elif method == "error":
        print(json.dumps({"id": req_id, "ok": False, "error": "simulated failure"}), flush=True)
    elif method == "crash":
        sys.exit(1)
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write test worker script: %v", err)
	}
	return scriptPath
}

func TestManager_LifecycleAndCall(t *testing.T) {
	scriptPath := createTestWorkerScript(t)

	cfg := DefaultConfig("python3", scriptPath)
	cfg.AutoRestart = false
	mgr := NewManager(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mgr.Stop()

	if err := mgr.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	resp, err := mgr.Call(ctx, "echo", map[string]any{"payload": "cadence"})
	if err != nil {
		t.Fatalf("Call echo failed: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK true, got false: %s", resp.Error)
	}
	if val := resp.GetString("echo"); val != "cadence" {
		t.Fatalf("expected echo 'cadence', got %q", val)
	}

	_, err = mgr.Call(ctx, "error", nil)
	if err == nil {
		t.Fatal("expected error on error method, got nil")
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	_, err = mgr.Call(ctx, "echo", nil)
	if err != ErrWorkerStopped {
		t.Fatalf("expected ErrWorkerStopped, got %v", err)
	}
}

func TestManager_ContextCancellation(t *testing.T) {
	scriptPath := createTestWorkerScript(t)

	cfg := DefaultConfig("python3", scriptPath)
	cfg.AutoRestart = false
	mgr := NewManager(cfg)

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mgr.Stop()

	callCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, err := mgr.Call(callCtx, "echo", map[string]any{"payload": "hello"})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestManager_AutoRestartOnCrash(t *testing.T) {
	scriptPath := createTestWorkerScript(t)

	cfg := DefaultConfig("python3", scriptPath)
	cfg.InitialRestartDelay = 50 * time.Millisecond
	cfg.AutoRestart = true
	mgr := NewManager(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mgr.Stop()

	// Cause worker to crash
	_, _ = mgr.Call(ctx, "crash", nil)

	// Wait for supervisor to restart process
	time.Sleep(200 * time.Millisecond)

	var restarted bool
	for i := 0; i < 10; i++ {
		pingCtx, pingCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		if err := mgr.Ping(pingCtx); err == nil {
			restarted = true
			pingCancel()
			break
		}
		pingCancel()
		time.Sleep(100 * time.Millisecond)
	}

	if !restarted {
		t.Fatal("worker failed to automatically restart after crash")
	}

	resp, err := mgr.Call(ctx, "echo", map[string]any{"payload": "recovered"})
	if err != nil {
		t.Fatalf("Call after restart failed: %v", err)
	}
	if resp.GetString("echo") != "recovered" {
		t.Fatalf("expected 'recovered', got %q", resp.GetString("echo"))
	}
}
