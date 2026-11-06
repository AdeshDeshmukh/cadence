package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/config"
	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/selection"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
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
