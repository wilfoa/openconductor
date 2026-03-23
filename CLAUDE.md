# CLAUDE.md

## Project

OpenConductor is a Go TUI that manages multiple AI coding agents (Claude Code, OpenCode) in parallel. Each project runs its own agent in a real PTY with full VT100 terminal emulation. A two-layer attention detection system watches every agent and notifies the user when action is needed.

## Quick start

```bash
make build       # build binary
make test        # tests with race detector
make lint        # golangci-lint
make check       # fmt + vet + lint + test (run before pushing)
```

## Tech stack

- **Language**: Go 1.24+
- **TUI**: Bubble Tea + Lipgloss
- **Terminal emulation**: vt10x (VT100/VT220)
- **PTY**: creack/pty
- **LLM clients**: Anthropic SDK, OpenAI SDK, Google GenAI
- **Config**: YAML (`~/.openconductor/config.yaml`)
- **State**: JSON (`~/.openconductor/state.json`) — restores open tabs on restart
- **Logging**: slog + JSON to `~/.openconductor/openconductor.log`

## Architecture

```
cmd/openconductor/        CLI entry point, flag parsing, initialization
internal/
  agent/                  AgentAdapter interface + per-agent implementations
    agent.go              Interface definitions, adapter registry, optional interfaces
    claude.go             Claude Code adapter (chrome filtering, CSI stripping, history)
    opencode.go           OpenCode adapter (sidebar cropping, question dialogs)
  attention/              Two-layer attention detection
    types.go              AttentionType, ProcessState, HeuristicResult
    detector.go           L1 heuristics + L2 LLM escalation
    classifier.go         LLM classification with per-session throttling
    autoapprove.go        Permission auto-approval engine
    heuristics.go         Generic pattern matching
    process_darwin.go     macOS process state detection
    process_linux.go      Linux process state detection
  config/                 YAML config + ephemeral app state (tab restoration)
  session/                PTY lifecycle, vt10x state, scroll-off capture
    session.go            Session struct, ReadLoop, captureScrollOff, GetScreenLines
    manager.go            Session lifecycle management, InjectSession
  tui/                    Bubble Tea app
    app.go                Main model: tabs, attention loop, scrollback, keyboard/mouse
    terminal.go           vt10x rendering, text selection, scrollback views
    sidebar.go            Project list, status badges, forms, drag-to-resize
    scrollback.go         Ring buffer (Push with dedup, PushForce without)
    statusbar.go          Summary bar with context-sensitive keybinding hints
    messages.go           Message types and SessionState enum
    styles.go             Colors and layout constants
    keys.go               Key definitions and helpers
    form.go               Multi-step add-project wizard with tab-completion
    completion.go         Directory completion suggestions
  llm/                    Multi-provider LLM client (Anthropic, OpenAI, Google)
  permission/             Permission classification (L1 patterns + L2 LLM)
  telegram/               Bidirectional Telegram bot bridge
  notification/           Desktop notifications via beeep
  bootstrap/              Repo scaffolding with Go templates
  logging/                Structured file logger
```

## Key patterns

- **Agent adapters**: All agent-specific behavior is behind the `AgentAdapter` interface in `internal/agent/agent.go`. Optional interfaces (`ChromeLineFilter`, `ChromeLayout`, `OutputFilter`, `QuestionResponder`, `HistoryProvider`, `ScreenFilter`, `SubmitDelay`, `ImageInputFormatter`) extend behavior without bloating the core interface.
- **Attention detection**: L1 heuristics run every 500ms. Uncertain results escalate to L2 (LLM classifier), throttled to 1 call per 5s with exponential backoff. Six states: working, needs input, needs permission, asking, error, done.
- **Scrollback capture**: Two strategies based on session type:
  - **Non-alt-screen (Claude Code)**: `session.Session.captureScrollOff()` runs per `VT.Write()`, detecting scroll shifts by comparing before/after snapshots. Lines are pushed with `PushForce` (no dedup) to preserve legitimate duplicate content (tables, blank separators).
  - **Alt-screen (OpenCode)**: Snapshot-based detection with buffer-wide dedup to prevent TUI repaint flooding. `pushAltScreenDiff` captures disappeared content.
- **Text selection**: `terminalModel` tracks mouse drag state. When the child process doesn't request mouse tracking, the TUI handles click-and-drag with reverse-video highlighting and auto-copies to clipboard.

## Code conventions

- SPDX license header on every source file:
  ```go
  // SPDX-License-Identifier: MIT
  // Copyright (c) 2025 The OpenConductor Authors.
  ```
- Commit messages prefixed with type: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `ci:`, `deps:`
- Table-driven tests preferred
- No global state; pass dependencies explicitly
- Branch from `master`, PR back to `master`

## Testing

```bash
make test                        # all tests with race detector
go test ./internal/agent/...     # agent adapter tests
go test ./internal/tui/...       # TUI + scrollback + selection tests
go test ./internal/attention/... # attention detection tests
go test ./internal/config/...    # config loading tests
go test ./internal/permission/...# permission classification tests
```

## Common tasks

- **Add a new agent adapter**: Implement `AgentAdapter` in a new file under `internal/agent/`, register it in `agent.go`. Optionally implement any combination of: `ChromeLineFilter`, `ChromeLayout`, `OutputFilter`, `QuestionResponder`, `HistoryProvider`, `ScreenFilter`, `SubmitDelay`, `ImageInputFormatter`.
- **Add a keyboard shortcut**: Handle in `App.Update()` key event section in `internal/tui/app.go`. Add hint to `statusbar.go`.
- **Change attention heuristics**: Edit the agent's `CheckAttention()` or `IsChromeLine()` methods. Add test cases.
- **Modify scrollback rendering**: `terminal.go` has `viewScrollbackOnly()`, `viewScrollbackMixed()`, `renderGlyphRow()`, `renderViewportRow()`.
- **Add a sidebar action**: Handle in `sidebarModel.Update()` in `internal/tui/sidebar.go`. Add help text to sidebar hints.
