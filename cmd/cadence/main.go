package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/agent"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/developer"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/selection"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/internal/system"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

func usage(out io.Writer) {
	fmt.Fprintf(out, `cadence - Local AI-first voice dictation CLI

Usage:
  cadence <command> [arguments]

Commands:
  status             Show daemon runtime status and active recording state
  dictate [start|stop] Start or stop a live dictation session
  history [limit]    List recent saved dictation transcripts
  recover [flags]    Retrieve text from last cancelled or failed session
  vocab <subcommand> Manage custom vocabulary (list, add, remove, import, export)
  snippet <subcommand> Manage voice templates with dynamic vars (list, add, remove, test)
  profile <subcommand> Manage prompt profiles (list, show, set, delete)
  route <subcommand>   Manage active app-to-profile routing rules (list, set, delete, test)
  polish <text>      Format and polish text using configured LLM (Ollama/OpenAI)
  undo [--raw]       Revert last AI polish transformation to raw post-processed text
  correct <text>     Detect and fix ASR acoustic and homophone errors using context
  selection [flags]  Capture highlighted text from active application (Edit Mode)
  parse <text>       Parse voice edit instruction into intent and parameters
  edit <instruction> Execute voice edit on highlighted text or custom input
  cmd <instruction>  Generate shell/CLI command from voice instruction (e.g. "run tests", "git commit...")
  exec <command>     Safely execute shell command with dry-run preview and confirmation guard
  risk <command>     Audit and classify shell command risk level (Safe, Risky, Destructive)
  git <subcommand>   Git AI integration: commit message and PR description generation (commit, pr)
  code [flags] <words> Format code identifiers (snake, camel, pascal, kebab) and docstrings
  sys <subcommand>   System command mode: app launch, window control, screenshots (sys, app, window, screenshot, volume)
  mode [name]        Show or set active operating mode (dictation, code, command, agent)
  control <phrase>   Execute voice self-control command (e.g. "switch to code mode", "start recording")
  agent [flags] <goal> Autonomous agent loop: plan, tool call, observe, report
  tools [list|schema] Inspect registered agent tools and function schemas
  tool <name> [k=v...] Invoke a specific agent tool directly (e.g. "tool fs action=list path=.")
  permissions [list|grant|revoke|check] Manage per-tool permission policies (allow, deny, ask)
  policy [show|set|evaluate] Manage action confirmation policies (always_ask, auto_approve_safe, etc.)
  meeting [subcommand] Continuous meeting recording and capture (start, stop, pause, resume, status, list, show, delete)
  context [show|set|clear] Show or manage developer/code editor context (file, language, selection)
  terminal [status|hook] Inspect terminal/shell context or generate shell integration hooks
  app                Show detected active foreground application and category
  config [show|path] Inspect configuration and system file paths
  doctor             Perform diagnostics on system, daemon, and storage
  version            Print the version of cadence

Flags:
  -h, --help         Show this help message
  -v, --version      Show cadence version

Recover Flags:
  -p, --paste        Paste recovered text into the active application
  -c, --copy         Copy recovered text to system clipboard
  -l, --list [n]     List recent recoverable sessions (default 10)
`)
}

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return 0
	}

	cfg := config.Default()
	client := ipc.Client{
		Network: "unix",
		Address: cfg.IPC.SocketPath,
	}

	ctx := context.Background()

	switch args[0] {
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "cadence version %s\n", version.Version)
		return 0

	case "help", "-h", "--help":
		usage(stdout)
		return 0

	case "status":
		if err := cmdStatus(ctx, client, stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "dictate":
		if err := cmdDictate(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "history":
		if err := cmdHistory(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "recover":
		if err := cmdRecover(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "vocab", "vocabulary":
		if err := cmdVocab(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "snippet", "snippets", "snip":
		if err := cmdSnippet(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "profile", "profiles":
		if err := cmdProfile(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "route", "routes":
		if err := cmdRoute(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "polish":
		if err := cmdPolish(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "app", "active-app":
		if err := cmdApp(ctx, client, stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "context", "ctx":
		if err := cmdContext(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "terminal", "term":
		if err := cmdTerminal(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "cmd", "dev", "command":
		if err := cmdGenerate(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "exec", "execute", "run":
		if err := cmdExec(ctx, client, args[1:], os.Stdin, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "risk", "classify":
		if err := cmdRisk(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "git":
		if err := cmdGit(ctx, client, args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "code", "casing":
		if err := cmdCode(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "sys", "system":
		if err := cmdSys(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "mode":
		if err := cmdMode(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "control", "self":
		if err := cmdControl(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "agent":
		if err := cmdAgent(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "tools":
		if err := cmdTools(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "tool":
		if err := cmdTool(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "permissions", "permission", "perms":
		if err := cmdPermissions(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "policy", "action-policy":
		if err := cmdPolicy(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "meeting":
		if err := cmdMeeting(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "undo":
		if err := cmdUndo(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "correct":
		if err := cmdCorrect(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "selection", "select":
		if err := cmdSelection(ctx, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "parse":
		if err := cmdParse(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "edit":
		if err := cmdEdit(ctx, cfg, client, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "config":
		if err := cmdConfig(cfg, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	case "doctor":
		if err := cmdDoctor(ctx, cfg, client, stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(stderr, "Unknown command: %s\n\n", args[0])
		usage(stderr)
		return 1
	}
}

func cmdStatus(ctx context.Context, client ipc.Client, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	res, err := client.Call(ctx, "status", nil)
	if err != nil {
		return fmt.Errorf("cadenced daemon is not running (socket: %s)\nStart the daemon with: cadenced", client.Address)
	}

	data, ok := res.(map[string]any)
	if !ok {
		return errors.New("unexpected response from cadenced")
	}

	fmt.Fprintln(out, "Cadence Daemon Status:")
	fmt.Fprintf(out, "  Status:    %v\n", data["status"])
	fmt.Fprintf(out, "  Version:   %v\n", data["version"])
	fmt.Fprintf(out, "  Recording: %v\n", data["recording"])
	if sess, ok := data["session"].(string); ok && sess != "" {
		fmt.Fprintf(out, "  Session:   %s\n", sess)
	}
	fmt.Fprintf(out, "  Shortcut:  %v\n", data["shortcut"])
	fmt.Fprintf(out, "  Mode:      %v\n", data["mode"])
	if enabled, ok := data["polish_enabled"].(bool); ok && enabled {
		backend := data["polish_backend"]
		model := data["polish_model"]
		fmt.Fprintf(out, "  Polish:    enabled (%v / %v)\n", backend, model)
	}
	if appName, ok := data["active_app"].(string); ok && appName != "" {
		fmt.Fprintf(out, "  Active App: %s\n", appName)
	}
	return nil
}

func cmdDictate(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "start"
	if len(args) > 0 {
		action = args[0]
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	switch action {
	case "start":
		if _, err := client.Call(ctx, "dictate_start", nil); err != nil {
			return fmt.Errorf("failed to start dictation: %w", err)
		}
		fmt.Fprintln(out, "Dictation started.")
	case "stop":
		if _, err := client.Call(ctx, "dictate_stop", nil); err != nil {
			return fmt.Errorf("failed to stop dictation: %w", err)
		}
		fmt.Fprintln(out, "Dictation stopped.")
	default:
		return fmt.Errorf("unknown action: %s (valid actions: start, stop)", action)
	}
	return nil
}

func cmdHistory(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	limit := 10
	if len(args) > 0 {
		if val, err := strconv.Atoi(args[0]); err == nil && val > 0 {
			limit = val
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := client.Call(ctx, "history", map[string]any{"limit": float64(limit)})
	if err != nil {
		return fmt.Errorf("failed to retrieve history from daemon: %w", err)
	}

	raw, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("encode history response: %w", err)
	}

	var transcripts []storage.Transcript
	if err := json.Unmarshal(raw, &transcripts); err != nil {
		return fmt.Errorf("decode history response: %w", err)
	}

	if len(transcripts) == 0 {
		fmt.Fprintln(out, "No transcripts found.")
		return nil
	}

	fmt.Fprintf(out, "Recent Transcripts (%d):\n", len(transcripts))
	for i, t := range transcripts {
		timeStr := t.CreatedAt.Local().Format("2006-01-02 15:04:05")
		appStr := ""
		if t.Application != "" {
			appStr = fmt.Sprintf(" [%s]", t.Application)
		}
		fmt.Fprintf(out, "%2d. %s%s: %s\n", i+1, timeStr, appStr, t.Text)
	}
	return nil
}

func cmdConfig(cfg config.Config, args []string, out io.Writer) error {
	action := "show"
	if len(args) > 0 {
		action = args[0]
	}

	switch action {
	case "path":
		fmt.Fprintln(out, "Configuration Paths:")
		fmt.Fprintf(out, "  Database: %s\n", cfg.Storage.DBPath)
		fmt.Fprintf(out, "  Socket:   %s\n", cfg.IPC.SocketPath)
	case "show":
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
	default:
		return fmt.Errorf("unknown config option: %s (usage: cadence config [show|path])", action)
	}
	return nil
}

func cmdDoctor(ctx context.Context, cfg config.Config, client ipc.Client, out io.Writer) error {
	fmt.Fprintln(out, "Cadence Diagnostics:")
	fmt.Fprintf(out, "  • Version:        %s\n", version.Version)
	fmt.Fprintf(out, "  • Database path:  %s\n", cfg.Storage.DBPath)
	fmt.Fprintf(out, "  • Socket path:    %s\n", cfg.IPC.SocketPath)

	callCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	res, err := client.Call(callCtx, "ping", nil)
	if err != nil {
		fmt.Fprintln(out, "  • Daemon status:  not running")
	} else if res == "pong" {
		fmt.Fprintln(out, "  • Daemon status:  running and healthy")
	} else {
		fmt.Fprintf(out, "  • Daemon status:  responded with %v\n", res)
	}

	return nil
}

func cmdRecover(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	var shouldPaste bool
	var shouldCopy bool
	var shouldList bool
	limit := 10

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-p", "--paste":
			shouldPaste = true
		case "-c", "--copy":
			shouldCopy = true
		case "-l", "--list":
			shouldList = true
			if i+1 < len(args) {
				if val, err := strconv.Atoi(args[i+1]); err == nil && val > 0 {
					limit = val
					i++
				}
			}
		default:
			if val, err := strconv.Atoi(arg); err == nil && val > 0 {
				limit = val
			}
		}
	}

	if shouldList {
		return cmdRecoverList(ctx, cfg, client, limit, out)
	}

	return cmdRecoverLast(ctx, cfg, client, shouldPaste, shouldCopy, out)
}

func cmdRecoverList(ctx context.Context, cfg config.Config, client ipc.Client, limit int, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var recs []storage.SessionRecovery

	res, err := client.Call(callCtx, "recoveries", map[string]any{"limit": float64(limit)})
	if err == nil {
		raw, err := json.Marshal(res)
		if err == nil {
			_ = json.Unmarshal(raw, &recs)
		}
	} else {
		// Fallback to SQLite direct read if daemon is offline
		s, err := storage.Open(cfg.Storage.DBPath)
		if err != nil {
			return fmt.Errorf("failed to reach daemon (%v) and failed to open database: %w", err, err)
		}
		defer s.Close()
		recs, _ = s.ListRecoveries(limit)
	}

	if len(recs) == 0 {
		fmt.Fprintln(out, "No recoverable sessions found.")
		return nil
	}

	fmt.Fprintf(out, "Recent Recoverable Sessions (%d):\n", len(recs))
	for i, r := range recs {
		timeStr := r.UpdatedAt.Local().Format("2006-01-02 15:04:05")
		text := r.BestText()
		if len(text) > 60 {
			text = text[:57] + "..."
		}
		errStr := ""
		if r.Error != "" {
			errStr = fmt.Sprintf(" (err: %s)", r.Error)
		}
		fmt.Fprintf(out, "%2d. [%s] (%s) %s: %q%s\n", i+1, timeStr, r.State, r.ID, text, errStr)
	}
	return nil
}

func cmdRecoverLast(ctx context.Context, cfg config.Config, client ipc.Client, shouldPaste, shouldCopy bool, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var rec *storage.SessionRecovery

	res, err := client.Call(callCtx, "recover", map[string]any{"paste": shouldPaste})
	if err == nil && res != nil {
		raw, err := json.Marshal(res)
		if err == nil {
			var r storage.SessionRecovery
			if err := json.Unmarshal(raw, &r); err == nil && r.ID != "" {
				rec = &r
			}
		}
	}

	if rec == nil {
		// Fallback to SQLite direct read if daemon is offline or returned nil
		s, err := storage.Open(cfg.Storage.DBPath)
		if err == nil {
			defer s.Close()
			rec, _ = s.GetLastRecovery()
		}
	}

	if rec == nil || rec.BestText() == "" {
		fmt.Fprintln(out, "No recoverable sessions found.")
		return nil
	}

	bestText := rec.BestText()
	elapsed := time.Since(rec.UpdatedAt).Round(time.Second)
	if elapsed < 0 {
		elapsed = 0
	}
	timeStr := rec.UpdatedAt.Local().Format("2006-01-02 15:04:05")

	fmt.Fprintln(out, "Recovered Session:")
	fmt.Fprintf(out, "  ID:       %s\n", rec.ID)
	fmt.Fprintf(out, "  State:    %s\n", rec.State)
	fmt.Fprintf(out, "  Time:     %s (%s ago)\n", timeStr, elapsed)
	if rec.Error != "" {
		fmt.Fprintf(out, "  Error:    %s\n", rec.Error)
	}
	fmt.Fprintf(out, "  Text:     %s\n", bestText)

	if shouldPaste {
		paster := paste.NewDefaultCascade()
		if _, err := paster.Paste(bestText); err != nil {
			fmt.Fprintf(out, "  Action:   Paste failed (%v)\n", err)
		} else {
			fmt.Fprintln(out, "  Action:   Pasted to active application")
		}
	}

	if shouldCopy {
		if err := paste.SetSystemClipboard(bestText); err != nil {
			fmt.Fprintf(out, "  Action:   Copy failed (%v)\n", err)
		} else {
			fmt.Fprintln(out, "  Action:   Copied to system clipboard")
		}
	}

	return nil
}

func cmdVocab(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	action := "list"
	var subArgs []string
	if len(args) > 0 {
		action = args[0]
		subArgs = args[1:]
	}

	switch action {
	case "list":
		return cmdVocabList(ctx, cfg, client, out)
	case "add":
		return cmdVocabAdd(ctx, cfg, client, subArgs, out)
	case "remove", "delete", "rm":
		return cmdVocabRemove(ctx, cfg, client, subArgs, out)
	case "import":
		return cmdVocabImport(ctx, cfg, client, subArgs, out)
	case "export":
		return cmdVocabExport(ctx, cfg, client, subArgs, out)
	default:
		return fmt.Errorf("unknown vocab subcommand: %s (usage: cadence vocab [list|add|remove|import|export])", action)
	}
}

func cmdVocabList(ctx context.Context, cfg config.Config, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var entries []storage.VocabularyEntry
	res, err := client.Call(callCtx, "vocab_list", nil)
	if err == nil {
		raw, err := json.Marshal(res)
		if err == nil {
			_ = json.Unmarshal(raw, &entries)
		}
	} else {
		// Offline storage fallback
		s, err := storage.Open(cfg.Storage.DBPath)
		if err != nil {
			return fmt.Errorf("failed to reach daemon (%v) and cannot open database: %w", err, err)
		}
		defer s.Close()
		entries, _ = s.Vocabulary()
	}

	if len(entries) == 0 {
		fmt.Fprintln(out, "No custom vocabulary entries found.")
		return nil
	}

	fmt.Fprintf(out, "Custom Vocabulary (%d entries):\n", len(entries))
	for i, e := range entries {
		fmt.Fprintf(out, "%2d. %-20s -> %s\n", i+1, e.Spoken, e.Canonical)
	}
	return nil
}

func cmdVocabAdd(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: cadence vocab add <spoken phrase> <canonical text>")
	}
	spoken := args[0]
	canonical := args[1]

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "vocab_add", map[string]any{
		"spoken":    spoken,
		"canonical": canonical,
	})
	if err != nil {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to update vocabulary via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		if err := s.SetVocabulary(storage.VocabularyEntry{
			Spoken:    spoken,
			Canonical: canonical,
		}); err != nil {
			return fmt.Errorf("set vocabulary: %w", err)
		}
	}

	fmt.Fprintf(out, "Added vocabulary: %q -> %q\n", spoken, canonical)
	return nil
}

func cmdVocabRemove(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: cadence vocab remove <spoken phrase>")
	}
	spoken := args[0]

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "vocab_remove", map[string]any{
		"spoken": spoken,
	})
	if err != nil {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to remove vocabulary via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		if err := s.DeleteVocabulary(spoken); err != nil {
			return fmt.Errorf("delete vocabulary: %w", err)
		}
	}

	fmt.Fprintf(out, "Removed vocabulary: %q\n", spoken)
	return nil
}

func cmdVocabImport(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: cadence vocab import <file> [--format csv|json|txt]")
	}
	filePath := args[0]
	format := "auto"
	for i := 1; i < len(args); i++ {
		if (args[i] == "--format" || args[i] == "-f") && i+1 < len(args) {
			format = args[i+1]
			i++
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file %q: %w", filePath, err)
	}

	if format == "auto" {
		format = storage.DetectFormat(filePath, data)
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	count := 0
	res, err := client.Call(callCtx, "vocab_import", map[string]any{
		"content": string(data),
		"format":  format,
	})
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			if cnt, ok := m["count"].(float64); ok {
				count = int(cnt)
			}
		}
	} else {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to import via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		cnt, err := storage.ImportVocabulary(s, strings.NewReader(string(data)), format)
		if err != nil {
			return fmt.Errorf("import vocabulary: %w", err)
		}
		count = cnt
	}

	fmt.Fprintf(out, "Successfully imported %d vocabulary entries from %s (%s format).\n", count, filePath, format)
	return nil
}

func cmdVocabExport(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	var targetFile string
	format := "json"

	for i := 0; i < len(args); i++ {
		if (args[i] == "--format" || args[i] == "-f") && i+1 < len(args) {
			format = args[i+1]
			i++
		} else if !strings.HasPrefix(args[i], "-") && targetFile == "" {
			targetFile = args[i]
		}
	}

	if targetFile != "" {
		detected := storage.DetectFormat(targetFile, nil)
		if detected != "" {
			format = detected
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var content string
	res, err := client.Call(callCtx, "vocab_export", map[string]any{"format": format})
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			if cnt, ok := m["content"].(string); ok {
				content = cnt
			}
		}
	} else {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to export via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		var buf bytes.Buffer
		if err := storage.ExportVocabulary(s, format, &buf); err != nil {
			return fmt.Errorf("export vocabulary: %w", err)
		}
		content = buf.String()
	}

	if targetFile != "" {
		if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("write export file %q: %w", targetFile, err)
		}
		fmt.Fprintf(out, "Exported vocabulary to %s (%s format).\n", targetFile, format)
	} else {
		fmt.Fprint(out, content)
	}

	return nil
}

func cmdSnippet(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	action := "list"
	var subArgs []string
	if len(args) > 0 {
		action = args[0]
		subArgs = args[1:]
	}

	switch action {
	case "list":
		return cmdSnippetList(ctx, cfg, client, out)
	case "add", "set":
		return cmdSnippetAdd(ctx, cfg, client, subArgs, out)
	case "remove", "delete", "rm":
		return cmdSnippetRemove(ctx, cfg, client, subArgs, out)
	case "test", "expand":
		return cmdSnippetTest(ctx, cfg, client, subArgs, out)
	default:
		return fmt.Errorf("unknown snippet subcommand: %s (usage: cadence snippet [list|add|remove|test])", action)
	}
}

func cmdSnippetList(ctx context.Context, cfg config.Config, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var snippets []storage.Snippet
	res, err := client.Call(callCtx, "snippet_list", nil)
	if err == nil {
		raw, err := json.Marshal(res)
		if err == nil {
			_ = json.Unmarshal(raw, &snippets)
		}
	} else {
		// Offline storage fallback
		s, err := storage.Open(cfg.Storage.DBPath)
		if err != nil {
			return fmt.Errorf("failed to reach daemon (%v) and cannot open database: %w", err, err)
		}
		defer s.Close()
		snippets, _ = s.ListSnippets()
	}

	if len(snippets) == 0 {
		fmt.Fprintln(out, "No custom snippets found.")
		return nil
	}

	fmt.Fprintf(out, "Custom Snippets (%d):\n", len(snippets))
	for i, snip := range snippets {
		descStr := ""
		if snip.Description != "" {
			descStr = fmt.Sprintf(" (%s)", snip.Description)
		}
		tmplPreview := snip.Template
		if len(tmplPreview) > 50 {
			tmplPreview = tmplPreview[:47] + "..."
		}
		tmplPreview = strings.ReplaceAll(tmplPreview, "\n", "\\n")
		fmt.Fprintf(out, "%2d. %-20s -> %q%s\n", i+1, snip.Trigger, tmplPreview, descStr)
	}
	return nil
}

func cmdSnippetAdd(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: cadence snippet add <trigger> <template> [description]")
	}
	trigger := args[0]
	template := args[1]
	desc := ""
	if len(args) > 2 {
		desc = strings.Join(args[2:], " ")
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "snippet_set", map[string]any{
		"trigger":     trigger,
		"template":    template,
		"description": desc,
	})
	if err != nil {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to set snippet via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		if err := s.SetSnippet(storage.Snippet{
			Trigger:     trigger,
			Template:    template,
			Description: desc,
		}); err != nil {
			return fmt.Errorf("set snippet: %w", err)
		}
	}

	fmt.Fprintf(out, "Saved snippet: %q\n", trigger)
	return nil
}

func cmdSnippetRemove(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: cadence snippet remove <trigger>")
	}
	trigger := args[0]

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "snippet_delete", map[string]any{
		"trigger": trigger,
	})
	if err != nil {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to delete snippet via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		if err := s.DeleteSnippet(trigger); err != nil {
			return fmt.Errorf("delete snippet: %w", err)
		}
	}

	fmt.Fprintf(out, "Removed snippet: %q\n", trigger)
	return nil
}

func cmdSnippetTest(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: cadence snippet test <trigger>")
	}
	trigger := args[0]

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var template string
	var expanded string

	res, err := client.Call(callCtx, "snippet_expand", map[string]any{
		"trigger": trigger,
	})
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			template, _ = m["template"].(string)
			expanded, _ = m["expanded"].(string)
		}
	} else {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to test snippet via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		snip, err := s.GetSnippet(trigger)
		if err != nil {
			return err
		}
		if snip == nil {
			return fmt.Errorf("snippet %q not found", trigger)
		}
		template = snip.Template
		clip, _ := paste.GetSystemClipboard()
		expanded = postprocess.ExpandVariables(template, clip, time.Now())
	}

	if template == "" {
		return fmt.Errorf("snippet %q not found", trigger)
	}

	fmt.Fprintf(out, "Snippet Preview:\n")
	fmt.Fprintf(out, "  Trigger:   %s\n", trigger)
	fmt.Fprintf(out, "  Template:  %s\n", template)
	fmt.Fprintln(out, "  Expanded:")
	fmt.Fprintln(out, "----------------------------------------")
	fmt.Fprintln(out, expanded)
	fmt.Fprintln(out, "----------------------------------------")
	return nil
}

func cmdPolish(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence polish <text> [--profile <name>] [--app <name>]")
	}

	var textParts []string
	profile := ""
	application := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--profile", "-p":
			if i+1 < len(args) {
				profile = args[i+1]
				i++
			}
		case "--app", "-a":
			if i+1 < len(args) {
				application = args[i+1]
				i++
			}
		default:
			textParts = append(textParts, args[i])
		}
	}

	text := strings.Join(textParts, " ")
	if strings.TrimSpace(text) == "" {
		return errors.New("empty text to polish")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	params := map[string]any{
		"text":        text,
		"profile":     profile,
		"application": application,
	}

	res, err := client.Call(ctx, "polish", params)
	if err != nil {
		return fmt.Errorf("polish failed: %w", err)
	}

	m, ok := res.(map[string]any)
	if !ok {
		return errors.New("unexpected response from cadenced")
	}

	polished, _ := m["text"].(string)
	model, _ := m["model"].(string)
	latency, _ := m["latency_ms"].(float64)

	fmt.Fprintf(out, "%s\n", polished)
	if model != "" && model != "none" {
		fmt.Fprintf(out, "(model: %s, latency: %dms)\n", model, int64(latency))
	}
	return nil
}

func cmdProfile(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}

	switch action {
	case "list":
		return cmdProfileList(ctx, cfg, client, out)
	case "show", "get":
		if len(args) < 2 {
			return errors.New("usage: cadence profile show <name>")
		}
		return cmdProfileShow(ctx, cfg, client, args[1], out)
	case "set", "add":
		if len(args) < 2 {
			return errors.New("usage: cadence profile set <name> [--desc <description>] [--prompt <prompt>] [--temp <float>] [--rule <rule>]")
		}
		return cmdProfileSet(ctx, cfg, client, args[1], args[2:], out)
	case "delete", "remove", "rm":
		if len(args) < 2 {
			return errors.New("usage: cadence profile delete <name>")
		}
		return cmdProfileDelete(ctx, cfg, client, args[1], out)
	default:
		return fmt.Errorf("unknown profile action: %s (supported: list, show, set, delete)", action)
	}
}

func cmdProfileList(ctx context.Context, cfg config.Config, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var profiles []polish.Profile

	res, err := client.Call(callCtx, "polish_profile_list", nil)
	if err == nil {
		if rawList, ok := res.([]any); ok {
			for _, item := range rawList {
				if m, ok := item.(map[string]any); ok {
					var p polish.Profile
					p.Name, _ = m["name"].(string)
					p.Description, _ = m["description"].(string)
					p.SystemPrompt, _ = m["system_prompt"].(string)
					p.Temperature, _ = m["temperature"].(float64)
					p.IsBuiltin, _ = m["is_builtin"].(bool)
					if rules, ok := m["custom_rules"].([]any); ok {
						for _, r := range rules {
							if s, ok := r.(string); ok {
								p.CustomRules = append(p.CustomRules, s)
							}
						}
					}
					profiles = append(profiles, p)
				}
			}
		}
	} else {
		// Offline fallback via storage and builtins
		reg := polish.NewProfileRegistry(nil)
		if s, dbErr := storage.Open(cfg.Storage.DBPath); dbErr == nil {
			defer s.Close()
			reg = polish.NewProfileRegistry(s)
		}
		profiles = reg.List()
	}

	if len(profiles) == 0 {
		fmt.Fprintln(out, "No prompt profiles available.")
		return nil
	}

	fmt.Fprintf(out, "%-14s  %-8s  %-50s\n", "NAME", "TYPE", "DESCRIPTION")
	fmt.Fprintf(out, "%-14s  %-8s  %-50s\n", "----", "----", "-----------")
	for _, p := range profiles {
		pType := "custom"
		if p.IsBuiltin {
			pType = "builtin"
		}
		desc := p.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Fprintf(out, "%-14s  %-8s  %-50s\n", p.Name, pType, desc)
	}
	return nil
}

func cmdProfileShow(ctx context.Context, cfg config.Config, client ipc.Client, name string, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var p polish.Profile
	found := false

	res, err := client.Call(callCtx, "polish_profile_get", map[string]any{"name": name})
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			p.Name, _ = m["name"].(string)
			p.Description, _ = m["description"].(string)
			p.SystemPrompt, _ = m["system_prompt"].(string)
			p.Temperature, _ = m["temperature"].(float64)
			p.IsBuiltin, _ = m["is_builtin"].(bool)
			if rules, ok := m["custom_rules"].([]any); ok {
				for _, r := range rules {
					if s, ok := r.(string); ok {
						p.CustomRules = append(p.CustomRules, s)
					}
				}
			}
			found = true
		}
	} else {
		// Offline fallback
		var store storage.Store
		if s, dbErr := storage.Open(cfg.Storage.DBPath); dbErr == nil {
			defer s.Close()
			store = s
		}
		reg := polish.NewProfileRegistry(store)
		p, found = reg.Get(name)
	}

	if !found {
		return fmt.Errorf("profile %q not found", name)
	}

	pType := "custom"
	if p.IsBuiltin {
		pType = "builtin"
	}

	fmt.Fprintf(out, "Profile: %s (%s)\n", p.Name, pType)
	fmt.Fprintf(out, "  Description:  %s\n", p.Description)
	fmt.Fprintf(out, "  Temperature:  %.2f\n", p.Temperature)
	if len(p.CustomRules) > 0 {
		fmt.Fprintln(out, "  Rules:")
		for _, r := range p.CustomRules {
			fmt.Fprintf(out, "    - %s\n", r)
		}
	}
	fmt.Fprintln(out, "  System Prompt:")
	fmt.Fprintln(out, "----------------------------------------")
	fmt.Fprintln(out, p.SystemPrompt)
	fmt.Fprintln(out, "----------------------------------------")
	return nil
}

func cmdProfileSet(ctx context.Context, cfg config.Config, client ipc.Client, name string, flags []string, out io.Writer) error {
	desc := ""
	prompt := ""
	temp := 0.2
	var rules []string

	for i := 0; i < len(flags); i++ {
		switch flags[i] {
		case "--desc", "-d":
			if i+1 < len(flags) {
				desc = flags[i+1]
				i++
			}
		case "--prompt", "-p":
			if i+1 < len(flags) {
				prompt = flags[i+1]
				i++
			}
		case "--temp", "-t":
			if i+1 < len(flags) {
				if f, err := strconv.ParseFloat(flags[i+1], 64); err == nil {
					temp = f
				}
				i++
			}
		case "--rule", "-r":
			if i+1 < len(flags) {
				rules = append(rules, flags[i+1])
				i++
			}
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	params := map[string]any{
		"name":          name,
		"description":   desc,
		"system_prompt": prompt,
		"temperature":   temp,
		"custom_rules":  rules,
	}

	_, err := client.Call(callCtx, "polish_profile_set", params)
	if err != nil {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to save profile via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		reg := polish.NewProfileRegistry(s)
		if err := reg.Set(polish.Profile{
			Name:         name,
			Description:  desc,
			SystemPrompt: prompt,
			Temperature:  temp,
			CustomRules:  rules,
		}); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Profile %q configured successfully.\n", name)
	return nil
}

func cmdProfileDelete(ctx context.Context, cfg config.Config, client ipc.Client, name string, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "polish_profile_delete", map[string]any{"name": name})
	if err != nil {
		// Offline fallback
		s, dbErr := storage.Open(cfg.Storage.DBPath)
		if dbErr != nil {
			return fmt.Errorf("failed to delete profile via daemon (%v) and database: %w", err, dbErr)
		}
		defer s.Close()
		reg := polish.NewProfileRegistry(s)
		if err := reg.Delete(name); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Profile %q removed successfully.\n", name)
	return nil
}

func cmdApp(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var info appcontext.AppInfo
	res, err := client.Call(callCtx, "active_app", nil)
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			info.Name, _ = m["name"].(string)
			info.BundleID, _ = m["bundle_id"].(string)
			info.WindowTitle, _ = m["window_title"].(string)
			if pid, ok := m["pid"].(float64); ok {
				info.ProcessID = int(pid)
			}
			if cat, ok := m["category"].(string); ok {
				info.Category = appcontext.Category(cat)
			}
		}
	} else {
		// Offline fallback detector
		d := appcontext.NewDefaultDetector()
		info, err = d.Detect(ctx)
		if err != nil {
			return fmt.Errorf("detect active application: %w", err)
		}
	}

	fmt.Fprintf(out, "Active Application:\n")
	fmt.Fprintf(out, "  Name:         %s\n", info.Name)
	if info.BundleID != "" {
		fmt.Fprintf(out, "  Bundle ID:    %s\n", info.BundleID)
	}
	if info.WindowTitle != "" {
		fmt.Fprintf(out, "  Window Title: %s\n", info.WindowTitle)
	}
	if info.ProcessID > 0 {
		fmt.Fprintf(out, "  PID:          %d\n", info.ProcessID)
	}
	fmt.Fprintf(out, "  Category:     %s\n", info.Category)
	return nil
}

func cmdContext(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}

	switch action {
	case "show", "get", "status":
		return cmdContextShow(ctx, client, args, out)
	case "set":
		return cmdContextSet(ctx, client, args, out)
	case "clear", "reset":
		return cmdContextClear(ctx, client, out)
	default:
		return fmt.Errorf("unknown context action %q (expected show, set, or clear)", action)
	}
}

func cmdContextShow(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "-j" || arg == "--json" {
			jsonOutput = true
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Query active app
	var appInfo any
	if res, err := client.Call(callCtx, "active_app", nil); err == nil {
		appInfo = res
	}

	// Query editor context
	var editorCtx map[string]any
	if res, err := client.Call(callCtx, "editor_context_get", nil); err == nil {
		if m, ok := res.(map[string]any); ok {
			editorCtx = m
		}
	}

	// Query terminal context
	var terminalCtx map[string]any
	if res, err := client.Call(callCtx, "terminal_context_get", nil); err == nil {
		if m, ok := res.(map[string]any); ok {
			terminalCtx = m
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"app":      appInfo,
			"editor":   editorCtx,
			"terminal": terminalCtx,
		})
	}

	// Format text output
	fmt.Fprintf(out, "Active Application Context:\n")
	if m, ok := appInfo.(map[string]any); ok && len(m) > 0 {
		name, _ := m["name"].(string)
		cat, _ := m["category"].(string)
		title, _ := m["window_title"].(string)
		fmt.Fprintf(out, "  Name:         %s\n", name)
		if title != "" {
			fmt.Fprintf(out, "  Window Title: %s\n", title)
		}
		fmt.Fprintf(out, "  Category:     %s\n", cat)
	} else {
		fmt.Fprintf(out, "  (not detected or daemon offline)\n")
	}

	fmt.Fprintf(out, "\nCode Editor Context:\n")
	if editorCtx != nil && editorCtx["active"] == true {
		editorName, _ := editorCtx["editor_name"].(string)
		filePath, _ := editorCtx["file_path"].(string)
		fileName, _ := editorCtx["file_name"].(string)
		lang, _ := editorCtx["language_id"].(string)
		line, _ := editorCtx["line"].(float64)
		col, _ := editorCtx["column"].(float64)
		selection, _ := editorCtx["selection_text"].(string)
		summary, _ := editorCtx["summary"].(string)

		if editorName == "" {
			editorName = "vscode"
		}
		fmt.Fprintf(out, "  Editor:       %s\n", editorName)
		if filePath != "" {
			fmt.Fprintf(out, "  File:         %s\n", filePath)
		} else if fileName != "" {
			fmt.Fprintf(out, "  File:         %s\n", fileName)
		}
		if lang != "" {
			fmt.Fprintf(out, "  Language:     %s\n", lang)
		}
		if line > 0 {
			fmt.Fprintf(out, "  Cursor:       line %.0f, col %.0f\n", line, col)
		}
		if selection != "" {
			fmt.Fprintf(out, "  Selection:    %d characters\n", len(selection))
		}
		if summary != "" {
			fmt.Fprintf(out, "  Summary:      %s\n", summary)
		}
	} else {
		fmt.Fprintf(out, "  Status:       Not active (no editor connected)\n")
	}

	fmt.Fprintf(out, "\nTerminal & Shell Context:\n")
	if terminalCtx != nil && terminalCtx["active"] == true {
		termApp, _ := terminalCtx["terminal_app"].(string)
		shell, _ := terminalCtx["shell"].(string)
		pid, _ := terminalCtx["pid"].(float64)
		workingDir, _ := terminalCtx["working_dir"].(string)
		gitBranch, _ := terminalCtx["git_branch"].(string)
		lastCmd, _ := terminalCtx["last_command"].(string)
		lastExit, _ := terminalCtx["last_exit_code"].(float64)
		summary, _ := terminalCtx["summary"].(string)

		if termApp == "" {
			termApp = "Terminal"
		}
		fmt.Fprintf(out, "  Terminal:     %s\n", termApp)
		if shell != "" {
			if pid > 0 {
				fmt.Fprintf(out, "  Shell:        %s (PID: %.0f)\n", shell, pid)
			} else {
				fmt.Fprintf(out, "  Shell:        %s\n", shell)
			}
		}
		if workingDir != "" {
			fmt.Fprintf(out, "  Directory:    %s\n", workingDir)
		}
		if gitBranch != "" {
			fmt.Fprintf(out, "  Git Branch:   %s\n", gitBranch)
		}
		if lastCmd != "" {
			fmt.Fprintf(out, "  Last Command: %s (Exit Code: %.0f)\n", lastCmd, lastExit)
		}
		if summary != "" {
			fmt.Fprintf(out, "  Summary:      %s\n", summary)
		}
	} else {
		fmt.Fprintf(out, "  Status:       Not active (no terminal tracked)\n")
	}

	return nil
}

func cmdContextSet(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	editorPayload := map[string]any{}
	terminalPayload := map[string]any{}
	hasEditor := false
	hasTerminal := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--file":
			if i+1 < len(args) {
				editorPayload["file_path"] = args[i+1]
				editorPayload["file_name"] = filepath.Base(args[i+1])
				hasEditor = true
				i++
			}
		case "-l", "--lang", "--language":
			if i+1 < len(args) {
				editorPayload["language_id"] = args[i+1]
				hasEditor = true
				i++
			}
		case "--line":
			if i+1 < len(args) {
				var line int
				fmt.Sscanf(args[i+1], "%d", &line)
				editorPayload["line"] = line
				hasEditor = true
				i++
			}
		case "--col", "--column":
			if i+1 < len(args) {
				var col int
				fmt.Sscanf(args[i+1], "%d", &col)
				editorPayload["column"] = col
				hasEditor = true
				i++
			}
		case "-s", "--selection":
			if i+1 < len(args) {
				editorPayload["selection_text"] = args[i+1]
				hasEditor = true
				i++
			}
		case "-e", "--editor":
			if i+1 < len(args) {
				ed := args[i+1]
				if ed == "terminal" {
					hasTerminal = true
				} else {
					editorPayload["editor_name"] = ed
					hasEditor = true
				}
				i++
			}
		case "-r", "--root", "--workspace":
			if i+1 < len(args) {
				editorPayload["workspace_root"] = args[i+1]
				hasEditor = true
				i++
			}
		case "-c", "--context":
			if i+1 < len(args) {
				editorPayload["surrounding_context"] = args[i+1]
				hasEditor = true
				i++
			}
		case "--shell":
			if i+1 < len(args) {
				terminalPayload["shell"] = args[i+1]
				hasTerminal = true
				i++
			}
		case "--pwd", "--dir":
			if i+1 < len(args) {
				terminalPayload["working_dir"] = args[i+1]
				hasTerminal = true
				i++
			}
		case "--cmd":
			if i+1 < len(args) {
				terminalPayload["last_command"] = args[i+1]
				hasTerminal = true
				i++
			}
		case "--exit-code", "--code":
			if i+1 < len(args) {
				var code int
				fmt.Sscanf(args[i+1], "%d", &code)
				terminalPayload["last_exit_code"] = code
				hasTerminal = true
				i++
			}
		case "--pid":
			if i+1 < len(args) {
				var pid int
				fmt.Sscanf(args[i+1], "%d", &pid)
				terminalPayload["pid"] = pid
				hasTerminal = true
				i++
			}
		case "--branch", "--git":
			if i+1 < len(args) {
				terminalPayload["git_branch"] = args[i+1]
				hasTerminal = true
				i++
			}
		case "--term", "--terminal-app":
			if i+1 < len(args) {
				terminalPayload["terminal_app"] = args[i+1]
				hasTerminal = true
				i++
			}
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if hasTerminal {
		if terminalPayload["terminal_app"] == nil {
			terminalPayload["terminal_app"] = "Terminal"
		}
		if _, err := client.Call(callCtx, "terminal_context_set", terminalPayload); err != nil {
			return fmt.Errorf("set terminal context: %w", err)
		}
		fmt.Fprintln(out, "Terminal context updated.")
	}

	if hasEditor || !hasTerminal {
		if editorPayload["editor_name"] == nil {
			editorPayload["editor_name"] = "vscode"
		}
		if _, err := client.Call(callCtx, "editor_context_set", editorPayload); err != nil {
			return fmt.Errorf("set editor context: %w", err)
		}
		fmt.Fprintln(out, "Editor context updated.")
	}

	return nil
}

func cmdContextClear(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, _ = client.Call(callCtx, "editor_context_clear", nil)
	_, _ = client.Call(callCtx, "terminal_context_clear", nil)

	fmt.Fprintln(out, "Developer context cleared.")
	return nil
}

func cmdTerminal(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}

	switch action {
	case "status", "show", "get":
		return cmdContextShow(ctx, client, args, out)
	case "set":
		return cmdContextSet(ctx, client, args, out)
	case "hook":
		shell := "zsh"
		if len(args) > 0 {
			shell = strings.ToLower(args[0])
		}
		switch shell {
		case "zsh":
			fmt.Fprint(out, zshHookScript)
		case "bash":
			fmt.Fprint(out, bashHookScript)
		case "fish":
			fmt.Fprint(out, fishHookScript)
		default:
			return fmt.Errorf("unsupported shell %q (supported: zsh, bash, fish)", shell)
		}
		return nil
	default:
		return fmt.Errorf("unknown terminal action %q (expected status or hook)", action)
	}
}

const zshHookScript = `if [[ -n "$ZSH_VERSION" ]]; then
  _cadence_zsh_preexec() {
    local cmd="$1"
    export _CADENCE_LAST_CMD="$cmd"
    (cadence context set --editor terminal --shell zsh --pid "$$" --pwd "$PWD" --cmd "$cmd" >/dev/null 2>&1 &)
  }

  _cadence_zsh_precmd() {
    local exit_code=$?
    if [[ -n "$_CADENCE_LAST_CMD" ]]; then
      (cadence context set --editor terminal --shell zsh --pid "$$" --pwd "$PWD" --cmd "$_CADENCE_LAST_CMD" --exit-code "$exit_code" >/dev/null 2>&1 &)
      unset _CADENCE_LAST_CMD
    else
      (cadence context set --editor terminal --shell zsh --pid "$$" --pwd "$PWD" --exit-code "$exit_code" >/dev/null 2>&1 &)
    fi
  }

  autoload -Uz add-zsh-hook
  add-zsh-hook preexec _cadence_zsh_preexec
  add-zsh-hook precmd _cadence_zsh_precmd
fi
`

const bashHookScript = `if [ -n "$BASH_VERSION" ]; then
  _cadence_bash_prompt() {
    local exit_code=$?
    (cadence context set --editor terminal --shell bash --pid "$$" --pwd "$PWD" --exit-code "$exit_code" >/dev/null 2>&1 &)
  }

  if [[ ";$PROMPT_COMMAND;" != *";_cadence_bash_prompt;"* ]]; then
    PROMPT_COMMAND="_cadence_bash_prompt;${PROMPT_COMMAND:-}"
  fi
fi
`

const fishHookScript = `function _cadence_fish_preexec --on-event fish_preexec
    set -g _CADENCE_LAST_CMD $argv[1]
    command cadence context set --editor terminal --shell fish --pid $fish_pid --pwd "$PWD" --cmd "$argv[1]" >/dev/null 2>&1 &
end

function _cadence_fish_postexec --on-event fish_postexec
    set -l exit_code $status
    if set -q _CADENCE_LAST_CMD
        command cadence context set --editor terminal --shell fish --pid $fish_pid --pwd "$PWD" --cmd "$_CADENCE_LAST_CMD" --exit-code $exit_code >/dev/null 2>&1 &
        set -e _CADENCE_LAST_CMD
    else
        command cadence context set --editor terminal --shell fish --pid $fish_pid --pwd "$PWD" --exit-code $exit_code >/dev/null 2>&1 &
    end
end
`

func cmdGenerate(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	jsonOutput := false
	explain := false
	pasteOutput := false
	showRisk := false
	dryRun := false
	var workingDir string
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintln(out, `Usage: cadence cmd [flags] <voice-instruction>

Translate natural language voice instructions into executable shell commands.

Flags:
  -j, --json     Output result as JSON
  -e, --explain  Print human-readable explanation, ecosystem, and risk metadata
  -r, --risk     Prefix command with risk classification badge ([SAFE], [RISKY], [DESTRUCTIVE])
  -n, --dry-run  Simulate and preview projected impact without executing
  -p, --paste    Paste the generated command into the active application
  -d, --dir      Specify project working directory (default: active terminal or current dir)
  -h, --help     Show this help message

Examples:
  cadence cmd "run the tests"
  cadence cmd "git commit with message updated terminal context"
  cadence cmd --risk "reset hard head"
  cadence cmd --dry-run "delete test directory"
  cadence cmd --explain "install dependencies"`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		case "-e", "--explain":
			explain = true
		case "-r", "--risk":
			showRisk = true
		case "-n", "--dry-run":
			dryRun = true
		case "-p", "--paste":
			pasteOutput = true
		case "-d", "--dir", "--pwd":
			if i+1 < len(args) {
				workingDir = args[i+1]
				i++
			}
		default:
			words = append(words, args[i])
		}
	}

	instruction := strings.TrimSpace(strings.Join(words, " "))
	if instruction == "" {
		return errors.New("missing voice instruction for command generation (usage: cadence cmd <instruction>)")
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var result developer.GeneratedCommand

	// 1. Try querying daemon over IPC
	ipcRes, err := client.Call(callCtx, "dev_command_generate", map[string]any{
		"instruction": instruction,
		"working_dir": workingDir,
	})

	if err == nil {
		if raw, mErr := json.Marshal(ipcRes); mErr == nil {
			_ = json.Unmarshal(raw, &result)
		}
	} else {
		// 2. Offline fallback generator
		gen := developer.NewGenerator(nil)
		result, err = gen.Generate(ctx, instruction, workingDir)
		if err != nil {
			return fmt.Errorf("generate command: %w", err)
		}
	}

	// If paste requested
	if pasteOutput {
		paster := paste.NewDefaultCascade()
		if _, pErr := paster.Paste(result.Command); pErr != nil {
			return fmt.Errorf("paste command: %w", pErr)
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if explain {
		fmt.Fprintf(out, "Generated Command:\n  %s\n\n", result.Command)
		fmt.Fprintf(out, "Details:\n")
		fmt.Fprintf(out, "  Instruction:  %s\n", result.RawInstruction)
		if result.Ecosystem != "" && result.Ecosystem != developer.EcosystemUnknown {
			fmt.Fprintf(out, "  Ecosystem:    %s\n", result.Ecosystem)
		}
		if result.Explanation != "" {
			fmt.Fprintf(out, "  Explanation:  %s\n", result.Explanation)
		}
		fmt.Fprintf(out, "  Source:       %s (confidence: %.2f)\n", result.Source, result.Confidence)
		riskLevel := strings.ToUpper(string(result.Risk.Level))
		if riskLevel == "" {
			riskLevel = "SAFE"
		}
		fmt.Fprintf(out, "  Risk Level:   %s\n", riskLevel)
		if result.Risk.Reason != "" {
			fmt.Fprintf(out, "  Risk Reason:  %s\n", result.Risk.Reason)
		}
		confirmStr := "No"
		if result.Risk.RequiresConfirmation {
			confirmStr = "Yes"
		}
		fmt.Fprintf(out, "  Confirmation: %s\n", confirmStr)
		return nil
	}

	if dryRun {
		engine := developer.NewPreviewEngine()
		preview, _ := engine.Preview(ctx, result.Command, workingDir)
		if jsonOutput {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(preview)
		}
		fmt.Fprintf(out, "Dry-Run Preview:\n")
		fmt.Fprintf(out, "  Command:          %s\n", preview.Command)
		fmt.Fprintf(out, "  Working Dir:      %s\n", preview.WorkingDir)
		fmt.Fprintf(out, "  Risk Level:       %s\n", strings.ToUpper(string(preview.Risk.Level)))
		fmt.Fprintf(out, "  Requires Confirm: %v\n", preview.RequiresConfirmation)
		fmt.Fprintf(out, "  Projected Impact: %s\n", preview.ProjectedImpact)
		if len(preview.AffectedTargets) > 0 {
			fmt.Fprintf(out, "  Affected Targets: %s\n", strings.Join(preview.AffectedTargets, ", "))
		}
		return nil
	}

	if showRisk {
		badge := fmt.Sprintf("[%s]", strings.ToUpper(string(result.Risk.Level)))
		fmt.Fprintf(out, "%s %s\n", badge, result.Command)
		return nil
	}

	fmt.Fprintln(out, result.Command)
	return nil
}

func cmdExec(ctx context.Context, client ipc.Client, args []string, in io.Reader, out, errOut io.Writer) error {
	dryRun := false
	confirm := false
	jsonOutput := false
	var workingDir string
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintln(out, `Usage: cadence exec [flags] <command...>

Safely execute shell commands with dry-run preview and confirmation guards.

Flags:
  -n, --dry-run   Preview the command and projected impact without executing
  -y, --confirm   Auto-confirm risky or destructive execution
  -d, --dir       Specify working directory (default: active terminal or current dir)
  -j, --json      Output preview or execution result as JSON
  -h, --help      Show this help message

Examples:
  cadence exec "git status"
  cadence exec --dry-run "git reset --hard HEAD~1"
  cadence exec --confirm "git commit -m 'feat: ship milestone'"
  cadence exec "rm -rf /tmp/test"`)
			return nil
		case "-n", "--dry-run":
			dryRun = true
		case "-y", "--confirm", "--yes":
			confirm = true
		case "-j", "--json":
			jsonOutput = true
		case "-d", "--dir", "--pwd":
			if i+1 < len(args) {
				workingDir = args[i+1]
				i++
			}
		default:
			words = append(words, args[i])
		}
	}

	command := strings.TrimSpace(strings.Join(words, " "))
	if command == "" {
		return errors.New("missing command to execute (usage: cadence exec <command>)")
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. Preview step
	var preview developer.DryRunPreview
	prevRes, prevErr := client.Call(callCtx, "dev_command_preview", map[string]any{
		"command":     command,
		"working_dir": workingDir,
	})
	if prevErr == nil {
		if raw, mErr := json.Marshal(prevRes); mErr == nil {
			_ = json.Unmarshal(raw, &preview)
		}
	} else {
		// Offline preview fallback
		engine := developer.NewPreviewEngine()
		preview, _ = engine.Preview(ctx, command, workingDir)
	}

	if dryRun {
		if jsonOutput {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(preview)
		}

		fmt.Fprintf(out, "Dry-Run Preview:\n")
		fmt.Fprintf(out, "  Command:          %s\n", preview.Command)
		fmt.Fprintf(out, "  Working Dir:      %s\n", preview.WorkingDir)
		fmt.Fprintf(out, "  Risk Level:       %s\n", strings.ToUpper(string(preview.Risk.Level)))
		fmt.Fprintf(out, "  Requires Confirm: %v\n", preview.RequiresConfirmation)
		fmt.Fprintf(out, "  Projected Impact: %s\n", preview.ProjectedImpact)
		if len(preview.AffectedTargets) > 0 {
			fmt.Fprintf(out, "  Affected Targets: %s\n", strings.Join(preview.AffectedTargets, ", "))
		}
		return nil
	}

	// 2. Interactive prompt if requires confirmation and not confirmed
	if preview.RequiresConfirmation && !confirm {
		if jsonOutput {
			return fmt.Errorf("command execution rejected: '%s' is %s and requires confirmation (pass -y/--confirm)", preview.Command, preview.Risk.Level)
		}

		fmt.Fprintf(out, "Dry-Run Preview:\n")
		fmt.Fprintf(out, "  Command:          %s\n", preview.Command)
		fmt.Fprintf(out, "  Risk Level:       %s\n", strings.ToUpper(string(preview.Risk.Level)))
		fmt.Fprintf(out, "  Projected Impact: %s\n", preview.ProjectedImpact)
		if len(preview.AffectedTargets) > 0 {
			fmt.Fprintf(out, "  Affected Targets: %s\n", strings.Join(preview.AffectedTargets, ", "))
		}
		fmt.Fprintf(out, "\n⚠️  This command is %s. Are you sure you want to execute? [y/N]: ", strings.ToUpper(string(preview.Risk.Level)))

		var response string
		if in != nil {
			scanner := bufio.NewScanner(in)
			if scanner.Scan() {
				response = strings.TrimSpace(scanner.Text())
			}
		}

		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			return errors.New("execution aborted by user")
		}
		confirm = true
	}

	// 3. Execution step
	var result developer.ExecutionResult
	execRes, execErr := client.Call(callCtx, "dev_command_execute", map[string]any{
		"command":     command,
		"working_dir": workingDir,
		"confirm":     confirm,
	})
	if execErr == nil {
		if raw, mErr := json.Marshal(execRes); mErr == nil {
			_ = json.Unmarshal(raw, &result)
		}
	} else {
		engine := developer.NewPreviewEngine()
		result, execErr = engine.Execute(ctx, command, workingDir, confirm)
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Stdout != "" {
		fmt.Fprint(out, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(errOut, result.Stderr)
	}

	if execErr != nil {
		return execErr
	}
	return nil
}

func cmdRisk(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	jsonOutput := false
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintln(out, `Usage: cadence risk [flags] <command...>

Audit and classify shell commands into Safe, Risky, or Destructive levels.

Flags:
  -j, --json     Output classification as JSON
  -h, --help     Show this help message

Examples:
  cadence risk "git status"
  cadence risk "git commit -m 'feat: add auth'"
  cadence risk "rm -rf /tmp/data"
  cadence risk "git reset --hard HEAD~1"`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		default:
			words = append(words, args[i])
		}
	}

	command := strings.TrimSpace(strings.Join(words, " "))
	if command == "" {
		return errors.New("missing command to classify (usage: cadence risk <command>)")
	}

	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var assessment developer.RiskAssessment
	ipcRes, err := client.Call(callCtx, "dev_command_risk", map[string]any{
		"command": command,
	})
	if err == nil {
		if raw, mErr := json.Marshal(ipcRes); mErr == nil {
			_ = json.Unmarshal(raw, &assessment)
		}
	} else {
		// Offline fallback
		assessment = developer.ClassifyCommand(command)
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(assessment)
	}

	// Human readable output
	fmt.Fprintf(out, "Command:      %s\n", command)
	fmt.Fprintf(out, "Risk Level:   %s\n", strings.ToUpper(string(assessment.Level)))
	fmt.Fprintf(out, "Rule:         %s\n", assessment.Rule)
	confirmStr := "No"
	if assessment.RequiresConfirmation {
		confirmStr = "Yes"
	}
	fmt.Fprintf(out, "Confirm:      %s\n", confirmStr)
	fmt.Fprintf(out, "Reason:       %s\n", assessment.Reason)
	if len(assessment.HighlightedTokens) > 0 {
		fmt.Fprintf(out, "Key Tokens:   %s\n", strings.Join(assessment.HighlightedTokens, ", "))
	}
	return nil
}

func cmdGit(ctx context.Context, client ipc.Client, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(out, `Usage: cadence git <subcommand> [flags]

AI-driven Git integrations powered by staged diffs, commit deltas, and voice hints.

Subcommands:
  commit [flags] [hint...]  Generate Conventional Commit message from staged changes
  pr [flags] [notes...]     Generate Pull Request title and Markdown description

Flags:
  -h, --help                Show this help message

Run 'cadence git <subcommand> --help' for details on specific commands.`)
		return nil
	}

	switch args[0] {
	case "commit":
		return cmdGitCommit(ctx, client, args[1:], out)
	case "pr":
		return cmdGitPR(ctx, client, args[1:], out)
	case "-h", "--help", "help":
		fmt.Fprintln(out, `Usage: cadence git <subcommand> [flags]

Subcommands:
  commit [flags] [hint...]  Generate Conventional Commit message from staged changes
  pr [flags] [notes...]     Generate Pull Request title and Markdown description`)
		return nil
	default:
		return fmt.Errorf("unknown git subcommand '%s' (available: commit, pr)", args[0])
	}
}

func cmdGitCommit(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	jsonOutput := false
	pasteOutput := false
	apply := false
	var workingDir, hint string
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintln(out, `Usage: cadence git commit [flags] [spoken-hint...]

Generate a Conventional Commit message from staged Git changes and optional voice hints.

Flags:
  -m, --hint     Provide spoken voice hint / instruction for the commit message
  -a, --apply    Immediately execute 'git commit' with the generated message
  -p, --paste    Paste the commit message into the active application
  -d, --dir      Specify repository working directory (default: current directory)
  -j, --json     Output message as JSON
  -h, --help     Show this help message

Examples:
  cadence git commit
  cadence git commit "fix auth token expiration bug"
  cadence git commit --hint "refactor storage layer" --apply
  cadence git commit --json`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		case "-p", "--paste":
			pasteOutput = true
		case "-a", "--apply":
			apply = true
		case "-m", "--hint":
			if i+1 < len(args) {
				hint = args[i+1]
				i++
			}
		case "-d", "--dir", "--pwd":
			if i+1 < len(args) {
				workingDir = args[i+1]
				i++
			}
		default:
			words = append(words, args[i])
		}
	}

	if hint == "" && len(words) > 0 {
		hint = strings.TrimSpace(strings.Join(words, " "))
	}

	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var msg developer.CommitMessage
	ipcRes, err := client.Call(callCtx, "git_commit_generate", map[string]any{
		"working_dir": workingDir,
		"spoken_hint": hint,
	})
	if err == nil {
		if raw, mErr := json.Marshal(ipcRes); mErr == nil {
			_ = json.Unmarshal(raw, &msg)
		}
	} else {
		assistant := developer.NewGitAssistant(nil)
		msg, err = assistant.GenerateCommit(ctx, developer.CommitGenRequest{
			WorkingDir: workingDir,
			SpokenHint: hint,
		})
		if err != nil {
			return err
		}
	}

	if apply {
		targetDir := workingDir
		if targetDir == "" {
			if wd, gErr := os.Getwd(); gErr == nil {
				targetDir = wd
			}
		}
		var commitCmd *exec.Cmd
		if msg.Body != "" {
			commitCmd = exec.CommandContext(ctx, "git", "commit", "-m", msg.Title, "-m", msg.Body)
		} else {
			commitCmd = exec.CommandContext(ctx, "git", "commit", "-m", msg.Title)
		}
		commitCmd.Dir = targetDir
		outBytes, cErr := commitCmd.CombinedOutput()
		if cErr != nil {
			return fmt.Errorf("git commit failed: %w (output: %s)", cErr, string(outBytes))
		}
		fmt.Fprintf(out, "Committed successfully:\n%s\n", strings.TrimSpace(string(outBytes)))
		return nil
	}

	if pasteOutput {
		paster := paste.NewDefaultCascade()
		if _, pErr := paster.Paste(msg.FullMessage); pErr != nil {
			return fmt.Errorf("paste commit message: %w", pErr)
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(msg)
	}

	fmt.Fprintln(out, msg.FullMessage)
	return nil
}

func cmdGitPR(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	jsonOutput := false
	pasteOutput := false
	var workingDir, baseBranch, notes string
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintln(out, `Usage: cadence git pr [flags] [notes...]

Generate a Pull Request title and Markdown description from branch commit deltas and voice notes.

Flags:
  -b, --base     Base branch to compare against (default: origin/main or main)
  -n, --notes    Voice notes / summary for the pull request description
  -p, --paste    Paste the PR description into the active application
  -d, --dir      Specify repository working directory (default: current directory)
  -j, --json     Output PR description as JSON
  -h, --help     Show this help message

Examples:
  cadence git pr
  cadence git pr --notes "Implements JWT auth and token caching"
  cadence git pr --base develop --json`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		case "-p", "--paste":
			pasteOutput = true
		case "-b", "--base":
			if i+1 < len(args) {
				baseBranch = args[i+1]
				i++
			}
		case "-n", "--notes":
			if i+1 < len(args) {
				notes = args[i+1]
				i++
			}
		case "-d", "--dir", "--pwd":
			if i+1 < len(args) {
				workingDir = args[i+1]
				i++
			}
		default:
			words = append(words, args[i])
		}
	}

	if notes == "" && len(words) > 0 {
		notes = strings.TrimSpace(strings.Join(words, " "))
	}

	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var pr developer.PRDescription
	ipcRes, err := client.Call(callCtx, "git_pr_generate", map[string]any{
		"working_dir":  workingDir,
		"base_branch":  baseBranch,
		"spoken_notes": notes,
	})
	if err == nil {
		if raw, mErr := json.Marshal(ipcRes); mErr == nil {
			_ = json.Unmarshal(raw, &pr)
		}
	} else {
		assistant := developer.NewGitAssistant(nil)
		pr, err = assistant.GeneratePR(ctx, developer.PRGenRequest{
			WorkingDir:  workingDir,
			BaseBranch:  baseBranch,
			SpokenNotes: notes,
		})
		if err != nil {
			return err
		}
	}

	if pasteOutput {
		paster := paste.NewDefaultCascade()
		fullContent := fmt.Sprintf("# %s\n\n%s", pr.Title, pr.Body)
		if _, pErr := paster.Paste(fullContent); pErr != nil {
			return fmt.Errorf("paste PR description: %w", pErr)
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	}

	fmt.Fprintf(out, "Title: %s\n\n%s\n", pr.Title, pr.Body)
	return nil
}

func cmdCode(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	jsonOutput := false
	pasteOutput := false
	isDoc := false
	acr := false
	var style, lang string
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintln(out, `Usage: cadence code [flags] <words...>

Transform spoken words into formatted code identifiers, operators, or docstrings.

Flags:
  -c, --case     Target casing: snake, camel, pascal, kebab, constant, dot, or auto (default: auto)
  -l, --lang     Programming language for docstrings (e.g. go, python, ts, rust)
  -d, --doc      Format input as language-specific docstring comment block
      --acr      Preserve developer acronyms (e.g. URL, HTTP, ID, JSON)
  -p, --paste    Paste formatted result into the active application
  -j, --json     Output result as JSON
  -h, --help     Show this help message

Voice Casing Triggers (auto-detected):
  "snake case user id"           -> user_id
  "camel case get account"       -> getAccount
  "pascal case token provider"   -> TokenProvider
  "kebab case api route"         -> api-route
  "constant case max limit"      -> MAX_LIMIT

Examples:
  cadence code "snake case user account id"
  cadence code --case camel "get auth token"
  cadence code --case constant "max buffer size"
  cadence code --doc --lang go "Run starts the background worker."`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		case "-p", "--paste":
			pasteOutput = true
		case "-d", "--doc", "--docstring":
			isDoc = true
		case "--acr", "--acronyms":
			acr = true
		case "-c", "--case", "--style":
			if i+1 < len(args) {
				style = args[i+1]
				i++
			}
		case "-l", "--lang", "--language":
			if i+1 < len(args) {
				lang = args[i+1]
				i++
			}
		default:
			words = append(words, args[i])
		}
	}

	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" {
		return errors.New("missing text for code formatting (usage: cadence code [flags] <words...>)")
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var result developer.CodeFormatResult
	ipcRes, err := client.Call(callCtx, "dev_code_format", map[string]any{
		"text":              text,
		"style":             style,
		"language":          lang,
		"is_docstring":      isDoc,
		"preserve_acronyms": acr,
	})
	if err == nil {
		if raw, mErr := json.Marshal(ipcRes); mErr == nil {
			_ = json.Unmarshal(raw, &result)
		}
	} else {
		result = developer.FormatCode(developer.CodeFormatRequest{
			Text:             text,
			Style:            developer.CasingStyle(style),
			Language:         lang,
			IsDocstring:      isDoc,
			PreserveAcronyms: acr,
		})
	}

	if pasteOutput {
		paster := paste.NewDefaultCascade()
		if _, pErr := paster.Paste(result.Result); pErr != nil {
			return fmt.Errorf("paste code format result: %w", pErr)
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Fprintln(out, result.Result)
	return nil
}

var systemControllerOverride system.Controller

func getSystemController() system.Controller {
	if systemControllerOverride != nil {
		return systemControllerOverride
	}
	return system.NewDefaultController()
}

func cmdSys(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(out, `System command automation:
Usage:
  cadence sys "<voice instruction>"
  cadence sys app [launch|focus|quit] <app-name>
  cadence sys window [minimize|maximize|close|hide]
  cadence sys screenshot [--window] [--clipboard] [--file <path>]
  cadence sys volume <0-100|mute|unmute>
`)
		return nil
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "app":
		return cmdSysApp(ctx, client, args[1:], out)
	case "window", "win":
		return cmdSysWindow(ctx, client, args[1:], out)
	case "screenshot", "screen", "shot":
		return cmdSysScreenshot(ctx, client, args[1:], out)
	case "volume", "vol":
		return cmdSysVolume(ctx, client, args[1:], out)
	default:
		// Natural speech command
		input := strings.Join(args, " ")
		callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()

		ipcRes, err := client.Call(callCtx, "system_command_execute", map[string]any{
			"input": input,
		})
		if err == nil {
			if m, ok := ipcRes.(map[string]any); ok {
				if parsed, pOk := m["parsed"].(bool); pOk && parsed {
					if msg, ok := m["message"].(string); ok {
						fmt.Fprintln(out, msg)
						return nil
					}
				}
			}
		}

		// Fallback to standalone execution
		parsedCmd, ok := system.ParseSystemCommand(input)
		if !ok {
			return fmt.Errorf("unrecognized system command: %q", input)
		}
		ctrl := getSystemController()
		msg, err := system.ExecuteSystemCommand(ctx, ctrl, parsedCmd)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, msg)
		return nil
	}
}

func cmdSysApp(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence sys app [launch|focus|quit] <name>")
	}
	action := "launch"
	appName := ""
	first := strings.ToLower(args[0])
	if first == "launch" || first == "focus" || first == "quit" || first == "open" || first == "close" {
		action = first
		if len(args) < 2 {
			return fmt.Errorf("missing app name for action %q", action)
		}
		appName = strings.Join(args[1:], " ")
	} else {
		appName = strings.Join(args, " ")
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	ipcRes, err := client.Call(callCtx, "system_app_action", map[string]any{
		"action": action,
		"app":    appName,
	})
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if msg, ok := m["message"].(string); ok {
				fmt.Fprintln(out, msg)
				return nil
			}
		}
	}

	// Standalone fallback
	ctrl := getSystemController()
	var runErr error
	var msg string
	switch action {
	case "focus":
		runErr = ctrl.FocusApp(ctx, appName)
		msg = fmt.Sprintf("Switched to %s", appName)
	case "quit", "close":
		runErr = ctrl.QuitApp(ctx, appName)
		msg = fmt.Sprintf("Quit %s", appName)
	default:
		runErr = ctrl.LaunchApp(ctx, appName)
		msg = fmt.Sprintf("Launched %s", appName)
	}
	if runErr != nil {
		return runErr
	}
	fmt.Fprintln(out, msg)
	return nil
}

func cmdSysWindow(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "minimize"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	ipcRes, err := client.Call(callCtx, "system_window_action", map[string]any{
		"action": action,
	})
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if msg, ok := m["message"].(string); ok {
				fmt.Fprintln(out, msg)
				return nil
			}
		}
	}

	// Standalone fallback
	ctrl := getSystemController()
	if err := ctrl.ControlWindow(ctx, system.WindowAction(action)); err != nil {
		return err
	}
	fmt.Fprintf(out, "Window action executed: %s\n", action)
	return nil
}

func cmdSysScreenshot(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	var opts system.ScreenshotOptions
	var jsonOutput bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-w" || arg == "--window":
			opts.ActiveWindowOnly = true
		case arg == "-c" || arg == "--clipboard":
			opts.CopyToClipboard = true
		case arg == "--json":
			jsonOutput = true
		case arg == "-f" || arg == "--file":
			if i+1 < len(args) {
				opts.SavePath = args[i+1]
				i++
			}
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ipcRes, err := client.Call(callCtx, "system_screenshot", map[string]any{
		"active_window_only": opts.ActiveWindowOnly,
		"copy_to_clipboard":  opts.CopyToClipboard,
		"save_path":          opts.SavePath,
	})
	if err == nil {
		if jsonOutput {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(ipcRes)
		}
		if m, ok := ipcRes.(map[string]any); ok {
			if msg, ok := m["message"].(string); ok {
				fmt.Fprintln(out, msg)
				return nil
			}
		}
	}

	// Standalone fallback
	ctrl := getSystemController()
	res, err := ctrl.CaptureScreenshot(ctx, opts)
	if err != nil {
		return err
	}
	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res.CopiedToClipboard {
		fmt.Fprintln(out, "Screenshot copied to clipboard")
	} else {
		fmt.Fprintf(out, "Screenshot saved to %s\n", res.Path)
	}
	return nil
}

func cmdSysVolume(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence sys volume <0-100|mute|unmute>")
	}
	arg := strings.ToLower(args[0])
	params := map[string]any{}

	if arg == "mute" {
		params["mute"] = true
	} else if arg == "unmute" {
		params["unmute"] = true
	} else {
		cleanLvl := strings.TrimSuffix(arg, "%")
		lvl, err := strconv.Atoi(cleanLvl)
		if err != nil {
			return fmt.Errorf("invalid volume value %q (must be 0-100, mute, or unmute)", arg)
		}
		params["level"] = float64(lvl)
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	ipcRes, err := client.Call(callCtx, "system_volume", params)
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if msg, ok := m["message"].(string); ok {
				fmt.Fprintln(out, msg)
				return nil
			}
		}
	}

	// Standalone fallback
	ctrl := getSystemController()
	if m, ok := params["mute"].(bool); ok && m {
		if err := ctrl.MuteVolume(ctx, true); err != nil {
			return err
		}
		fmt.Fprintln(out, "Volume muted")
		return nil
	}
	if u, ok := params["unmute"].(bool); ok && u {
		if err := ctrl.MuteVolume(ctx, false); err != nil {
			return err
		}
		fmt.Fprintln(out, "Volume unmuted")
		return nil
	}
	if l, ok := params["level"].(float64); ok {
		if err := ctrl.SetVolume(ctx, int(l)); err != nil {
			return err
		}
		fmt.Fprintf(out, "Volume set to %d%%\n", int(l))
		return nil
	}
	return nil
}

func cmdMode(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	if len(args) == 0 {
		ipcRes, err := client.Call(callCtx, "mode_get", nil)
		if err == nil {
			if m, ok := ipcRes.(map[string]any); ok {
				if cur, ok := m["mode"].(string); ok {
					fmt.Fprintf(out, "Active operating mode: %s\n", cur)
					return nil
				}
			}
		}
		// Standalone fallback
		fmt.Fprintln(out, "Active operating mode: dictation")
		return nil
	}

	target := strings.ToLower(strings.TrimSpace(args[0]))
	if !system.IsValidMode(target) {
		return fmt.Errorf("invalid operating mode %q (supported: dictation, code, command, agent)", target)
	}

	ipcRes, err := client.Call(callCtx, "mode_set", map[string]any{
		"mode": target,
	})
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if okVal, _ := m["ok"].(bool); okVal {
				fmt.Fprintf(out, "Operating mode switched to: %s\n", target)
				return nil
			}
		}
	}

	// Standalone feedback if daemon offline
	fmt.Fprintf(out, "Operating mode switched to: %s\n", target)
	return nil
}

func cmdControl(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing self-control voice phrase (usage: cadence control \"<phrase>\")")
	}
	input := strings.Join(args, " ")
	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	ipcRes, err := client.Call(callCtx, "self_control_execute", map[string]any{
		"input": input,
	})
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if parsed, pOk := m["parsed"].(bool); pOk && parsed {
				if msg, ok := m["message"].(string); ok {
					fmt.Fprintln(out, msg)
					return nil
				}
			} else {
				return fmt.Errorf("unrecognized self-control command: %q", input)
			}
		}
	}

	// Standalone fallback
	cmd, ok := system.ParseSelfCommand(input)
	if !ok {
		return fmt.Errorf("unrecognized self-control command: %q", input)
	}
	msg := system.FormatSelfFeedback(cmd, system.ModeDictation, false)
	fmt.Fprintln(out, msg)
	return nil
}

func cmdAgent(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(out, `Autonomous Agent mode:
Usage:
  cadence agent [flags] "<goal>"
  cadence agent history [limit]

Flags:
  -p, --plan-only    Generate execution plan without running tools
  -s, --max-steps    Maximum step execution limit (default 10)
      --json         Output complete execution trajectory in JSON format
`)
		return nil
	}

	if args[0] == "history" {
		limit := 10
		if len(args) > 1 {
			if l, err := strconv.Atoi(args[1]); err == nil && l > 0 {
				limit = l
			}
		}
		callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()

		ipcRes, err := client.Call(callCtx, "agent_history", map[string]any{
			"limit": float64(limit),
		})
		if err == nil {
			if list, ok := ipcRes.([]any); ok {
				if len(list) == 0 {
					fmt.Fprintln(out, "No agent executions recorded.")
					return nil
				}
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						fmt.Fprintf(out, "[%s] Goal: %s (Status: %s)\n", m["id"], m["goal"], m["status"])
					}
				}
				return nil
			}
		}
		fmt.Fprintln(out, "No agent executions recorded (daemon offline).")
		return nil
	}

	var planOnly bool
	var jsonOutput bool
	maxSteps := 10
	var goalWords []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-p" || arg == "--plan-only":
			planOnly = true
		case arg == "--json":
			jsonOutput = true
		case arg == "-s" || arg == "--max-steps":
			if i+1 < len(args) {
				if s, err := strconv.Atoi(args[i+1]); err == nil && s > 0 {
					maxSteps = s
					i++
				}
			}
		default:
			goalWords = append(goalWords, arg)
		}
	}

	if len(goalWords) == 0 {
		return errors.New("missing goal for agent execution (usage: cadence agent [flags] \"<goal>\")")
	}

	goal := strings.Join(goalWords, " ")
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var traj *agent.ExecutionTrajectory
	ipcRes, err := client.Call(callCtx, "agent_run", map[string]any{
		"goal":      goal,
		"plan_only": planOnly,
		"max_steps": float64(maxSteps),
	})
	if err == nil {
		if raw, mErr := json.Marshal(ipcRes); mErr == nil {
			var t agent.ExecutionTrajectory
			if uErr := json.Unmarshal(raw, &t); uErr == nil {
				traj = &t
			}
		}
	}

	// Standalone fallback
	if traj == nil {
		engine := agent.NewEngine(agent.WithSystemController(getSystemController()))
		var runErr error
		traj, runErr = engine.Run(ctx, goal, agent.RunOptions{
			PlanOnly: planOnly,
			MaxSteps: maxSteps,
		})
		if runErr != nil {
			return runErr
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(traj)
	}

	// Formatted human-readable output
	fmt.Fprintf(out, "🎯 Goal: %s\n", traj.Goal)
	fmt.Fprintf(out, "📋 Plan (%d step(s)):\n", len(traj.Plan.Steps))
	for _, step := range traj.Plan.Steps {
		fmt.Fprintf(out, "  %d. %s [%s]\n", step.Index, step.Description, step.Tool)
	}

	if planOnly {
		fmt.Fprintln(out, "\n(Plan-only mode: tools were not invoked)")
		return nil
	}

	if len(traj.Steps) > 0 {
		fmt.Fprintln(out, "\n⚡ Execution Steps:")
		for _, step := range traj.Steps {
			symbol := "✓"
			if !step.Observation.Success {
				symbol = "✗"
			}
			fmt.Fprintf(out, "  %s Step %d: %s -> %s\n", symbol, step.Index, step.Thought, step.Observation.Summary)
		}
	}

	fmt.Fprintf(out, "\n🏁 Final Report: %s\n", traj.FinalReport)
	fmt.Fprintf(out, "📊 Status: %s\n", traj.Status)
	return nil
}

