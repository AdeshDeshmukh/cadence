# Cadence VS Code Integration

The **Cadence VS Code Extension** connects your active editor state directly to the local **Cadence** voice dictation and AI editing daemon.

## Features

- **Real-Time Context Synchronization**: Automatically streams active file path, programming language ID (`go`, `python`, `typescript`, etc.), cursor coordinates, and selected code context to the Cadence daemon.
- **Voice Text & Code Transformations**: Trigger transformations like "rewrite as async", "convert into markdown table", or "add error handling" on highlighted code.
- **Direct Voice Dictation**: Start and stop dictation sessions directly inside VS Code with keybindings (`Cmd+Alt+D` on macOS, `Ctrl+Alt+D` on Linux/Windows).
- **Multi-Level Undo**: Quickly undo previous voice edits via `Cmd+Alt+Z` / `Ctrl+Alt+Z`.
- **Status Bar Indicator**: Real-time daemon connection status in the bottom right status bar.

## Requirements

The Cadence daemon (`cadenced`) must be running locally:
```bash
cadenced
```

By default, the extension connects to the UNIX socket at `~/.cadence/cadence.sock` (or named pipe on Windows).

## Extension Settings

- `cadence.socketPath`: Custom path to the Cadence daemon UNIX domain socket.
- `cadence.autoSync`: Automatically broadcast file and selection context on tab/cursor changes (default: `true`).
- `cadence.surroundingLines`: Number of lines around cursor/selection sent as context (default: `15`).

## Commands & Keybindings

| Command | Keybinding (macOS) | Keybinding (Windows/Linux) | Description |
|---|---|---|---|
| `cadence.dictate` | `Cmd+Alt+D` | `Ctrl+Alt+D` | Toggle voice dictation |
| `cadence.editSelection` | `Cmd+Alt+E` | `Ctrl+Alt+E` | Transform selected code with voice / text prompt |
| `cadence.undo` | `Cmd+Alt+Z` | `Ctrl+Alt+Z` | Undo last voice edit |
| `cadence.syncContext` | — | — | Manually push active editor context to daemon |
| `cadence.status` | — | — | Inspect Cadence daemon connection and model status |
