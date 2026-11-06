package appcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TerminalContext captures real-time context from a developer's terminal/shell session.
type TerminalContext struct {
	TerminalApp    string    `json:"terminal_app"`              // e.g. "Terminal", "iTerm2", "Alacritty", "Kitty", "Ghostty", "Warp"
	Shell          string    `json:"shell"`                     // e.g. "zsh", "bash", "fish", "sh"
	PID            int       `json:"pid,omitempty"`             // shell process ID
	WorkingDir     string    `json:"working_dir"`               // current working directory ($PWD)
	GitBranch      string    `json:"git_branch,omitempty"`      // active git branch name if within a repo
	LastCommand    string    `json:"last_command,omitempty"`    // last command executed
	LastExitCode   int       `json:"last_exit_code"`            // exit code of last command (0 = success)
	RecentCommands []string  `json:"recent_commands,omitempty"` // rolling buffer of recent shell commands
	Active         bool      `json:"active"`                    // whether terminal session is actively tracked
	UpdatedAt      time.Time `json:"updated_at"`
}

// TerminalContextManager provides thread-safe access to terminal and shell state.
type TerminalContextManager struct {
	mu      sync.RWMutex
	current TerminalContext
	ttl     time.Duration
}

// NewTerminalContextManager creates a TerminalContextManager with default 30-minute freshness TTL.
func NewTerminalContextManager() *TerminalContextManager {
	return &TerminalContextManager{
		ttl: 30 * time.Minute,
	}
}

// Set stores updated terminal context, automatically inferring git branch if not specified.
func (m *TerminalContextManager) Set(ctx TerminalContext) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx.WorkingDir != "" && ctx.GitBranch == "" {
		ctx.GitBranch = DetectGitBranch(ctx.WorkingDir)
	}
	if ctx.TerminalApp == "" {
		ctx.TerminalApp = "Terminal"
	}
	if ctx.Shell == "" {
		ctx.Shell = "zsh"
	}
	if ctx.UpdatedAt.IsZero() {
		ctx.UpdatedAt = time.Now()
	}

	// Maintain recent commands history
	if ctx.LastCommand != "" {
		existing := m.current.RecentCommands
		if len(existing) == 0 || existing[len(existing)-1] != ctx.LastCommand {
			ctx.RecentCommands = append(existing, ctx.LastCommand)
			if len(ctx.RecentCommands) > 20 {
				ctx.RecentCommands = ctx.RecentCommands[len(ctx.RecentCommands)-20:]
			}
		} else if len(ctx.RecentCommands) == 0 {
			ctx.RecentCommands = existing
		}
	} else if len(ctx.RecentCommands) == 0 {
		ctx.RecentCommands = m.current.RecentCommands
	}

	ctx.Active = true
	m.current = ctx
}

// RecordCommand records a newly executed command and its exit status.
func (m *TerminalContextManager) RecordCommand(cmd string, exitCode int, workingDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd = strings.TrimSpace(cmd)
	if cmd != "" {
		m.current.RecentCommands = append(m.current.RecentCommands, cmd)
		if len(m.current.RecentCommands) > 20 {
			m.current.RecentCommands = m.current.RecentCommands[len(m.current.RecentCommands)-20:]
		}
	}

	m.current.LastCommand = cmd
	m.current.LastExitCode = exitCode
	if workingDir != "" {
		m.current.WorkingDir = workingDir
		m.current.GitBranch = DetectGitBranch(workingDir)
	}
	m.current.UpdatedAt = time.Now()
	m.current.Active = true
}

// Get returns the current terminal context if valid and within TTL.
func (m *TerminalContextManager) Get() (TerminalContext, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.current.Active {
		return TerminalContext{}, false
	}
	if m.ttl > 0 && time.Since(m.current.UpdatedAt) > m.ttl {
		return TerminalContext{}, false
	}
	return m.current, true
}

// Clear resets the current terminal context.
func (m *TerminalContextManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.current = TerminalContext{}
}

// IsActive returns whether a valid, non-expired terminal context exists.
func (m *TerminalContextManager) IsActive() bool {
	_, ok := m.Get()
	return ok
}

