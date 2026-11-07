# Cadence Terminal & Shell Integration

Cadence terminal integration provides zero-latency context awareness for developer mode. It tracks:
- Active Shell (`zsh`, `bash`, `fish`) and PID
- Current working directory (`$PWD`)
- Active Git branch
- Last executed command and return code
- Rolling command history

This powers Cadence developer capabilities such as voice-driven git commits ("commit changes with message..."), running test suites ("run failing tests"), and terminal command execution.

## Quick Setup

### Zsh (`~/.zshrc`)
```zsh
eval "$(cadence terminal hook zsh)"
```

### Bash (`~/.bashrc`)
```bash
eval "$(cadence terminal hook bash)"
```

### Fish (`~/.config/fish/config.fish`)
```fish
cadence terminal hook fish | source
```

## CLI Usage

Inspect terminal and editor state anytime:
```bash
cadence context
cadence context show --json
```

Or manually broadcast terminal context:
```bash
cadence context set --editor terminal --shell zsh --pwd "$PWD" --cmd "go test ./..." --exit-code 0
```