func cmdTools(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "list"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var tools []agent.ToolDefinition
	var schemas []map[string]any

	ipcRes, err := client.Call(callCtx, "tool_list", nil)
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if raw, mErr := json.Marshal(m["tools"]); mErr == nil {
				_ = json.Unmarshal(raw, &tools)
			}
			if raw, mErr := json.Marshal(m["schemas"]); mErr == nil {
				_ = json.Unmarshal(raw, &schemas)
			}
		}
	} else {
		// Standalone fallback
		reg := agent.NewRegistry(getSystemController(), nil)
		tools = reg.List()
		schemas = reg.ExportJSONSchema()
	}

	if action == "schema" || action == "schemas" || action == "--json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(schemas)
	}

	fmt.Fprintf(out, "Registered Agent Tools (%d):\n", len(tools))
	for _, t := range tools {
		fmt.Fprintf(out, "  • %-12s [%-10s] %s\n", t.Name, t.Category, t.Description)
		if len(t.Parameters) > 0 {
			var params []string
			for _, p := range t.Parameters {
				req := ""
				if p.Required {
					req = "*"
				}
				params = append(params, fmt.Sprintf("%s%s (%s)", p.Name, req, p.Type))
			}
			fmt.Fprintf(out, "    Parameters: %s\n", strings.Join(params, ", "))
		}
	}
	return nil
}

