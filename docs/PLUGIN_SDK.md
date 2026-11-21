# Cadence Plugin SDK & Developer Guide

Cadence provides an extensible plugin system allowing developers to customize voice processing pipelines, add custom agent tools, hook into transcription lifecycles, and integrate with external developer environments.

---

## 1. Plugin Architecture

Plugins run in isolated sandbox environments with fine-grained permission enforcement. They interact with Cadence via standard hooks and JSON-RPC messaging.

```
┌────────────────────────────────────────────────────────┐
│                   Cadence Core                         │
│                                                        │
│  AudioStream ──► Transcription ──► PostProcess ──► Act │
│       │                 │                │          │  │
│  ┌────▼─────────────────▼────────────────▼──────────▼┐ │
│  │               Plugin Host Dispatcher              │ │
│  └──────────────────────┬────────────────────────────┘ │
└─────────────────────────┼──────────────────────────────┘
                          │ IPC / stdio
       ┌──────────────────┴──────────────────┐
       ▼                                     ▼
┌──────────────┐                      ┌──────────────┐
│ Git Assistant│                      │ Custom ITN   │
│ Plugin       │                      │ Plugin       │
└──────────────┘                      └──────────────┘
```

---

## 2. Plugin Manifest (`plugin.json`)

Every Cadence plugin must include a `plugin.json` at its root:

```json
{
  "name": "cadence-jira-assistant",
  "version": "1.0.0",
  "description": "Voice-enabled Jira issue creator and ticket linker",
  "author": "Developer <dev@example.com>",
  "entrypoint": "bin/jira-plugin",
  "hooks": [
    "on_transcription",
    "on_command_execute"
  ],
  "permissions": [
    "network:outbound",
    "secrets:read"
  ],
  "tools": [
    {
      "name": "jira_create_issue",
      "description": "Create a new Jira issue with summary and description",
      "parameters": {
        "type": "object",
        "properties": {
          "project": { "type": "string" },
          "summary": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["project", "summary"]
      }
    }
  ]
}
```

### Manifest Fields
| Field | Type | Description |
|---|---|---|
| `name` | string | Unique plugin identifier (kebab-case). |
| `version` | string | Semantic version (`MAJOR.MINOR.PATCH`). |
| `entrypoint` | string | Executable path relative to plugin root. |
| `hooks` | array | Lifecycle hooks subscribed to by the plugin. |
| `permissions` | array | Granted capability flags (`network:outbound`, `fs:read`, `fs:write`, `secrets:read`). |
| `tools` | array | Registered agent tools with JSON Schema definitions. |

---

## 3. Supported Lifecycle Hooks

| Hook | Invocation Point | Payload | Return Value |
|---|---|---|---|
| `on_transcription` | After raw ASR text generation | `{ "raw_text": string, "session_id": string }` | Transformed text or `{ "pass": true }` |
| `on_postprocess` | After ITN and homophone correction | `{ "text": string, "app": string }` | Formatted text |
| `on_command_execute` | When a shell/CLI command is about to run | `{ "command": string, "risk": string }` | `{ "allow": bool, "modified": string }` |
| `on_app_switch` | When foreground window changes | `{ "app": string, "category": string }` | Profile override |

---

## 4. Managing Plugins via CLI

```bash
# Install a plugin from a local directory or Git repository
cadence plugin install ./my-plugin

# List installed plugins and active status
cadence plugin list

# Inspect plugin metadata, tools, and permissions
cadence plugin info my-plugin

# Activate or deactivate a plugin
cadence plugin activate my-plugin
cadence plugin deactivate my-plugin

# Uninstall a plugin
cadence plugin uninstall my-plugin
```

---

## 5. Security & Sandbox Policies

All plugins run with least-privilege defaults:
- **Zero Ingress Network:** Plugins cannot open listening ports.
- **Outbound Network:** Outbound requests require `network:outbound` permission.
- **Keychain Access:** Plugins cannot access system keychain secrets unless explicitly granted `secrets:read` with specific key naming patterns.
- **Filesystem Isolation:** Plugins can only read/write inside their isolated plugin state directory (`~/.local/share/cadence/plugins/<name>`).
