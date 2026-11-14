package plugins_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/plugins"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestSandbox_SanitizedEnv(t *testing.T) {
	env := plugins.BuildSanitizedEnv("my-plugin", "/tmp/plugins/my-plugin", plugins.Permissions{}, []string{"CUSTOM_VAR=abc"})
	hasPluginID := false
	hasCustom := false
	hasPath := false

	for _, e := range env {
		if e == "CADENCE_PLUGIN_ID=my-plugin" {
			hasPluginID = true
		}
		if e == "CUSTOM_VAR=abc" {
			hasCustom = true
		}
		if len(e) > 5 && e[:5] == "PATH=" {
			hasPath = true
		}
	}

	if !hasPluginID || !hasCustom || !hasPath {
		t.Fatalf("expected plugin env vars, got: %v", env)
	}
}

func TestSandbox_ResolveEntrypoint(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "runner.sh")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatalf("write runner.sh: %v", err)
	}

	cmd, args, err := plugins.ResolveEntrypoint(tempDir, "./runner.sh --flag")
	if err != nil {
		t.Fatalf("ResolveEntrypoint failed: %v", err)
	}
	if filepath.Base(cmd) != "runner.sh" && cmd != "bash" && cmd != "sh" {
		t.Fatalf("unexpected cmd: %s", cmd)
	}
	if len(args) == 0 {
		t.Fatalf("expected args to be parsed")
	}
}

func TestManager_LifecycleAndDispatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping shell script test on windows")
	}

	tempDir := t.TempDir()
	pluginsRoot := filepath.Join(tempDir, "installed_plugins")
	srcDir := filepath.Join(tempDir, "source_plugin")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("create srcDir: %v", err)
	}

	// Create shell script plugin that responds to protocol messages
	scriptPath := filepath.Join(srcDir, "plugin.sh")
	scriptContent := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *"\"type\":\"init\""*)
      echo '{"type":"init_ack","payload":{"ok":true}}'
      ;;
    *"\"type\":\"ping\""*)
      echo '{"type":"pong"}'
      ;;
    *"\"type\":\"transcript_hook\""*)
      echo '{"type":"transcript_hook_resp","payload":{"modified_text":"POLISHED_BY_PLUGIN"}}'
      ;;
    *"\"type\":\"tool_call\""*)
      echo '{"type":"tool_call_resp","payload":{"result":"tool_success"}}'
      ;;
    *"\"type\":\"command\""*)
      echo '{"type":"command_resp","payload":{"output":"ran_command_ok"}}'
      ;;
    *)
      echo '{"type":"error","error":"unknown"}'
      ;;
  esac
done
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("write plugin.sh: %v", err)
	}

	manifestYAML := `
id: test-lifecycle-plugin
name: Test Lifecycle Plugin
version: 1.0.0
description: Tests plugin installation, activation, and message dispatch.
entrypoint: ./plugin.sh
capabilities:
  - transcription_hook
  - agent_tool
  - command
tools:
  - name: test_echo
    description: Echo tool
commands:
  - name: sync-all
    description: Sync command
`
	if err := os.WriteFile(filepath.Join(srcDir, "plugin.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatalf("write plugin.yaml: %v", err)
	}

	store := storage.NewMemory()
	reg := plugins.NewRegistry()
	mgr := plugins.NewManager(store, reg, pluginsRoot)

	// 1. Install
	status, err := mgr.Install(srcDir)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if status.ID != "test-lifecycle-plugin" || status.State != plugins.StateInstalled {
		t.Fatalf("unexpected install status: %+v", status)
	}

	// 2. Status & List
	st, err := mgr.GetStatus("test-lifecycle-plugin")
	if err != nil || st.State != plugins.StateInstalled {
		t.Fatalf("GetStatus failed: %v, got: %+v", err, st)
	}

	list, err := mgr.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List failed: %v, got %d items", err, len(list))
	}

	// 3. Activate
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actStatus, err := mgr.Activate(ctx, "test-lifecycle-plugin")
	if err != nil {
		t.Fatalf("Activate failed: %v", err)
	}
	if actStatus.State != plugins.StateActive || actStatus.PID <= 0 {
		t.Fatalf("expected StateActive with PID > 0, got: %+v", actStatus)
	}

	// 4. Dispatch Transcript Hook
	hookRes, err := mgr.DispatchTranscriptHook(ctx, plugins.TranscriptEvent{
		SessionID: "sess-1",
		Text:      "original text",
	})
	if err != nil {
		t.Fatalf("DispatchTranscriptHook failed: %v", err)
	}
	if hookRes.ModifiedText != "POLISHED_BY_PLUGIN" {
		t.Fatalf("expected modified text POLISHED_BY_PLUGIN, got %q", hookRes.ModifiedText)
	}

	// 5. Dispatch Tool Call
	toolRes, err := mgr.DispatchToolCall(ctx, "test_echo", map[string]any{"arg": "val"})
	if err != nil {
		t.Fatalf("DispatchToolCall failed: %v", err)
	}
	if toolRes.Result != "tool_success" {
		t.Fatalf("expected tool_success, got: %v", toolRes.Result)
	}

	// 6. Dispatch Command
	cmdRes, err := mgr.DispatchCommand(ctx, "sync-all", []string{"--fast"})
	if err != nil {
		t.Fatalf("DispatchCommand failed: %v", err)
	}
	if cmdRes.Output != "ran_command_ok" {
		t.Fatalf("expected ran_command_ok, got: %v", cmdRes.Output)
	}

	// 7. Deactivate
	if err := mgr.Deactivate("test-lifecycle-plugin"); err != nil {
		t.Fatalf("Deactivate failed: %v", err)
	}

	deactStatus, err := mgr.GetStatus("test-lifecycle-plugin")
	if err != nil || deactStatus.State != plugins.StateInactive {
		t.Fatalf("expected StateInactive after deactivate, got: %+v", deactStatus)
	}

	// 8. Uninstall
	if err := mgr.Uninstall("test-lifecycle-plugin"); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if _, err := mgr.GetStatus("test-lifecycle-plugin"); err == nil {
		t.Fatalf("expected error getting uninstalled plugin")
	}
}
