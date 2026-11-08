package developer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PreviewStatus represents the confirmation and execution lifecycle of a previewed command.
type PreviewStatus string

const (
	PreviewStatusPending   PreviewStatus = "pending"
	PreviewStatusConfirmed PreviewStatus = "confirmed"
	PreviewStatusRejected  PreviewStatus = "rejected"
	PreviewStatusExecuted  PreviewStatus = "executed"
)

// DryRunPreview captures the projected impact, risk assessment, and simulation of a shell command.
type DryRunPreview struct {
	ID                   string         `json:"id"`
	Command              string         `json:"command"`
	WorkingDir           string         `json:"working_dir"`
	Risk                 RiskAssessment `json:"risk"`
	ProjectedImpact      string         `json:"projected_impact"`
	AffectedTargets      []string       `json:"affected_targets,omitempty"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	Status               PreviewStatus  `json:"status"`
	CreatedAt            time.Time      `json:"created_at"`
}

// ExecutionResult holds the output and exit metadata of an executed command.
type ExecutionResult struct {
	PreviewID  string        `json:"preview_id,omitempty"`
	Command    string        `json:"command"`
	WorkingDir string        `json:"working_dir"`
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	ExitCode   int           `json:"exit_code"`
	Duration   time.Duration `json:"duration"`
	ExecutedAt time.Time     `json:"executed_at"`
}

// PreviewEngine manages dry-run preview simulation and safe guarded execution.
type PreviewEngine struct {
	mu       sync.RWMutex
	previews map[string]DryRunPreview
}

// NewPreviewEngine creates a new dry-run preview and execution engine.
func NewPreviewEngine() *PreviewEngine {
	return &PreviewEngine{
		previews: make(map[string]DryRunPreview),
	}
}

// Preview generates a dry-run preview for a shell command without executing it.
func (e *PreviewEngine) Preview(ctx context.Context, cmdStr, workingDir string) (DryRunPreview, error) {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return DryRunPreview{}, errors.New("empty command cannot be previewed")
	}

	cleanDir := strings.TrimSpace(workingDir)
	if cleanDir == "" {
		if wd, err := os.Getwd(); err == nil {
			cleanDir = wd
		} else {
			cleanDir = "."
		}
	}
	cleanDir = filepath.Clean(cleanDir)

	risk := ClassifyCommand(trimmed)
	impact, targets := simulateImpact(ctx, trimmed, cleanDir, risk)

	id := generatePreviewID()
	preview := DryRunPreview{
		ID:                   id,
		Command:              trimmed,
		WorkingDir:           cleanDir,
		Risk:                 risk,
		ProjectedImpact:      impact,
		AffectedTargets:      targets,
		RequiresConfirmation: risk.RequiresConfirmation,
		Status:               PreviewStatusPending,
		CreatedAt:            time.Now().UTC(),
	}

	e.mu.Lock()
	e.previews[id] = preview
	e.mu.Unlock()

	return preview, nil
}

// GetPreview retrieves a previously generated dry-run preview by ID.
func (e *PreviewEngine) GetPreview(id string) (DryRunPreview, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.previews[id]
	return p, ok
}

// Execute performs safety checks and runs the command if allowed or confirmed.
func (e *PreviewEngine) Execute(ctx context.Context, cmdStr, workingDir string, confirm bool) (ExecutionResult, error) {
	preview, err := e.Preview(ctx, cmdStr, workingDir)
	if err != nil {
		return ExecutionResult{}, err
	}

	if preview.RequiresConfirmation && !confirm {
		e.mu.Lock()
		preview.Status = PreviewStatusRejected
		e.previews[preview.ID] = preview
		e.mu.Unlock()
		return ExecutionResult{
			PreviewID:  preview.ID,
			Command:    preview.Command,
			WorkingDir: preview.WorkingDir,
			ExecutedAt: time.Now().UTC(),
		}, fmt.Errorf("command execution rejected: '%s' is %s and requires explicit confirmation (pass --confirm or confirm interactively)", preview.Command, preview.Risk.Level)
	}

	return e.executeRaw(ctx, preview)
}

// ExecutePreview executes a previously generated preview by its ID.
func (e *PreviewEngine) ExecutePreview(ctx context.Context, previewID string, confirm bool) (ExecutionResult, error) {
	preview, ok := e.GetPreview(previewID)
	if !ok {
		return ExecutionResult{}, fmt.Errorf("preview '%s' not found", previewID)
	}

	if preview.RequiresConfirmation && !confirm {
		return ExecutionResult{
			PreviewID:  preview.ID,
			Command:    preview.Command,
			WorkingDir: preview.WorkingDir,
			ExecutedAt: time.Now().UTC(),
		}, fmt.Errorf("command execution rejected: preview '%s' is %s and requires confirmation", previewID, preview.Risk.Level)
	}

	return e.executeRaw(ctx, preview)
}

func (e *PreviewEngine) executeRaw(ctx context.Context, preview DryRunPreview) (ExecutionResult, error) {
	start := time.Now()

	// Execute inside shell
	cmd := exec.CommandContext(ctx, "sh", "-c", preview.Command)
	cmd.Dir = preview.WorkingDir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	res := ExecutionResult{
		PreviewID:  preview.ID,
		Command:    preview.Command,
		WorkingDir: preview.WorkingDir,
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		ExitCode:   exitCode,
		Duration:   duration,
		ExecutedAt: time.Now().UTC(),
	}

	e.mu.Lock()
	preview.Status = PreviewStatusExecuted
	e.previews[preview.ID] = preview
	e.mu.Unlock()

	if runErr != nil {
		return res, fmt.Errorf("command failed (exit code %d): %w", exitCode, runErr)
	}

	return res, nil
}

func simulateImpact(ctx context.Context, cmdStr, workingDir string, risk RiskAssessment) (string, []string) {
	tokens := tokenize(cmdStr)
	if len(tokens) == 0 {
		return "Empty command; no action taken", nil
	}

	baseBin := strings.ToLower(tokens[0])
	if (baseBin == "sudo" || baseBin == "doas" || baseBin == "env") && len(tokens) > 1 {
		baseBin = strings.ToLower(tokens[1])
		tokens = tokens[1:]
	}

	// 1. Safe commands
	if risk.Level == RiskSafe {
		return "Read-only inspection; will execute without altering files, git history, or system state", nil
	}

	// 2. Git command simulations
	if baseBin == "git" && len(tokens) > 1 {
		sub := strings.ToLower(tokens[1])
		switch sub {
		case "reset":
			if hasAnyToken(tokens[2:], "--hard") {
				dirtyFiles := getGitDirtyFiles(ctx, workingDir)
				if len(dirtyFiles) > 0 {
					return fmt.Sprintf("Hard reset: will permanently discard uncommitted changes in %d tracked file(s)", len(dirtyFiles)), dirtyFiles
				}
				return "Hard reset: working tree clean; will reset current branch HEAD to specified reference", nil
			}
			return "Soft/mixed reset: will update git HEAD while preserving working directory edits", nil

		case "clean":
			untracked := getGitUntrackedFiles(ctx, workingDir)
			if len(untracked) > 0 {
				return fmt.Sprintf("Clean: will permanently delete %d untracked file(s) and directories", len(untracked)), untracked
			}
			return "Clean: no untracked files detected in working directory", nil

		case "push":
			if hasAnyToken(tokens[2:], "--force", "-f", "--force-with-lease") {
				branch := getGitCurrentBranch(ctx, workingDir)
				return fmt.Sprintf("Force push: will overwrite remote commit history on branch '%s'", branch), []string{branch}
			}
			return "Push: will publish local commits to configured remote repository", nil

		case "branch":
			if hasAnyToken(tokens[2:], "-D", "-d") {
				var targets []string
				for _, tok := range tokens[2:] {
					if !strings.HasPrefix(tok, "-") {
						targets = append(targets, tok)
					}
				}
				return fmt.Sprintf("Delete branch: will remove local branch(es): %s", strings.Join(targets, ", ")), targets
			}
			return "Branch: will create or update git branches", nil

		case "commit":
			return "Commit: will record staged modifications into a new git commit", nil

		case "checkout", "switch":
			var targets []string
			for _, tok := range tokens[2:] {
				if !strings.HasPrefix(tok, "-") && tok != "--" {
					targets = append(targets, tok)
				}
			}
			if len(targets) > 0 {
				return fmt.Sprintf("Switch: will change active git working branch or files to '%s'", targets[0]), targets
			}
			return "Checkout/switch: will update active branch or restore working files", nil

		case "add":
			return "Add: will stage file modifications into the git index", nil
		}
	}

	// 3. Filesystem deletions
	if baseBin == "rm" {
		var targets []string
		for _, tok := range tokens[1:] {
			if !strings.HasPrefix(tok, "-") {
				targets = append(targets, tok)
			}
		}
		if len(targets) > 0 {
			return fmt.Sprintf("Remove: will permanently delete %d target file(s) or directorie(s) from disk", len(targets)), targets
		}
		return "Remove: will delete specified targets permanently from filesystem", nil
	}

	if baseBin == "mv" {
		var targets []string
		for _, tok := range tokens[1:] {
			if !strings.HasPrefix(tok, "-") {
				targets = append(targets, tok)
			}
		}
		if len(targets) >= 2 {
			return fmt.Sprintf("Move: will relocate '%s' to '%s'", targets[0], targets[1]), targets
		}
		return "Move: will move or rename files", targets
	}

	if baseBin == "cp" {
		return "Copy: will copy file contents, potentially overwriting destination files", nil
	}

	if baseBin == "mkdir" {
		return "Make directory: will create new folder structures on the filesystem", nil
	}

	// 4. Process management
	if baseBin == "kill" || baseBin == "killall" || baseBin == "pkill" {
		if hasAnyToken(tokens[1:], "-9", "-SIGKILL", "-KILL") || baseBin != "kill" {
			return "Process termination: will forcefully kill target processes without cleanup", nil
		}
		return "Process signal: will request graceful process termination", nil
	}

	// 5. Package Managers
	switch baseBin {
	case "npm", "pnpm", "yarn", "bun", "pip", "cargo", "go":
		return fmt.Sprintf("Package manager: will download, install, or update dependencies in '%s'", filepath.Base(workingDir)), nil
	}

	return fmt.Sprintf("%s: %s", strings.ToUpper(string(risk.Level)), risk.Reason), nil
}

func getGitDirtyFiles(ctx context.Context, workingDir string) []string {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if len(trimmed) > 3 && !strings.HasPrefix(trimmed, "??") {
			files = append(files, strings.TrimSpace(trimmed[2:]))
		}
	}
	return files
}

func getGitUntrackedFiles(ctx context.Context, workingDir string) []string {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "??") {
			files = append(files, strings.TrimSpace(trimmed[2:]))
		}
	}
	return files
}

func getGitCurrentBranch(ctx context.Context, workingDir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" && branch != "HEAD" {
			return branch
		}
	}
	return "HEAD"
}

func generatePreviewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("prev-%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}