func cmdTool(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence tool <tool-name> [action] [param=value...]")
	}
	name := strings.ToLower(args[0])
	params := make(map[string]any)

	for i := 1; i < len(args); i++ {
		arg := args[i]
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			params[parts[0]] = parts[1]
		} else if i == 1 && !strings.HasPrefix(arg, "-") {
			params["action"] = arg
		} else if i == 2 && params["action"] != nil && !strings.HasPrefix(arg, "-") {
			switch name {
			case "fs":
				params["path"] = arg
			case "system":
				params["target"] = arg
			case "web_search":
				params["query"] = arg
			case "clipboard":
				params["text"] = arg
			}
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var result agent.ToolResult
	ipcRes, err := client.Call(callCtx, "tool_call", map[string]any{
		"name":   name,
		"params": params,
	})
	if err == nil {
		if m, ok := ipcRes.(map[string]any); ok {
			if raw, mErr := json.Marshal(m["result"]); mErr == nil {
				_ = json.Unmarshal(raw, &result)
			}
		}
	} else {
		// Standalone fallback
		reg := agent.NewRegistry(getSystemController(), nil)
		var callErr error
		result, callErr = reg.Call(ctx, name, params)
		if callErr != nil {
			return callErr
		}
	}

	if !result.Success {
		return fmt.Errorf("tool execution failed: %s", result.Error)
	}

	if result.Output != "" {
		fmt.Fprintln(out, result.Output)
	} else {
		fmt.Fprintln(out, "Tool executed successfully.")
	}
	return nil
}

