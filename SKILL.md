---
name: openconductor-tester
description: |
  Run and test OpenConductor interactively using tmux. Use when you need to:
  - Test a fix by running the app with real agents
  - Debug heuristic detection issues (spinner, permission, question dialogs)
  - Verify scrollback behavior
  - Check Telegram notification flow
  - Reproduce a user-reported bug
  Triggers on: "test this", "run the app", "try it", "verify the fix", "check if it works", "reproduce the bug", "manual test"
---

# OpenConductor Interactive Testing

Run OpenConductor in tmux to test changes with real agents.

## Quick Start

### Build and Run

```bash
# Build the binary
go build -o /tmp/oc-test ./cmd/openconductor

# Create a tmux session with the app
tmux new-session -d -s oc-test -x 200 -y 50 "/tmp/oc-test --debug"

# To use isolated config/logs (won't affect the user's real setup):
OC_CONFIG_PATH=/tmp/oc-test-config.yaml \
OC_LOG_DIR=/tmp/oc-test-logs \
OC_STATE_PATH=/tmp/oc-test-state.json \
tmux new-session -d -s oc-test -x 200 -y 50 "/tmp/oc-test --debug"
```

### Read Terminal State

```bash
# Get visible screen content as text
tmux capture-pane -t oc-test -p

# Get specific line range (0-indexed)
tmux capture-pane -t oc-test -p -S 0 -E 5   # first 6 lines
tmux capture-pane -t oc-test -p -S -5        # last 5 lines
```

### Send Keystrokes

```bash
# Type text and press Enter (send a prompt to the agent)
tmux send-keys -t oc-test "fix the login bug" Enter

# Press special keys
tmux send-keys -t oc-test Enter          # Enter
tmux send-keys -t oc-test Escape         # Escape
tmux send-keys -t oc-test C-c            # Ctrl+C
tmux send-keys -t oc-test C-s            # Ctrl+S (toggle sidebar focus)
tmux send-keys -t oc-test C-j            # Ctrl+J (prev tab)
tmux send-keys -t oc-test C-k            # Ctrl+K (next tab)
tmux send-keys -t oc-test Tab            # Tab

# Send raw escape sequences (arrow keys)
tmux send-keys -t oc-test -l $'\x1b[A'   # Up arrow
tmux send-keys -t oc-test -l $'\x1b[B'   # Down arrow
tmux send-keys -t oc-test -l $'\x1b[C'   # Right arrow
tmux send-keys -t oc-test -l $'\x1b[D'   # Left arrow
```

### Read Logs

```bash
# Tail the log file (structured JSON)
tail -f /tmp/oc-test-logs/openconductor.log

# Search for specific events
grep 'attention check' /tmp/oc-test-logs/openconductor.log | tail -5
grep 'heuristic.*claude' /tmp/oc-test-logs/openconductor.log | tail -5
grep 'auto-approve' /tmp/oc-test-logs/openconductor.log | tail -5
grep 'auto-confirm' /tmp/oc-test-logs/openconductor.log | tail -5
grep 'telegram' /tmp/oc-test-logs/openconductor.log | tail -5
grep 'no signal' /tmp/oc-test-logs/openconductor.log | tail -5
```

### Cleanup

```bash
tmux kill-session -t oc-test
rm -f /tmp/oc-test
```

## Testing Workflows

### Test Spinner Detection (Claude Code)

1. Start the app with a Claude Code project
2. Send a prompt: `tmux send-keys -t oc-test "What is 2+2?" Enter`
3. Check logs: `grep 'claude-code spinner detected' /tmp/oc-test-logs/openconductor.log | tail -3`
4. Check screen: `tmux capture-pane -t oc-test -p | grep -i 'working\|spinner'`
5. Expected: log shows "isWorking: true", sidebar shows "working" badge

### Test Permission Detection (OpenCode)

1. Start with an OpenCode project
2. Send a prompt that triggers file access outside the repo
3. Check logs: `grep 'permission' /tmp/oc-test-logs/openconductor.log | tail -5`
4. Check screen: `tmux capture-pane -t oc-test -p | grep -i 'permission required\|allow once'`
5. Expected: sidebar shows "permission" badge, log shows NeedsPermission

### Test Auto-Approve

1. Verify auto-approve is configured (LLM section in config)
2. Trigger a permission dialog
3. Check logs: `grep 'auto-approve' /tmp/oc-test-logs/openconductor.log | tail -5`
4. Expected: log shows "auto-approve: sending keystroke" then "auto-confirm: confirmed always-allow dialog"

### Test Scrollback (Claude Code)

1. Send a prompt that produces long output: `tmux send-keys -t oc-test "Run go test ./..." Enter`
2. Wait for completion
3. Scroll up: `tmux send-keys -t oc-test -l $'\x1b[5~'` (PageUp — only works if not alt-screen)
4. Check screen: `tmux capture-pane -t oc-test -p`
5. Expected: scrollback shows earlier output without gaps

### Test Question Series (OpenCode)

1. Send a prompt designed to trigger AskUser: `tmux send-keys -t oc-test "Ask me which framework to use before starting" Enter`
2. Check logs: `grep 'NeedsAnswer\|question' /tmp/oc-test-logs/openconductor.log | tail -5`
3. Check screen: `tmux capture-pane -t oc-test -p | grep -i 'select\|dismiss'`
4. Expected: question dialog detected, state shows "asking"

## Config File Format

For E2E testing, create a minimal config:

```yaml
projects:
  - name: TestProject
    repo: /path/to/test/repo
    agent: claude-code          # or "opencode"

# Optional: enable auto-approve
llm:
  provider: anthropic
  model: claude-sonnet-4-20250514
  api_key: ANTHROPIC_API_KEY    # env var name, not the actual key

# Optional: Telegram (usually disabled for testing)
telegram:
  enabled: false
```

## Key Log Messages

| Log Message | Meaning |
|---|---|
| `"attention check"` | Periodic check with `hasEvent`, `isWorking` fields |
| `"attention state transition"` | State changed — `from` and `to` fields |
| `"heuristic: claude-code spinner detected"` | Spinner found |
| `"heuristic: claude-code no signal"` | Nothing detected — check `lines` field |
| `"heuristic: opencode scan result"` | OpenCode detection flags |
| `"auto-approve: sending keystroke"` | Auto-approve fired |
| `"auto-approve: stuck, falling through"` | Auto-approve failed too many times |
| `"auto-confirm: confirmed always-allow dialog"` | Second-stage confirm auto-dismissed |
| `"auto-confirm: submitted question series confirm tab"` | Question confirm auto-submitted |
| `"telegram: sending permission keystroke"` | Telegram button callback processed |
| `"session: PTY write failed"` | Write to PTY returned error |

## Architecture Reference

- **Session**: `internal/session/session.go` — PTY + vt10x wrapper
- **Agent adapters**: `internal/agent/claude.go`, `internal/agent/opencode.go`
- **Attention loop**: `internal/tui/app.go:checkAttention()` — runs every 2s
- **Scrollback**: `internal/tui/app.go:checkScrollback()` — runs every 100ms
- **Telegram handler**: `internal/telegram/handler.go:HandleCallback()`
- **Auto-approve**: `internal/tui/app.go` — runs before state transition for NeedsPermission
- **CSI filter**: `internal/agent/claude.go:csiFilter` — strips kitty keyboard sequences
