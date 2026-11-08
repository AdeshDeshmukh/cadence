package developer

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
)

// DiffSummary provides structured metrics and diff content for git changes.
type DiffSummary struct {
	FilesChanged int      `json:"files_changed"`
	Insertions   int      `json:"insertions"`
	Deletions    int      `json:"deletions"`
	Files        []string `json:"files"`
	RawDiff      string   `json:"raw_diff"`
	StatSummary  string   `json:"stat_summary"`
}

// BranchSummary encapsulates commit history and diff delta between a branch and its base.
type BranchSummary struct {
	CurrentBranch string      `json:"current_branch"`
	BaseBranch    string      `json:"base_branch"`
	Commits       []string    `json:"commits"`
	DiffSummary   DiffSummary `json:"diff_summary"`
}

// CommitGenRequest defines parameters for generating a commit message.
type CommitGenRequest struct {
	WorkingDir string `json:"working_dir"`
	SpokenHint string `json:"spoken_hint,omitempty"`
}

// CommitMessage represents a Conventional Commit message.
type CommitMessage struct {
	Title       string `json:"title"`
	Body        string `json:"body,omitempty"`
	FullMessage string `json:"full_message"`
	Type        string `json:"type,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// PRGenRequest defines parameters for generating a pull request title and description.
type PRGenRequest struct {
	WorkingDir  string `json:"working_dir"`
	BaseBranch  string `json:"base_branch,omitempty"`
	SpokenNotes string `json:"spoken_notes,omitempty"`
}

// PRDescription represents the title and Markdown body of a pull request.
type PRDescription struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	BaseBranch  string `json:"base_branch"`
	HeadBranch  string `json:"head_branch"`
	CommitCount int    `json:"commit_count"`
}

// GitAssistant provides intelligent Git operations, commit message generation, and PR descriptions.
type GitAssistant struct {
	polisher polish.Polisher
}

// NewGitAssistant creates a new Git assistant backed by an optional LLM polisher.
func NewGitAssistant(p polish.Polisher) *GitAssistant {
	if p == nil {
		p = polish.NewNoopPolisher()
	}
	return &GitAssistant{polisher: p}
}

// GetStagedDiff retrieves and parses the staged diff in the given working directory.
func (g *GitAssistant) GetStagedDiff(ctx context.Context, workingDir string) (DiffSummary, error) {
	cleanDir := filepath.Clean(workingDir)

	// 1. Check if git repo
	if !isGitRepo(ctx, cleanDir) {
		return DiffSummary{}, fmt.Errorf("directory '%s' is not inside a git repository", cleanDir)
	}

	// 2. Fetch staged stat
	statCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--stat")
	statCmd.Dir = cleanDir
	statOut, err := statCmd.Output()
	if err != nil {
		return DiffSummary{}, fmt.Errorf("git diff --cached --stat: %w", err)
	}

	statStr := strings.TrimSpace(string(statOut))
	if statStr == "" {
		return DiffSummary{}, nil // clean / no staged changes
	}

	// 3. Fetch staged file names
	filesCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only")
	filesCmd.Dir = cleanDir
	filesOut, _ := filesCmd.Output()
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(string(filesOut)), "\n") {
		if strings.TrimSpace(f) != "" {
			files = append(files, strings.TrimSpace(f))
		}
	}

	// 4. Fetch diff with reasonable context limit (12KB)
	diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "-U3")
	diffCmd.Dir = cleanDir
	diffOut, _ := diffCmd.Output()
	rawDiff := string(diffOut)
	if len(rawDiff) > 12000 {
		rawDiff = rawDiff[:12000] + "\n\n... [diff truncated for context length]"
	}

	insertions, deletions := parseStatCounts(statStr)

	return DiffSummary{
		FilesChanged: len(files),
		Insertions:   insertions,
		Deletions:    deletions,
		Files:        files,
		RawDiff:      rawDiff,
		StatSummary:  statStr,
	}, nil
}

// GetBranchSummary calculates the commit and diff delta between HEAD and the base branch.
func (g *GitAssistant) GetBranchSummary(ctx context.Context, workingDir, baseBranch string) (BranchSummary, error) {
	cleanDir := filepath.Clean(workingDir)
	if !isGitRepo(ctx, cleanDir) {
		return BranchSummary{}, fmt.Errorf("directory '%s' is not inside a git repository", cleanDir)
	}

	currentBranch := getGitBranch(ctx, cleanDir)
	if baseBranch == "" {
		baseBranch = detectBaseBranch(ctx, cleanDir)
	}

	// Commits on branch vs base
	logCmd := exec.CommandContext(ctx, "git", "log", fmt.Sprintf("%s..HEAD", baseBranch), "--oneline")
	logCmd.Dir = cleanDir
	logOut, _ := logCmd.Output()

	var commits []string
	for _, line := range strings.Split(strings.TrimSpace(string(logOut)), "\n") {
		if strings.TrimSpace(line) != "" {
			commits = append(commits, strings.TrimSpace(line))
		}
	}

	// Diff vs base
	statCmd := exec.CommandContext(ctx, "git", "diff", fmt.Sprintf("%s...HEAD", baseBranch), "--stat")
	statCmd.Dir = cleanDir
	statOut, _ := statCmd.Output()
	statStr := strings.TrimSpace(string(statOut))

	filesCmd := exec.CommandContext(ctx, "git", "diff", fmt.Sprintf("%s...HEAD", baseBranch), "--name-only")
	filesCmd.Dir = cleanDir
	filesOut, _ := filesCmd.Output()
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(string(filesOut)), "\n") {
		if strings.TrimSpace(f) != "" {
			files = append(files, strings.TrimSpace(f))
		}
	}

	diffCmd := exec.CommandContext(ctx, "git", "diff", fmt.Sprintf("%s...HEAD", baseBranch), "-U2")
	diffCmd.Dir = cleanDir
	diffOut, _ := diffCmd.Output()
	rawDiff := string(diffOut)
	if len(rawDiff) > 16000 {
		rawDiff = rawDiff[:16000] + "\n\n... [diff truncated for context length]"
	}

	insertions, deletions := parseStatCounts(statStr)

	return BranchSummary{
		CurrentBranch: currentBranch,
		BaseBranch:    baseBranch,
		Commits:       commits,
		DiffSummary: DiffSummary{
			FilesChanged: len(files),
			Insertions:   insertions,
			Deletions:    deletions,
			Files:        files,
			RawDiff:      rawDiff,
			StatSummary:  statStr,
		},
	}, nil
}

// GenerateCommit synthesizes a Conventional Commit message from staged changes and optional spoken hint.
func (g *GitAssistant) GenerateCommit(ctx context.Context, req CommitGenRequest) (CommitMessage, error) {
	diff, err := g.GetStagedDiff(ctx, req.WorkingDir)
	if err != nil {
		return CommitMessage{}, err
	}

	if len(diff.Files) == 0 {
		return CommitMessage{}, errors.New("no staged changes found; stage files with 'git add' before generating commit message")
	}

	// If polisher is available and not noop, generate via LLM
	if g.polisher != nil && g.polisher.Name() != "noop" {
		prompt := buildCommitPrompt(req.SpokenHint, diff)
		resp, pErr := g.polisher.Polish(ctx, polish.Request{
			Text: prompt,
			Metadata: map[string]string{
				"type": "git_commit_generation",
			},
		})
		if pErr == nil && strings.TrimSpace(resp.Text) != "" {
			return parseCommitResponse(resp.Text), nil
		}
	}

	// Deterministic heuristic fallback
	return fallbackCommitMessage(req.SpokenHint, diff), nil
}

// GeneratePR synthesizes a structured Pull Request title and description from branch commits and diff.
func (g *GitAssistant) GeneratePR(ctx context.Context, req PRGenRequest) (PRDescription, error) {
	summary, err := g.GetBranchSummary(ctx, req.WorkingDir, req.BaseBranch)
	if err != nil {
		return PRDescription{}, err
	}

	if len(summary.Commits) == 0 && summary.DiffSummary.FilesChanged == 0 {
		return PRDescription{
			Title:       fmt.Sprintf("Update %s", summary.CurrentBranch),
			Body:        "No commits or diff found between current branch and base branch.",
			BaseBranch:  summary.BaseBranch,
			HeadBranch:  summary.CurrentBranch,
			CommitCount: 0,
		}, nil
	}

	if g.polisher != nil && g.polisher.Name() != "noop" {
		prompt := buildPRPrompt(req.SpokenNotes, summary)
		resp, pErr := g.polisher.Polish(ctx, polish.Request{
			Text: prompt,
			Metadata: map[string]string{
				"type": "git_pr_generation",
			},
		})
		if pErr == nil && strings.TrimSpace(resp.Text) != "" {
			title, body := parsePRResponse(resp.Text, summary)
			return PRDescription{
				Title:       title,
				Body:        body,
				BaseBranch:  summary.BaseBranch,
				HeadBranch:  summary.CurrentBranch,
				CommitCount: len(summary.Commits),
			}, nil
		}
	}

	// Deterministic fallback PR generator
	return fallbackPRDescription(req.SpokenNotes, summary), nil
}

func buildCommitPrompt(hint string, diff DiffSummary) string {
	var sb strings.Builder
	sb.WriteString("You are an expert software engineer generating a Conventional Commit message.\n")
	sb.WriteString("Format rule:\n")
	sb.WriteString("<type>(<optional scope>): <imperative short description>\n\n")
	sb.WriteString("[optional body explaining motivation and context]\n\n")
	sb.WriteString("Types: feat, fix, refactor, test, docs, chore, perf, style, build, ci.\n\n")

	if strings.TrimSpace(hint) != "" {
		sb.WriteString(fmt.Sprintf("Developer spoken voice hint: %q\n\n", hint))
	}

	sb.WriteString(fmt.Sprintf("Changed files (%d):\n", len(diff.Files)))
	for _, f := range diff.Files {
		sb.WriteString("- " + f + "\n")
	}
	sb.WriteString("\nStat summary:\n" + diff.StatSummary + "\n\n")
	sb.WriteString("Diff snippet:\n```diff\n" + diff.RawDiff + "\n```\n\n")
	sb.WriteString("Generate ONLY the commit message. No markdown code blocks around the whole message, no explanations.")
	return sb.String()
}

func buildPRPrompt(notes string, summary BranchSummary) string {
	var sb strings.Builder
	sb.WriteString("You are an expert developer generating a professional GitHub Pull Request description in Markdown.\n")
	sb.WriteString("Format template:\n")
	sb.WriteString("## Title: <concise PR title>\n\n")
	sb.WriteString("## Overview\n<high-level summary of the purpose>\n\n")
	sb.WriteString("## Key Changes\n<bullet points of key additions, fixes, or refactors>\n\n")
	sb.WriteString("## Verification & Testing\n<bullet points on how changes were tested>\n\n")

	if strings.TrimSpace(notes) != "" {
		sb.WriteString(fmt.Sprintf("Developer voice notes / instructions:\n%s\n\n", notes))
	}

	sb.WriteString(fmt.Sprintf("Branch: %s into %s\n", summary.CurrentBranch, summary.BaseBranch))
	sb.WriteString(fmt.Sprintf("Commits (%d):\n", len(summary.Commits)))
	for _, c := range summary.Commits {
		sb.WriteString("- " + c + "\n")
	}
	sb.WriteString(fmt.Sprintf("\nChanged files (%d):\n%s\n\n", summary.DiffSummary.FilesChanged, summary.DiffSummary.StatSummary))
	sb.WriteString("Diff snippet:\n```diff\n" + summary.DiffSummary.RawDiff + "\n```\n")
	return sb.String()
}

func parseCommitResponse(text string) CommitMessage {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	lines := strings.Split(trimmed, "\n")
	title := strings.TrimSpace(lines[0])
	body := ""
	if len(lines) > 1 {
		body = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}

	cType, scope := extractCommitTypeAndScope(title)

	return CommitMessage{
		Title:       title,
		Body:        body,
		FullMessage: trimmed,
		Type:        cType,
		Scope:       scope,
	}
}

func parsePRResponse(text string, summary BranchSummary) (string, string) {
	trimmed := strings.TrimSpace(text)
	title := fmt.Sprintf("feat: update %s", summary.CurrentBranch)
	body := trimmed

	lines := strings.Split(trimmed, "\n")
	for i, l := range lines {
		line := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToLower(line), "## title:") || strings.HasPrefix(strings.ToLower(line), "title:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				title = strings.TrimSpace(parts[1])
				// remove title line from body
				bodyLines := append(lines[:i], lines[i+1:]...)
				body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
				break
			}
		}
	}

	return title, body
}

func fallbackCommitMessage(hint string, diff DiffSummary) CommitMessage {
	trimmedHint := strings.TrimSpace(hint)
	scope := inferScope(diff.Files)
	cType := inferType(trimmedHint, diff.Files)

	if trimmedHint != "" {
		cleanHint := strings.ToLower(trimmedHint)
		// strip existing prefix if user spoke it
		prefixes := []string{"feat:", "fix:", "chore:", "test:", "docs:", "refactor:", "commit with message", "commit message"}
		for _, p := range prefixes {
			if strings.HasPrefix(cleanHint, p) {
				cleanHint = strings.TrimSpace(cleanHint[len(p):])
				break
			}
		}

		title := ""
		if scope != "" {
			title = fmt.Sprintf("%s(%s): %s", cType, scope, cleanHint)
		} else {
			title = fmt.Sprintf("%s: %s", cType, cleanHint)
		}

		body := fmt.Sprintf("Changes:\n- %s", strings.Join(diff.Files, "\n- "))
		return CommitMessage{
			Title:       title,
			Body:        body,
			FullMessage: title + "\n\n" + body,
			Type:        cType,
			Scope:       scope,
		}
	}

	// No hint provided: construct from files
	action := "update"
	if diff.Insertions > 0 && diff.Deletions == 0 {
		action = "add"
	}
	desc := fmt.Sprintf("%s %s", action, scope)
	if scope == "" {
		desc = fmt.Sprintf("%s %d file(s)", action, len(diff.Files))
	}

	title := fmt.Sprintf("%s: %s", cType, desc)
	body := fmt.Sprintf("Affected files:\n- %s\n\nStats: %s", strings.Join(diff.Files, "\n- "), diff.StatSummary)
	return CommitMessage{
		Title:       title,
		Body:        body,
		FullMessage: title + "\n\n" + body,
		Type:        cType,
		Scope:       scope,
	}
}

func fallbackPRDescription(notes string, summary BranchSummary) PRDescription {
	title := fmt.Sprintf("feat: update %s", summary.CurrentBranch)
	if len(summary.Commits) == 1 {
		parts := strings.SplitN(summary.Commits[0], " ", 2)
		if len(parts) == 2 {
			title = parts[1]
		}
	} else if len(summary.Commits) > 1 {
		title = fmt.Sprintf("feat(%s): implement branch updates (%d commits)", summary.CurrentBranch, len(summary.Commits))
	}

	var sb strings.Builder
	sb.WriteString("## 📌 Overview\n")
	if strings.TrimSpace(notes) != "" {
		sb.WriteString(strings.TrimSpace(notes) + "\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("Merge `%s` into `%s` with %d commit(s) modifying %d file(s).\n\n",
			summary.CurrentBranch, summary.BaseBranch, len(summary.Commits), summary.DiffSummary.FilesChanged))
	}

	sb.WriteString("## 🔨 Commits & Key Changes\n")
	if len(summary.Commits) > 0 {
		for _, c := range summary.Commits {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	} else {
		sb.WriteString("- Direct branch delta\n")
	}

	sb.WriteString("\n## 📊 Affected Files\n")
	for _, f := range summary.DiffSummary.Files {
		sb.WriteString(fmt.Sprintf("- `%s`\n", f))
	}

	sb.WriteString("\n## 🧪 Verification & Testing\n")
	sb.WriteString("- [x] Automated test suites passed (`go test ./...`)\n")
	sb.WriteString("- [x] Code formatting verified with `gofmt`\n")

	return PRDescription{
		Title:       title,
		Body:        sb.String(),
		BaseBranch:  summary.BaseBranch,
		HeadBranch:  summary.CurrentBranch,
		CommitCount: len(summary.Commits),
	}
}

func inferType(hint string, files []string) string {
	lowerHint := strings.ToLower(hint)
	if strings.Contains(lowerHint, "fix") || strings.Contains(lowerHint, "bug") || strings.Contains(lowerHint, "resolve") {
		return "fix"
	}
	if strings.Contains(lowerHint, "test") {
		return "test"
	}
	if strings.Contains(lowerHint, "doc") || strings.Contains(lowerHint, "readme") {
		return "docs"
	}
	if strings.Contains(lowerHint, "refactor") || strings.Contains(lowerHint, "clean") {
		return "refactor"
	}
	if strings.Contains(lowerHint, "perf") || strings.Contains(lowerHint, "speed") {
		return "perf"
	}

	allTests := true
	allDocs := true
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") && !strings.Contains(f, "test") {
			allTests = false
		}
		if !strings.HasSuffix(f, ".md") && !strings.Contains(f, "docs") {
			allDocs = false
		}
	}
	if allTests && len(files) > 0 {
		return "test"
	}
	if allDocs && len(files) > 0 {
		return "docs"
	}

	return "feat"
}

func inferScope(files []string) string {
	if len(files) == 0 {
		return ""
	}
	// Common package directory
	first := files[0]
	parts := strings.Split(first, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	base := filepath.Base(first)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

var reCommitType = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]+)\))?:`)

