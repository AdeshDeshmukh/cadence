package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// CommandLauncher allows customizing or mocking command execution in tests.
type CommandLauncher func(ctx context.Context, dir string, env []string, command string, args ...string) (*exec.Cmd, error)

// DefaultCommandLauncher is the standard production process launcher.
func DefaultCommandLauncher(ctx context.Context, dir string, env []string, command string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}

// SandboxConfig specifies the runtime boundaries and arguments for running a plugin.
type SandboxConfig struct {
	PluginID    string
	InstallDir  string
	Entrypoint  string
	Permissions Permissions
	ExtraEnv    []string
	Launcher    CommandLauncher
}

// SandboxProcess represents a monitored plugin subprocess running in an isolated environment.
type SandboxProcess struct {
	cmd        *exec.Cmd
	stdinPipe  io.WriteCloser
	stdoutPipe io.ReadCloser
	stderrBuf  *bytes.Buffer
	pid        int
	exited     chan struct{}
	exitErr    error
	cancel     context.CancelFunc
}

// BuildSanitizedEnv builds a restricted environment containing only required safe variables.
func BuildSanitizedEnv(id string, installDir string, permissions Permissions, extra []string) []string {
	env := []string{
		fmt.Sprintf("CADENCE_PLUGIN_ID=%s", id),
		fmt.Sprintf("CADENCE_PLUGIN_DIR=%s", installDir),
		"CADENCE_VERSION=1.0.0",
	}

	// Forward standard system PATH
	pathVal := os.Getenv("PATH")
	if pathVal == "" {
		pathVal = "/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin"
	}
	env = append(env, "PATH="+pathVal)

	// Forward safe standard variables
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		env = append(env, "TMPDIR="+tmp)
	} else {
		env = append(env, "TMPDIR="+os.TempDir())
	}
	if user := os.Getenv("USER"); user != "" {
		env = append(env, "USER="+user)
	}
	if lang := os.Getenv("LANG"); lang != "" {
		env = append(env, "LANG="+lang)
	}

	for _, e := range extra {
		if strings.TrimSpace(e) != "" {
			env = append(env, e)
		}
	}

	return env
}

// ResolveEntrypoint returns the command and arguments needed to execute the entrypoint.
func ResolveEntrypoint(installDir, entrypoint string) (string, []string, error) {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		return "", nil, errors.New("empty entrypoint")
	}

	parts := strings.Fields(entrypoint)
	if len(parts) == 0 {
		return "", nil, errors.New("invalid entrypoint")
	}

	target := parts[0]
	var cmdArgs []string
	if len(parts) > 1 {
		cmdArgs = parts[1:]
	}

	// If relative path, join with installDir
	var fullPath string
	if filepath.IsAbs(target) {
		fullPath = target
	} else {
		fullPath = filepath.Clean(filepath.Join(installDir, target))
	}

	// If file exists on disk, check permissions and shebang/extension
	if info, err := os.Stat(fullPath); err == nil {
		if !info.IsDir() {
			// Ensure executable bit on Unix
			if runtime.GOOS != "windows" {
				if info.Mode()&0111 == 0 {
					_ = os.Chmod(fullPath, info.Mode()|0111)
				}
			}

			// If file is a python or node script without executable bits or standard invocation
			ext := strings.ToLower(filepath.Ext(fullPath))
			if ext == ".py" && !isDirectExecutable(fullPath) {
				return "python3", append([]string{fullPath}, cmdArgs...), nil
			}
			if (ext == ".js" || ext == ".mjs") && !isDirectExecutable(fullPath) {
				return "node", append([]string{fullPath}, cmdArgs...), nil
			}
			if ext == ".sh" && !isDirectExecutable(fullPath) {
				return "bash", append([]string{fullPath}, cmdArgs...), nil
			}

			return fullPath, cmdArgs, nil
		}
	}

	// If not directly a file in installDir, it may be a binary on PATH
	return target, cmdArgs, nil
}

func isDirectExecutable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 2)
	n, _ := f.Read(header)
	if n >= 2 && header[0] == '#' && header[1] == '!' {
		return true // Has shebang
	}
	return false
}

// SpawnSandbox initializes and starts the sandboxed process.
func SpawnSandbox(ctx context.Context, cfg SandboxConfig) (*SandboxProcess, io.ReadCloser, io.WriteCloser, error) {
	if cfg.InstallDir == "" {
		return nil, nil, nil, errors.New("install directory must be specified")
	}
	if cfg.Entrypoint == "" {
		return nil, nil, nil, errors.New("plugin entrypoint must be specified")
	}

	bin, args, err := ResolveEntrypoint(cfg.InstallDir, cfg.Entrypoint)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve entrypoint %q: %w", cfg.Entrypoint, err)
	}

	env := BuildSanitizedEnv(cfg.PluginID, cfg.InstallDir, cfg.Permissions, cfg.ExtraEnv)

	launcher := cfg.Launcher
	if launcher == nil {
		launcher = DefaultCommandLauncher
	}

	procCtx, cancel := context.WithCancel(context.Background())
	cmd, err := launcher(procCtx, cfg.InstallDir, env, bin, args...)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("create process: %w", err)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		cancel()
		return nil, nil, nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		cancel()
		return nil, nil, nil, fmt.Errorf("start plugin process: %w", err)
	}

	sp := &SandboxProcess{
		cmd:        cmd,
		stdinPipe:  stdinPipe,
		stdoutPipe: stdoutPipe,
		stderrBuf:  stderrBuf,
		pid:        cmd.Process.Pid,
		exited:     make(chan struct{}),
		cancel:     cancel,
	}

	go func() {
		defer close(sp.exited)
		err := cmd.Wait()
		sp.exitErr = err
	}()

	return sp, stdoutPipe, stdinPipe, nil
}

// PID returns the OS process ID of the sandboxed plugin.
func (sp *SandboxProcess) PID() int {
	return sp.pid
}

// Stderr returns any captured standard error output.
func (sp *SandboxProcess) Stderr() string {
	if sp.stderrBuf == nil {
		return ""
	}
	return sp.stderrBuf.String()
}

// Exited returns a channel that is closed when the process terminates.
func (sp *SandboxProcess) Exited() <-chan struct{} {
	return sp.exited
}

// ExitError returns the process exit error, if any, after it terminates.
func (sp *SandboxProcess) ExitError() error {
	return sp.exitErr
}

// Stop terminates the process gracefully, falling back to SIGKILL if necessary.
func (sp *SandboxProcess) Stop(timeout time.Duration) error {
	if sp == nil || sp.cmd == nil || sp.cmd.Process == nil {
		return nil
	}

	select {
	case <-sp.exited:
		// Already exited
		_ = sp.stdinPipe.Close()
		_ = sp.stdoutPipe.Close()
		sp.cancel()
		return nil
	default:
	}

	// 1. Try graceful termination
	if runtime.GOOS == "windows" {
		_ = sp.cmd.Process.Signal(os.Interrupt)
	} else {
		_ = sp.cmd.Process.Signal(syscall.SIGTERM)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-sp.exited:
		_ = sp.stdinPipe.Close()
		_ = sp.stdoutPipe.Close()
		sp.cancel()
		return nil
	case <-timer.C:
		// 2. Timeout reached, force kill
		_ = sp.cmd.Process.Kill()
		sp.cancel()
		_ = sp.stdinPipe.Close()
		_ = sp.stdoutPipe.Close()
		<-sp.exited
		return errors.New("process killed after termination timeout")
	}
}
