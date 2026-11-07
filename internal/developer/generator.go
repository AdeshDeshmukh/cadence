package developer

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
)

// GeneratedCommand represents the generated shell command outcome from a natural voice instruction.
type GeneratedCommand struct {
	RawInstruction string         `json:"raw_instruction"`
	Command        string         `json:"command"`
	Explanation    string         `json:"explanation,omitempty"`
	Ecosystem      Ecosystem      `json:"ecosystem,omitempty"`
	Confidence     float64        `json:"confidence"`
	Source         string         `json:"source"` // "rule" or "llm"
	Latency        time.Duration  `json:"latency"`
	Risk           RiskAssessment `json:"risk"`
}

// Generator translates natural language voice instructions into executable shell commands.
type Generator struct {
	polisher polish.Polisher
}

// NewGenerator creates a new developer command generator.
func NewGenerator(p polish.Polisher) *Generator {
	if p == nil {
		p = polish.NewNoopPolisher()
	}
	return &Generator{
		polisher: p,
	}
}

var (
	reTest         = regexp.MustCompile(`(?i)^(?:run\s+(?:the\s+|unit\s+)?tests?|test\s+this|run\s+tests?|execute\s+tests?|run\s+test\s+suite)$`)
	reFailingTests = regexp.MustCompile(`(?i)^(?:run\s+failing\s+tests?|rerun\s+failing\s+tests?|run\s+failed\s+tests?|rerun\s+tests?)$`)
	reCommit       = regexp.MustCompile(`(?i)^(?:git\s+)?commit(?:\s+with\s+message|\s+message)?\s+(.+)$`)
	reStatus       = regexp.MustCompile(`(?i)^(?:git\s+)?(?:status|check\s+status|what\s+changed|show\s+status)$`)
	reDiff         = regexp.MustCompile(`(?i)^(?:git\s+)?(?:diff|show\s+diff|check\s+diff|show\s+changes)$`)
	rePush         = regexp.MustCompile(`(?i)^(?:git\s+)?(?:push|push\s+to\s+origin|push\s+changes|git\s+push\s+origin)$`)
	rePull         = regexp.MustCompile(`(?i)^(?:git\s+)?(?:pull|pull\s+latest|pull\s+from\s+origin|git\s+pull\s+origin)$`)
	reNewBranch    = regexp.MustCompile(`(?i)^(?:create\s+(?:a\s+)?new\s+branch|checkout\s+(?:a\s+)?new\s+branch|new\s+branch|branch)\s+(?:called\s+|named\s+)?(.+)$`)
	reSwitchBranch = regexp.MustCompile(`(?i)^(?:switch\s+to\s+branch|checkout\s+branch|switch\s+to|switch\s+branch)\s+(.+)$`)
	reStageAll     = regexp.MustCompile(`(?i)^(?:git\s+add\s+all|stage\s+all\s+changes|stage\s+all|add\s+all\s+files|git\s+add\s+everything)$`)
	reLog          = regexp.MustCompile(`(?i)^(?:git\s+log|recent\s+commits|show\s+commit\s+history|commit\s+history)$`)
	reBuild        = regexp.MustCompile(`(?i)^(?:build\s+(?:the\s+)?project|build|compile|make\s+build)$`)
	reInstall      = regexp.MustCompile(`(?i)^(?:install\s+dependencies|install\s+packages|npm\s+install|tidy\s+dependencies|tidy\s+modules)$`)
	reFormat       = regexp.MustCompile(`(?i)^(?:format\s+code|run\s+formatter|format\s+files|fmt)$`)
	reClean        = regexp.MustCompile(`(?i)^(?:clean\s+build|clean\s+cache|clean)$`)
)

