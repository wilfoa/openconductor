// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"os/exec"
	"strings"
	"unicode"

	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
)

// claudeAdapter implements AgentAdapter for the Claude Code CLI.
type claudeAdapter struct{}

func init() {
	Register(&claudeAdapter{})
}

// Type returns config.AgentClaudeCode.
func (a *claudeAdapter) Type() config.AgentType {
	return config.AgentClaudeCode
}

// Command returns an *exec.Cmd that launches the "claude" CLI in the given
// repo directory.
func (a *claudeAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	args := []string{}
	if opts.Prompt != "" {
		args = append(args, "--prompt", opts.Prompt)
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = repoPath
	return cmd
}

// ApproveKeystroke returns "y\n" — Claude Code uses y/n prompts.
func (a *claudeAdapter) ApproveKeystroke() []byte { return []byte("y\n") }

// ApproveSessionKeystroke returns nil — Claude Code has no session-wide approval.
func (a *claudeAdapter) ApproveSessionKeystroke() []byte { return nil }

// DenyKeystroke returns "n\n".
func (a *claudeAdapter) DenyKeystroke() []byte { return []byte("n\n") }

// maxScanLines limits how many non-empty lines from the bottom of the
// screen we inspect. Scanning too far up risks false positives from
// stale output.
const maxScanLines = 5

// CheckAttention detects Claude Code's working/idle state from its terminal
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
func (a *claudeAdapter) CheckAttention(lastLines []string) (attention.HeuristicResult, *attention.AttentionEvent) {
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
			logging.Debug("heuristic: claude-code spinner detected",
				"line", trimmed,
			)
			break
		}
		// Claude Code's prompt: line ends with "> " (with the trailing
		// space) or the trimmed content is exactly ">".
		if !hasPrompt && (strings.HasSuffix(lastLines[i], "> ") || trimmed == ">") {
			hasPrompt = true
			logging.Debug("heuristic: claude-code prompt detected",
				"line", trimmed,
			)
		}
	}

	if hasSpinner {
		// Agent is actively working — suppress all generic patterns.
		return attention.Working, nil
	}

	if hasPrompt {
		// No spinner + prompt visible = agent is idle, waiting for input.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsInput,
			Detail: "claude code is idle, waiting for prompt",
			Source: "heuristic",
		}
	}

	logging.Debug("heuristic: claude-code no signal",
		"scanned", scanned,
	)
	return attention.No, nil
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

// BootstrapFiles returns a placeholder CLAUDE.md for the repository.
func (a *claudeAdapter) BootstrapFiles() []BootstrapFile {
	return []BootstrapFile{
		{
			Path: "CLAUDE.md",
			Content: []byte(`# CLAUDE.md — Project Context for Claude Code

## Overview
This file provides context to Claude Code about the project.

## Guidelines
- Follow existing code style and conventions
- Write tests for new functionality
- Keep changes focused and minimal
`),
		},
	}
}
