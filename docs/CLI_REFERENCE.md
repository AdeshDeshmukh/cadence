# Cadence CLI Reference

Complete reference for the `cadence` command-line interface.

```bash
cadence <command> [arguments] [flags]
```

---

## 1. Core Daemon & Dictation

### `status`
Displays daemon operational health, socket path, version, and active session state.
```bash
cadence status
```

### `dictate [start|stop]`
Controls a live voice dictation session.
```bash
# Start recording
cadence dictate start

# Stop recording, transcribe, format, and paste into active application
cadence dictate stop
```

### `history [limit]`
Lists recent saved transcription sessions and transcripts.
```bash
cadence history 10
```

### `recover [flags]`
Recovers text from the most recent cancelled or crashed session.
```bash
cadence recover
cadence recover --copy
```

---

## 2. Vocabulary & Snippets

### `vocab <subcommand>`
Manages custom developer vocabulary, acronyms, and phonetic corrections.
```bash
cadence vocab list
cadence vocab add "Kubernetes" "k8s"
cadence vocab remove "Kubernetes"
cadence vocab export vocab.json
cadence vocab import vocab.json
```

### `snippet <subcommand>`
Manages voice templates with dynamic variable expansion (`{{date}}`, `{{time}}`, `{{clipboard}}`, `{{cursor}}`).
```bash
cadence snippet list
cadence snippet add "log-err" "log.Printf(\"[ERROR] %v: %v\", err, {{cursor}})"
cadence snippet test "log-err"
cadence snippet remove "log-err"
```

---

## 3. AI Polish & Profiles

### `profile <subcommand>`
Configures prompt profiles for specialized dictation formatting.
```bash
cadence profile list
cadence profile show developer
cadence profile set developer --model "qwen2.5-coder:7b" --temperature 0.2
cadence profile delete custom-profile
```

### `route <subcommand>`
Routes active applications to dedicated prompt profiles.
```bash
cadence route list
cadence route set "com.microsoft.VSCode" "developer"
cadence route delete "com.microsoft.VSCode"
cadence route test "com.microsoft.VSCode"
```

### `polish <text>`
Formats raw transcribed speech using local or configured LLMs.
```bash
cadence polish "function calculate total array items return sum"
```

### `undo [--raw]`
Reverts the last AI polish transformation back to raw post-processed speech.
```bash
cadence undo
cadence undo --raw
```

### `correct <text>`
Detects and repairs phonetic and acoustic speech recognition errors using language context.
```bash
cadence correct "their going to there house"
```

---

## 4. Developer & Edit Mode

### `selection [flags]`
Captures highlighted text from the active foreground application.
```bash
cadence selection
cadence selection --format json
```

### `parse <text>`
Parses voice edit commands into structured intent and parameters.
```bash
cadence parse "replace username with account_id in the selected line"
```

### `edit <instruction>`
Executes natural language voice edits on selected text in your editor.
```bash
cadence edit "sort imports alphabetically"
cadence edit "wrap function in try catch"
```

### `cmd <instruction>`
Translates spoken commands into executable shell commands.
```bash
cadence cmd "build docker container with tag v1.0 and push"
```

### `exec <command>`
Executes shell commands with interactive safety inspection.
```bash
cadence exec "git status"
cadence exec --dry-run "rm -rf ./tmp"
```

### `risk <command>`
Audits and classifies command safety level (`Safe`, `Risky`, `Destructive`).
```bash
cadence risk "ls -la"
cadence risk "npm publish"
cadence risk "rm -rf /"
```

### `git <commit|pr>`
Generates git commit messages or PR descriptions based on repository diffs.
```bash
cadence git commit
cadence git commit --amend
cadence git pr
```

### `code [flags] <words>`
Transforms words into language-idiomatic identifiers.
```bash
cadence code --case snake "get user profile"    # get_user_profile
cadence code --case camel "get user profile"    # getUserProfile
cadence code --case pascal "user account dto"   # UserAccountDto
cadence code --case kebab "api v1 users"        # api-v1-users
```