// Generate translates a natural voice instruction into a shell command based on project context.
func (g *Generator) Generate(ctx context.Context, instruction string, workingDir string) (GeneratedCommand, error) {
	clean := strings.TrimSpace(instruction)
	if clean == "" {
		return GeneratedCommand{}, fmt.Errorf("empty command instruction")
	}

	start := time.Now()
	proj := DetectProject(workingDir)

	// 1. Deterministic Rule Matching (zero-latency)
	if cmd, expl, ok := matchDeterministicRules(clean, proj); ok {
		return GeneratedCommand{
			RawInstruction: clean,
			Command:        cmd,
			Explanation:    expl,
			Ecosystem:      proj.Ecosystem,
			Confidence:     1.0,
			Source:         "rule",
			Latency:        time.Since(start),
			Risk:           ClassifyCommand(cmd),
		}, nil
	}

	// 2. LLM Fallback Generation
	prompt := buildDeveloperPrompt(clean, proj)
	req := polish.Request{
		Text: prompt,
		Metadata: map[string]string{
			"type":      "command_generation",
			"ecosystem": string(proj.Ecosystem),
			"root_path": proj.RootPath,
		},
	}

	resp, err := g.polisher.Polish(ctx, req)
	if err != nil {
		// Fallback to raw instruction
		return GeneratedCommand{
			RawInstruction: clean,
			Command:        clean,
			Explanation:    "Pass-through command",
			Ecosystem:      proj.Ecosystem,
			Confidence:     0.5,
			Source:         "fallback",
			Latency:        time.Since(start),
			Risk:           ClassifyCommand(clean),
		}, nil
	}

	genCmd := cleanLLMCommandOutput(resp.Text)
	if genCmd == "" {
		genCmd = clean
	}

	return GeneratedCommand{
		RawInstruction: clean,
		Command:        genCmd,
		Explanation:    fmt.Sprintf("Generated for %s project", proj.Ecosystem),
		Ecosystem:      proj.Ecosystem,
		Confidence:     0.9,
		Source:         "llm",
		Latency:        time.Since(start),
		Risk:           ClassifyCommand(genCmd),
	}, nil
}

func matchDeterministicRules(instr string, proj ProjectInfo) (string, string, bool) {
	// Tests
	if reTest.MatchString(instr) {
		switch proj.Ecosystem {
		case EcosystemGo:
			return "go test -v ./...", "Run all Go tests verbosely", true
		case EcosystemRust:
			return "cargo test", "Run all Rust package tests", true
		case EcosystemNode:
			return proj.PackageManager + " test", fmt.Sprintf("Run Node tests via %s", proj.PackageManager), true
		case EcosystemPython:
			return "pytest", "Run Python pytest test suite", true
		case EcosystemMake:
			return "make test", "Run tests via Makefile target", true
		default:
			return "go test ./...", "Run project tests", true
		}
	}

	// Failing Tests
	if reFailingTests.MatchString(instr) {
		switch proj.Ecosystem {
		case EcosystemGo:
			return "go test -v -run Fail ./...", "Run failing Go unit tests", true
		case EcosystemPython:
			return "pytest --lf", "Rerun last failing tests in pytest", true
		case EcosystemNode:
			return proj.PackageManager + " test -- --onlyFailures", "Rerun failing tests", true
		case EcosystemRust:
			return "cargo test", "Rerun Rust test suite", true
		default:
			return "make test", "Rerun test suite", true
		}
	}

	// Git Commit
	if m := reCommit.FindStringSubmatch(instr); len(m) > 1 {
		msg := cleanCommitMessage(m[1])
		return fmt.Sprintf("git commit -m %q", msg), fmt.Sprintf("Commit staged changes with message %q", msg), true
	}

	// Git Status
	if reStatus.MatchString(instr) {
		return "git status", "Show working tree status", true
	}

	// Git Diff
	if reDiff.MatchString(instr) {
		return "git diff", "Show unstaged changes in working tree", true
	}

	// Git Push
	if rePush.MatchString(instr) {
		if proj.GitBranch != "" {
			return fmt.Sprintf("git push origin %s", proj.GitBranch), fmt.Sprintf("Push current branch %s to origin", proj.GitBranch), true
		}
		return "git push", "Push current branch to remote", true
	}

	// Git Pull
	if rePull.MatchString(instr) {
		return "git pull", "Fetch and rebase/merge latest changes", true
	}

	// Git Checkout New Branch
	if m := reNewBranch.FindStringSubmatch(instr); len(m) > 1 {
		branchName := cleanBranchName(m[1])
		return fmt.Sprintf("git checkout -b %s", branchName), fmt.Sprintf("Create and switch to new branch %s", branchName), true
	}

	// Git Switch Branch
	if m := reSwitchBranch.FindStringSubmatch(instr); len(m) > 1 {
		branchName := cleanBranchName(m[1])
		return fmt.Sprintf("git checkout %s", branchName), fmt.Sprintf("Switch to branch %s", branchName), true
	}

	// Stage All
	if reStageAll.MatchString(instr) {
		return "git add -A", "Stage all changes and untracked files", true
	}

	// Git Log
	if reLog.MatchString(instr) {
		return "git log --oneline -n 10", "Show last 10 commits on current branch", true
	}

	// Build
	if reBuild.MatchString(instr) {
		switch proj.Ecosystem {
		case EcosystemGo:
			return "go build ./...", "Compile all Go packages", true
		case EcosystemRust:
			return "cargo build", "Compile Rust binary and packages", true
		case EcosystemNode:
			return proj.PackageManager + " run build", "Execute build script", true
		case EcosystemMake:
			return "make build", "Run build target via Makefile", true
		default:
			return "go build ./...", "Build project", true
		}
	}

	// Install Dependencies
	if reInstall.MatchString(instr) {
		switch proj.Ecosystem {
		case EcosystemGo:
			return "go mod tidy", "Download and prune Go module dependencies", true
		case EcosystemRust:
			return "cargo check", "Verify and download crate dependencies", true
		case EcosystemNode:
			return proj.PackageManager + " install", fmt.Sprintf("Install dependencies via %s", proj.PackageManager), true
		case EcosystemPython:
			return "pip install -r requirements.txt", "Install Python dependencies", true
		default:
			return "go mod tidy", "Install project dependencies", true
		}
	}

	// Format
	if reFormat.MatchString(instr) {
		switch proj.Ecosystem {
		case EcosystemGo:
			return "gofmt -w .", "Format all Go source files in-place", true
		case EcosystemRust:
			return "cargo fmt", "Format Rust crate files", true
		case EcosystemNode:
			return "npx prettier --write .", "Format code using Prettier", true
		case EcosystemPython:
			return "black .", "Format Python files with Black", true
		default:
			return "gofmt -w .", "Format source files", true
		}
	}

	// Clean
	if reClean.MatchString(instr) {
		switch proj.Ecosystem {
		case EcosystemGo:
			return "go clean -cache", "Clear Go build and test cache", true
		case EcosystemRust:
			return "cargo clean", "Remove target build directory", true
		case EcosystemNode:
			return "rm -rf node_modules", "Clean dependencies", true
		case EcosystemMake:
			return "make clean", "Run clean target via Makefile", true
		default:
			return "git clean -fd", "Clean untracked files", true
		}
	}

	return "", "", false
}

