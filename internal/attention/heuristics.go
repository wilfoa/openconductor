// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"strings"

	"github.com/openconductorhq/openconductor/internal/logging"
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

// CheckHeuristics applies L1 heuristic checks against recent terminal output
// and process state to determine if the project needs user attention.
//
// checker is an optional AttentionChecker (typically an agent adapter) that
// provides agent-specific pattern matching. Pass nil for generic-only checks
// (unknown agent types).
//
// It scans up to maxScanLines non-empty lines from the bottom of the screen.
// All pattern matching is case-insensitive.
//
// It returns a HeuristicResult indicating confidence level and, when the
// result is Certain or Uncertain, an AttentionEvent describing what was found.
func CheckHeuristics(lastLines []string, processState ProcessState, checker AttentionChecker) (HeuristicResult, *AttentionEvent) {
	// Check process state first — these are the highest confidence signals.
	if processState == Exited {
		logging.Debug("heuristic: process exited")
		return Certain, &AttentionEvent{
			Type:   NeedsReview,
			Detail: "process has exited",
			Source: "heuristic",
		}
	}

	if processState == BlockedOnRead {
		logging.Debug("heuristic: process blocked on read")
		return Certain, &AttentionEvent{
			Type:   NeedsInput,
			Detail: "process is waiting for input",
			Source: "heuristic",
		}
	}

	// Agent-specific heuristics run before generic patterns. They can:
	// - Suppress false positives by returning Working (agent is clearly busy)
	// - Detect idle/done states with higher certainty than generic patterns
	if checker != nil {
		result, event := checker.CheckAttention(lastLines)
		if result != No {
			if event != nil {
				logging.Debug("heuristic: agent-specific match",
					"result", result,
					"type", event.Type.String(),
					"detail", event.Detail,
				)
			} else {
				logging.Debug("heuristic: agent-specific working signal")
			}
			return result, event
		}

		// Known agent returned No — skip generic patterns to avoid
		// false positives from broad matches like "error:" or "> " in
		// normal agent output. Only agent-specific and process-state
		// heuristics apply when a checker is provided.
		logging.Debug("heuristic: agent checker returned no signal")
		return No, nil
	}

	// Generic patterns: only when no checker is provided (unknown agent
	// types where we have no agent-specific heuristics to rely on).
	return checkGenericPatterns(lastLines)
}

// checkGenericPatterns scans the last few non-empty lines for broad patterns
// that suggest the process needs attention. Only used for unknown agent types.
func checkGenericPatterns(lastLines []string) (HeuristicResult, *AttentionEvent) {
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
				logging.Debug("heuristic: generic permission match",
					"pattern", pattern,
					"line", trimmed,
				)
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
				logging.Debug("heuristic: generic error match",
					"pattern", pattern,
					"line", trimmed,
				)
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
				logging.Debug("heuristic: generic done match",
					"pattern", pattern,
					"line", trimmed,
				)
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
					logging.Debug("heuristic: generic prompt suffix match",
						"suffix", suffix,
						"line", trimmed,
					)
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