---

## 5. System Control & Operating Modes

### `sys <subcommand>`
Controls operating system windows, applications, screenshots, and volume.
```bash
cadence sys app "Terminal"
cadence sys window minimize
cadence sys screenshot --fullscreen
cadence sys volume set 50
```

### `mode [name]`
Displays or changes the active operating mode (`dictation`, `code`, `command`, `agent`).
```bash
cadence mode
cadence mode code
```

### `control <phrase>`
Executes voice self-control commands.
```bash
cadence control "switch to command mode"
cadence control "mute microphone"
```

---

## 6. AI Agent, Tools & MCP

### `agent [flags] <goal>`
Runs an autonomous agent loop that plans, calls tools, observes outputs, and delivers results.
```bash
cadence agent "Find all files referencing old API endpoint and report line numbers"
```

### `tools [list|schema]`
Inspects registered tools and their JSON schemas.
```bash
cadence tools list
cadence tools schema fs
```

### `tool <name> [k=v...]`
Directly invokes an agent tool.
```bash
cadence tool fs action=list path=.
cadence tool git action=status
```

### `permissions [list|grant|revoke|check]`
Manages per-tool execution permission policies (`allow`, `deny`, `ask`).
```bash
cadence permissions list
cadence permissions grant fs write
cadence permissions revoke shell exec
```

### `mcp [serve|info]`
Provides Model Context Protocol (MCP) server for integration with Claude Code, Cursor, and Antigravity.
```bash
cadence mcp info
cadence mcp serve
```

### `serve [flags]`
Starts standalone HTTP REST, WebSocket, and Server-Sent Event (SSE) server.
```bash
cadence serve --port 8080 --bind 127.0.0.1
```

---

## 7. Meeting Transcription & Knowledge Base

### `meeting <subcommand>`
Continuous recording, speaker diarization, and automated meeting summaries.
```bash
cadence meeting start --title "Sprint Planning"
cadence meeting status
cadence meeting pause
cadence meeting resume
cadence meeting stop
cadence meeting list
cadence meeting show <id>
cadence meeting diarize <id>
cadence meeting summarize <id>
```

### `note <subcommand>`
Knowledge base note management.
```bash
cadence note create --title "API Design" "Endpoints must follow REST standards."
cadence note list
cadence note show <id>
cadence note from-meeting <meeting-id>
```

### `search <query>`
Full-text (FTS5) or vector semantic search across notes, transcripts, and meetings.
```bash
cadence search "database migration"
cadence search --semantic "how to configure ssl"
```

### `export <type> <id>`
Exports meetings, transcripts, or notes to Markdown, SRT, VTT, JSON, or TXT.
```bash
cadence export markdown mtg-12345 -o summary.md
cadence export srt mtg-12345 -o subtitles.srt
```

---

## 8. Security, Service & Diagnostics

### `secret <subcommand>`
Manages API keys encrypted in the operating system keychain.
```bash
cadence secret set OPENAI_API_KEY
cadence secret get OPENAI_API_KEY
cadence secret list
cadence secret delete OPENAI_API_KEY
```

### `model <subcommand>`
Verifies cryptographic SHA256 integrity of local speech and VAD models.
```bash
cadence model list
cadence model verify whisper-base
cadence model info whisper-base
```

### `privacy <subcommand>`
Audits and manages privacy settings.
```bash
cadence privacy status
cadence privacy audit
cadence privacy set save_transcripts false
cadence privacy purge
```

### `service <subcommand>`
Controls OS background service registration (`launchd` on macOS, `systemd` on Linux).
```bash
cadence service status
cadence service install
cadence service uninstall
cadence service definition
cadence service crashes
```

### `doctor`
Runs comprehensive system diagnostics.
```bash
cadence doctor
```

### `audit`
Executes dependency security and supply-chain vulnerability audit.
```bash
cadence audit
```

### `version`
Displays Cadence release version and architecture details.
```bash
cadence version
```
