package developer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
)

func TestDetectProject(t *testing.T) {
	// 1. Go Project
	goDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test"), 0644)
	goProj := DetectProject(goDir)
	if goProj.Ecosystem != EcosystemGo || goProj.TestRunner != "go test ./..." || goProj.BuildTool != "go build ./..." {
		t.Fatalf("unexpected go project detection: %+v", goProj)
	}

	// 2. Rust Project
	rustDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(rustDir, "Cargo.toml"), []byte("[package]"), 0644)
	rustProj := DetectProject(rustDir)
	if rustProj.Ecosystem != EcosystemRust || rustProj.PackageManager != "cargo" {
		t.Fatalf("unexpected rust project detection: %+v", rustProj)
	}

	// 3. Node (pnpm)
	nodeDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte("{}"), 0644)
	_ = os.WriteFile(filepath.Join(nodeDir, "pnpm-lock.yaml"), []byte(""), 0644)
	nodeProj := DetectProject(nodeDir)
	if nodeProj.Ecosystem != EcosystemNode || nodeProj.PackageManager != "pnpm" || nodeProj.TestRunner != "pnpm test" {
		t.Fatalf("unexpected node project detection: %+v", nodeProj)
	}

	// 4. Python Project
	pyDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(pyDir, "pyproject.toml"), []byte(""), 0644)
	pyProj := DetectProject(pyDir)
	if pyProj.Ecosystem != EcosystemPython || pyProj.TestRunner != "pytest" {
		t.Fatalf("unexpected python project detection: %+v", pyProj)
	}

	// 5. Make Project
	makeDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(makeDir, "Makefile"), []byte("test:\n\techo test"), 0644)
	makeProj := DetectProject(makeDir)
	if makeProj.Ecosystem != EcosystemMake || makeProj.TestRunner != "make test" {
		t.Fatalf("unexpected make project detection: %+v", makeProj)
	}
}

func TestGenerator_DeterministicRules(t *testing.T) {
	gen := NewGenerator(nil)
	ctx := context.Background()

	// Setup mock Go project dir
	goDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test"), 0644)
	gitDir := filepath.Join(goDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/cmd-gen\n"), 0644)

	// Test 1: Run tests
	res1, err := gen.Generate(ctx, "run the tests", goDir)
	if err != nil || res1.Command != "go test -v ./..." || res1.Source != "rule" {
		t.Fatalf("expected 'go test -v ./...', got %+v, err: %v", res1, err)
	}

	// Test 2: Run failing tests
	res2, err := gen.Generate(ctx, "run failing tests", goDir)
	if err != nil || !strings.Contains(res2.Command, "Fail") {
		t.Fatalf("expected failing test runner, got %+v", res2)
	}

	// Test 3: Git Commit
	res3, err := gen.Generate(ctx, "git commit with message updated terminal context.", goDir)
	if err != nil || res3.Command != `git commit -m "updated terminal context"` {
		t.Fatalf("expected commit command, got %+v", res3)
	}

	// Test 4: Git Status
	res4, err := gen.Generate(ctx, "check status", goDir)
	if err != nil || res4.Command != "git status" {
		t.Fatalf("expected 'git status', got %+v", res4)
	}

	// Test 5: Git Diff
	res5, err := gen.Generate(ctx, "show diff", goDir)
	if err != nil || res5.Command != "git diff" {
		t.Fatalf("expected 'git diff', got %+v", res5)
	}

	// Test 6: Git Push
	res6, err := gen.Generate(ctx, "push to origin", goDir)
	if err != nil || res6.Command != "git push origin feature/cmd-gen" {
		t.Fatalf("expected push to branch, got %+v", res6)
	}

	// Test 7: Create new branch
	res7, err := gen.Generate(ctx, "create a new branch called feat voice auth", goDir)
	if err != nil || res7.Command != "git checkout -b feat-voice-auth" {
		t.Fatalf("expected 'git checkout -b feat-voice-auth', got %+v", res7)
	}

	// Test 8: Switch branch
	res8, err := gen.Generate(ctx, "switch to branch main", goDir)
	if err != nil || res8.Command != "git checkout main" {
		t.Fatalf("expected 'git checkout main', got %+v", res8)
	}

	// Test 9: Install dependencies (Go)
	res9, err := gen.Generate(ctx, "install dependencies", goDir)
	if err != nil || res9.Command != "go mod tidy" {
		t.Fatalf("expected 'go mod tidy', got %+v", res9)
	}

	// Test 10: Format code (Go)
	res10, err := gen.Generate(ctx, "format code", goDir)
	if err != nil || res10.Command != "gofmt -w ." {
		t.Fatalf("expected 'gofmt -w .', got %+v", res10)
	}
}

func TestGenerator_NodeAndRustRules(t *testing.T) {
	gen := NewGenerator(nil)
	ctx := context.Background()

	// 1. Rust tests
	rustDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(rustDir, "Cargo.toml"), []byte("[package]"), 0644)
	resRust, err := gen.Generate(ctx, "run tests", rustDir)
	if err != nil || resRust.Command != "cargo test" {
		t.Fatalf("expected 'cargo test', got %+v", resRust)
	}

	// 2. Node (pnpm) tests & install
	nodeDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte("{}"), 0644)
	_ = os.WriteFile(filepath.Join(nodeDir, "pnpm-lock.yaml"), []byte(""), 0644)
	resNode, err := gen.Generate(ctx, "run tests", nodeDir)
	if err != nil || resNode.Command != "pnpm test" {
		t.Fatalf("expected 'pnpm test', got %+v", resNode)
	}

	resInstall, err := gen.Generate(ctx, "install dependencies", nodeDir)
	if err != nil || resInstall.Command != "pnpm install" {
		t.Fatalf("expected 'pnpm install', got %+v", resInstall)
	}
}

type mockPolisher struct {
	output string
}

func (m mockPolisher) Name() string {
	return "mock-polisher"
}

func (m mockPolisher) Polish(_ context.Context, _ polish.Request) (polish.Response, error) {
	return polish.Response{
		Text:  m.output,
		Model: "mock-llm",
	}, nil
}

func TestGenerator_LLMFallback(t *testing.T) {
	mockP := mockPolisher{output: "docker ps -a --filter status=running"}
	gen := NewGenerator(mockP)

	res, err := gen.Generate(context.Background(), "list all running docker containers", "/tmp")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if res.Command != "docker ps -a --filter status=running" {
		t.Fatalf("unexpected llm command: %s", res.Command)
	}
	if res.Source != "llm" {
		t.Fatalf("expected source 'llm', got %s", res.Source)
	}
}
