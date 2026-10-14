# Cadence

> **Free, open-source, on-device AI voice interface for developers.**

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://golang.org)
[![Status](https://img.shields.io/badge/status-pre--alpha-orange)](https://github.com/AdeshDeshmukh/cadence/releases)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

Cadence is a **local-first, cross-platform voice interface** that goes far beyond dictation. Speak naturally — Cadence understands whether you want to dictate text, edit existing content, run a developer workflow, control your computer, or invoke an AI agent.

---

## Vision

> **Voice → Understanding → Context → Action**

```
Speak
  ↓
VAD (Silero)
  ↓
ASR (Whisper / Parakeet)
  ↓
Intent Router
  ├── Dictate    → format → paste
  ├── Edit       → transform selected text
  ├── Command    → structured desktop action
  ├── Developer  → code / terminal / git
  └── Agent      → multi-step tool loop
  ↓
Context Engine (app / file / terminal / git / URL)
  ↓
Model Router (local LLM / BYOK cloud)
  ↓
Output
```

No cloud dependency. No mandatory account. No transcript sent anywhere you didn't approve.

---

## Why Cadence

| Capability | Wispr Flow | Superwhisper | **Cadence** |
|---|---|---|---|
| macOS support | ✅ | ✅ | ✅ |
| Linux support | ❌ | ❌ | ✅ |
| Windows | ❌ | ❌ | 🔄 Planned |
| Local ASR | ✅ | ✅ | ✅ |
| Streaming transcription | ✅ | ❌ | ✅ |
| Voice editing | ✅ | ❌ | ✅ |
| Developer Mode (IDE + terminal) | ❌ | ❌ | ✅ |
| Command Mode | ❌ | ❌ | ✅ |
| Agent Mode | ❌ | ❌ | ✅ |
| Meeting transcription + diarization | ❌ | ❌ | ✅ |
| CLI-first | ❌ | ❌ | ✅ |
| Plugin / MCP API | ❌ | ❌ | ✅ |
| Fully open source | ❌ | ❌ | ✅ Apache 2.0 |

---

## Architecture

Cadence runs as a **background daemon** (Go) with Python ML workers. Clients — CLI, Desktop UI (Tauri), REST/WebSocket — all talk to the daemon over a local Unix socket.

```
┌─────────────────────────────────────────────────┐
│               cadence-daemon (Go)                │
│                                                  │
│  AudioCapture → VAD → ASR → Intent → Context    │
│                                  ↓               │
│                        Model / Tool Router       │
│                                  ↓               │
│             PostProcess / Action / Paste         │
│                                                  │
│  IPC: Unix Socket · JSON-RPC 2.0                │
└─────────────┬──────────────────────┬─────────────┘
              │                      │
         Desktop UI               CLI / REST
         (Tauri)                  (Go)
              │                      │
         Python ML Workers
         Whisper · Parakeet · Silero VAD
```

---

## Technology Stack

| Layer | Technology |
|---|---|
| Daemon / Core | Go 1.23+ |
| ML Workers | Python 3.12 + faster-whisper + Silero VAD |
| Desktop UI | Tauri 2.0 + React + TypeScript |
| CLI | Go + Cobra |
| IPC | JSON-RPC 2.0 over Unix socket |
| Config | TOML |
| Storage | SQLite |
| Release | GoReleaser |

---

## Quick Start

> ⚠️ Cadence is in **pre-alpha**. This section will be updated as the project matures.

```bash
# Clone
git clone https://github.com/AdeshDeshmukh/cadence
cd cadence

# Build
make build

# Run the daemon
./cadenced

# Start dictating
./cadence dictate
```

---

## CLI

```
cadence dictate                          # Start a voice session
cadence transcribe meeting.mp3           # Transcribe a file
cadence transcribe *.mp3 --format srt   # Batch transcribe
cadence recover                          # Recover last cancelled session
cadence history --search "kubernetes"    # Search transcripts
cadence vocabulary add "gRPC" "gRPC"    # Add custom vocabulary
cadence mode list                        # List available modes
cadence model list                       # List available ASR models
cadence benchmark asr                    # Run ASR benchmarks
cadence daemon status                    # Check daemon health
cadence doctor                           # Diagnose setup issues
```

---

## Development Phases

| Phase | Milestone | Goal |
|---|---|---|
| 0 | Foundation | Repo + architecture |
| 1 | Dictation MVP | Speak → ASR → paste |
| 2 | Great Dictation | Streaming + formatting |
| 3 | AI Dictation | Context + polish |
| 4 | Edit Mode | Voice editing |
| 5 | Developer Mode | IDE + terminal + git |
| 6 | Command + Agent | Desktop control |
| 7 | Meeting + Knowledge | Diarization + search |
| 8 | Ecosystem | Plugins + MCP |
| 9 | Cross-platform | Windows |
| 10 | v1.0 | Production release |

---

## Contributing

Cadence is built in public. Contributions are very welcome.

See [CONTRIBUTING.md](CONTRIBUTING.md) to get started.

Good first issues are tagged [`good-first-issue`](https://github.com/AdeshDeshmukh/cadence/labels/good-first-issue).

---

## Privacy

Cadence is **local-first by design**:

- All speech recognition runs on-device by default
- No data is sent to any cloud without explicit opt-in
- API keys are stored in the OS keychain, never in config files
- Audio is never logged or retained beyond the active session by default

See the [Privacy Guide](docs/docs/privacy.md) *(coming soon)* for full details.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).

---

*Built by [Adesh Deshmukh](https://github.com/AdeshDeshmukh) and the open-source community.*
