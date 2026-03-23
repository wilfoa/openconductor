# AGENTS.md

## Project

OpenConductor — Go TUI managing multiple AI coding agents (Claude Code, OpenCode) in parallel with real PTY terminal emulation and two-layer attention detection.

## Build & test

```bash
make build       # build binary
make test        # tests with race detector
make lint        # golangci-lint
make check       # fmt + vet + lint + test
```

## Architecture

```
cmd/openconductor/     Entry point, flag parsing, config/state loading
internal/agent/        AgentAdapter interface + implementations (claude.go, opencode.go)
internal/attention/    L1 heuristics + L2 LLM attention detection + auto-approve
internal/config/       YAML config (AgentType, ApprovalLevel) + ephemeral state (tabs)
internal/session/      PTY lifecycle, vt10x terminal, captureScrollOff, session manager
internal/tui/          Bubble Tea app: app.go (main model), terminal.go, sidebar.go,
                       scrollback.go, statusbar.go, messages.go, form.go, styles.go
internal/llm/          Multi-provider LLM client (Anthropic, OpenAI, Google)
internal/permission/   Permission classification (L1 patterns + L2 LLM)
internal/telegram/     Bidirectional Telegram bot bridge
internal/notification/ Desktop notifications
internal/bootstrap/    Repo scaffolding with Go templates
internal/logging/      Structured JSON logger (slog)
```

## Code conventions

- SPDX license header: `// SPDX-License-Identifier: MIT` + copyright on every `.go` file
- Commit prefix: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `ci:`, `deps:`
- Table-driven tests, no global state, explicit dependency passing
- Branch from `master`, PR back to `master`

## Key interfaces

The agent adapter system (`internal/agent/agent.go`) uses a core interface plus optional extension interfaces:

- **`AgentAdapter`** (required): `Type()`, `Command()`, `BootstrapFiles()`, `ApproveKeystroke()`, `ApproveSessionKeystroke()`, `DenyKeystroke()`
- **`AttentionChecker`**: Agent-specific heuristics returning `(HeuristicResult, *AttentionEvent)`
- **`ChromeLineFilter`**: Per-line content filtering for TUI chrome (spinners, status bars)
- **`ChromeLayout`**: Fixed chrome row counts (top/bottom) to exclude from scrollback capture
- **`OutputFilter`**: Stateful preprocessing of raw PTY output before vt10x parsing
- **`QuestionResponder`**: Navigation keystrokes for structured question dialogs
- **`HistoryProvider`**: Load previous conversation history for scrollback pre-population
- **`ScreenFilter`**: Crop agent UI (e.g. OpenCode sidebar) from Telegram screen snapshots
- **`SubmitDelay`**: Pause between typing text and Enter for event-loop sync
- **`ImageInputFormatter`**: Custom formatting for image file paths in Telegram input

## Scrollback system

Two detection strategies in `checkScrollback` (`internal/tui/app.go`):

- **Non-alt-screen** (Claude Code): `Session.captureScrollOff()` runs per `VT.Write()`. Lines pushed with `PushForce` (no buffer-wide dedup) to preserve legitimate duplicate content.
- **Alt-screen** (OpenCode): Snapshot-based detection with buffer-wide dedup (`Push`) to prevent TUI repaint flooding. `pushAltScreenDiff` captures disappeared content.

Ring buffer capacity: 10,000 lines. Text dedup set maintained for correct eviction on wrap.

## Testing

```bash
go test -race ./...              # all tests
go test ./internal/agent/...     # agent adapters
go test ./internal/tui/...       # TUI rendering, scrollback, selection
go test ./internal/attention/... # attention detection
go test ./internal/config/...    # config loading
```
