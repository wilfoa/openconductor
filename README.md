# OpenConductor

A terminal multiplexer for AI coding agents. Run Claude Code, OpenCode, Codex,
and Gemini CLI side by side — one terminal, all your projects.

OpenConductor embeds each agent in its own PTY with full vt10x terminal emulation,
watches for moments the agent needs you (input prompts, permission requests,
errors), and pings you with a desktop notification so you can context-switch
only when it matters.

## Features

- **Multi-tab layout** — Browser-style tabs with a project sidebar. Switch
  between agents with `Ctrl+J`/`Ctrl+K`.
- **Real terminal emulation** — Each agent runs in a real PTY backed by vt10x.
  Full color, cursor positioning, and alternate screen support.
- **Attention detection** — Two-layer system: fast heuristic pattern matching
  (L1), with optional LLM classification (L2) for ambiguous cases.
- **Desktop notifications** — Get notified when an agent needs input, hits an
  error, or finishes its task. Per-project cooldown prevents notification spam.
- **Agent-specific heuristics** — Recognizes Claude Code's spinner
  (`✦ Thinking…`) and OpenCode's `esc interrupt` indicator to distinguish
  "working" from "idle" states, eliminating false positives.
- **Scrollback buffer** — Scroll up through agent output history with the
  mouse wheel. Smart pinning keeps your position when new output arrives.
- **Mouse support** — Click tabs and sidebar items. Drag the sidebar border
  to resize. Scroll wheel navigates terminal scrollback.
- **File logging** — All diagnostics go to `~/.openconductor/openconductor.log`.
  Crash recovery with full stack traces. Use `--debug` for verbose output.
- **Bootstrap CLI** — `openconductor bootstrap <repo>` seeds agent config files
  (CLAUDE.md, .codex/instructions.md, GEMINI.md) into your repo with
  auto-detected language context.

## Architecture

```
cmd/openconductor/    Entry point, CLI flags, wiring
internal/
  agent/              AgentAdapter interface + per-agent CLI wrappers
  attention/          Two-layer attention detection (heuristics + LLM)
  bootstrap/          Repo scaffolding with embedded Go templates
  config/             YAML config loader + validation
  llm/                LLM client abstraction (Anthropic, OpenAI, Google)
  logging/            File-based structured logger (slog + JSON)
  notification/       Desktop notifications via beeep (macOS/Linux/Windows)
  session/            Agent process lifecycle, PTY management, vt10x state
  tui/                Bubble Tea app — tabs, sidebar, terminal, status bar
```

**Key dependencies:**

| Library | Purpose |
|---|---|
| [Bubble Tea](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm architecture) |
| [Lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling and layout |
| [vt10x](https://github.com/hinshun/vt10x) | VT100/VT220 terminal emulation |
| [creack/pty](https://github.com/creack/pty) | PTY allocation and management |
| [beeep](https://github.com/gen2brain/beeep) | Cross-platform desktop notifications |

### Attention detection

OpenConductor continuously reads the embedded terminal screen and runs a detection
pipeline on each tick:

```
Terminal screen buffer
        │
        ▼
 ┌──────────────┐    Certain    ┌───────────┐
 │ Agent-specific├──────────────►│  Emit     │
 │ heuristics   │               │  event    │
 └──────┬───────┘               └───────────┘
        │ No / Working                ▲
        ▼                             │
 ┌──────────────┐    Certain    ┌─────┴─────┐
 │   Generic    ├──────────────►│  Emit     │
 │  patterns    │               │  event    │
 └──────┬───────┘               └───────────┘
        │ Uncertain                   ▲
        ▼                             │
 ┌──────────────┐    Classified ┌─────┴─────┐
 │  L2 LLM     ├──────────────►│  Emit     │
 │  classifier  │  (optional)   │  event    │
 └──────────────┘               └───────────┘
```

**L1 heuristics** match patterns like `[Y/n]`, `(yes/no)`, `error:`, stack
traces, and agent-specific spinners. They're fast and run every tick (500ms).

**L2 LLM classifier** is called only when L1 returns `Uncertain`. It sends the
last ~20 terminal lines to an LLM and asks for a structured classification.
Throttled to max once per 5 seconds with backoff when the agent is working.

## Getting started

### Requirements

- Go 1.24+
- At least one AI coding agent installed:
  - [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude`)
  - [OpenCode](https://github.com/opencode-ai/opencode) (`opencode`)
  - [Codex](https://github.com/openai/codex) (`codex`)
  - [Gemini CLI](https://github.com/google-gemini/gemini-cli) (`gemini`)

### Install

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

Create `~/.openconductor/config.yaml`:

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
  api_key_env: ANTHROPIC_API_KEY  # env var name (not the key itself)

notifications:
  enabled: true
  cooldown_seconds: 30
```

### Run

```bash
openconductor            # normal mode
openconductor --debug    # verbose logging to ~/.openconductor/openconductor.log
```

### Bootstrap agent configs

Seed agent-specific configuration files into a repository:

```bash
openconductor bootstrap ~/code/my-project --agent claude-code
```

This creates `CLAUDE.md` and `.mcp.json` with project context auto-detected
from the repo (language, build system, etc.). Also supports `codex` and
`gemini`.

## Keybindings

| Key | Action |
|---|---|
| `Ctrl+S` | Toggle sidebar / terminal focus |
| `Ctrl+J` / `Ctrl+K` | Next / previous tab |
| `j` / `k` | Navigate sidebar (when focused) |
| `Enter` | Select project |
| `a` | Add project |
| `d` | Delete project |
| `Ctrl+C` | Press twice to exit |

Mouse: click tabs and sidebar items. Drag sidebar border to resize.
Scroll wheel in terminal area navigates scrollback history.

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

## Development

```bash
make build       # Build binary
make test        # Run all tests
make run         # Build and run
make lint        # Run golangci-lint
```

### Testing

```bash
# Unit tests
go test ./...

# With L2 LLM integration tests (requires API key)
ANTHROPIC_API_KEY=sk-... go test ./internal/attention/ -run TestIntegration -v
```

### Project structure

The codebase is ~7,700 lines of Go across 8 packages with 134 tests covering
attention detection heuristics, TUI layout logic, and the LLM classifier.

## License

MIT
