// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"strings"
	"unicode"
)

// permissionPatterns are lowercase strings that indicate the process is
// asking for a permission decision from the user.
var permissionPatterns = []string{
	"[y/n]",
	"(yes/no)",
	"allow?",
	"approve?",
	"do you want to proceed",
	"permission denied",
	"confirm?",
}

// errorPatterns are lowercase strings that indicate the process has
// encountered an error.
var errorPatterns = []string{
	"error:",
	"failed",
	"panic:",
	"fatal:",
}

// promptSuffixes are line endings that suggest an interactive prompt is
// waiting for input, though with less certainty than explicit patterns.
var promptSuffixes = []string{
	"> ",
	"? ",
	"$ ",
	">>> ",
}

// donePatterns are lowercase strings that indicate the process has
// completed its task.
var donePatterns = []string{
	"task completed",
	"all done",
	"completed",
	"finished",
}

// maxScanLines limits how many non-empty lines from the bottom of the
// screen we inspect. Scanning too far up risks false positives from
// stale output.
const maxScanLines = 5

// Agent type constants used for agent-specific heuristic patterns.
// These mirror config.AgentType values but live in the attention package
// to avoid an import cycle.
const (
	AgentClaudeCode = "claude-code"
	AgentOpenCode   = "opencode"
)

// CheckHeuristics applies L1 heuristic checks against recent terminal output
// and process state to determine if the project needs user attention.
//
// agentType identifies the coding agent (e.g. "claude-code", "opencode") to
// enable agent-specific pattern matching. Pass "" for generic-only checks.
//
// It scans up to maxScanLines non-empty lines from the bottom of the screen.
// All pattern matching is case-insensitive.
//
// It returns a HeuristicResult indicating confidence level and, when the
// result is Certain or Uncertain, an AttentionEvent describing what was found.
func CheckHeuristics(lastLines []string, processState ProcessState, agentType string) (HeuristicResult, *AttentionEvent) {
	// Check process state first — these are the highest confidence signals.
	if processState == Exited {
		return Certain, &AttentionEvent{
			Type:   NeedsReview,
			Detail: "process has exited",
			Source: "heuristic",
		}
	}

	if processState == BlockedOnRead {
		return Certain, &AttentionEvent{
			Type:   NeedsInput,
			Detail: "process is waiting for input",
			Source: "heuristic",
		}
	}

	// Agent-specific heuristics run before generic patterns. They can:
	// - Suppress false positives by returning No (agent is clearly working)
	// - Detect idle/done states with higher certainty than generic patterns
	if agentType != "" {
		result, event := checkAgentSpecific(lastLines, agentType)
		if result != No {
			return result, event
		}
	}

	// Scan lines from bottom to top for the most recent relevant output.
	scanned := 0
	for i := len(lastLines) - 1; i >= 0 && scanned < maxScanLines; i-- {
		line := lastLines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		scanned++

		lower := strings.ToLower(trimmed)

		// Check permission patterns (highest priority).
		for _, pattern := range permissionPatterns {
			if strings.Contains(lower, pattern) {
				return Certain, &AttentionEvent{
					Type:   NeedsPermission,
					Detail: "permission prompt detected: " + pattern,
					Source: "heuristic",
				}
			}
		}

		// Check error patterns.
		for _, pattern := range errorPatterns {
			if strings.Contains(lower, pattern) {
				return Certain, &AttentionEvent{
					Type:   HitError,
					Detail: "error detected: " + trimmed,
					Source: "heuristic",
				}
			}
		}

		// Check done patterns.
		for _, pattern := range donePatterns {
			if strings.Contains(lower, pattern) {
				return Certain, &AttentionEvent{
					Type:   NeedsReview,
					Detail: "completion detected: " + pattern,
					Source: "heuristic",
				}
			}
		}

		// Check prompt suffixes (uncertain — could be normal output).
		// Only check on the last non-empty line (most recent).
		if scanned == 1 {
			for _, suffix := range promptSuffixes {
				if strings.HasSuffix(line, suffix) {
					return Uncertain, &AttentionEvent{
						Type:   NeedsInput,
						Detail: "possible prompt detected: " + trimmed,
						Source: "heuristic",
					}
				}
			}
		}
	}

	return No, nil
}

// checkAgentSpecific runs agent-specific heuristics on the bottom terminal
// lines. Returns No if no agent-specific pattern matched (fall through to
// generic checks).
func checkAgentSpecific(lastLines []string, agentType string) (HeuristicResult, *AttentionEvent) {
	switch agentType {
	case AgentClaudeCode:
		return checkClaudeCode(lastLines)
	case AgentOpenCode:
		return checkOpenCode(lastLines)
	default:
		return No, nil
	}
}

// checkClaudeCode detects Claude Code's working/idle state from its terminal
// output.
//
// Working: Claude Code shows an animated spinner line like "✦ Sublimating…"
// or "· Sublimating…" — the prefix alternates between ✦ (U+2726) and ·
// (U+00B7) while a verb + ellipsis describes the current activity.
//
// Idle: When the spinner is absent, Claude Code shows "> " as its input
// prompt. The absence of a spinner + presence of "> " is a definitive
// signal that the agent is waiting for input (Certain, not Uncertain).
// This avoids relying on the L2 classifier for the most common idle case.
func checkClaudeCode(lastLines []string) (HeuristicResult, *AttentionEvent) {
	hasSpinner := false
	hasPrompt := false
	scanned := 0

	for i := len(lastLines) - 1; i >= 0 && scanned < maxScanLines; i-- {
		trimmed := strings.TrimSpace(lastLines[i])
		if trimmed == "" {
			continue
		}
		scanned++

		if isClaudeCodeSpinner(trimmed) {
			hasSpinner = true
			break
		}
		// Claude Code's prompt: line ends with "> " (with the trailing
		// space) or the trimmed content is exactly ">".
		if !hasPrompt && (strings.HasSuffix(lastLines[i], "> ") || trimmed == ">") {
			hasPrompt = true
		}
	}

	if hasSpinner {
		// Agent is actively working — suppress all generic patterns.
		return Working, nil
	}

	if hasPrompt {
		// No spinner + prompt visible = agent is idle, waiting for input.
		return Certain, &AttentionEvent{
			Type:   NeedsInput,
			Detail: "claude code is idle, waiting for prompt",
			Source: "heuristic",
		}
	}

	return No, nil
}

