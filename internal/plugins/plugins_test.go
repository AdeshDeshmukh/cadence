package plugins_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/plugins"
)

func TestManifest_Validation(t *testing.T) {
	valid := &plugins.Manifest{
		ID:          "com.example.github-sync",
		Name:        "GitHub Sync",
		Version:     "1.0.0",
		Description: "Syncs notes to GitHub repos",
		Entrypoint:  "./bin/github-sync",
		Capabilities: []plugins.Capability{
			plugins.CapAgentTool,
			plugins.CapCommand,
		},
	}

	if err := plugins.ValidateManifest(valid); err != nil {
		t.Fatalf("expected valid manifest, got error: %v", err)
	}

	// Invalid ID
	badID := *valid
	badID.ID = "invalid id with spaces!"
	if err := plugins.ValidateManifest(&badID); err == nil {
		t.Fatalf("expected error for invalid ID")
	}

	// Bad SemVer
	badVer := *valid
	badVer.Version = "not-semver"
	if err := plugins.ValidateManifest(&badVer); err == nil {
		t.Fatalf("expected error for invalid semver")
	}

	// Empty Entrypoint
	badEntry := *valid
	badEntry.Entrypoint = ""
	if err := plugins.ValidateManifest(&badEntry); err == nil {
		t.Fatalf("expected error for empty entrypoint")
	}

	// Invalid Capability
	badCap := *valid
	badCap.Capabilities = []plugins.Capability{"unsupported_cap_xyz"}
	if err := plugins.ValidateManifest(&badCap); err == nil {
		t.Fatalf("expected error for invalid capability")
	}
}

