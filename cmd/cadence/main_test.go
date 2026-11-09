package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/agent"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/developer"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/internal/system"
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
	case "editor_context_set":
		return map[string]any{"status": "ok"}, nil
	case "editor_context_get":
		return map[string]any{
			"active":         true,
			"editor_name":    "vscode",
			"file_path":      "/path/to/main.go",
			"file_name":      "main.go",
			"language_id":    "go",
			"line":           42.0,
			"column":         10.0,
			"selection_text": "fmt.Println(\"test\")",
			"summary":        "Editor: vscode | File: main.go | Lang: go | Line: 42:10 | Selected: 18 chars",
		}, nil
	case "editor_context_clear":
		return map[string]any{"status": "cleared"}, nil
	case "terminal_context_set":
		return map[string]any{"status": "ok"}, nil
	case "terminal_context_get":
		return map[string]any{
			"active":          true,
			"terminal_app":    "iTerm2",
			"shell":           "zsh",
			"pid":             12345.0,
			"working_dir":     "/Users/test/Cadence",
			"git_branch":      "main",
			"last_command":    "go test ./...",
			"last_exit_code":  0.0,
			"recent_commands": []string{"git status", "go test ./..."},
			"summary":         "Terminal: iTerm2 | Shell: zsh | pwd: /Users/test/Cadence (main) | Last: go test ./... [0]",
		}, nil
	case "terminal_context_clear":
		return map[string]any{"status": "cleared"}, nil
	case "dev_command_generate":
		params, _ := req.Params.(map[string]any)
		instr, _ := params["instruction"].(string)
		gen := developer.NewGenerator(nil)
		wd, _ := params["working_dir"].(string)
		return gen.Generate(context.Background(), instr, wd)
	case "dev_command_risk":
		params, _ := req.Params.(map[string]any)
		cmd, _ := params["command"].(string)
		return developer.ClassifyCommand(cmd), nil
	case "dev_command_preview":
		params, _ := req.Params.(map[string]any)
		cmd, _ := params["command"].(string)
		wd, _ := params["working_dir"].(string)
		engine := developer.NewPreviewEngine()
		return engine.Preview(context.Background(), cmd, wd)
	case "dev_command_execute":
		params, _ := req.Params.(map[string]any)
		cmd, _ := params["command"].(string)
		wd, _ := params["working_dir"].(string)
		confirm, _ := params["confirm"].(bool)
		engine := developer.NewPreviewEngine()
		return engine.Execute(context.Background(), cmd, wd, confirm)
	case "git_commit_generate":
		params, _ := req.Params.(map[string]any)
		wd, _ := params["working_dir"].(string)
		hint, _ := params["spoken_hint"].(string)
		assistant := developer.NewGitAssistant(nil)
		return assistant.GenerateCommit(context.Background(), developer.CommitGenRequest{
			WorkingDir: wd,
			SpokenHint: hint,
		})
	case "git_pr_generate":
		params, _ := req.Params.(map[string]any)
		wd, _ := params["working_dir"].(string)
		base, _ := params["base_branch"].(string)
		notes, _ := params["spoken_notes"].(string)
		assistant := developer.NewGitAssistant(nil)
		return assistant.GeneratePR(context.Background(), developer.PRGenRequest{
			WorkingDir:  wd,
			BaseBranch:  base,
			SpokenNotes: notes,
		})
	case "dev_code_format":
		params, _ := req.Params.(map[string]any)
		text, _ := params["text"].(string)
		style, _ := params["style"].(string)
		lang, _ := params["language"].(string)
		isDoc, _ := params["is_docstring"].(bool)
		acr, _ := params["preserve_acronyms"].(bool)
		return developer.FormatCode(developer.CodeFormatRequest{
			Text:             text,
			Style:            developer.CasingStyle(style),
			Language:         lang,
			IsDocstring:      isDoc,
			PreserveAcronyms: acr,
		}), nil
	case "system_command_execute":
		params, _ := req.Params.(map[string]any)
		input, _ := params["input"].(string)
		ctrl := system.NewMockController()
		cmd, ok := system.ParseSystemCommand(input)
		if !ok {
			return map[string]any{"parsed": false, "message": "unrecognized system command"}, nil
		}
		msg, err := system.ExecuteSystemCommand(context.Background(), ctrl, cmd)
		if err != nil {
			return nil, err
		}
		return map[string]any{"parsed": true, "command": cmd, "message": msg}, nil
	case "system_app_action":
		params, _ := req.Params.(map[string]any)
		action, _ := params["action"].(string)
		app, _ := params["app"].(string)
		return map[string]any{"success": true, "action": action, "app": app, "message": fmt.Sprintf("App action executed: %s %s", action, app)}, nil
	case "system_window_action":
		params, _ := req.Params.(map[string]any)
		action, _ := params["action"].(string)
		return map[string]any{"success": true, "action": action, "message": fmt.Sprintf("Window action executed: %s", action)}, nil
	case "system_screenshot":
		params, _ := req.Params.(map[string]any)
		clip, _ := params["copy_to_clipboard"].(bool)
		msg := "Screenshot saved to /tmp/screenshot.png"
		if clip {
			msg = "Screenshot copied to clipboard"
		}
		return map[string]any{"success": true, "message": msg}, nil
	case "system_volume":
		params, _ := req.Params.(map[string]any)
		if m, ok := params["mute"].(bool); ok && m {
			return map[string]any{"success": true, "message": "Volume muted"}, nil
		}
		if u, ok := params["unmute"].(bool); ok && u {
			return map[string]any{"success": true, "message": "Volume unmuted"}, nil
		}
		if l, ok := params["level"].(float64); ok {
			return map[string]any{"success": true, "message": fmt.Sprintf("Volume set to %d%%", int(l))}, nil
		}
		return map[string]any{"success": true, "message": "Volume unchanged"}, nil
	case "mode_get":
		return map[string]any{"mode": "dictation", "supported_modes": system.ValidModes()}, nil
	case "mode_set":
		params, _ := req.Params.(map[string]any)
		m, _ := params["mode"].(string)
		return map[string]any{"ok": true, "mode": m}, nil
	case "self_control_execute":
		params, _ := req.Params.(map[string]any)
		input, _ := params["input"].(string)
		cmd, ok := system.ParseSelfCommand(input)
		if !ok {
			return map[string]any{"parsed": false, "message": "unrecognized self-control command"}, nil
		}
		msg := system.FormatSelfFeedback(cmd, system.ModeDictation, false)
		return map[string]any{"parsed": true, "command": cmd, "message": msg}, nil
	case "agent_run":
		params, _ := req.Params.(map[string]any)
		goal, _ := params["goal"].(string)
		planOnly, _ := params["plan_only"].(bool)
		maxSteps, _ := params["max_steps"].(float64)
		eng := agent.NewEngine(agent.WithSystemController(system.NewMockController()))
		return eng.Run(context.Background(), goal, agent.RunOptions{
			PlanOnly: planOnly,
			MaxSteps: int(maxSteps),
		})
	case "agent_history":
		return []map[string]any{
			{
				"id":     "traj-test-1",
				"goal":   "open chrome and take screenshot",
				"status": "success",
			},
		}, nil
	case "tool_list":
		reg := agent.NewRegistry(system.NewMockController(), nil)
		return map[string]any{
			"tools":      reg.List(),
			"categories": reg.Categories(),
			"schemas":    reg.ExportJSONSchema(),
		}, nil
	case "tool_call":
		params, _ := req.Params.(map[string]any)
		name, _ := params["name"].(string)
		pMap, _ := params["params"].(map[string]any)
		reg := agent.NewRegistry(system.NewMockController(), nil)
		res, err := reg.Call(context.Background(), name, pMap)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"name":   name,
			"result": res,
		}, nil
	case "permission_list":
		pe := agent.NewDefaultPermissionEngine()
		return map[string]any{
			"default_level": string(pe.DefaultLevel()),
			"rules":         pe.Rules(),
			"count":         len(pe.Rules()),
		}, nil
	case "permission_grant":
		params, _ := req.Params.(map[string]any)
		tool, _ := params["tool"].(string)
		action, _ := params["action"].(string)
		level, _ := params["level"].(string)
		return map[string]any{
			"ok": true,
			"rule": map[string]any{
				"tool":   tool,
				"action": action,
				"level":  level,
			},
		}, nil
	case "permission_revoke":
		return map[string]any{
			"ok":      true,
			"revoked": true,
		}, nil
	case "permission_check":
		params, _ := req.Params.(map[string]any)
		tool, _ := params["tool"].(string)
		action, _ := params["action"].(string)
		pMap, _ := params["params"].(map[string]any)
		pe := agent.NewDefaultPermissionEngine()
		dec := pe.Check(tool, action, pMap)
		return map[string]any{
			"granted":       dec.Granted,
			"level":         string(dec.Level),
			"reason":        dec.Reason,
			"requires_user": dec.RequiresUser,
			"matched_rule":  dec.MatchedRule,
		}, nil
	case "policy_get":
		cp := agent.DefaultConfirmationPolicy()
		return map[string]any{
			"mode":        string(cp.Mode()),
			"description": agent.ModeDescription(cp.Mode()),
			"supported_modes": []string{
				string(agent.ModeAlwaysAsk),
				string(agent.ModeAutoApproveSafe),
				string(agent.ModeAutoApproveRisky),
				string(agent.ModeAutoApproveAll),
				string(agent.ModeReadOnly),
			},
		}, nil
	case "policy_set":
		params, _ := req.Params.(map[string]any)
		mode, _ := params["mode"].(string)
		return map[string]any{
			"ok":          true,
			"mode":        mode,
			"description": agent.ModeDescription(agent.ConfirmationMode(mode)),
		}, nil
	case "policy_evaluate":
		params, _ := req.Params.(map[string]any)
		tool, _ := params["tool"].(string)
		action, _ := params["action"].(string)
		pMap, _ := params["params"].(map[string]any)
		cp := agent.DefaultConfirmationPolicy()
		dec := cp.Evaluate(tool, action, pMap)
		return map[string]any{
			"allowed":               dec.Allowed,
			"requires_confirmation": dec.RequiresConfirmation,
			"risk_level":            string(dec.RiskLevel),
			"reason":                dec.Reason,
			"prompt":                dec.Prompt,
		}, nil
	case "meeting_start":
		params, _ := req.Params.(map[string]any)
		title, _ := params["title"].(string)
		if title == "" {
			title = "Mock Meeting"
		}
		return map[string]any{
			"id":         "mtg-mock-1",
			"title":      title,
			"state":      "recording",
			"audio_path": "/tmp/mock-meeting.wav",
		}, nil
	case "meeting_stop":
		return map[string]any{
			"id":          "mtg-mock-1",
			"title":       "Mock Meeting",
			"state":       "stopped",
			"audio_path":  "/tmp/mock-meeting.wav",
			"duration_ms": float64(125000),
			"segments": []any{
				map[string]any{"id": "seg-1", "text": "Segment one"},
			},
		}, nil
	case "meeting_pause":
		return map[string]any{"ok": true, "status": map[string]any{"is_paused": true}}, nil
	case "meeting_resume":
		return map[string]any{"ok": true, "status": map[string]any{"is_paused": false}}, nil
	case "meeting_status":
		return map[string]any{
			"is_recording":  true,
			"is_paused":     false,
			"elapsed_ms":    float64(45000),
			"segment_count": float64(3),
			"active_session": map[string]any{
				"id":         "mtg-mock-1",
				"title":      "Mock Sync",
				"audio_path": "/tmp/mock-meeting.wav",
			},
		}, nil
	case "meeting_list":
		return []any{
			map[string]any{
				"id":          "mtg-mock-1",
				"title":       "Architecture Sync",
				"state":       "stopped",
				"duration_ms": float64(3600000),
				"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
			},
		}, nil
	case "meeting_get":
		return map[string]any{
			"meeting": map[string]any{
				"id":          "mtg-mock-1",
				"title":       "Architecture Sync",
				"state":       "stopped",
				"audio_path":  "/tmp/mock-meeting.wav",
				"duration_ms": float64(3600000),
				"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
			},
			"segments": []any{
				map[string]any{
					"start_ms": float64(0),
					"end_ms":   float64(15000),
					"text":     "Welcome team to the architecture review.",
				},
				map[string]any{
					"start_ms": float64(15000),
					"end_ms":   float64(30000),
					"text":     "Today we are discussing continuous recording mode.",
				},
			},
		}, nil
	case "meeting_delete":
		return map[string]any{"ok": true, "id": "mtg-mock-1"}, nil
	case "meeting_diarize":
		params, _ := req.Params.(map[string]any)
		id, _ := params["id"].(string)
		if id == "" {
			return nil, errors.New("missing meeting id")
		}
		return map[string]any{
			"ok":            true,
			"meeting_id":    id,
			"speaker_count": float64(2),
			"speakers":      []any{"SPEAKER_00", "SPEAKER_01"},
			"segments": []any{
				map[string]any{
					"start_ms": float64(0),
					"end_ms":   float64(15000),
					"speaker":  "SPEAKER_00",
					"text":     "Welcome team to the architecture review.",
				},
				map[string]any{
					"start_ms": float64(15000),
					"end_ms":   float64(30000),
					"speaker":  "SPEAKER_01",
					"text":     "Today we are discussing continuous recording mode.",
				},
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

func TestCLI_Context(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_ctx_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()

	var out bytes.Buffer

	// 1. Context show (text)
	if err := cmdContext(ctx, client, []string{"show"}, &out); err != nil {
		t.Fatalf("cmdContext show failed: %v", err)
	}
	if !strings.Contains(out.String(), "Active Application Context:") ||
		!strings.Contains(out.String(), "Code Editor Context:") ||
		!strings.Contains(out.String(), "Editor:       vscode") ||
		!strings.Contains(out.String(), "File:         /path/to/main.go") ||
		!strings.Contains(out.String(), "Terminal & Shell Context:") ||
		!strings.Contains(out.String(), "Terminal:     iTerm2") {
		t.Fatalf("unexpected context show output:\n%s", out.String())
	}

	// 2. Context show (--json)
	out.Reset()
	if err := cmdContext(ctx, client, []string{"show", "--json"}, &out); err != nil {
		t.Fatalf("cmdContext show --json failed: %v", err)
	}
	var jsonMap map[string]any
	if err := json.Unmarshal(out.Bytes(), &jsonMap); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if jsonMap["editor"] == nil || jsonMap["terminal"] == nil {
		t.Fatalf("expected editor and terminal keys in json output: %+v", jsonMap)
	}

	// 3. Context set
	out.Reset()
	setArgs := []string{"set", "--file", "internal/server.go", "--lang", "go", "--line", "55", "--col", "12", "--selection", "go func() {}()"}
	if err := cmdContext(ctx, client, setArgs, &out); err != nil {
		t.Fatalf("cmdContext set failed: %v", err)
	}
	if !strings.Contains(out.String(), "Editor context updated.") {
		t.Fatalf("unexpected set output: %s", out.String())
	}

	// 3b. Context set terminal
	out.Reset()
	termArgs := []string{"set", "--shell", "zsh", "--pwd", "/workspace", "--cmd", "go test ./...", "--exit-code", "0"}
	if err := cmdContext(ctx, client, termArgs, &out); err != nil {
		t.Fatalf("cmdContext set terminal failed: %v", err)
	}
	if !strings.Contains(out.String(), "Terminal context updated.") {
		t.Fatalf("unexpected set terminal output: %s", out.String())
	}

	// 4. Context clear
	out.Reset()
	if err := cmdContext(ctx, client, []string{"clear"}, &out); err != nil {
		t.Fatalf("cmdContext clear failed: %v", err)
	}
	if !strings.Contains(out.String(), "Developer context cleared.") {
		t.Fatalf("unexpected clear output: %s", out.String())
	}
}

func TestCLI_Terminal(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_term_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()

	var out bytes.Buffer

	// 1. Terminal status
	if err := cmdTerminal(ctx, client, []string{"status"}, &out); err != nil {
		t.Fatalf("cmdTerminal status failed: %v", err)
	}
	if !strings.Contains(out.String(), "Terminal:     iTerm2") {
		t.Fatalf("unexpected terminal status: %s", out.String())
	}

	// 2. Terminal hooks
	for _, sh := range []string{"zsh", "bash", "fish"} {
		out.Reset()
		if err := cmdTerminal(ctx, client, []string{"hook", sh}, &out); err != nil {
			t.Fatalf("cmdTerminal hook %s failed: %v", sh, err)
		}
		if !strings.Contains(out.String(), "cadence context set") {
			t.Fatalf("expected hook script for %s, got:\n%s", sh, out.String())
		}
	}
}

func TestCLI_Cmd(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_cmd_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	cfg := config.Default()

	var out bytes.Buffer

	// 1. Run tests command (mock daemon returns go test)
	if err := cmdGenerate(ctx, cfg, client, []string{"run", "the", "tests"}, &out); err != nil {
		t.Fatalf("cmdGenerate failed: %v", err)
	}
	if !strings.Contains(out.String(), "go test") {
		t.Fatalf("expected 'go test', got %s", out.String())
	}

	// 2. Explain flag
	out.Reset()
	if err := cmdGenerate(ctx, cfg, client, []string{"--explain", "git", "commit", "with", "message", "fix", "auth"}, &out); err != nil {
		t.Fatalf("cmdGenerate --explain failed: %v", err)
	}
	if !strings.Contains(out.String(), "Generated Command:") || !strings.Contains(out.String(), `git commit -m "fix auth"`) {
		t.Fatalf("unexpected explain output: %s", out.String())
	}

	// 3. JSON flag
	out.Reset()
	if err := cmdGenerate(ctx, cfg, client, []string{"--json", "check", "status"}, &out); err != nil {
		t.Fatalf("cmdGenerate --json failed: %v", err)
	}
	var resMap map[string]any
	if err := json.Unmarshal(out.Bytes(), &resMap); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if resMap["command"] != "git status" {
		t.Fatalf("expected 'git status', got %v", resMap["command"])
	}

	// 4. Missing instruction error
	if err := cmdGenerate(ctx, cfg, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty instruction")
	}

	// 5. Help flag
	out.Reset()
	if err := cmdGenerate(ctx, cfg, client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdGenerate --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence cmd") {
		t.Fatalf("unexpected help output: %s", out.String())
	}

	// 6. Risk flag
	out.Reset()
	if err := cmdGenerate(ctx, cfg, client, []string{"--risk", "run", "the", "tests"}, &out); err != nil {
		t.Fatalf("cmdGenerate --risk failed: %v", err)
	}
	if !strings.Contains(out.String(), "[SAFE]") {
		t.Fatalf("expected [SAFE] badge, got %s", out.String())
	}

	// 7. Offline fallback (offline socket)
	offlineClient := ipc.Client{Network: "unix", Address: "/nonexistent.sock"}
	out.Reset()
	if err := cmdGenerate(ctx, cfg, offlineClient, []string{"show", "diff"}, &out); err != nil {
		t.Fatalf("offline fallback failed: %v", err)
	}
	if !strings.Contains(out.String(), "git diff") {
		t.Fatalf("expected offline fallback to generate 'git diff', got %s", out.String())
	}
}

func TestCLI_Risk(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_risk_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()

	var out bytes.Buffer

	// 1. Safe command
	if err := cmdRisk(ctx, client, []string{"git", "status"}, &out); err != nil {
		t.Fatalf("cmdRisk safe failed: %v", err)
	}
	if !strings.Contains(out.String(), "SAFE") || !strings.Contains(out.String(), "Confirm:      No") {
		t.Fatalf("unexpected safe output: %s", out.String())
	}

	// 2. Destructive command
	out.Reset()
	if err := cmdRisk(ctx, client, []string{"rm", "-rf", "/tmp/cadence"}, &out); err != nil {
		t.Fatalf("cmdRisk destructive failed: %v", err)
	}
	if !strings.Contains(out.String(), "DESTRUCTIVE") || !strings.Contains(out.String(), "Confirm:      Yes") {
		t.Fatalf("unexpected destructive output: %s", out.String())
	}

	// 3. JSON output
	out.Reset()
	if err := cmdRisk(ctx, client, []string{"--json", "git", "reset", "--hard"}, &out); err != nil {
		t.Fatalf("cmdRisk --json failed: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if res["level"] != "destructive" || res["requires_confirmation"] != true {
		t.Fatalf("expected destructive in json, got %+v", res)
	}

	// 4. Missing command error
	if err := cmdRisk(ctx, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty command")
	}

	// 5. Help flag
	out.Reset()
	if err := cmdRisk(ctx, client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdRisk --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence risk") {
		t.Fatalf("unexpected help output: %s", out.String())
	}

	// 6. Offline fallback
	offlineClient := ipc.Client{Network: "unix", Address: "/nonexistent.sock"}
	out.Reset()
	if err := cmdRisk(ctx, offlineClient, []string{"kill", "-9", "1234"}, &out); err != nil {
		t.Fatalf("cmdRisk offline failed: %v", err)
	}
	if !strings.Contains(out.String(), "DESTRUCTIVE") {
		t.Fatalf("expected offline destructive classification, got %s", out.String())
	}
}

func TestCLI_Cmd_DryRun(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_dry_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	cfg := config.Default()

	var out bytes.Buffer

	// Test cadence cmd --dry-run
	if err := cmdGenerate(ctx, cfg, client, []string{"--dry-run", "run", "the", "tests"}, &out); err != nil {
		t.Fatalf("cmdGenerate --dry-run failed: %v", err)
	}
	if !strings.Contains(out.String(), "Dry-Run Preview:") || !strings.Contains(out.String(), "go test") {
		t.Fatalf("unexpected dry-run output: %s", out.String())
	}
}

func TestCLI_Exec(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_exec_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	tmpDir := t.TempDir()

	var out, errOut bytes.Buffer

	// 1. Safe command executes
	if err := cmdExec(ctx, client, []string{"echo", "cadence exec safe"}, nil, &out, &errOut); err != nil {
		t.Fatalf("cmdExec safe failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "cadence exec safe" {
		t.Fatalf("unexpected output: %s", out.String())
	}

	// 2. Dry-run flag
	out.Reset()
	if err := cmdExec(ctx, client, []string{"--dry-run", "rm", "-rf", "/tmp/cadence"}, nil, &out, &errOut); err != nil {
		t.Fatalf("cmdExec dry-run failed: %v", err)
	}
	if !strings.Contains(out.String(), "Dry-Run Preview:") || !strings.Contains(out.String(), "DESTRUCTIVE") {
		t.Fatalf("unexpected dry-run output: %s", out.String())
	}

	// 3. Destructive command blocked without confirmation in non-interactive/json mode
	out.Reset()
	if err := cmdExec(ctx, client, []string{"--json", "rm", "-rf", "/tmp/cadence"}, nil, &out, &errOut); err == nil {
		t.Fatal("expected error rejecting destructive command without confirm")
	}

	// 4. Destructive command with --confirm flag succeeds
	dummyFile := filepath.Join(tmpDir, "to_delete.txt")
	_ = os.WriteFile(dummyFile, []byte("test"), 0644)
	out.Reset()
	if err := cmdExec(ctx, client, []string{"--confirm", "rm", "-f", dummyFile}, nil, &out, &errOut); err != nil {
		t.Fatalf("cmdExec confirmed failed: %v", err)
	}

	// 5. Interactive prompt with 'y' succeeds
	dummyInteractive := filepath.Join(tmpDir, "interactive.txt")
	_ = os.WriteFile(dummyInteractive, []byte("test"), 0644)
	inBuf := bytes.NewBufferString("y\n")
	out.Reset()
	if err := cmdExec(ctx, client, []string{"rm", "-f", dummyInteractive}, inBuf, &out, &errOut); err != nil {
		t.Fatalf("cmdExec interactive yes failed: %v", err)
	}

	// 6. Interactive prompt with 'n' aborts
	dummyAbort := filepath.Join(tmpDir, "abort.txt")
	_ = os.WriteFile(dummyAbort, []byte("test"), 0644)
	inAbort := bytes.NewBufferString("n\n")
	out.Reset()
	if err := cmdExec(ctx, client, []string{"rm", "-f", dummyAbort}, inAbort, &out, &errOut); err == nil {
		t.Fatal("expected error on interactive abort")
	}

	// 7. Missing command error
	out.Reset()
	if err := cmdExec(ctx, client, []string{}, nil, &out, &errOut); err == nil {
		t.Fatal("expected error on missing command")
	}

	// 8. Help flag
	out.Reset()
	if err := cmdExec(ctx, client, []string{"--help"}, nil, &out, &errOut); err != nil {
		t.Fatalf("cmdExec --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence exec") {
		t.Fatalf("unexpected help text: %s", out.String())
	}
}

func TestCLI_Git(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_git_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()

	// 1. Setup mock repo
	tmpDir := t.TempDir()
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		_ = cmd.Run()
	}
	runCmd("init")
	runCmd("checkout", "-B", "main")
	runCmd("config", "user.name", "Tester")
	runCmd("config", "user.email", "test@test.local")
	_ = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Initial\n"), 0644)
	runCmd("add", "README.md")
	runCmd("commit", "-m", "chore: init")

	var out, errOut bytes.Buffer

	// 2. Help output
	if err := cmdGit(ctx, client, []string{"--help"}, &out, &errOut); err != nil {
		t.Fatalf("cmdGit --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence git") {
		t.Fatalf("unexpected git help text: %s", out.String())
	}

	// 3. Commit with no staged changes should fail
	out.Reset()
	if err := cmdGit(ctx, client, []string{"commit", "--dir", tmpDir}, &out, &errOut); err == nil {
		t.Fatal("expected error on empty staged changes")
	}

	// 4. Commit with staged change and hint
	_ = os.WriteFile(filepath.Join(tmpDir, "service.go"), []byte("package service\n"), 0644)
	runCmd("add", "service.go")

	out.Reset()
	if err := cmdGit(ctx, client, []string{"commit", "--dir", tmpDir, "--hint", "implement service registry"}, &out, &errOut); err != nil {
		t.Fatalf("cmdGit commit failed: %v", err)
	}
	if !strings.Contains(out.String(), "implement service registry") {
		t.Fatalf("expected hint in commit output: %s", out.String())
	}

	// 5. Commit with --json
	out.Reset()
	if err := cmdGit(ctx, client, []string{"commit", "--dir", tmpDir, "--json", "quick fix"}, &out, &errOut); err != nil {
		t.Fatalf("cmdGit commit --json failed: %v", err)
	}
	var commitMap map[string]any
	if err := json.Unmarshal(out.Bytes(), &commitMap); err != nil {
		t.Fatalf("invalid json commit output: %v", err)
	}
	if commitMap["title"] == nil {
		t.Fatalf("expected title in json commit: %+v", commitMap)
	}

	// 6. PR command
	runCmd("commit", "-m", "feat(service): implement service registry")
	out.Reset()
	if err := cmdGit(ctx, client, []string{"pr", "--dir", tmpDir, "--base", "HEAD~1", "--notes", "Added service registry."}, &out, &errOut); err != nil {
		t.Fatalf("cmdGit pr failed: %v", err)
	}
	if !strings.Contains(out.String(), "Title:") || !strings.Contains(out.String(), "Overview") {
		t.Fatalf("unexpected PR output: %s", out.String())
	}

	// 7. PR with --json
	out.Reset()
	if err := cmdGit(ctx, client, []string{"pr", "--dir", tmpDir, "--base", "HEAD~1", "--json"}, &out, &errOut); err != nil {
		t.Fatalf("cmdGit pr --json failed: %v", err)
	}
	var prMap map[string]any
	if err := json.Unmarshal(out.Bytes(), &prMap); err != nil {
		t.Fatalf("invalid json PR output: %v", err)
	}
	if prMap["title"] == nil || prMap["body"] == nil {
		t.Fatalf("expected title and body in json PR: %+v", prMap)
	}
}

func TestCLI_Code(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_code_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()

	var out bytes.Buffer

	// 1. Basic casing conversion via flag
	if err := cmdCode(ctx, client, []string{"--case", "snake", "max", "retry", "limit"}, &out); err != nil {
		t.Fatalf("cmdCode snake failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "max_retry_limit" {
		t.Fatalf("expected max_retry_limit, got %s", out.String())
	}

	// 2. Voice trigger prefix auto-detection
	out.Reset()
	if err := cmdCode(ctx, client, []string{"camel", "case", "user", "profile", "account"}, &out); err != nil {
		t.Fatalf("cmdCode camel failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "userProfileAccount" {
		t.Fatalf("expected userProfileAccount, got %s", out.String())
	}

	// 3. Constant case
	out.Reset()
	if err := cmdCode(ctx, client, []string{"constant", "case", "default", "timeout", "seconds"}, &out); err != nil {
		t.Fatalf("cmdCode constant failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "DEFAULT_TIMEOUT_SECONDS" {
		t.Fatalf("expected DEFAULT_TIMEOUT_SECONDS, got %s", out.String())
	}

	// 4. Docstring formatting
	out.Reset()
	if err := cmdCode(ctx, client, []string{"--doc", "--lang", "go", "StartListener", "initializes", "the", "IPC", "socket."}, &out); err != nil {
		t.Fatalf("cmdCode docstring failed: %v", err)
	}
	if !strings.HasPrefix(out.String(), "//") {
		t.Fatalf("expected Go docstring comment prefix, got %s", out.String())
	}

	// 5. JSON output
	out.Reset()
	if err := cmdCode(ctx, client, []string{"--json", "--case", "kebab", "api", "route", "handler"}, &out); err != nil {
		t.Fatalf("cmdCode json failed: %v", err)
	}
	var resMap map[string]any
	if err := json.Unmarshal(out.Bytes(), &resMap); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resMap["result"] != "api-route-handler" {
		t.Fatalf("expected api-route-handler in json, got %v", resMap["result"])
	}

	// 6. Help flag
	out.Reset()
	if err := cmdCode(ctx, client, []string{"--help"}, &out); err != nil {
		t.Fatalf("cmdCode help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence code") {
		t.Fatalf("unexpected help text: %s", out.String())
	}

	// 7. Missing text error
	out.Reset()
	if err := cmdCode(ctx, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty text")
	}
}

func TestCLI_Sys(t *testing.T) {
	mockCtrl := system.NewMockController()
	systemControllerOverride = mockCtrl
	defer func() { systemControllerOverride = nil }()

	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_sys_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	var out bytes.Buffer

	// 1. Sys usage when no args
	out.Reset()
	if err := cmdSys(ctx, client, []string{}, &out); err != nil {
		t.Fatalf("cmdSys no args failed: %v", err)
	}
	if !strings.Contains(out.String(), "System command automation:") {
		t.Fatalf("expected usage text, got: %s", out.String())
	}

	// 2. Natural speech command: open app
	out.Reset()
	if err := cmdSys(ctx, client, []string{"open", "Visual", "Studio", "Code"}, &out); err != nil {
		t.Fatalf("cmdSys voice open failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "visual studio code") {
		t.Fatalf("expected visual studio code in output, got: %s", out.String())
	}

	// 3. Sys app explicit action
	out.Reset()
	if err := cmdSys(ctx, client, []string{"app", "launch", "Chrome"}, &out); err != nil {
		t.Fatalf("cmdSys app launch failed: %v", err)
	}
	if !strings.Contains(out.String(), "Chrome") {
		t.Fatalf("expected Chrome in output, got: %s", out.String())
	}

	out.Reset()
	if err := cmdSys(ctx, client, []string{"app", "quit", "Slack"}, &out); err != nil {
		t.Fatalf("cmdSys app quit failed: %v", err)
	}
	if !strings.Contains(out.String(), "Slack") {
		t.Fatalf("expected Slack in output, got: %s", out.String())
	}

	// 4. Sys window action
	out.Reset()
	if err := cmdSys(ctx, client, []string{"window", "minimize"}, &out); err != nil {
		t.Fatalf("cmdSys window minimize failed: %v", err)
	}
	if !strings.Contains(out.String(), "minimize") {
		t.Fatalf("expected minimize in output, got: %s", out.String())
	}

	// 5. Sys screenshot
	out.Reset()
	if err := cmdSys(ctx, client, []string{"screenshot", "--clipboard"}, &out); err != nil {
		t.Fatalf("cmdSys screenshot failed: %v", err)
	}
	if !strings.Contains(out.String(), "clipboard") {
		t.Fatalf("expected clipboard in output, got: %s", out.String())
	}

	// 6. Sys volume
	out.Reset()
	if err := cmdSys(ctx, client, []string{"volume", "65"}, &out); err != nil {
		t.Fatalf("cmdSys volume failed: %v", err)
	}
	if !strings.Contains(out.String(), "65%") {
		t.Fatalf("expected 65%% in output, got: %s", out.String())
	}

	out.Reset()
	if err := cmdSys(ctx, client, []string{"volume", "mute"}, &out); err != nil {
		t.Fatalf("cmdSys volume mute failed: %v", err)
	}
	if !strings.Contains(out.String(), "muted") {
		t.Fatalf("expected muted in output, got: %s", out.String())
	}

	// 7. Error handling
	if err := cmdSys(ctx, client, []string{"app"}, &out); err == nil {
		t.Fatal("expected error on empty app args")
	}
	if err := cmdSys(ctx, client, []string{"volume"}, &out); err == nil {
		t.Fatal("expected error on empty volume args")
	}
	if err := cmdSys(ctx, client, []string{"volume", "not-a-number"}, &out); err == nil {
		t.Fatal("expected error on invalid volume")
	}

	// 8. Integration through run() entrypoint
	var stdout, stderr bytes.Buffer
	code := run([]string{"sys", "take", "a", "screenshot"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run sys failed with code %d, stderr: %s", code, stderr.String())
	}
}

func TestCLI_ModeAndControl(t *testing.T) {
	mockCtrl := system.NewMockController()
	systemControllerOverride = mockCtrl
	defer func() { systemControllerOverride = nil }()

	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_mode_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	var out bytes.Buffer

	// 1. Get current mode
	out.Reset()
	if err := cmdMode(ctx, client, []string{}, &out); err != nil {
		t.Fatalf("cmdMode get failed: %v", err)
	}
	if !strings.Contains(out.String(), "dictation") {
		t.Fatalf("expected dictation in mode get output, got: %s", out.String())
	}

	// 2. Set mode to code
	out.Reset()
	if err := cmdMode(ctx, client, []string{"code"}, &out); err != nil {
		t.Fatalf("cmdMode set code failed: %v", err)
	}
	if !strings.Contains(out.String(), "code") {
		t.Fatalf("expected code in mode set output, got: %s", out.String())
	}

	// 3. Set invalid mode
	if err := cmdMode(ctx, client, []string{"nonexistent"}, &out); err == nil {
		t.Fatal("expected error on invalid mode")
	}

	// 4. Voice control: switch mode
	out.Reset()
	if err := cmdControl(ctx, client, []string{"cadence", "switch", "to", "agent", "mode"}, &out); err != nil {
		t.Fatalf("cmdControl switch to agent failed: %v", err)
	}
	if !strings.Contains(out.String(), "agent mode") {
		t.Fatalf("expected agent mode in control output, got: %s", out.String())
	}

	// 5. Voice control: query status
	out.Reset()
	if err := cmdControl(ctx, client, []string{"what", "mode", "are", "you", "in?"}, &out); err != nil {
		t.Fatalf("cmdControl query status failed: %v", err)
	}
	if !strings.Contains(out.String(), "mode") {
		t.Fatalf("expected mode in query status output, got: %s", out.String())
	}

	// 6. Voice control error handling
	if err := cmdControl(ctx, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty control args")
	}
	if err := cmdControl(ctx, client, []string{"unrecognized", "gibberish"}, &out); err == nil {
		t.Fatal("expected error on unrecognized voice control command")
	}

	// 7. Integration through run() entrypoint
	var stdout, stderr bytes.Buffer
	code := run([]string{"mode", "command"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run mode command failed with code %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	code = run([]string{"control", "what", "is", "your", "mode"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run control failed with code %d, stderr: %s", code, stderr.String())
	}
}

func TestCLI_Agent(t *testing.T) {
	mockCtrl := system.NewMockController()
	systemControllerOverride = mockCtrl
	defer func() { systemControllerOverride = nil }()

	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_agent_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	var out bytes.Buffer

	// 1. Agent usage when no args
	out.Reset()
	if err := cmdAgent(ctx, client, []string{}, &out); err != nil {
		t.Fatalf("cmdAgent no args failed: %v", err)
	}
	if !strings.Contains(out.String(), "Autonomous Agent mode:") {
		t.Fatalf("expected usage text, got: %s", out.String())
	}

	// 2. Agent history
	out.Reset()
	if err := cmdAgent(ctx, client, []string{"history"}, &out); err != nil {
		t.Fatalf("cmdAgent history failed: %v", err)
	}
	if !strings.Contains(out.String(), "open chrome and take screenshot") {
		t.Fatalf("expected history item, got: %s", out.String())
	}

	// 3. Plan-only agent run
	out.Reset()
	if err := cmdAgent(ctx, client, []string{"--plan-only", "open", "Chrome", "and", "then", "take", "a", "screenshot"}, &out); err != nil {
		t.Fatalf("cmdAgent plan-only failed: %v", err)
	}
	if !strings.Contains(out.String(), "Plan-only mode") {
		t.Fatalf("expected plan-only mode in output, got: %s", out.String())
	}

	// 4. Full agent run
	out.Reset()
	if err := cmdAgent(ctx, client, []string{"open", "Slack", "and", "then", "minimize", "window"}, &out); err != nil {
		t.Fatalf("cmdAgent run failed: %v", err)
	}
	if !strings.Contains(out.String(), "Goal: open Slack and then minimize window") {
		t.Fatalf("expected goal in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Status: success") {
		t.Fatalf("expected success status, got: %s", out.String())
	}

	// 5. JSON output
	out.Reset()
	if err := cmdAgent(ctx, client, []string{"--json", "open", "VS", "Code"}, &out); err != nil {
		t.Fatalf("cmdAgent json failed: %v", err)
	}
	var resMap map[string]any
	if err := json.Unmarshal(out.Bytes(), &resMap); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if resMap["status"] != "success" {
		t.Fatalf("expected status success in json, got: %v", resMap["status"])
	}

	// 6. Missing goal error
	if err := cmdAgent(ctx, client, []string{"--plan-only"}, &out); err == nil {
		t.Fatal("expected error on missing goal")
	}

	// 7. Integration through run() entrypoint
	var stdout, stderr bytes.Buffer
	code := run([]string{"agent", "--plan-only", "open", "Chrome"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run agent failed with code %d, stderr: %s", code, stderr.String())
	}
}

func TestCLI_Tools(t *testing.T) {
	mockCtrl := system.NewMockController()
	systemControllerOverride = mockCtrl
	defer func() { systemControllerOverride = nil }()

	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_tools_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{
		Network: "unix",
		Address: sockPath,
	}
	ctx := context.Background()
	var out bytes.Buffer

	// 1. tools list
	out.Reset()
	if err := cmdTools(ctx, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdTools list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Registered Agent Tools") {
		t.Fatalf("expected tools header, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "fs") || !strings.Contains(out.String(), "clipboard") {
		t.Fatalf("expected fs and clipboard tools in output, got: %s", out.String())
	}

	// 2. tools schema
	out.Reset()
	if err := cmdTools(ctx, client, []string{"schema"}, &out); err != nil {
		t.Fatalf("cmdTools schema failed: %v", err)
	}
	var schemas []map[string]any
	if err := json.Unmarshal(out.Bytes(), &schemas); err != nil {
		t.Fatalf("invalid schema json: %v", err)
	}
	if len(schemas) < 5 {
		t.Fatalf("expected at least 5 tool schemas, got %d", len(schemas))
	}

	// 3. tool call echo
	out.Reset()
	if err := cmdTool(ctx, client, []string{"echo", "message=registry-test-output"}, &out); err != nil {
		t.Fatalf("cmdTool echo failed: %v", err)
	}
	if !strings.Contains(out.String(), "registry-test-output") {
		t.Fatalf("expected echo output, got: %s", out.String())
	}

	// 4. tool call errors
	if err := cmdTool(ctx, client, []string{}, &out); err == nil {
		t.Fatal("expected error on empty tool args")
	}
	if err := cmdTool(ctx, client, []string{"unknown_tool"}, &out); err == nil {
		t.Fatal("expected error on unknown tool")
	}

	// 5. Integration through run() entrypoint
	var stdout, stderr bytes.Buffer
	code := run([]string{"tools"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run tools failed with code %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	code = run([]string{"tool", "echo", "message=run-integration"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run tool echo failed with code %d, stderr: %s", code, stderr.String())
	}
}

func TestCLI_Permissions(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_perm_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{Network: "unix", Address: sockPath}
	ctx := context.Background()
	var out bytes.Buffer

	// 1. permissions list
	if err := cmdPermissions(ctx, client, []string{"list"}, &out); err != nil {
		t.Fatalf("cmdPermissions list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Permission Policies") || !strings.Contains(out.String(), "fs") {
		t.Fatalf("unexpected permissions list output: %s", out.String())
	}

	// 2. permissions grant
	out.Reset()
	if err := cmdPermissions(ctx, client, []string{"grant", "fs", "write", "--deny"}, &out); err != nil {
		t.Fatalf("cmdPermissions grant failed: %v", err)
	}
	if !strings.Contains(out.String(), "tool=fs action=write level=deny") {
		t.Fatalf("unexpected grant output: %s", out.String())
	}

	// 3. permissions revoke
	out.Reset()
	if err := cmdPermissions(ctx, client, []string{"revoke", "fs", "write"}, &out); err != nil {
		t.Fatalf("cmdPermissions revoke failed: %v", err)
	}
	if !strings.Contains(out.String(), "Revoked permission rule") {
		t.Fatalf("unexpected revoke output: %s", out.String())
	}

	// 4. permissions check (allowed)
	out.Reset()
	if err := cmdPermissions(ctx, client, []string{"check", "fs", "read", "path=main.go"}, &out); err != nil {
		t.Fatalf("cmdPermissions check failed: %v", err)
	}
	if !strings.Contains(out.String(), "Permission Check: ALLOWED") {
		t.Fatalf("expected allowed check output, got: %s", out.String())
	}

	// 5. permissions check (denied on sensitive path)
	out.Reset()
	if err := cmdPermissions(ctx, client, []string{"check", "fs", "read", "path=/etc/shadow"}, &out); err != nil {
		t.Fatalf("cmdPermissions check failed: %v", err)
	}
	if !strings.Contains(out.String(), "Permission Check: DENIED") {
		t.Fatalf("expected denied check output for /etc/shadow, got: %s", out.String())
	}

	// 6. Error handling
	if err := cmdPermissions(ctx, client, []string{"grant"}, &out); err == nil {
		t.Fatal("expected error on empty grant args")
	}
	if err := cmdPermissions(ctx, client, []string{"revoke"}, &out); err == nil {
		t.Fatal("expected error on empty revoke args")
	}
	if err := cmdPermissions(ctx, client, []string{"check"}, &out); err == nil {
		t.Fatal("expected error on empty check args")
	}

	// 7. run() CLI entrypoint integration
	var stdout, stderr bytes.Buffer
	code := run([]string{"permissions", "list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run permissions list failed with code %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	code = run([]string{"perms", "check", "echo", "message=test"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run perms check failed with code %d, stderr: %s", code, stderr.String())
	}
}

func TestCLI_Policy(t *testing.T) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cad_pol_%d.sock", time.Now().UnixNano()))
	cleanup := startMockDaemon(t, sockPath)
	defer cleanup()

	client := ipc.Client{Network: "unix", Address: sockPath}
	ctx := context.Background()
	var out bytes.Buffer

	// 1. policy show
	if err := cmdPolicy(ctx, client, []string{"show"}, &out); err != nil {
		t.Fatalf("cmdPolicy show failed: %v", err)
	}
	if !strings.Contains(out.String(), "Action Confirmation Policy") || !strings.Contains(out.String(), "auto_approve_safe") {
		t.Fatalf("unexpected policy show output: %s", out.String())
	}

	// 2. policy set
	out.Reset()
	if err := cmdPolicy(ctx, client, []string{"set", "always_ask"}, &out); err != nil {
		t.Fatalf("cmdPolicy set failed: %v", err)
	}
	if !strings.Contains(out.String(), "Action confirmation policy updated to: always_ask") {
		t.Fatalf("unexpected policy set output: %s", out.String())
	}

	// 3. policy direct mode setting (e.g. cadence policy auto_approve_all)
	out.Reset()
	if err := cmdPolicy(ctx, client, []string{"auto_approve_all"}, &out); err != nil {
		t.Fatalf("cmdPolicy direct mode set failed: %v", err)
	}
	if !strings.Contains(out.String(), "Action confirmation policy updated to: auto_approve_all") {
		t.Fatalf("unexpected direct mode output: %s", out.String())
	}

	// 4. policy evaluate
	out.Reset()
	if err := cmdPolicy(ctx, client, []string{"evaluate", "fs", "write", "path=test.go"}, &out); err != nil {
		t.Fatalf("cmdPolicy evaluate failed: %v", err)
	}
	if !strings.Contains(out.String(), "Action Policy Decision:") || !strings.Contains(out.String(), "risky") {
		t.Fatalf("unexpected evaluate output: %s", out.String())
	}

	// 5. Error handling
	if err := cmdPolicy(ctx, client, []string{"set"}, &out); err == nil {
		t.Fatal("expected error on empty set args")
	}
	if err := cmdPolicy(ctx, client, []string{"set", "invalid_mode"}, &out); err == nil {
		t.Fatal("expected error on invalid set mode")
	}
	if err := cmdPolicy(ctx, client, []string{"evaluate"}, &out); err == nil {
		t.Fatal("expected error on empty evaluate args")
	}

	// 6. run() CLI integration
	var stdout, stderr bytes.Buffer
	code := run([]string{"policy"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run policy failed with code %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	code = run([]string{"policy", "eval", "echo", "run", "message=ping"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run policy eval failed with code %d, stderr: %s", code, stderr.String())
	}
}

func TestCLI_Meeting(t *testing.T) {
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("cad_mtg_%d.sock", time.Now().UnixNano()))
	stop := startMockDaemon(t, sock)
	defer stop()

	client := ipc.Client{
		Network: "unix",
		Address: sock,
	}
	ctx := context.Background()

	// 1. meeting status
	var out bytes.Buffer
	if err := cmdMeeting(ctx, client, []string{"status"}, &out); err != nil {
		t.Fatalf("cmdMeeting status failed: %v", err)
	}
	if !strings.Contains(out.String(), "Meeting Session:") || !strings.Contains(out.String(), "Mock Sync") {
		t.Fatalf("unexpected status output: %s", out.String())
	}

	// 2. meeting start
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"start", "Product", "Review"}, &out); err != nil {
		t.Fatalf("cmdMeeting start failed: %v", err)
	}
	if !strings.Contains(out.String(), "Continuous meeting recording started.") || !strings.Contains(out.String(), "Product Review") {
		t.Fatalf("unexpected start output: %s", out.String())
	}

	// 3. meeting pause
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"pause"}, &out); err != nil {
		t.Fatalf("cmdMeeting pause failed: %v", err)
	}
	if !strings.Contains(out.String(), "Meeting recording paused.") {
		t.Fatalf("unexpected pause output: %s", out.String())
	}

	// 4. meeting resume
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"resume"}, &out); err != nil {
		t.Fatalf("cmdMeeting resume failed: %v", err)
	}
	if !strings.Contains(out.String(), "Meeting recording resumed.") {
		t.Fatalf("unexpected resume output: %s", out.String())
	}

	// 5. meeting stop
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"stop"}, &out); err != nil {
		t.Fatalf("cmdMeeting stop failed: %v", err)
	}
	if !strings.Contains(out.String(), "Meeting recording stopped.") || !strings.Contains(out.String(), "02:05") {
		t.Fatalf("unexpected stop output: %s", out.String())
	}

	// 6. meeting list
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"list", "10"}, &out); err != nil {
		t.Fatalf("cmdMeeting list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Architecture Sync") || !strings.Contains(out.String(), "60:00") {
		t.Fatalf("unexpected list output: %s", out.String())
	}

	// 7. meeting show
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"show", "mtg-mock-1"}, &out); err != nil {
		t.Fatalf("cmdMeeting show failed: %v", err)
	}
	if !strings.Contains(out.String(), "Meeting:    Architecture Sync (mtg-mock-1)") || !strings.Contains(out.String(), "Welcome team to the architecture review.") {
		t.Fatalf("unexpected show output: %s", out.String())
	}

	// 8. meeting diarize
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"diarize", "mtg-mock-1", "--speakers", "2"}, &out); err != nil {
		t.Fatalf("cmdMeeting diarize failed: %v", err)
	}
	if !strings.Contains(out.String(), "Speaker diarization complete") || !strings.Contains(out.String(), "SPEAKER_00") {
		t.Fatalf("unexpected diarize output: %s", out.String())
	}

	// 9. meeting delete
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"delete", "mtg-mock-1"}, &out); err != nil {
		t.Fatalf("cmdMeeting delete failed: %v", err)
	}
	if !strings.Contains(out.String(), "Meeting mtg-mock-1 deleted.") {
		t.Fatalf("unexpected delete output: %s", out.String())
	}

	// 10. meeting help
	out.Reset()
	if err := cmdMeeting(ctx, client, []string{"help"}, &out); err != nil {
		t.Fatalf("cmdMeeting help failed: %v", err)
	}
	if !strings.Contains(out.String(), "Usage: cadence meeting <subcommand>") {
		t.Fatalf("unexpected help output: %s", out.String())
	}

	// 11. Error validation
	if err := cmdMeeting(ctx, client, []string{"show"}, &out); err == nil {
		t.Fatal("expected error showing meeting without id")
	}
	if err := cmdMeeting(ctx, client, []string{"diarize"}, &out); err == nil {
		t.Fatal("expected error diarizing meeting without id")
	}
	if err := cmdMeeting(ctx, client, []string{"delete"}, &out); err == nil {
		t.Fatal("expected error deleting meeting without id")
	}
	if err := cmdMeeting(ctx, client, []string{"unknown_action"}, &out); err == nil {
		t.Fatal("expected error for unknown meeting subcommand")
	}
}