func cleanCommitMessage(msg string) string {
	m := strings.TrimSpace(msg)
	m = strings.Trim(m, `"'`)
	m = strings.TrimSpace(m)
	// Remove trailing period if present
	m = strings.TrimSuffix(m, ".")
	return m
}

func cleanBranchName(name string) string {
	b := strings.TrimSpace(name)
	b = strings.Trim(b, `"'`)
	b = strings.ToLower(b)
	// Replace spaces and special characters with hyphens
	b = regexp.MustCompile(`[^a-z0-9/_-]+`).ReplaceAllString(b, "-")
	b = strings.Trim(b, "-")
	return b
}

func buildDeveloperPrompt(instr string, proj ProjectInfo) string {
	var sb strings.Builder
	sb.WriteString("You are a developer terminal assistant. Convert the user's natural language request into a single executable shell command.\n")
	sb.WriteString(fmt.Sprintf("Project Ecosystem: %s\n", proj.Ecosystem))
	if proj.GitBranch != "" {
		sb.WriteString(fmt.Sprintf("Git Branch: %s\n", proj.GitBranch))
	}
	sb.WriteString("Respond ONLY with the exact shell command to execute. Do not wrap in markdown or backticks. No explanations.\n\n")
	sb.WriteString(fmt.Sprintf("Request: %s\nCommand:", instr))
	return sb.String()
}

func cleanLLMCommandOutput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	// Remove markdown backticks if any
	trimmed = strings.TrimPrefix(trimmed, "```bash")
	trimmed = strings.TrimPrefix(trimmed, "```sh")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	lines := strings.Split(trimmed, "\n")
	for _, l := range lines {
		line := strings.TrimSpace(l)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return strings.TrimSpace(trimmed)
}
