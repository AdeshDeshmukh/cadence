package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/system"
)

func TestRegistry_BuiltinTools(t *testing.T) {
	mockCtrl := system.NewMockController()
	mockSearch := func(_ context.Context, query string, limit int) ([]SearchResult, error) {
		return []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Snippet: "First result for " + query},
		}, nil
	}

	reg := NewRegistry(mockCtrl, mockSearch)
	ctx := context.Background()

	// 1. Tool listing and categories
	tools := reg.List()
	if len(tools) < 6 {
		t.Fatalf("expected at least 6 built-in tools, got %d", len(tools))
	}
	cats := reg.Categories()
	if len(cats) < 4 {
		t.Fatalf("expected at least 4 categories, got %d (%v)", len(cats), cats)
	}

	// 2. Export JSON Schema
	schemas := reg.ExportJSONSchema()
	if len(schemas) != len(tools) {
		t.Fatalf("expected %d schemas, got %d", len(tools), len(schemas))
	}

	// 3. Filesystem tool tests
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.txt")

	// 3a. fs write
	res, err := reg.Call(ctx, "fs", map[string]any{
		"action":  "write",
		"path":    testFile,
		"content": "Hello World!\nSecond line.",
	})
	if err != nil || !res.Success {
		t.Fatalf("fs write failed: %v, res: %+v", err, res)
	}

	// 3b. fs read
	res, err = reg.Call(ctx, "fs", map[string]any{
		"action": "read",
		"path":   testFile,
	})
	if err != nil || !res.Success || !strings.Contains(res.Output, "Hello World!") {
		t.Fatalf("fs read failed: %v, res: %+v", err, res)
	}

	// 3c. fs info
	res, err = reg.Call(ctx, "fs", map[string]any{
		"action": "info",
		"path":   testFile,
	})
	if err != nil || !res.Success || !strings.Contains(res.Output, "hello.txt") {
		t.Fatalf("fs info failed: %v, res: %+v", err, res)
	}

	// 3d. fs list
	res, err = reg.Call(ctx, "fs", map[string]any{
		"action": "list",
		"path":   tmpDir,
	})
	if err != nil || !res.Success || !strings.Contains(res.Output, "hello.txt") {
		t.Fatalf("fs list failed: %v, res: %+v", err, res)
	}

	// 3e. fs search
	res, err = reg.Call(ctx, "fs", map[string]any{
		"action":  "search",
		"path":    tmpDir,
		"pattern": "*.txt",
	})
	if err != nil || !res.Success || !strings.Contains(res.Output, "hello.txt") {
		t.Fatalf("fs search failed: %v, res: %+v", err, res)
	}

	// 4. Clipboard tool tests
	res, err = reg.Call(ctx, "clipboard", map[string]any{
		"action": "set",
		"text":   "cadence-test-clipboard-content",
	})
	if err != nil || !res.Success {
		t.Fatalf("clipboard set failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "clipboard", map[string]any{"action": "get"})
	if err != nil || !res.Success {
		t.Fatalf("clipboard get failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "clipboard", map[string]any{"action": "clear"})
	if err != nil || !res.Success {
		t.Fatalf("clipboard clear failed: %v, res: %+v", err, res)
	}

	// 5. Web Search tool tests
	res, err = reg.Call(ctx, "web_search", map[string]any{
		"query": "Go programming language",
	})
	if err != nil || !res.Success || !strings.Contains(res.Output, "Result 1") {
		t.Fatalf("web_search failed: %v, res: %+v", err, res)
	}

	// 6. System Info tool tests
	res, err = reg.Call(ctx, "system_info", map[string]any{"action": "os"})
	if err != nil || !res.Success || !strings.Contains(res.Output, "OS:") {
		t.Fatalf("system_info os failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "system_info", map[string]any{"action": "hostname"})
	if err != nil || !res.Success || res.Output == "" {
		t.Fatalf("system_info hostname failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "system_info", map[string]any{"action": "network"})
	if err != nil || !res.Success {
		t.Fatalf("system_info network failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "system_info", map[string]any{"action": "battery"})
	if err != nil || !res.Success {
		t.Fatalf("system_info battery failed: %v, res: %+v", err, res)
	}

	// 7. System action tool tests (launch, focus, window, volume, screenshot)
	res, err = reg.Call(ctx, "system", map[string]any{
		"action": "launch",
		"target": "Slack",
	})
	if err != nil || !res.Success || len(mockCtrl.LaunchedApps) != 1 {
		t.Fatalf("system launch failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "system", map[string]any{
		"action": "window",
		"target": "minimize",
	})
	if err != nil || !res.Success || len(mockCtrl.Windows) != 1 {
		t.Fatalf("system window failed: %v, res: %+v", err, res)
	}

	res, err = reg.Call(ctx, "system", map[string]any{
		"action": "volume",
		"level":  75.0,
	})
	if err != nil || !res.Success || mockCtrl.Volume != 75 {
		t.Fatalf("system volume failed: %v, res: %+v", err, res)
	}

	// 8. Error handling
	// Missing required parameter
	res, err = reg.Call(ctx, "fs", map[string]any{})
	if res.Success {
		t.Fatal("expected failure on missing action")
	}

	// Unregistered tool
	res, err = reg.Call(ctx, "non_existent_tool", map[string]any{})
	if res.Success {
		t.Fatal("expected failure on unregistered tool")
	}

	// Empty name registration error
	if err := reg.Register(ToolDefinition{}); err == nil {
		t.Fatal("expected error on empty tool name")
	}
}

func TestEngine_RichPlanning(t *testing.T) {
	mockCtrl := system.NewMockController()
	eng := NewEngine(WithSystemController(mockCtrl))
	ctx := context.Background()

	// 1. Filesystem goal decomposition
	p, err := eng.Plan(ctx, "read file main.go and then list files in src")
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(p.Steps))
	}
	if p.Steps[0].Tool != "fs" || p.Steps[0].Params["action"] != "read" {
		t.Errorf("step 0 = %+v", p.Steps[0])
	}
	if p.Steps[1].Tool != "fs" || p.Steps[1].Params["action"] != "list" {
		t.Errorf("step 1 = %+v", p.Steps[1])
	}

	// 2. Clipboard goal decomposition
	p, err = eng.Plan(ctx, "copy hello world to clipboard and then get clipboard")
	if err != nil {
		t.Fatalf("clipboard Plan failed: %v", err)
	}
	if len(p.Steps) != 2 || p.Steps[0].Tool != "clipboard" || p.Steps[1].Tool != "clipboard" {
		t.Fatalf("expected 2 clipboard steps, got %+v", p.Steps)
	}

	// 3. Web search goal decomposition
	p, err = eng.Plan(ctx, "search web for Go tutorials")
	if err != nil {
		t.Fatalf("web_search Plan failed: %v", err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Tool != "web_search" {
		t.Fatalf("expected web_search step, got %+v", p.Steps)
	}

	// 4. System info goal decomposition
	p, err = eng.Plan(ctx, "check battery and os info")
	if err != nil {
		t.Fatalf("system_info Plan failed: %v", err)
	}
	if len(p.Steps) != 2 || p.Steps[0].Tool != "system_info" || p.Steps[1].Tool != "system_info" {
		t.Fatalf("expected 2 system_info steps, got %+v", p.Steps)
	}
}

func TestEngine_CustomToolRegistration(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()

	eng.RegisterTool("multiply", func(_ context.Context, params map[string]any) (ToolResult, error) {
		a, _ := params["a"].(float64)
		b, _ := params["b"].(float64)
		return ToolResult{
			Success: true,
			Output:  "success",
			Data:    map[string]any{"product": a * b},
		}, nil
	})

	res, err := eng.Registry().Call(ctx, "multiply", map[string]any{"a": 3.0, "b": 4.0})
	if err != nil || !res.Success {
		t.Fatalf("multiply failed: %v, res: %+v", err, res)
	}
	if res.Data["product"] != 12.0 {
		t.Fatalf("expected product 12, got %v", res.Data["product"])
	}
}
