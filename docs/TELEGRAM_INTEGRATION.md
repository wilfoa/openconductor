# Telegram Integration Plan

Bidirectional Telegram bridge giving users full visibility into agent work and
the ability to respond remotely. Messages are organized per project using
Telegram Forum Topics.

## Design Principles

- **State-transition driven**: only send messages on meaningful events, not every terminal line
- **One Telegram Forum Topic per project**: clean separation
- **Delightful format**: structured HTML messages, not raw terminal dumps
- **Bidirectional**: user can reply, approve permissions, answer questions, and send new prompts
- **Single user**: one authorized Telegram user interacts with agents
- **TUI required**: runs alongside the TUI (headless mode is a future goal)

## Config

```yaml
telegram:
  enabled: true
  bot_token_env: "OPENCONDUCTOR_TELEGRAM_TOKEN"  # env var name (not the token itself)
  chat_id: -100123456789                          # supergroup with Forum Topics enabled
```

Topic-to-project mappings are stored separately in `~/.openconductor/telegram_state.json`:

```json
{"project-alpha": 42, "project-beta": 87}
```

## Architecture

### New Package: `internal/telegram/`

| File | Responsibility |
|------|---------------|
| `bot.go` | Bot lifecycle, long-polling loop, topic auto-creation |
| `bridge.go` | Event types, event channel, screen content diffing |
| `formatter.go` | Terminal text to Telegram HTML formatting |
| `handler.go` | Incoming messages/callbacks to `session.Write()` |
| `state.go` | Topic ID <-> project mapping (persisted to disk) |

### Dependency

`github.com/go-telegram-bot-api/telegram-bot-api/v5`

## Event Bridge (Agent to Telegram)

### Trigger

State transitions detected by `App.checkAttention()` (every 2s tick).

### Event Type

```go
type Event struct {
    Project   string
    State     SessionState
    PrevState SessionState
    Detail    string
    Screen    []string
}
```

### State Transition to Message Mapping

| Transition | Telegram message |
|-----------|-----------------|
| `* -> NeedsPermission` | Permission request + inline keyboard |
| `* -> Asking` | Question + inline keyboard |
| `* -> NeedsAttention` | "Needs input" + screen content |
| `* -> Error` | Error alert + screen content |
| `* -> Done` | Completion summary |
| `Working -> Idle` | Agent response (screen content) |

## Message Formats (Telegram HTML)

### Agent Response (Working -> Idle)

```html
<b>ProjectName</b>

<pre>I've analyzed the codebase and found 3 issues:
1. Missing error handling in auth.go
2. Unused import in server.go
3. Race condition in cache.go</pre>
```

### Permission Request (with inline keyboard)

```html
<b>ProjectName</b> <emoji>lock</emoji>

Permission required:
<code>Read file: src/config.ts</code>
```

Inline keyboard: `[Allow Once] [Allow Always] [Deny]`

### Question (with inline keyboard)

```html
<b>ProjectName</b> <emoji>question</emoji>

<pre>Which testing framework would you prefer?
1. Jest
2. Vitest
3. Mocha</pre>
```

Inline keyboard: `[1. Jest] [2. Vitest] [3. Mocha]`

### Error

```html
<b>ProjectName</b> <emoji>red_circle</emoji>

<pre>Connection timeout to database on port 5432</pre>
```

### Task Complete

```html
<b>ProjectName</b> <emoji>check</emoji>

<pre>All 3 issues have been fixed and tests pass.</pre>
```

### Long Output

Split at 4000 chars (96 char buffer for HTML tags), break on newline boundaries.
Send as sequential messages with no header on continuation messages.

## Input Handling (Telegram to Agent)

### Text Messages

1. Look up project by `message_thread_id` -> project name
2. Get session: `mgr.GetSession(name)`
3. Write to PTY: `session.Write([]byte(text + "\n"))`

### Inline Keyboard Callbacks

- `perm:<project>:allow` -> `session.Write(adapter.ApproveKeystroke())`
- `perm:<project>:allowall` -> `session.Write(adapter.ApproveSessionKeystroke())`
- `perm:<project>:deny` -> `session.Write(adapter.DenyKeystroke())`
- `opt:<project>:<number>` -> write number selection to PTY
- After handling: edit original message to show the action taken

## Topic Management

On startup, for each project in config:

1. Load `~/.openconductor/telegram_state.json`
2. If project has no topic ID, call `createForumTopic(chatID, projectName)`
3. Store the mapping and persist to disk

Topic names = project names.

## Wiring into main.go

```go
if cfg.Telegram.Enabled:
    bot = telegram.NewBot(cfg.Telegram, mgr, cfg.Projects)
    app.SetTelegramBridge(bot.EventChannel())
    bot.Start()
    defer bot.Stop()
```

## Implementation Phases

| Phase | What | Files |
|-------|------|-------|
| 0 | Fix question detection (`"enter confirm"`) | `heuristics.go`, tests |
| 1 | Config + state types | `config.go`, `telegram/state.go` |
| 2 | Bot skeleton + topic management | `telegram/bot.go` |
| 3 | Formatter | `telegram/formatter.go` |
| 4 | Outbound bridge (event channel, send messages) | `telegram/bridge.go`, `app.go` |
| 5 | Inbound handler (text replies + callbacks) | `telegram/handler.go` |
| 6 | Wire into main.go | `main.go` |
| 7 | Polish: dedup, rate limiting, message editing | `telegram/bridge.go` |