func TestManifest_ParseJSONAndYAML(t *testing.T) {
	// JSON
	jsonData := `{
		"id": "jira-sync",
		"name": "Jira Sync",
		"version": "0.2.1",
		"description": "Jira ticket integration",
		"entrypoint": "./jira-plugin",
		"capabilities": ["agent_tool"],
		"tools": [
			{"name": "create_ticket", "description": "Creates a Jira ticket"}
		]
	}`

	mJson, err := plugins.ParseManifest([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseManifest JSON failed: %v", err)
	}
	if mJson.ID != "jira-sync" || len(mJson.Tools) != 1 || mJson.Tools[0].Name != "create_ticket" {
		t.Fatalf("unexpected parsed JSON manifest: %+v", mJson)
	}

	// YAML
	yamlData := `
id: slack-notify
name: Slack Notification Plugin
version: 1.1.0
description: Sends dictation summaries to Slack
author: Acme Corp
license: Apache-2.0
entrypoint: ./slack-bot
capabilities:
  - transcription_hook
  - command
permissions:
  read_transcripts: true
  network:
    - hooks.slack.com
tools:
  - name: send_slack_msg
    description: Posts message to channel
commands:
  - name: slack-share
    description: Shares note to slack
    voice_triggers:
      - share note on slack
`

	mYaml, err := plugins.ParseManifest([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseManifest YAML failed: %v", err)
	}
	if mYaml.ID != "slack-notify" || mYaml.Name != "Slack Notification Plugin" {
		t.Fatalf("mYaml metadata mismatch: %+v", mYaml)
	}
	if !mYaml.Permissions.ReadTranscripts || len(mYaml.Permissions.Network) != 1 {
		t.Fatalf("mYaml permissions mismatch: %+v", mYaml.Permissions)
	}
	if len(mYaml.Tools) != 1 || mYaml.Tools[0].Name != "send_slack_msg" {
		t.Fatalf("mYaml tools mismatch: %+v", mYaml.Tools)
	}
	if len(mYaml.Commands) != 1 || mYaml.Commands[0].Name != "slack-share" {
		t.Fatalf("mYaml commands mismatch: %+v", mYaml.Commands)
	}

	// Serialize YAML
	outYAML, err := mYaml.SerializeYAML()
	if err != nil {
		t.Fatalf("SerializeYAML failed: %v", err)
	}
	if !strings.Contains(string(outYAML), "id: slack-notify") || !strings.Contains(string(outYAML), "send_slack_msg") {
		t.Fatalf("serialized YAML missing content:\n%s", string(outYAML))
	}
}

func TestManifest_LoadFromDir(t *testing.T) {
	tempDir := t.TempDir()
	pluginDir := filepath.Join(tempDir, "sample-plugin")
	_ = os.MkdirAll(pluginDir, 0755)

	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	template := plugins.GenerateManifestTemplate("sample.test", "Sample Plugin", "Testing loader")
	yamlBytes, err := template.SerializeYAML()
	if err != nil {
		t.Fatalf("SerializeYAML: %v", err)
	}
	if err := os.WriteFile(manifestPath, yamlBytes, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := plugins.LoadManifest(pluginDir)
	if err != nil {
		t.Fatalf("LoadManifest from directory failed: %v", err)
	}
	if loaded.ID != "sample.test" || loaded.Name != "Sample Plugin" {
		t.Fatalf("unexpected loaded manifest: %+v", loaded)
	}
}

func TestRegistry_Operations(t *testing.T) {
	reg := plugins.NewRegistry()

	m1 := &plugins.Manifest{
		ID:          "p1",
		Name:        "Plugin One",
		Version:     "1.0.0",
		Description: "First plugin",
		Entrypoint:  "./p1",
		Tools: []plugins.ToolDefinition{
			{Name: "p1_search", Description: "Search with P1"},
		},
		Commands: []plugins.CommandDefinition{
			{Name: "p1_run", Aliases: []string{"run-p1"}, VoiceTriggers: []string{"launch p1"}},
		},
	}
	m2 := &plugins.Manifest{
		ID:          "p2",
		Name:        "Plugin Two",
		Version:     "2.0.0",
		Description: "Second plugin",
		Entrypoint:  "./p2",
	}

	if err := reg.Register(m1); err != nil {
		t.Fatalf("Register m1 failed: %v", err)
	}
	if err := reg.Register(m2); err != nil {
		t.Fatalf("Register m2 failed: %v", err)
	}

	// Get
	found, ok := reg.Get("p1")
	if !ok || found.Name != "Plugin One" {
		t.Fatalf("Get p1 failed: %+v", found)
	}

	// List
	list := reg.List()
	if len(list) != 2 || list[0].ID != "p1" || list[1].ID != "p2" {
		t.Fatalf("List mismatch: %+v", list)
	}

	// FindTool
	mTool, tool, ok := reg.FindTool("p1_search")
	if !ok || mTool.ID != "p1" || tool.Name != "p1_search" {
		t.Fatalf("FindTool failed: ok=%v, m=%+v, tool=%+v", ok, mTool, tool)
	}

	// FindCommand by voice trigger
	mCmd, cmd, ok := reg.FindCommand("launch p1")
	if !ok || mCmd.ID != "p1" || cmd.Name != "p1_run" {
		t.Fatalf("FindCommand failed: ok=%v, m=%+v, cmd=%+v", ok, mCmd, cmd)
	}

	// Unregister
	if err := reg.Unregister("p1"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}
	if _, ok := reg.Get("p1"); ok {
		t.Fatalf("p1 should be unregistered")
	}
}

type testPluginHandler struct {
	plugins.BaseHandler
	inited bool
}

func (h *testPluginHandler) OnInit(ctx context.Context, config map[string]any) error {
	h.inited = true
	return nil
}

func (h *testPluginHandler) OnTranscript(ctx context.Context, event plugins.TranscriptEvent) (*plugins.TranscriptResult, error) {
	return &plugins.TranscriptResult{
		ModifiedText: strings.ToUpper(event.Text),
		Tags:         []string{"plugin-processed"},
	}, nil
}

func (h *testPluginHandler) OnToolCall(ctx context.Context, event plugins.ToolCallEvent) (*plugins.ToolCallResult, error) {
	if event.ToolName == "calc" {
		return &plugins.ToolCallResult{
			Result: 42,
		}, nil
	}
	return nil, io.EOF
}

func (h *testPluginHandler) OnCommand(ctx context.Context, event plugins.CommandEvent) (*plugins.CommandResult, error) {
	return &plugins.CommandResult{
		Output: "executed " + event.Command,
	}, nil
}

func TestSDK_ClientServerPipes(t *testing.T) {
	hostToPluginR, hostToPluginW := io.Pipe()
	pluginToHostR, pluginToHostW := io.Pipe()
	defer hostToPluginR.Close()
	defer hostToPluginW.Close()
	defer pluginToHostR.Close()
	defer pluginToHostW.Close()

	handler := &testPluginHandler{}
	runner := plugins.NewPluginRunner(handler, hostToPluginR, pluginToHostW)
	client := plugins.NewHostClient(pluginToHostR, hostToPluginW)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = runner.Run(ctx)
	}()

	// 1. Ping
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("client.Ping failed: %v", err)
	}

	// 2. Init
	initResp, err := client.SendCall(ctx, plugins.Message{
		Type: plugins.TypeInit,
		Payload: map[string]any{
			"config": map[string]any{"apiKey": "secret-123"},
		},
	})
	if err != nil {
		t.Fatalf("SendCall Init failed: %v", err)
	}
	if initResp.Type != plugins.TypeInitAck || !handler.inited {
		t.Fatalf("expected InitAck, got: %+v", initResp)
	}

	// 3. Transcript Hook
	trResp, err := client.SendCall(ctx, plugins.Message{
		Type: plugins.TypeTranscriptHook,
		Payload: map[string]any{
			"session_id": "sess-1",
			"text":       "hello world",
		},
	})
	if err != nil {
		t.Fatalf("TranscriptHook failed: %v", err)
	}
	if trResp.Payload["modified_text"] != "HELLO WORLD" {
		t.Fatalf("unexpected modified text: %+v", trResp.Payload)
	}

	// 4. Tool Call
	toolResp, err := client.SendCall(ctx, plugins.Message{
		Type: plugins.TypeToolCall,
		Payload: map[string]any{
			"tool_name": "calc",
		},
	})
	if err != nil {
		t.Fatalf("ToolCall failed: %v", err)
	}
	if toolResp.Payload["result"].(float64) != 42 {
		t.Fatalf("expected result 42, got: %+v", toolResp.Payload)
	}

	// 5. Command
	cmdResp, err := client.SendCall(ctx, plugins.Message{
		Type: plugins.TypeCommand,
		Payload: map[string]any{
			"command": "run-sync",
		},
	})
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	if cmdResp.Payload["output"] != "executed run-sync" {
		t.Fatalf("unexpected command output: %+v", cmdResp.Payload)
	}
}