func cmdPermissions(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	sub := "list"
	var rest []string
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
		rest = args[1:]
	}

	switch sub {
	case "list", "show", "ls":
		return cmdPermissionsList(ctx, client, out)
	case "grant", "set", "allow", "deny", "ask":
		return cmdPermissionsGrant(ctx, client, sub, rest, out)
	case "revoke", "rm", "delete":
		return cmdPermissionsRevoke(ctx, client, rest, out)
	case "check", "test":
		return cmdPermissionsCheck(ctx, client, rest, out)
	default:
		// If first argument is a tool name (e.g. "cadence permissions fs ..."), treat as check or list
		if len(args) >= 1 && !strings.HasPrefix(args[0], "-") {
			return cmdPermissionsCheck(ctx, client, args, out)
		}
		return fmt.Errorf("unknown permissions subcommand: %s (supported: list, grant, revoke, check)", sub)
	}
}

func cmdPermissionsList(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var defaultLvl string
	var rules []agent.PermissionRule

	res, err := client.Call(callCtx, "permission_list", nil)
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			if dl, ok := m["default_level"].(string); ok {
				defaultLvl = dl
			}
			if raw, mErr := json.Marshal(m["rules"]); mErr == nil {
				_ = json.Unmarshal(raw, &rules)
			}
		}
	} else {
		// Standalone fallback
		pe := agent.NewDefaultPermissionEngine()
		defaultLvl = string(pe.DefaultLevel())
		rules = pe.Rules()
	}

	fmt.Fprintf(out, "Permission Policies (Default: %s):\n", defaultLvl)
	fmt.Fprintf(out, "  %-15s %-12s %-10s %s\n", "TOOL", "ACTION", "LEVEL", "SCOPE")
	fmt.Fprintf(out, "  %-15s %-12s %-10s %s\n", "---------------", "------------", "----------", "------------------------------")

	for _, r := range rules {
		scopeStr := "-"
		if len(r.Scope) > 0 {
			scopeStr = strings.Join(r.Scope, ", ")
		}
		fmt.Fprintf(out, "  %-15s %-12s %-10s %s\n", r.Tool, r.Action, string(r.Level), scopeStr)
	}
	return nil
}

