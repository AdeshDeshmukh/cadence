package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
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