// isClaudeCodeSpinner returns true if the line matches Claude Code's animated
// status pattern: a prefix character (✦ or ·) followed by a space and a
// capitalized verb ending in "…".
//
// Examples:
//
//	"✦ Sublimating…"  → true
//	"· Thinking…"     → true
//	"· some output"   → false (no trailing …)
//	"normal text"     → false (no prefix)
func isClaudeCodeSpinner(line string) bool {
	// Check for ✦ (U+2726 four-pointed star) prefix.
	if rest, ok := strings.CutPrefix(line, "✦ "); ok {
		return isVerbEllipsis(rest)
	}
	// Check for · (U+00B7 middle dot) prefix.
	if rest, ok := strings.CutPrefix(line, "· "); ok {
		return isVerbEllipsis(rest)
	}
	// Check for * (ASCII asterisk, sometimes seen in plain captures).
	if rest, ok := strings.CutPrefix(line, "* "); ok {
		return isVerbEllipsis(rest)
	}
	return false
}

// isVerbEllipsis returns true if s starts with an uppercase letter and ends
// with "…" (U+2026 horizontal ellipsis) or "..." (three ASCII dots).
func isVerbEllipsis(s string) bool {
	if s == "" {
		return false
	}
	runes := []rune(s)
	if !unicode.IsUpper(runes[0]) {
		return false
	}
	return strings.HasSuffix(s, "…") || strings.HasSuffix(s, "...")
}

// checkOpenCode detects OpenCode's working/idle/permission/question state
// from its terminal output.
//
// Working: OpenCode shows a progress bar with "esc interrupt" at the bottom
// while an LLM is actively generating a response.
//
// Permission dialog: OpenCode overlays a modal with the header
// "Permission required" (with a ⚠ prefix) and buttons
// "Allow once  Allow always  Reject" at the bottom. The action being
// requested appears as "← <Action> <detail>" inside the modal.
// Example screenshot content:
//
//	⚠ Permission required
//	← Access external directory ~/Downloads/...
//	Patterns
//	- /path/to/glob/*
//	Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm
//
// Question dialog: OpenCode shows a multi-option question modal when the agent
// needs a decision from the user. The footer is uniquely:
//
//	↕ select  enter submit  esc dismiss
//
// Idle/done: The progress bar disappears and the bottom shows keyboard
// shortcuts like "ctrl+t variants  tab agents  ctrl+p commands" without
// "esc interrupt". This means the agent has finished and is waiting for
// the user's next prompt.
func checkOpenCode(lastLines []string) (HeuristicResult, *AttentionEvent) {
	hasEscInterrupt := false
	hasIdleShortcuts := false
	hasPermissionRequired := false
	hasAllowOnce := false
	hasQuestionDialog := false

	// Scan all visible lines (not just maxScanLines) because the permission
	// and question dialogs span multiple rows and header text may appear
	// higher up the screen while the button row is at the bottom.
	for i := len(lastLines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lastLines[i])
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		if strings.Contains(lower, "esc interrupt") {
			hasEscInterrupt = true
		}
		if strings.Contains(lower, "ctrl+p commands") || strings.Contains(lower, "ctrl+t variants") {
			hasIdleShortcuts = true
		}
		if strings.Contains(lower, "permission required") {
			hasPermissionRequired = true
		}
		// "Allow once" appears in the button row of the permission dialog.
		if strings.Contains(lower, "allow once") || strings.Contains(lower, "allow always") {
			hasAllowOnce = true
		}
		// "enter submit  esc dismiss" or "enter confirm  esc dismiss" is
		// the footer of OpenCode's question/selection dialog. Match either
		// variant paired with "esc dismiss".
		if (strings.Contains(lower, "enter submit") || strings.Contains(lower, "enter confirm")) && strings.Contains(lower, "esc dismiss") {
			hasQuestionDialog = true
		}
	}

	if hasEscInterrupt {
		// Agent is actively working — suppress generic patterns.
		return Working, nil
	}

	if hasPermissionRequired || hasAllowOnce {
		// Permission dialog is visible.
		return Certain, &AttentionEvent{
			Type:   NeedsPermission,
			Detail: "opencode permission dialog detected",
			Source: "heuristic",
		}
	}

	if hasQuestionDialog {
		// Question dialog is visible — agent is asking for a decision.
		return Certain, &AttentionEvent{
			Type:   NeedsAnswer,
			Detail: "opencode question dialog detected",
			Source: "heuristic",
		}
	}

	if hasIdleShortcuts {
		// Agent is idle, waiting for user input.
		return Certain, &AttentionEvent{
			Type:   NeedsInput,
			Detail: "opencode is idle, waiting for prompt",
			Source: "heuristic",
		}
	}

	return No, nil
}
