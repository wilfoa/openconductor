# OpenConductor — Attention Detection System

## Overview

The core innovation of OpenConductor: **agent-agnostic attention detection**. Determines when a coding agent needs human input without understanding each agent's internal protocol.

## Why Not Screenshots?

The initial instinct is to screenshot the terminal and use a vision model. But since OpenConductor owns the PTY, it already has the terminal output. A VT100 emulator renders raw ANSI output into clean text — giving the same information at a fraction of the cost.

| Approach                        | Model needed              | Cost per check  | Latency   |
|---------------------------------|---------------------------|-----------------|-----------|
| Screenshot + Vision model       | Vision model (expensive)  | ~$0.01-0.03    | ~1-2s     |
| Rendered text buffer + text LLM | Haiku-class (cheap)       | ~$0.001         | ~200-500ms|

## Two-Layer Detection Pipeline

```
PTY output stream
    │
    ▼
┌─────────────────────────────────┐
│  Layer 1: Heuristics (free)     │  ← runs on every output change
│                                  │
│  Process blocked on stdin?       │──► NEEDS ATTENTION (certain)
│  Process exited?                 │──► NEEDS ATTENTION (certain)
│  No output for 30s + alive?      │──► maybe, escalate to L2
│  Contains [y/N] or [Y/n]?       │──► NEEDS ATTENTION (certain)
│  Ends with "> " or "? "?        │──► maybe, escalate to L2
└──────────────┬──────────────────┘
               │ uncertain
               ▼
┌─────────────────────────────────┐
│  Layer 2: LLM (cheap)           │  ← runs only when heuristics unsure
│                                  │
│  Send last ~50 lines of         │
│  rendered terminal buffer        │
│  to Haiku-class model:           │
│                                  │
│  "Is this coding agent waiting   │
│   for human input? Classify:     │
│   WAITING_INPUT, WORKING,        │
│   ERROR, DONE"                   │
│                                  │
└─────────────────────────────────┘
```

## Layer 1: Heuristics

### Process State Detection (OS-level)

The most reliable signal — check if the agent process is blocked on a read syscall:

```go
// macOS: use proc_pidinfo to check process state
// Linux: read /proc/{pid}/wchan or /proc/{pid}/status
func isBlockedOnStdin(pid int) bool {
    // If the process is sleeping in a TTY read,
    // it's waiting for user input
}
```

This catches the most common case: agent printed output and is now showing a prompt.

### Terminal Output Pattern Matching

Simple regex/string matching on the last few lines of the rendered terminal buffer:

```go
type HeuristicResult int

const (
    Certain    HeuristicResult = iota  // definitely needs attention
    Uncertain                           // escalate to LLM
    No                                  // definitely working, ignore
)

func checkHeuristics(lastLines []string, processState ProcessState) HeuristicResult {
    lastLine := lastLines[len(lastLines)-1]

    // Certain: process exited
    if processState == Exited {
        return Certain
    }

    // Certain: process blocked on stdin
    if processState == BlockedOnRead {
        return Certain
    }

    // Certain: explicit permission prompts
    if matchesPermissionPrompt(lastLine) {
        // [y/N], [Y/n], (yes/no), Allow?, Approve?, etc.
        return Certain
    }

    // Uncertain: ends with prompt-like characters
    if endsWithPrompt(lastLine) {
        // "> ", "? ", "$ ", ">>> ", etc.
        return Uncertain
    }

    // Uncertain: no output for a while but process alive
    if timeSinceLastOutput > 30*time.Second && processState == Running {
        return Uncertain
    }

    // No: output is still flowing
    return No
}
```

### Agent-Specific Heuristic Hints

Each agent adapter can contribute additional heuristics:

```go
// Claude Code specific
func (a *ClaudeCodeAdapter) AttentionHints(lastLines []string) *AttentionType {
    // Claude Code shows a specific prompt when waiting
    // Claude Code asks for permission with specific formatting
}

// Codex specific
func (a *CodexAdapter) AttentionHints(lastLines []string) *AttentionType {
    // Codex has its own prompt patterns
}
```