func cmdPermissionsGrant(ctx context.Context, client ipc.Client, subcmd string, args []string, out io.Writer) error {
	var tool string
	action := "*"
	level := "allow"
	var scope []string
	var desc string

	if subcmd == "allow" || subcmd == "deny" || subcmd == "ask" {
		level = subcmd
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--allow" {
			level = "allow"
		} else if arg == "--deny" {
			level = "deny"
		} else if arg == "--ask" {
			level = "ask"
		} else if strings.HasPrefix(arg, "--level=") {
			level = strings.TrimPrefix(arg, "--level=")
		} else if arg == "--level" && i+1 < len(args) {
			level = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "--scope=") {
			for _, p := range strings.Split(strings.TrimPrefix(arg, "--scope="), ",") {
				if t := strings.TrimSpace(p); t != "" {
					scope = append(scope, t)
				}
			}
		} else if arg == "--scope" && i+1 < len(args) {
			for _, p := range strings.Split(args[i+1], ",") {
				if t := strings.TrimSpace(p); t != "" {
					scope = append(scope, t)
				}
			}
			i++
		} else if strings.HasPrefix(arg, "--desc=") {
			desc = strings.TrimPrefix(arg, "--desc=")
		} else if arg == "--desc" && i+1 < len(args) {
			desc = args[i+1]
			i++
		} else if !strings.HasPrefix(arg, "-") {
			if tool == "" {
				tool = arg
			} else if action == "*" {
				action = arg
			}
		}
	}

	if tool == "" {
		return errors.New("usage: cadence permissions grant <tool> [action] [--allow|--deny|--ask] [--scope <paths/cmds>]")
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "permission_grant", map[string]any{
		"tool":        tool,
		"action":      action,
		"level":       level,
		"scope":       scope,
		"description": desc,
	})
	if err != nil {
		fmt.Fprintf(out, "Warning: daemon offline or grant failed (%v). Rule valid locally.\n", err)
	}

	fmt.Fprintf(out, "Permission rule granted: tool=%s action=%s level=%s", tool, action, level)
	if len(scope) > 0 {
		fmt.Fprintf(out, " scope=[%s]", strings.Join(scope, ", "))
	}
	fmt.Fprintln(out, "")
	return nil
}