// FormatContextSummary returns a concise one-line summary of terminal context.
func (m *TerminalContextManager) FormatContextSummary() string {
	ctx, ok := m.Get()
	if !ok {
		return "No active terminal context"
	}

	var parts []string
	if ctx.TerminalApp != "" {
		parts = append(parts, fmt.Sprintf("Terminal: %s", ctx.TerminalApp))
	}
	if ctx.Shell != "" {
		parts = append(parts, fmt.Sprintf("Shell: %s", ctx.Shell))
	}
	if ctx.WorkingDir != "" {
		dir := ctx.WorkingDir
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(dir, home) {
			dir = "~" + strings.TrimPrefix(dir, home)
		}
		if ctx.GitBranch != "" {
			parts = append(parts, fmt.Sprintf("pwd: %s (%s)", dir, ctx.GitBranch))
		} else {
			parts = append(parts, fmt.Sprintf("pwd: %s", dir))
		}
	}
	if ctx.LastCommand != "" {
		parts = append(parts, fmt.Sprintf("Last: %s [%d]", ctx.LastCommand, ctx.LastExitCode))
	}

	return strings.Join(parts, " | ")
}

// FormatPromptContext formats terminal context into an LLM prompt block.
func (m *TerminalContextManager) FormatPromptContext() string {
	ctx, ok := m.Get()
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Terminal & Shell Context]\n")
	if ctx.TerminalApp != "" {
		sb.WriteString(fmt.Sprintf("Terminal: %s\n", ctx.TerminalApp))
	}
	if ctx.Shell != "" {
		if ctx.PID > 0 {
			sb.WriteString(fmt.Sprintf("Shell: %s (PID: %d)\n", ctx.Shell, ctx.PID))
		} else {
			sb.WriteString(fmt.Sprintf("Shell: %s\n", ctx.Shell))
		}
	}
	if ctx.WorkingDir != "" {
		sb.WriteString(fmt.Sprintf("Working Directory: %s\n", ctx.WorkingDir))
	}
	if ctx.GitBranch != "" {
		sb.WriteString(fmt.Sprintf("Git Branch: %s\n", ctx.GitBranch))
	}
	if ctx.LastCommand != "" {
		sb.WriteString(fmt.Sprintf("Last Executed Command: %s (Exit Code: %d)\n", ctx.LastCommand, ctx.LastExitCode))
	}
	if len(ctx.RecentCommands) > 0 {
		sb.WriteString("Recent Commands:\n")
		start := 0
		if len(ctx.RecentCommands) > 5 {
			start = len(ctx.RecentCommands) - 5
		}
		for _, cmd := range ctx.RecentCommands[start:] {
			sb.WriteString(fmt.Sprintf("  - %s\n", cmd))
		}
	}
	return sb.String()
}

// DetectGitBranch performs a pure Go, zero-latency filesystem inspection to resolve current git branch.
func DetectGitBranch(dir string) string {
	curr := filepath.Clean(dir)
	for {
		gitPath := filepath.Join(curr, ".git")
		fi, err := os.Stat(gitPath)
		if err == nil {
			if fi.IsDir() {
				// Standard git directory
				headBytes, hErr := os.ReadFile(filepath.Join(gitPath, "HEAD"))
				if hErr == nil {
					return parseGitHead(string(headBytes))
				}
			} else {
				// Worktree or submodule (.git file pointing to gitdir)
				content, rErr := os.ReadFile(gitPath)
				if rErr == nil {
					line := strings.TrimSpace(string(content))
					if strings.HasPrefix(line, "gitdir:") {
						targetDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
						if !filepath.IsAbs(targetDir) {
							targetDir = filepath.Join(curr, targetDir)
						}
						headBytes, hErr := os.ReadFile(filepath.Join(targetDir, "HEAD"))
						if hErr == nil {
							return parseGitHead(string(headBytes))
						}
					}
				}
			}
			return ""
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

func parseGitHead(headContent string) string {
	trimmed := strings.TrimSpace(headContent)
	if strings.HasPrefix(trimmed, "ref: refs/heads/") {
		return strings.TrimPrefix(trimmed, "ref: refs/heads/")
	}
	if len(trimmed) >= 7 {
		// Detached HEAD, return short SHA
		return trimmed[:7]
	}
	return trimmed
}
