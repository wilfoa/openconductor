# OpenConductor

![OpenConductor demo](assets/demo.gif)

A terminal multiplexer for AI coding agents. Run Claude Code, OpenCode, Codex, and Gemini CLI side by side — one terminal, all your projects.

## Overview

OpenConductor is a Go-based TUI that manages multiple AI coding agent processes in parallel. Each project runs its own agent in a real PTY with full terminal emulation. A two-layer attention detection system watches every agent and notifies you exactly when you're needed.

## Features

- **Tabbed workspace**: Project sidebar on the left, full terminal view on the right. Each project gets its own agent process with color, cursor positioning, and alternate screen support (powered by [vt10x](https://github.com/hinshun/vt10x))
- **Multiple sessions**: Spawn multiple agent sessions per project. The sidebar rolls up aggregate state across sessions
- **Attention detection**: Two-layer system — fast agent-specific heuristics (L1) every 500ms, plus an LLM classifier (L2) for ambiguous cases, throttled to once per 5s with backoff
- **Auto-approve**: Per-project configurable permission auto-approval with three levels: `off`, `safe`, `full`
- **Desktop notifications**: Per-project cooldown so you don't get spammed
- **Telegram control center**: Bidirectional bridge to a Telegram supergroup — monitor agents, approve permissions, answer questions, and send input from your phone
- **Scrollback + mouse**: Scroll with mouse wheel, smart pinning keeps your place. Click tabs, drag sidebar border to resize
- **Repo bootstrapping**: Seed `CLAUDE.md`, `.mcp.json`, or agent-specific config into any repository with auto-detected project context
- **Structured logging**: JSON output to `~/.openconductor/openconductor.log`, `--debug` for verbose diagnostics

## Installation

### Pre-built binaries

Grab a binary from the [Releases](https://github.com/openconductorhq/openconductor/releases) page — Linux and macOS, amd64 and arm64.

### Using Go

```bash
go install github.com/openconductorhq/openconductor/cmd/openconductor@latest
```

### Building from source

```bash
git clone https://github.com/openconductorhq/openconductor.git
cd openconductor
make build
```

### Prerequisites

- Go 1.24+
- At least one AI coding agent:
  - [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude`)
  - [OpenCode](https://github.com/opencode-ai/opencode) (`opencode`)
  - [Codex](https://github.com/openai/codex) (`codex`)
  - [Gemini CLI](https://github.com/google-gemini/gemini-cli) (`gemini`)

## Usage

```bash
# Launch the TUI
openconductor

# Launch with debug logging
openconductor --debug

# Bootstrap agent config for a repo
openconductor bootstrap ~/code/my-project --agent claude-code

# Set up Telegram bot
openconductor telegram setup
```

### Command-line flags

Flag | Description
---|---
`--debug` | Enable verbose debug logging to `~/.openconductor/openconductor.log`
`--help`, `-h` | Display help information

## Keyboard shortcuts

Shortcut | Action
---|---
`Ctrl+S` | Toggle focus between sidebar and terminal
`Ctrl+J` / `Ctrl+K` | Next / previous tab
`j` / `k` | Navigate sidebar (when focused)
`Enter` | New session for selected project
`a` | Add project
`d` | Delete project
`Ctrl+C` | Press twice to exit

Mouse: click tabs and sidebar items, drag sidebar border to resize, scroll terminal with mouse wheel.

## Attention detection

OpenConductor uses a two-layer system to determine each agent's state: working, waiting for input, asking a question, error, or idle.

### Layer 1: Heuristics

Agent-specific pattern matching runs every 500ms. Catches:
- Permission prompts (`[Y/n]`, `(yes/no)`)
- Error messages and stack traces
- Agent spinners (Claude Code's `Thinking...`, OpenCode's `esc interrupt`)
- Idle/completion states

Each agent implements the `AttentionChecker` interface — adding a new agent's heuristics is a single file.

### Layer 2: LLM classifier

When L1 is uncertain, the last ~20 terminal lines are sent to an LLM for structured classification. Throttled to once per 5 seconds with exponential backoff.

## Telegram

OpenConductor bridges every project to a Telegram supergroup with [Forum Topics](https://telegram.org/blog/topics-in-groups-collectible-usernames). Each project gets its own topic thread. You monitor agents, approve permissions, answer questions, and send freeform input — all from your phone.

```bash
openconductor telegram setup
```

The interactive wizard handles bot creation, group configuration, and chat ID discovery. See [docs/TELEGRAM_INTEGRATION.md](docs/TELEGRAM_INTEGRATION.md) for the full guide.

### Events

Every meaningful state change is pushed to the project's topic thread as a formatted HTML message with a screen snapshot.

| Event | Trigger | Keyboard |
|---|---|---|
| Response | Agent finishes responding | — |
| Permission request | Agent needs approval to run a tool or write a file | `[Allow Once]` `[Allow Always]` `[Deny]` |
| Question | Agent asks the user to choose an option | Numbered buttons parsed from screen (e.g. `[1. Jest]` `[2. Vitest]`) |
| Needs attention | Agent is stuck or waiting for input | `[yes]` `[no]` `[continue]` `[skip]` |
| Error | Agent hit an error | `[retry]` `[skip]` `[abort]` |
| Task complete | Session finished | — |

### Inbound actions

| Action | How |
|---|---|
| Approve permissions | Tap an inline button — agent continues immediately |
| Answer numbered questions | Tap a numbered button — option is typed into the agent's terminal |
| Quick-reply to attention/errors | Tap a quick-reply button — keyword is sent to the agent |
| Free text input | Send any message in a project's thread — typed directly into the agent's PTY |

After every button press, the original message is edited to show what action was taken and by whom.

### Example: permission request

```
🔒  my-api

  Claude wants to write to src/main.go

  Allow this action?
  (y)es / (n)o / (a)lways

  [ Allow Once ] [ Allow Always ] [ Deny ]
```

After tapping "Allow Once":

```
🔒  my-api

  Claude wants to write to src/main.go

  Allow this action?
  (y)es / (n)o / (a)lways

  Allowed once by Alice
```

### Message formatting

- TUI chrome and borders are stripped from screen content automatically
- Messages exceeding 4000 characters are split on newline boundaries
- Inline keyboards attach to the last message chunk
- 3-second minimum interval between messages per project, duplicate content suppressed

## Architecture

- **cmd/openconductor**: CLI entry point and flag parsing
- **internal/agent**: `AgentAdapter` interface + per-agent implementations (Claude Code, OpenCode, Codex, Gemini)
- **internal/attention**: Two-layer attention detection (heuristics + LLM classifier)
- **internal/bootstrap**: Repo scaffolding with embedded Go templates
- **internal/config**: YAML config loader and validation
- **internal/llm**: LLM client abstraction (Anthropic, OpenAI, Google)
- **internal/logging**: File-based structured logger (slog + JSON)
- **internal/notification**: Desktop notifications via beeep
- **internal/permission**: Permission request detection + auto-approve
- **internal/session**: Agent process lifecycle, PTY management, vt10x terminal state
- **internal/telegram**: Bidirectional Telegram bot bridge
- **internal/tui**: Bubble Tea app — tabs, sidebar, terminal, status bar

### Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — Terminal styling
- [vt10x](https://github.com/hinshun/vt10x) — VT100/VT220 terminal emulation
- [creack/pty](https://github.com/creack/pty) — PTY allocation
- [beeep](https://github.com/gen2brain/beeep) — Desktop notifications

## Development

```bash
make build       # Build binary
make test        # Tests with race detector
make lint        # golangci-lint
make coverage    # Tests + coverage report
make check       # fmt + vet + lint + test (run before pushing)
make help        # All available targets
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide — code structure, adding agent adapters, testing conventions, review process.

## License

[MIT](LICENSE) — Copyright (c) 2025 The OpenConductor Authors.