func cmdPermissionsRevoke(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence permissions revoke <tool> [action]")
	}
	tool := args[0]
	action := "*"
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		action = args[1]
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	res, err := client.Call(callCtx, "permission_revoke", map[string]any{
		"tool":   tool,
		"action": action,
	})
	if err != nil {
		return fmt.Errorf("permission revoke failed: %w", err)
	}

	revoked := true
	if m, ok := res.(map[string]any); ok {
		if r, ok := m["revoked"].(bool); ok {
			revoked = r
		}
	}

	if revoked {
		fmt.Fprintf(out, "Revoked permission rule for tool=%s action=%s\n", tool, action)
	} else {
		fmt.Fprintf(out, "No matching explicit rule found for tool=%s action=%s (using default policy)\n", tool, action)
	}
	return nil
}

func cmdPermissionsCheck(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence permissions check <tool> [action] [param=value...]")
	}
	tool := args[0]
	action := "*"
	params := make(map[string]any)

	idx := 1
	if idx < len(args) && !strings.Contains(args[idx], "=") && !strings.HasPrefix(args[idx], "-") {
		action = args[idx]
		idx++
	}

	for ; idx < len(args); idx++ {
		arg := args[idx]
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			params[parts[0]] = parts[1]
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var decision agent.PermissionDecision
	res, err := client.Call(callCtx, "permission_check", map[string]any{
		"tool":   tool,
		"action": action,
		"params": params,
	})
	if err == nil {
		if raw, mErr := json.Marshal(res); mErr == nil {
			_ = json.Unmarshal(raw, &decision)
		}
	} else {
		// Standalone fallback
		pe := agent.NewDefaultPermissionEngine()
		decision = pe.Check(tool, action, params)
	}

	status := "DENIED"
	if decision.Granted {
		status = "ALLOWED"
	} else if decision.RequiresUser {
		status = "CONFIRMATION_REQUIRED"
	}

	fmt.Fprintf(out, "Permission Check: %s\n", status)
	fmt.Fprintf(out, "  Tool:         %s\n", tool)
	fmt.Fprintf(out, "  Action:       %s\n", action)
	fmt.Fprintf(out, "  Level:        %s\n", string(decision.Level))
	fmt.Fprintf(out, "  Reason:       %s\n", decision.Reason)
	if decision.MatchedRule != nil {
		fmt.Fprintf(out, "  Matched Rule: %s.%s (%s)\n", decision.MatchedRule.Tool, decision.MatchedRule.Action, decision.MatchedRule.Level)
	}
	return nil
}

func cmdPolicy(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "show"
	var rest []string
	if len(args) > 0 {
		action = strings.ToLower(args[0])
		rest = args[1:]
	}

	switch action {
	case "show", "get", "status", "list":
		return cmdPolicyShow(ctx, client, out)
	case "set":
		return cmdPolicySet(ctx, client, rest, out)
	case "eval", "evaluate", "check", "test":
		return cmdPolicyEvaluate(ctx, client, rest, out)
	default:
		// If argument directly matches a valid confirmation mode (e.g. "cadence policy always_ask")
		if agent.IsValidConfirmationMode(action) {
			return cmdPolicySet(ctx, client, []string{action}, out)
		}
		return fmt.Errorf("unknown policy subcommand: %s (supported: show, set, evaluate)", action)
	}
}

func cmdPolicyShow(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	mode := string(agent.ModeAutoApproveSafe)
	desc := agent.ModeDescription(agent.ModeAutoApproveSafe)
	supported := []string{
		string(agent.ModeAlwaysAsk),
		string(agent.ModeAutoApproveSafe),
		string(agent.ModeAutoApproveRisky),
		string(agent.ModeAutoApproveAll),
		string(agent.ModeReadOnly),
	}

	res, err := client.Call(callCtx, "policy_get", nil)
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			if md, ok := m["mode"].(string); ok {
				mode = md
			}
			if d, ok := m["description"].(string); ok {
				desc = d
			}
			if s, ok := m["supported_modes"].([]any); ok {
				var sm []string
				for _, item := range s {
					if str, ok := item.(string); ok {
						sm = append(sm, str)
					}
				}
				if len(sm) > 0 {
					supported = sm
				}
			}
		}
	}

	fmt.Fprintln(out, "Action Confirmation Policy:")
	fmt.Fprintf(out, "  Active Mode: %s\n", mode)
	fmt.Fprintf(out, "  Description: %s\n\n", desc)
	fmt.Fprintln(out, "Supported Modes:")
	for _, m := range supported {
		marker := " "
		if m == mode {
			marker = "*"
		}
		fmt.Fprintf(out, " %s %-20s %s\n", marker, m, agent.ModeDescription(agent.ConfirmationMode(m)))
	}
	return nil
}

func cmdPolicySet(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence policy set <mode> (e.g. always_ask, auto_approve_safe, auto_approve_all, read_only)")
	}
	targetMode := strings.ToLower(strings.TrimSpace(args[0]))
	if !agent.IsValidConfirmationMode(targetMode) {
		return fmt.Errorf("invalid confirmation mode %q (supported: always_ask, auto_approve_safe, auto_approve_risky, auto_approve_all, read_only)", targetMode)
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "policy_set", map[string]any{
		"mode": targetMode,
	})
	if err != nil {
		fmt.Fprintf(out, "Warning: daemon offline (%v). Policy set for local session.\n", err)
	}

	fmt.Fprintf(out, "Action confirmation policy updated to: %s\n", targetMode)
	fmt.Fprintf(out, "  %s\n", agent.ModeDescription(agent.ConfirmationMode(targetMode)))
	return nil
}

func cmdPolicyEvaluate(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: cadence policy evaluate <tool> [action] [param=value...]")
	}
	tool := args[0]
	action := "*"
	params := make(map[string]any)

	idx := 1
	if idx < len(args) && !strings.Contains(args[idx], "=") && !strings.HasPrefix(args[idx], "-") {
		action = args[idx]
		idx++
	}

	for ; idx < len(args); idx++ {
		arg := args[idx]
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			params[parts[0]] = parts[1]
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var decision agent.ConfirmationDecision
	res, err := client.Call(callCtx, "policy_evaluate", map[string]any{
		"tool":   tool,
		"action": action,
		"params": params,
	})
	if err == nil {
		if raw, mErr := json.Marshal(res); mErr == nil {
			_ = json.Unmarshal(raw, &decision)
		}
	} else {
		// Standalone fallback
		cp := agent.DefaultConfirmationPolicy()
		decision = cp.Evaluate(tool, action, params)
	}

	status := "PROHIBITED"
	if decision.Allowed {
		status = "AUTO_APPROVED"
	} else if decision.RequiresConfirmation {
		status = "CONFIRMATION_REQUIRED"
	}

	fmt.Fprintf(out, "Action Policy Decision: %s\n", status)
	fmt.Fprintf(out, "  Tool:                  %s\n", tool)
	fmt.Fprintf(out, "  Action:                %s\n", action)
	fmt.Fprintf(out, "  Risk Level:            %s\n", string(decision.RiskLevel))
	fmt.Fprintf(out, "  Requires Confirmation: %v\n", decision.RequiresConfirmation)
	fmt.Fprintf(out, "  Allowed:               %v\n", decision.Allowed)
	fmt.Fprintf(out, "  Reason:                %s\n", decision.Reason)
	if decision.Prompt != "" {
		fmt.Fprintf(out, "  Prompt:                %s\n", decision.Prompt)
	}
	return nil
}