## Layer 2: LLM Classification

When heuristics are uncertain, send the terminal buffer to a fast, cheap LLM:

### Input

Last ~50 lines of the rendered terminal buffer (clean text, no ANSI codes). This is typically 1-3KB of text — very cheap to process.

### Prompt

```
You are analyzing the terminal output of a coding agent (like Claude Code, Codex, or Gemini CLI).
Based on the terminal output below, classify the agent's current state.

Reply with exactly one of:
- WAITING_INPUT: Agent is waiting for the user to type something (asked a question, showing a prompt, requesting clarification)
- NEEDS_PERMISSION: Agent is asking for permission to perform an action (file edit, command execution, etc.)
- DONE: Agent has finished its current task and is presenting results
- ERROR: Agent encountered an error and may need help
- WORKING: Agent is still actively working (producing output, running commands, etc.)
- STUCK: Agent appears to be looping or making no progress

Terminal output:
---
{last 50 lines}
---

Classification:
```

### Model Selection

Use the cheapest/fastest model available:
- Claude: `claude-haiku-4-5` (~$0.001 per classification)
- OpenAI: `gpt-4o-mini`
- Google: `gemini-2.0-flash`

The model choice can be configurable. The classification task is simple enough that any small model handles it well.

### Throttling

- Don't call the LLM more than once every 5 seconds per session
- Cache the last classification — don't re-classify if the buffer hasn't changed
- If the LLM returns WORKING, back off and don't check again for 15 seconds

## Attention Types

```go
type AttentionType int

const (
    NeedsInput      AttentionType = iota  // agent asked a question
    NeedsPermission                        // agent wants to do something risky
    NeedsReview                            // agent finished work
    HitError                               // agent encountered an error
    Stuck                                  // agent appears to be looping/stuck
)
```

Each type drives different notification urgency and messaging:

| Type            | Notification message example              | Urgency |
|-----------------|-------------------------------------------|---------|
| NeedsInput      | "Agent is asking a question"              | Medium  |
| NeedsPermission | "Agent needs permission to proceed"       | High    |
| NeedsReview     | "Agent finished, ready for review"        | Medium  |
| HitError        | "Agent encountered an error"              | High    |
| Stuck           | "Agent may be stuck"                      | Low     |

## Terminal Buffer (VT100 Rendering)

Raw PTY output contains ANSI escape codes that make text parsing unreliable:

```
\e[32m✓\e[0m Task complete\e[K\n\e[1m>\e[0m \e[?25h
```

A VT100 emulator renders this into what a human would see:

```
✓ Task complete
>
```

Implementation:

```go
type TerminalBuffer struct {
    lines       []string    // rendered screen content
    changed     bool        // new output since last check
    lastChange  time.Time   // when output last changed
    mu          sync.RWMutex
}

// Feed raw PTY output through VT100 parser
func (tb *TerminalBuffer) Write(p []byte) (n int, err error) {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    // Parse ANSI sequences, update screen buffer
    tb.parser.Process(p)
    tb.lines = tb.parser.Screen()
    tb.changed = true
    tb.lastChange = time.Now()

    return len(p), nil
}

// Get last N lines for attention detection
func (tb *TerminalBuffer) LastLines(n int) []string {
    tb.mu.RLock()
    defer tb.mu.RUnlock()

    if len(tb.lines) < n {
        return tb.lines
    }
    return tb.lines[len(tb.lines)-n:]
}
```

## Polling Strategy

```
On PTY output received:
    1. Update terminal buffer
    2. Run Layer 1 heuristics
    3. If Certain → emit attention event immediately
    4. If Uncertain → debounce 2 seconds, then run Layer 2
    5. If No → do nothing

On no output for 30 seconds:
    1. Check if process is alive
    2. If alive → run Layer 2 (might be stuck or silently waiting)
    3. If dead → emit attention event (process exited)
```
