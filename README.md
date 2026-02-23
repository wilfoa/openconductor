# OpenConductor

[![CI](https://github.com/openconductorhq/openconductor/actions/workflows/ci.yml/badge.svg)](https://github.com/openconductorhq/openconductor/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/openconductorhq/openconductor)](https://goreportcard.com/report/github.com/openconductorhq/openconductor)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/openconductorhq/openconductor)](go.mod)

A terminal multiplexer for AI coding agents. Run Claude Code, OpenCode, Codex,
and Gemini CLI side by side -- one terminal, all your projects.

OpenConductor embeds each agent in its own PTY with full vt10x terminal
emulation, watches for moments the agent needs you (input prompts, permission
requests, errors), and notifies you so you can context-switch only when it
matters.

<!-- TODO: replace with a screenshot or GIF of the TUI -->
<!-- ![OpenConductor TUI](assets/screenshot.png) -->

## Features

- **Multi-tab layout** -- Browser-style tabs with a project sidebar. Switch
  between agents with `Ctrl+J`/`Ctrl+K`.
- **Multi-session** -- Open the same project multiple times. Each Enter in the
  sidebar spawns a new agent process. Sessions are shown as individual tabs;
  the sidebar rolls up aggregate state.
- **Real terminal emulation** -- Each agent runs in a real PTY backed by vt10x.
  Full color, cursor positioning, and alternate screen support.
- **Attention detection** -- Two-layer system: fast heuristic pattern matching
  (L1), with optional LLM classification (L2) for ambiguous cases.
- **Desktop notifications** -- Get notified when an agent needs input, hits an
  error, or finishes its task. Per-project cooldown prevents spam.
- **Telegram integration** -- Monitor and interact with your agents remotely
  via a Telegram bot. Per-project Forum Topics, inline permission buttons,
  and formatted screen snapshots.
- **Agent-specific heuristics** -- Recognizes Claude Code's spinner
  (`Thinking...`), OpenCode's `esc interrupt` indicator, and more to
  distinguish "working" from "idle," eliminating false positives.
- **Scrollback buffer** -- Scroll up through agent output history with the
  mouse wheel. Smart pinning keeps your position when new output arrives.
- **Mouse support** -- Click tabs and sidebar items. Drag the sidebar border
  to resize. Scroll wheel navigates terminal scrollback.
- **Bootstrap CLI** -- `openconductor bootstrap <repo>` seeds agent config
  files (CLAUDE.md, .codex/instructions.md, GEMINI.md) into your repo with
  auto-detected language context.
- **File logging** -- Diagnostics go to `~/.openconductor/openconductor.log`.
  Crash recovery with full stack traces. Use `--debug` for verbose output.

## Getting started

### Requirements

- Go 1.24+
- At least one AI coding agent installed:
  - [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude`)
  - [OpenCode](https://github.com/opencode-ai/opencode) (`opencode`)
  - [Codex](https://github.com/openai/codex) (`codex`)
  - [Gemini CLI](https://github.com/google-gemini/gemini-cli) (`gemini`)

### Install

**Pre-built binaries** are available on the
[Releases](https://github.com/openconductorhq/openconductor/releases) page
(Linux and macOS, amd64 and arm64).

Or install with Go:

```bash
go install github.com/openconductorhq/openconductor/cmd/openconductor@latest
```

Or build from source:

```bash
git clone https://github.com/openconductorhq/openconductor.git
cd openconductor
make build
```

### Configure

Copy the [example config](config.example.yaml) to `~/.openconductor/config.yaml`
and edit it:

```yaml
projects:
  - name: api
    repo: ~/code/api
    agent: claude-code
  - name: frontend
    repo: ~/code/frontend
    agent: opencode
  - name: infra
    repo: ~/code/infra
    agent: codex

# Optional: L2 LLM classifier for ambiguous attention signals
llm:
  provider: anthropic          # anthropic | openai | google
  model: ""                    # empty = provider default
  api_key_env: ANTHROPIC_API_KEY

notifications:
  enabled: true
  cooldown_seconds: 30
```

### Run

```bash
openconductor            # normal mode
openconductor --debug    # verbose logging
```

### Bootstrap agent configs

Seed agent-specific configuration files into a repository:

```bash
openconductor bootstrap ~/code/my-project --agent claude-code
```

This creates `CLAUDE.md` and `.mcp.json` with project context auto-detected
from the repo (language, build system, etc.). Also supports `codex` and
`gemini`.

### Telegram setup

Connect a Telegram bot for remote monitoring:

```bash
openconductor telegram setup
```

The interactive wizard walks you through creating a bot, configuring a Forum
group, and linking projects to topics. See
[docs/TELEGRAM_INTEGRATION.md](docs/TELEGRAM_INTEGRATION.md) for details.

## Keybindings

| Key | Action |
|---|---|
| `Ctrl+S` | Toggle sidebar / terminal focus |
| `Ctrl+J` / `Ctrl+K` | Next / previous tab |
| `j` / `k` | Navigate sidebar (when focused) |
| `Enter` | Open new session for selected project |
| `a` | Add project |
| `d` | Delete project |
| `Ctrl+C` | Press twice to exit |

Mouse: click tabs and sidebar items. Drag sidebar border to resize.
Scroll wheel in terminal area navigates scrollback history.

## Architecture

```
cmd/openconductor/    Entry point, CLI flags
internal/
  agent/              AgentAdapter interface + per-agent CLI wrappers
  attention/          Two-layer attention detection (heuristics + LLM)
  bootstrap/          Repo scaffolding with embedded Go templates
  config/             YAML config loader + validation
  llm/                LLM client abstraction (Anthropic, OpenAI, Google)
  logging/            File-based structured logger (slog + JSON)
  notification/       Desktop notifications via beeep
  permission/         Permission request detection + auto-approve
  session/            Agent process lifecycle, PTY management, vt10x state
  telegram/           Bidirectional Telegram bot bridge
  tui/                Bubble Tea app -- tabs, sidebar, terminal, status bar
```

**Key dependencies:**

| Library | Purpose |
|---|---|
| [Bubble Tea](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm architecture) |
| [Lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling and layout |
| [vt10x](https://github.com/hinshun/vt10x) | VT100/VT220 terminal emulation |
| [creack/pty](https://github.com/creack/pty) | PTY allocation and management |
| [beeep](https://github.com/gen2brain/beeep) | Cross-platform desktop notifications |

### Attention detection pipeline

```
Terminal screen buffer
        |
        v
 +----------------+    Certain    +-----------+
 | Agent-specific  |------------->|  Emit     |
 | heuristics      |              |  event    |
 +-------+--------+              +-----------+
         | No / Working                ^
         v                             |
 +----------------+    Certain    +----+------+
 |   Generic      |------------->|  Emit     |
 |  patterns      |              |  event    |
 +-------+--------+              +-----------+
         | Uncertain                   ^
         v                             |
 +----------------+    Classified +----+------+
 |  L2 LLM       |------------->|  Emit     |
 |  classifier    |  (optional)  |  event    |
 +----------------+              +-----------+
```

**L1 heuristics** match patterns like `[Y/n]`, `(yes/no)`, `error:`, stack
traces, and agent-specific spinners. Fast, runs every 500ms.

**L2 LLM classifier** fires only when L1 returns `Uncertain`. Sends the
last ~20 terminal lines to an LLM for structured classification. Throttled
to once per 5 seconds with backoff.

## Configuration reference

### Project

| Field | Type | Description |
|---|---|---|
| `name` | string | Display name for the tab and sidebar |
| `repo` | string | Absolute or `~`-relative path to the repository |
| `agent` | string | One of `claude-code`, `opencode`, `codex`, `gemini` |

### LLM (optional)

| Field | Type | Description |
|---|---|---|
| `provider` | string | `anthropic`, `openai`, or `google` |
| `model` | string | Model override (empty uses provider default) |
| `api_key_env` | string | Name of env var containing the API key |

### Notifications

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable desktop notifications |
| `cooldown_seconds` | int | `30` | Min seconds between notifications per project |

### Telegram (optional)

| Field | Type | Description |
|---|---|---|
| `bot_token_env` | string | Env var name containing the bot token |
| `chat_id` | int | Telegram chat or group ID |

## Development

```bash
make build       # Build binary
make test        # Run tests with race detector
make lint        # Run golangci-lint
make coverage    # Tests + coverage report
make check       # fmt + vet + lint + test (run before pushing)
make help        # Show all targets
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development guide.

## License

[MIT](LICENSE) -- Copyright (c) 2025 The OpenConductor Authors