func cmdRoute(ctx context.Context, cfg config.Config, client ipc.Client, args []string, out io.Writer) error {
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}

	switch action {
	case "list":
		return cmdRouteList(ctx, cfg, client, out)
	case "set", "add":
		if len(args) < 3 {
			return errors.New("usage: cadence route set <app_name> <profile>")
		}
		return cmdRouteSet(ctx, client, args[1], args[2], out)
	case "delete", "remove", "rm":
		if len(args) < 2 {
			return errors.New("usage: cadence route delete <app_name>")
		}
		return cmdRouteDelete(ctx, client, args[1], out)
	case "test":
		if len(args) < 2 {
			return errors.New("usage: cadence route test <app_name>")
		}
		return cmdRouteTest(ctx, cfg, client, args[1], out)
	default:
		return fmt.Errorf("unknown route action: %s (supported: list, set, delete, test)", action)
	}
}

func cmdRouteList(ctx context.Context, cfg config.Config, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	customRoutes := make(map[string]string)
	res, err := client.Call(callCtx, "polish_route_list", nil)
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					customRoutes[k] = s
				}
			}
		}
	} else {
		// Offline fallback to config
		for k, v := range cfg.Polish.AppRoutes {
			customRoutes[k] = v
		}
	}

	fmt.Fprintln(out, "Application Polish Profile Routes:")
	fmt.Fprintln(out, "  Default Category Rules:")
	fmt.Fprintln(out, "    Code & Terminal   -> code")
	fmt.Fprintln(out, "    Medical (EMR/EHR) -> medical")
	fmt.Fprintln(out, "    Legal Systems     -> legal")
	fmt.Fprintln(out, "    Other / Browsers  -> general")

	if len(customRoutes) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "  %-25s  %-20s\n", "APPLICATION", "PROFILE")
		fmt.Fprintf(out, "  %-25s  %-20s\n", "-----------", "-------")
		for app, prof := range customRoutes {
			fmt.Fprintf(out, "  %-25s  %-20s\n", app, prof)
		}
	}
	return nil
}

func cmdRouteSet(ctx context.Context, client ipc.Client, app, profile string, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	params := map[string]any{"app": app, "profile": profile}
	if _, err := client.Call(callCtx, "polish_route_set", params); err != nil {
		return fmt.Errorf("route set failed: %w", err)
	}
	fmt.Fprintf(out, "Route configured: %q -> %q\n", app, profile)
	return nil
}

func cmdRouteDelete(ctx context.Context, client ipc.Client, app string, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	params := map[string]any{"app": app}
	if _, err := client.Call(callCtx, "polish_route_delete", params); err != nil {
		return fmt.Errorf("route delete failed: %w", err)
	}
	fmt.Fprintf(out, "Route for %q removed.\n", app)
	return nil
}

func cmdRouteTest(ctx context.Context, cfg config.Config, client ipc.Client, app string, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	res, err := client.Call(callCtx, "polish_route_test", map[string]any{"app": app})
	var cat string
	var prof string
	if err == nil {
		if m, ok := res.(map[string]any); ok {
			cat, _ = m["category"].(string)
			prof, _ = m["profile"].(string)
		}
	} else {
		// Offline fallback
		c := appcontext.Categorize(app, "", "")
		cat = string(c)
		r := polish.NewRouter(cfg.Polish.AppRoutes)
		prof = r.Route(app, c)
	}

	fmt.Fprintf(out, "Routing Resolution for %q:\n", app)
	fmt.Fprintf(out, "  Category: %s\n", cat)
	fmt.Fprintf(out, "  Profile:  %s\n", prof)
	return nil
}

func cmdUndo(ctx context.Context, client ipc.Client, args []string, stdout io.Writer) error {
	target := "unpolished"
	depth := 1
	var listMode bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--raw", "-r":
			target = "raw"
		case "-l", "--list":
			listMode = true
		case "-d", "--depth":
			if i+1 < len(args) {
				if d, err := strconv.Atoi(args[i+1]); err == nil && d > 0 {
					depth = d
					i++
				}
			}
		case "-h", "--help":
			fmt.Fprintf(stdout, `Usage: cadence undo [flags]

Reverts AI-polished dictation or voice text edits in the active application.

Flags:
  -r, --raw         Revert all the way back to the raw ASR transcript instead of post-processed text
  -d, --depth <n>   Number of successive transformations to undo from the stack (default 1)
  -l, --list        Show recent reversible transformations in the undo stack
  -h, --help        Show this help message
`)
			return nil
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if listMode {
		resp, err := client.Call(callCtx, "undo_list", nil)
		if err != nil {
			return fmt.Errorf("retrieve undo stack: %w", err)
		}
		data, _ := json.Marshal(resp)
		var stack []struct {
			SessionID      string    `json:"session_id"`
			RawText        string    `json:"raw_text"`
			UnpolishedText string    `json:"unpolished_text"`
			PolishedText   string    `json:"polished_text"`
			IsPolished     bool      `json:"is_polished"`
			PastedAt       time.Time `json:"pasted_at"`
			Undone         bool      `json:"undone"`
		}
		_ = json.Unmarshal(data, &stack)

		if len(stack) == 0 {
			fmt.Fprintln(stdout, "Undo Stack is empty (no recent transformations).")
			return nil
		}

		fmt.Fprintln(stdout, "Cadence Reversible Transformation Stack:")
		for i := len(stack) - 1; i >= 0; i-- {
			rec := stack[i]
			status := "Active"
			if rec.Undone {
				status = "Undone"
			}
			fmt.Fprintf(stdout, "  [%d] %s (Session: %s, Time: %s)\n",
				i+1, status, rec.SessionID, rec.PastedAt.Format("15:04:05"))
			fmt.Fprintf(stdout, "      Original:   %q\n", rec.UnpolishedText)
			fmt.Fprintf(stdout, "      Transformed: %q\n", rec.PolishedText)
		}
		return nil
	}

	resp, err := client.Call(callCtx, "undo", map[string]any{
		"target": target,
		"depth":  depth,
	})
	if err != nil {
		return err
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	var res struct {
		SessionID    string `json:"session_id"`
		RevertedFrom string `json:"reverted_from"`
		RevertedTo   string `json:"reverted_to"`
		Target       string `json:"target"`
		Remaining    int    `json:"remaining"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if res.Target == "raw" {
		fmt.Fprintf(stdout, "Reverted to raw ASR transcript [%s]:\n  From: %q\n  To:   %q\n",
			res.SessionID, res.RevertedFrom, res.RevertedTo)
	} else {
		fmt.Fprintf(stdout, "Reverted AI polish transformation [%s]:\n  From: %q\n  To:   %q\n",
			res.SessionID, res.RevertedFrom, res.RevertedTo)
	}
	if res.Remaining > 0 {
		fmt.Fprintf(stdout, "Remaining reversible transformations in stack: %d\n", res.Remaining)
	}
	return nil
}

func cmdCorrect(ctx context.Context, cfg config.Config, client ipc.Client, args []string, stdout io.Writer) error {
	var app string
	var textParts []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintf(stdout, `Usage: cadence correct <text> [--app <application>]

Detects and fixes obvious ASR acoustic misrecognitions and homophones using application context.

Flags:
  -a, --app <name>   Target application context (e.g. "VS Code", "Terminal", "Epic", "Clio")
  -h, --help         Show this help message
`)
			return nil
		case "-a", "--app":
			if i+1 < len(args) {
				app = args[i+1]
				i++
			}
		default:
			textParts = append(textParts, args[i])
		}
	}

	text := strings.Join(textParts, " ")
	if strings.TrimSpace(text) == "" {
		return errors.New("empty text: please provide text to correct")
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var report postprocess.CorrectionReport
	resp, err := client.Call(callCtx, "correct", map[string]any{
		"text": text,
		"app":  app,
	})

	if err == nil {
		data, _ := json.Marshal(resp)
		_ = json.Unmarshal(data, &report)
	} else {
		// Offline fallback
		corrector := postprocess.NewCorrector(cfg.Post.Corrections)
		report = corrector.Correct(text, app)
	}

	appDisplay := report.App
	if appDisplay == "" {
		appDisplay = "Default"
	}

	fmt.Fprintf(stdout, "ASR Self-Correction Resolution:\n")
	fmt.Fprintf(stdout, "  Original:  %s\n", report.Original)
	fmt.Fprintf(stdout, "  App:       %s (Category: %s)\n", appDisplay, report.Category)
	fmt.Fprintf(stdout, "  Corrected: %s\n", report.Corrected)
	if report.CorrectionsApplied > 0 {
		fmt.Fprintf(stdout, "  Changes:   %d correction(s) applied\n", report.CorrectionsApplied)
	} else {
		fmt.Fprintf(stdout, "  Changes:   None (text is clean)\n")
	}
	return nil
}

func cmdSelection(ctx context.Context, client ipc.Client, args []string, stdout io.Writer) error {
	var copyToClipboard bool
	var jsonOutput bool

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprintf(stdout, `Usage: cadence selection [flags]

Captures highlighted or selected text from the currently focused desktop application.

Flags:
  -c, --copy   Copy captured selected text to the system clipboard
  -j, --json   Output selection result as JSON
  -h, --help   Show this help message
`)
			return nil
		case "-c", "--copy":
			copyToClipboard = true
		case "-j", "--json":
			jsonOutput = true
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.Call(callCtx, "selection_get", nil)
	if err != nil {
		return fmt.Errorf("capture selection: %w", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	var res struct {
		Text        string `json:"text"`
		Method      string `json:"method"`
		Application string `json:"application,omitempty"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if jsonOutput {
		fmt.Fprintln(stdout, string(data))
	} else {
		appStr := res.Application
		if appStr == "" {
			appStr = "Active Application"
		}
		fmt.Fprintf(stdout, "Captured Selection from %s (via %s):\n---\n%s\n---\n",
			appStr, res.Method, res.Text)
	}

	if copyToClipboard {
		if err := paste.SetSystemClipboard(res.Text); err != nil {
			return fmt.Errorf("copy to clipboard: %w", err)
		}
		fmt.Fprintln(stdout, "Copied selection to clipboard.")
	}

	return nil
}

func cmdParse(ctx context.Context, cfg config.Config, client ipc.Client, args []string, stdout io.Writer) error {
	var jsonOutput bool
	var textParts []string

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprintf(stdout, `Usage: cadence parse <voice-command-text> [flags]

Parse voice edit instruction into structured intent and parameters.

Flags:
  -j, --json   Output parsed instruction as JSON
  -h, --help   Show this help message
`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		default:
			textParts = append(textParts, arg)
		}
	}

	text := strings.Join(textParts, " ")
	if strings.TrimSpace(text) == "" {
		return errors.New("empty text: please provide a voice instruction to parse")
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var instr edit.Instruction
	resp, err := client.Call(callCtx, "edit_parse", map[string]any{
		"text": text,
	})

	if err == nil {
		data, _ := json.Marshal(resp)
		_ = json.Unmarshal(data, &instr)
	} else {
		// Offline fallback
		parser := edit.NewParser(nil)
		instr = parser.Parse(ctx, text)
	}

	if jsonOutput {
		data, err := json.MarshalIndent(instr, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(data))
		return nil
	}

	fmt.Fprintln(stdout, "Voice Instruction Parse Result:")
	fmt.Fprintf(stdout, "  Raw:         %s\n", instr.Raw)
	fmt.Fprintf(stdout, "  Intent:      %s\n", instr.Intent)
	fmt.Fprintf(stdout, "  Is Edit:     %v\n", instr.IsEdit)
	if instr.TargetLanguage != "" {
		fmt.Fprintf(stdout, "  Target Lang: %s\n", instr.TargetLanguage)
	}
	if instr.Tone != "" {
		fmt.Fprintf(stdout, "  Tone:        %s\n", instr.Tone)
	}
	if instr.Format != "" {
		fmt.Fprintf(stdout, "  Format:      %s\n", instr.Format)
	}
	if instr.Prompt != "" {
		fmt.Fprintf(stdout, "  Prompt:      %s\n", instr.Prompt)
	}
	fmt.Fprintf(stdout, "  Confidence:  %.2f\n", instr.Confidence)
	return nil
}

func cmdEdit(ctx context.Context, cfg config.Config, client ipc.Client, args []string, stdout io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "apply":
			if len(args) < 2 {
				return errors.New("missing preview ID: cadence edit apply <id>")
			}
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			resp, err := client.Call(callCtx, "edit_apply", map[string]any{"id": args[1]})
			if err != nil {
				return fmt.Errorf("apply preview: %w", err)
			}
			data, _ := json.Marshal(resp)
			var sess edit.PreviewSession
			_ = json.Unmarshal(data, &sess)
			fmt.Fprintf(stdout, "Applied and pasted edit proposal [%s] into active application.\n", sess.ID)
			return nil

		case "reject":
			if len(args) < 2 {
				return errors.New("missing preview ID: cadence edit reject <id>")
			}
			callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			resp, err := client.Call(callCtx, "edit_reject", map[string]any{"id": args[1]})
			if err != nil {
				return fmt.Errorf("reject preview: %w", err)
			}
			data, _ := json.Marshal(resp)
			var sess edit.PreviewSession
			_ = json.Unmarshal(data, &sess)
			fmt.Fprintf(stdout, "Rejected edit proposal [%s].\n", sess.ID)
			return nil

		case "list", "pending":
			callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			resp, err := client.Call(callCtx, "edit_preview_list", nil)
			if err != nil {
				return fmt.Errorf("list pending previews: %w", err)
			}
			data, _ := json.Marshal(resp)
			var list []*edit.PreviewSession
			_ = json.Unmarshal(data, &list)
			if len(list) == 0 {
				fmt.Fprintln(stdout, "No pending edit proposals.")
				return nil
			}
			fmt.Fprintf(stdout, "Pending Edit Proposals (%d):\n", len(list))
			for _, s := range list {
				fmt.Fprintf(stdout, "  [%s] Intent: %s (Time: %s)\n", s.ID, s.Instruction.Intent, s.CreatedAt.Format("15:04:05"))
				fmt.Fprintf(stdout, "      Original:  %q\n", s.OriginalText)
				fmt.Fprintf(stdout, "      Candidate: %q\n", s.CandidateText)
			}
			return nil
		}
	}

	var jsonOutput bool
	var dryRun bool
	var previewMode bool
	var explicitText string
	var instructionParts []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			fmt.Fprintf(stdout, `Usage: cadence edit <voice-instruction> [flags]
       cadence edit apply <preview-id>
       cadence edit reject <preview-id>
       cadence edit list

Transform selected text in active application using voice edit instructions.

Flags:
  -p, --preview       Generate an interactive review proposal with diff without pasting
  -t, --text <text>   Provide custom text to edit instead of capturing active selection
  -d, --dry-run       Preview transformation without pasting into target application
  -j, --json          Output result as JSON
  -h, --help          Show this help message
`)
			return nil
		case "-j", "--json":
			jsonOutput = true
		case "-d", "--dry-run":
			dryRun = true
		case "-p", "--preview":
			previewMode = true
		case "-t", "--text":
			if i+1 < len(args) {
				explicitText = args[i+1]
				i++
			}
		default:
			instructionParts = append(instructionParts, arg)
		}
	}

	instruction := strings.Join(instructionParts, " ")
	if strings.TrimSpace(instruction) == "" {
		return errors.New("empty instruction: please provide a voice edit instruction (e.g. 'translate this to French', 'make this shorter')")
	}

	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if previewMode {
		params := map[string]any{"instruction": instruction}
		if explicitText != "" {
			params["text"] = explicitText
		}
		resp, err := client.Call(callCtx, "edit_preview", params)
		if err != nil {
			return fmt.Errorf("generate preview: %w", err)
		}
		data, _ := json.Marshal(resp)
		var sess edit.PreviewSession
		_ = json.Unmarshal(data, &sess)

		if jsonOutput {
			out, _ := json.MarshalIndent(sess, "", "  ")
			fmt.Fprintln(stdout, string(out))
			return nil
		}

		fmt.Fprintf(stdout, "Voice Text Edit Proposal [%s]:\n", sess.ID)
		fmt.Fprintf(stdout, "  Intent:      %s\n", sess.Instruction.Intent)
		fmt.Fprintln(stdout, "  Diff:")
		fmt.Fprintln(stdout, sess.DiffSummary)
		fmt.Fprintln(stdout, "\nActions:")
		fmt.Fprintf(stdout, "  Apply:   cadence edit apply %s\n", sess.ID)
		fmt.Fprintf(stdout, "  Reject:  cadence edit reject %s\n", sess.ID)
		return nil
	}

	params := map[string]any{
		"instruction": instruction,
		"dry_run":     dryRun,
	}
	if explicitText != "" {
		params["text"] = explicitText
	}

	var result struct {
		OriginalText string           `json:"original_text"`
		EditedText   string           `json:"edited_text"`
		Instruction  edit.Instruction `json:"instruction"`
		DryRun       bool             `json:"dry_run"`
		Pasted       bool             `json:"pasted"`
		Model        string           `json:"model,omitempty"`
		LatencyMs    int64            `json:"latency_ms,omitempty"`
	}

	resp, err := client.Call(callCtx, "edit_execute", params)
	if err == nil {
		data, _ := json.Marshal(resp)
		_ = json.Unmarshal(data, &result)
	} else {
		// Offline fallback
		text := explicitText
		if strings.TrimSpace(text) == "" {
			capturer := selection.NewDefaultCapturer()
			selRes, cErr := capturer.Capture(ctx)
			if cErr != nil {
				return fmt.Errorf("capture selected text: %w", cErr)
			}
			text = selRes.Text
		}
		if strings.TrimSpace(text) == "" {
			return errors.New("no text provided and no active text selection found")
		}

		parser := edit.NewParser(nil)
		instr := parser.Parse(ctx, instruction)

		var polisher polish.Polisher = polish.NewNoopPolisher()
		if cfg.Polish.Enabled {
			switch cfg.Polish.Backend {
			case "ollama":
				polisher = polish.NewOllamaPolisher(
					polish.WithOllamaEndpoint(cfg.Polish.Endpoint),
					polish.WithOllamaModel(cfg.Polish.Model),
				)
			case "openai":
				polisher = polish.NewOpenAIPolisher(
					polish.WithOpenAIEndpoint(cfg.Polish.Endpoint),
					polish.WithOpenAIModel(cfg.Polish.Model),
					polish.WithOpenAIAPIKey(cfg.Polish.APIKey),
				)
			}
		}

		editor := edit.NewEditor(polisher)
		editRes, edErr := editor.Execute(ctx, text, instr)
		if edErr != nil {
			return fmt.Errorf("execute edit transformation: %w", edErr)
		}

		pasted := false
		if !dryRun {
			paster := paste.NewDefaultCascade()
			if _, pErr := paster.Paste(editRes.EditedText); pErr == nil {
				pasted = true
			}
		}

		result = struct {
			OriginalText string           `json:"original_text"`
			EditedText   string           `json:"edited_text"`
			Instruction  edit.Instruction `json:"instruction"`
			DryRun       bool             `json:"dry_run"`
			Pasted       bool             `json:"pasted"`
			Model        string           `json:"model,omitempty"`
			LatencyMs    int64            `json:"latency_ms,omitempty"`
		}{
			OriginalText: editRes.OriginalText,
			EditedText:   editRes.EditedText,
			Instruction:  editRes.Instruction,
			DryRun:       dryRun,
			Pasted:       pasted,
			Model:        editRes.Model,
			LatencyMs:    editRes.Latency.Milliseconds(),
		}
	}

	if jsonOutput {
		data, jErr := json.MarshalIndent(result, "", "  ")
		if jErr != nil {
			return jErr
		}
		fmt.Fprintln(stdout, string(data))
		return nil
	}

	fmt.Fprintln(stdout, "Voice Text Edit Result:")
	fmt.Fprintf(stdout, "  Intent:      %s\n", result.Instruction.Intent)
	if result.Instruction.TargetLanguage != "" {
		fmt.Fprintf(stdout, "  Target Lang: %s\n", result.Instruction.TargetLanguage)
	}
	if result.Instruction.Tone != "" {
		fmt.Fprintf(stdout, "  Tone:        %s\n", result.Instruction.Tone)
	}
	fmt.Fprintf(stdout, "  Original:    %s\n", result.OriginalText)
	fmt.Fprintf(stdout, "  Transformed: %s\n", result.EditedText)
	if result.DryRun {
		fmt.Fprintln(stdout, "  Status:      Dry run (not pasted)")
	} else if result.Pasted {
		fmt.Fprintln(stdout, "  Status:      Applied & pasted into active window")
	} else {
		fmt.Fprintln(stdout, "  Status:      Ready")
	}
	return nil
}

func formatMeetingDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func cmdMeeting(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	action := "status"
	var rest []string
	if len(args) > 0 {
		action = strings.ToLower(args[0])
		rest = args[1:]
	}

	switch action {
	case "start":
		return cmdMeetingStart(ctx, client, rest, out)
	case "stop":
		return cmdMeetingStop(ctx, client, out)
	case "pause":
		return cmdMeetingPause(ctx, client, out)
	case "resume":
		return cmdMeetingResume(ctx, client, out)
	case "status":
		return cmdMeetingStatus(ctx, client, out)
	case "list", "ls":
		return cmdMeetingList(ctx, client, rest, out)
	case "show", "get":
		return cmdMeetingShow(ctx, client, rest, out)
	case "delete", "rm":
		return cmdMeetingDelete(ctx, client, rest, out)
	case "help", "-h", "--help":
		cmdMeetingUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown meeting subcommand: %s (supported: start, stop, pause, resume, status, list, show, delete)", action)
	}
}

func cmdMeetingStart(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	title := ""
	if len(args) > 0 {
		title = strings.Join(args, " ")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	params := map[string]any{}
	if title != "" {
		params["title"] = title
	}
	resp, err := client.Call(callCtx, "meeting_start", params)
	if err != nil {
		return fmt.Errorf("failed to start meeting recording: %w", err)
	}

	resMap, ok := resp.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected meeting_start response: %v", resp)
	}
	sessID, _ := resMap["id"].(string)
	sessTitle, _ := resMap["title"].(string)
	audioPath, _ := resMap["audio_path"].(string)

	fmt.Fprintln(out, "Continuous meeting recording started.")
	fmt.Fprintf(out, "Session ID: %s\n", sessID)
	if sessTitle != "" {
		fmt.Fprintf(out, "Title:      %s\n", sessTitle)
	}
	if audioPath != "" {
		fmt.Fprintf(out, "Audio File: %s\n", audioPath)
	}
	return nil
}

func cmdMeetingStop(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.Call(callCtx, "meeting_stop", nil)
	if err != nil {
		return fmt.Errorf("failed to stop meeting recording: %w", err)
	}

	resMap, ok := resp.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected meeting_stop response: %v", resp)
	}
	sessID, _ := resMap["id"].(string)
	title, _ := resMap["title"].(string)
	audioPath, _ := resMap["audio_path"].(string)
	durMs := int64(0)
	if d, ok := resMap["duration_ms"].(float64); ok {
		durMs = int64(d)
	}
	var segCount int
	if segs, ok := resMap["segments"].([]any); ok {
		segCount = len(segs)
	}

	fmt.Fprintln(out, "Meeting recording stopped.")
	fmt.Fprintf(out, "Session ID: %s\n", sessID)
	if title != "" {
		fmt.Fprintf(out, "Title:      %s\n", title)
	}
	fmt.Fprintf(out, "Duration:   %s\n", formatMeetingDuration(durMs))
	fmt.Fprintf(out, "Segments:   %d\n", segCount)
	if audioPath != "" {
		fmt.Fprintf(out, "Audio File: %s\n", audioPath)
	}
	return nil
}

func cmdMeetingPause(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "meeting_pause", nil)
	if err != nil {
		return fmt.Errorf("failed to pause meeting recording: %w", err)
	}
	fmt.Fprintln(out, "Meeting recording paused.")
	return nil
}

func cmdMeetingResume(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "meeting_resume", nil)
	if err != nil {
		return fmt.Errorf("failed to resume meeting recording: %w", err)
	}
	fmt.Fprintln(out, "Meeting recording resumed.")
	return nil
}

func cmdMeetingStatus(ctx context.Context, client ipc.Client, out io.Writer) error {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.Call(callCtx, "meeting_status", nil)
	if err != nil {
		return fmt.Errorf("failed to get meeting status: %w", err)
	}

	resMap, ok := resp.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected meeting_status response: %v", resp)
	}

	isRec, _ := resMap["is_recording"].(bool)
	isPaused, _ := resMap["is_paused"].(bool)
	if !isRec && !isPaused {
		fmt.Fprintln(out, "No active meeting recording.")
		return nil
	}

	sessMap, _ := resMap["active_session"].(map[string]any)
	sessID := ""
	title := ""
	audioPath := ""
	if sessMap != nil {
		sessID, _ = sessMap["id"].(string)
		title, _ = sessMap["title"].(string)
		audioPath, _ = sessMap["audio_path"].(string)
	}

	statusStr := "recording"
	if isPaused {
		statusStr = "paused"
	}

	elapsedMs := int64(0)
	if e, ok := resMap["elapsed_ms"].(float64); ok {
		elapsedMs = int64(e)
	}
	segCount := 0
	if c, ok := resMap["segment_count"].(float64); ok {
		segCount = int(c)
	}

	fmt.Fprintf(out, "Meeting Session: %s\n", sessID)
	if title != "" {
		fmt.Fprintf(out, "Title:           %s\n", title)
	}
	fmt.Fprintf(out, "Status:          %s\n", statusStr)
	fmt.Fprintf(out, "Elapsed:         %s\n", formatMeetingDuration(elapsedMs))
	fmt.Fprintf(out, "Segments:        %d\n", segCount)
	if audioPath != "" {
		fmt.Fprintf(out, "Audio File:      %s\n", audioPath)
	}
	return nil
}

func cmdMeetingList(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	limit := 20
	if len(args) > 0 {
		if l, err := strconv.Atoi(args[0]); err == nil && l > 0 {
			limit = l
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.Call(callCtx, "meeting_list", map[string]any{"limit": limit})
	if err != nil {
		return fmt.Errorf("failed to list meetings: %w", err)
	}

	items, ok := resp.([]any)
	if !ok || len(items) == 0 {
		fmt.Fprintln(out, "No recorded meetings found.")
		return nil
	}

	fmt.Fprintf(out, "%-16s  %-25s  %-10s  %-10s  %s\n", "ID", "TITLE", "DURATION", "STATUS", "DATE")
	fmt.Fprintf(out, "%-16s  %-25s  %-10s  %-10s  %s\n", "----------------", "-------------------------", "----------", "----------", "----------------")

	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		title, _ := m["title"].(string)
		status, _ := m["state"].(string)
		durMs := int64(0)
		if d, ok := m["duration_ms"].(float64); ok {
			durMs = int64(d)
		}
		createdAt := ""
		if ca, ok := m["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, ca); err == nil {
				createdAt = t.Local().Format("2006-01-02 15:04")
			} else {
				createdAt = ca
			}
		}

		if len(title) > 25 {
			title = title[:22] + "..."
		}
		if len(id) > 16 {
			id = id[:13] + "..."
		}

		fmt.Fprintf(out, "%-16s  %-25s  %-10s  %-10s  %s\n", id, title, formatMeetingDuration(durMs), status, createdAt)
	}
	return nil
}

func cmdMeetingShow(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing meeting id (usage: cadence meeting show <id>)")
	}
	id := args[0]

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.Call(callCtx, "meeting_get", map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("failed to get meeting: %w", err)
	}

	resMap, ok := resp.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected meeting_get response: %v", resp)
	}

	mtg, _ := resMap["meeting"].(map[string]any)
	if mtg == nil {
		return errors.New("meeting not found")
	}

	title, _ := mtg["title"].(string)
	state, _ := mtg["state"].(string)
	audioPath, _ := mtg["audio_path"].(string)
	durMs := int64(0)
	if d, ok := mtg["duration_ms"].(float64); ok {
		durMs = int64(d)
	}
	createdAt := ""
	if ca, ok := mtg["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, ca); err == nil {
			createdAt = t.Local().Format("2006-01-02 15:04:05")
		} else {
			createdAt = ca
		}
	}

	fmt.Fprintf(out, "Meeting:    %s (%s)\n", title, id)
	fmt.Fprintf(out, "Status:     %s\n", state)
	fmt.Fprintf(out, "Date:       %s\n", createdAt)
	fmt.Fprintf(out, "Duration:   %s\n", formatMeetingDuration(durMs))
	if audioPath != "" {
		fmt.Fprintf(out, "Audio File: %s\n", audioPath)
	}

	segs, _ := resMap["segments"].([]any)
	fmt.Fprintf(out, "\nSegments (%d):\n", len(segs))
	if len(segs) == 0 {
		fmt.Fprintln(out, "  (No segments transcribed yet)")
	} else {
		for _, s := range segs {
			sMap, ok := s.(map[string]any)
			if !ok {
				continue
			}
			startMs := int64(0)
			if st, ok := sMap["start_ms"].(float64); ok {
				startMs = int64(st)
			}
			endMs := int64(0)
			if en, ok := sMap["end_ms"].(float64); ok {
				endMs = int64(en)
			}
			text, _ := sMap["text"].(string)
			if text == "" {
				text = "..."
			}
			fmt.Fprintf(out, "  [%s - %s] %s\n", formatMeetingDuration(startMs), formatMeetingDuration(endMs), text)
		}
	}
	return nil
}

func cmdMeetingDelete(ctx context.Context, client ipc.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing meeting id (usage: cadence meeting delete <id>)")
	}
	id := args[0]

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Call(callCtx, "meeting_delete", map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("failed to delete meeting: %w", err)
	}
	fmt.Fprintf(out, "Meeting %s deleted.\n", id)
	return nil
}

func cmdMeetingUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: cadence meeting <subcommand> [arguments]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  start [title]        Start a continuous meeting recording session")
	fmt.Fprintln(out, "  stop                 Stop and finalize the active meeting recording")
	fmt.Fprintln(out, "  pause                Pause incoming audio recording")
	fmt.Fprintln(out, "  resume               Resume paused meeting recording")
	fmt.Fprintln(out, "  status               Show current meeting recording status and duration")
	fmt.Fprintln(out, "  list [limit]         List past meeting recordings")
	fmt.Fprintln(out, "  show <id>            Show details and transcribed segments for a meeting")
	fmt.Fprintln(out, "  delete <id>          Delete a meeting record and its audio segments")
}
