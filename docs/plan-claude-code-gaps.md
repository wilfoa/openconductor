# Claude Code Support Gaps — Implementation Plan

## Status: READY FOR IMPLEMENTATION

**Date**: 2026-03-01
**Priority**: CRITICAL (permission detection is completely broken)

---

## Background

Claude Code's `CheckAttention()` in `internal/agent/claude.go` only detects two states:

- **Working**: Spinner line (`✦ Thinking…`, `· Analyzing…`, `* Reading…`)
- **Idle**: Prompt line (`"> "`)

When a permission prompt like `"Do you want to proceed? (y/n)"` appears, the checker returns `(No, nil)`. Because `internal/attention/heuristics.go:106-111` **skips generic patterns when an agent-specific checker is provided**, no `NeedsPermission` event ever fires. This means:

- Auto-approve (`auto_approve: safe`) never triggers for Claude Code
- Telegram permission notifications never fire
- The user has no visibility into permission requests from Claude Code agents

## Scope

Three focused tasks. No unnecessary interface implementations.

---

## Task 1 — CRITICAL: Permission Detection in `CheckAttention()`

**File**: `internal/agent/claude.go`

### Problem

`CheckAttention()` scans the bottom 5 non-empty lines for spinner and `"> "` prompt. It has no awareness of permission prompts. Claude Code shows permission prompts inline:

```
  Edit main.go
  - if obj != nil {
  + if obj == nil { return }

  Do you want to proceed? (y/n)
```

```
  Allow running bash command: git status? [y/n]
```

### Fix

Add a permission scan between spinner detection and idle prompt detection. The new priority order:

| Priority | Pattern | Result | Rationale |
|----------|---------|--------|-----------|
| 1 | Spinner (`✦`/`·`/`*` + Verb + `…`) | `Working` | Agent is actively working, don't interrupt |
| 2 | Permission (`(y/n)`, `[y/n]`, etc.) | `Certain` + `NeedsPermission` | Agent is blocked waiting for approval |
| 3 | Idle prompt (`"> "`) | `Certain` + `NeedsInput` | Agent is idle, ready for new input |
| 4 | Nothing | `No` | No signal detected |

### Permission patterns to match (regex on each scanned line)

```
\(y/n\)           — "(y/n)" at end of permission prompt
\[y/n\]           — "[y/n]" at end of permission prompt
\(yes/no\)        — "(yes/no)" variant
\[yes/no\]        — "[yes/no]" variant
\bproceed\?\s*$   — "Do you want to proceed?"
\ballow\b.*\?\s*$ — "Allow running bash command: git status?"
```

### Design decisions

- **Spinner overrides permission**: If both a spinner and old permission text are visible, the agent has already moved past the permission and is working. Spinner means "don't interrupt."
- **Permission overrides idle prompt**: Claude Code always shows `"> "` on row 23. When a `(y/n)` prompt is visible, the agent is waiting for y/n input, not a new user prompt.
- **False positive risk is low**: Regular conversation text rarely contains `(y/n)` or `[y/n]`. A false positive would only trigger a Telegram notification (not harmful). The auto-approve flow also checks `permission/patterns.go` classification, so a misdetection won't auto-approve anything dangerous.

---

## Task 2 — Wire `--continue` Flag in `Command()`

**File**: `internal/agent/claude.go`

### Problem

`Command()` (line 30-39) ignores `opts.Continue`. When a user restores tabs from a previous session, the `--continue` flag should resume the last Claude Code conversation.

### Fix

Add before the prompt check:

```go
if opts.Continue {
    args = append(args, "--continue")
}
```

Claude CLI supports `--continue` / `-c` to resume the last conversation in a project directory.

---

## Task 3 — Tests

**File**: `internal/agent/claude_test.go`

### New test cases

| Test | What it verifies |
|------|------------------|
| `TestClaudeCode_PermissionYN` | `(y/n)` and `[y/n]` variants → `NeedsPermission` |
| `TestClaudeCode_PermissionYesNo` | `(yes/no)` and `[yes/no]` → `NeedsPermission` |
| `TestClaudeCode_PermissionProceed` | `"Do you want to proceed?"` → `NeedsPermission` |
| `TestClaudeCode_PermissionAllow` | `"Allow running bash: git status?"` → `NeedsPermission` |
| `TestClaudeCode_SpinnerOverridesPermission` | Spinner + old permission text → `Working` |
| `TestClaudeCode_PermissionOverridesPrompt` | Permission + `"> "` → `NeedsPermission` (not `NeedsInput`) |
| `TestClaudeCode_CommandContinue` | `opts.Continue=true` → `["claude", "--continue"]` |
| `TestClaudeCode_CommandPromptAndContinue` | Both flags → `["claude", "--continue", "--prompt", "..."]` |

### Existing tests to preserve (6 tests, all must keep passing)

- `TestClaudeCode_SpinnerWorking`
- `TestClaudeCode_SpinnerNotMatched`
- `TestClaudeCode_SpinnerSuppressesGenericError`
- `TestClaudeCode_PromptIdleCertain`
- `TestClaudeCode_SpinnerOverridesPrompt`
- `TestClaudeCode_NoPromptNoSpinnerReturnsNo`

---

## Out of Scope (with rationale)

These optional interfaces are **not needed** for Claude Code:

| Interface | Skip reason |
|-----------|-------------|
| `ScreenFilter` | Claude Code has no sidebar — default pass-through is correct |
| `ChromeLayout` | No persistent header/footer chrome — `(0, 0)` default is correct |
| `ChromeLineFilter` | No status bar, model selector, or shortcut hints to filter |
| `SubmitDelay` | Reads stdin directly, not Bubble Tea — 0ms default is correct |
| `ImageInputFormatter` | Generic fallback works fine for Claude Code |
| `HistoryProvider` | Requires reverse-engineering Claude's history format — defer to future session |

---

## Files Touched

| File | Changes |
|------|---------|
| `internal/agent/claude.go` | Permission regex patterns, `CheckAttention()` rewrite, `Command()` `--continue` flag |
| `internal/agent/claude_test.go` | 8+ new test cases |

## Verification

```bash
go test ./internal/agent/ -run TestClaudeCode -v
go test ./internal/attention/ -v
go vet ./...
```