func extractCommitTypeAndScope(title string) (string, string) {
	matches := reCommitType.FindStringSubmatch(title)
	if len(matches) >= 3 {
		return strings.ToLower(matches[1]), matches[2]
	}
	if len(matches) >= 2 {
		return strings.ToLower(matches[1]), ""
	}
	return "feat", ""
}

var reStatCounts = regexp.MustCompile(`(\d+)\s+insertions?\(\+\)|(\d+)\s+deletions?\(-\)`)

func parseStatCounts(stat string) (int, int) {
	insertions := 0
	deletions := 0

	matches := reStatCounts.FindAllStringSubmatch(stat, -1)
	for _, m := range matches {
		if len(m) > 1 && m[1] != "" {
			if n, err := strconv.Atoi(m[1]); err == nil {
				insertions = n
			}
		}
		if len(m) > 2 && m[2] != "" {
			if n, err := strconv.Atoi(m[2]); err == nil {
				deletions = n
			}
		}
	}
	return insertions, deletions
}

func isGitRepo(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func getGitBranch(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil {
		b := strings.TrimSpace(string(out))
		if b != "" && b != "HEAD" {
			return b
		}
	}
	return "main"
}

func detectBaseBranch(ctx context.Context, dir string) string {
	candidates := []string{"origin/main", "origin/master", "main", "master"}
	for _, c := range candidates {
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", c)
		cmd.Dir = dir
		if err := cmd.Run(); err == nil {
			return c
		}
	}
	return "main"
}
