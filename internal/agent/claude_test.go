// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"testing"

	"github.com/openconductorhq/openconductor/internal/attention"
)

// ── CheckAttention (claude code attention heuristics) ────────────

func TestClaudeCode_SpinnerWorking(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"star prefix", "✦ Sublimating…"},
		{"dot prefix", "· Thinking…"},
		{"asterisk prefix", "* Reading…"},
		{"three dots", "✦ Processing..."},
		{"dot three dots", "· Analyzing..."},
	}

	adapter := &claudeAdapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"some output above", "", tt.line, ""}
			result, event := adapter.CheckAttention(lines)
			if result != attention.Working {
				t.Errorf("expected Working, got %v", result)
			}
			if event != nil {
				t.Errorf("expected nil event, got %v", event)
			}
		})
	}
}

func TestClaudeCode_SpinnerNotMatched(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"no prefix", "Sublimating…"},
		{"lowercase after dot", "· lowercase…"},
		{"no ellipsis", "✦ Reading"},
		{"just dot", "·"},
		{"normal output", "Building project..."},
	}

	adapter := &claudeAdapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{tt.line}
			result, _ := adapter.CheckAttention(lines)
			if result == attention.Working {
				t.Errorf("did not expect Working for line %q", tt.line)
			}
		})
	}
}

func TestClaudeCode_SpinnerSuppressesGenericError(t *testing.T) {
	// When Claude Code shows a spinner AND output contains "error:",
	// the spinner takes priority — agent is working, not stuck on error.
	adapter := &claudeAdapter{}
	lines := []string{
		"error: some compile error in output",
		"✦ Fixing…",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working (spinner suppresses error), got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestClaudeCode_PromptIdleCertain(t *testing.T) {
	// When Claude Code shows "> " without a spinner, it's idle — Certain.
	tests := []struct {
		name  string
		lines []string
	}{
		{
			"bare prompt",
			[]string{"The answer is March 5th, 2026.", "", "> "},
		},
		{
			"prompt with trailing space",
			[]string{"some output", "> "},
		},
		{
			"trimmed to just >",
			[]string{"some output", ">"},
		},
		{
			"welcome screen prompt",
			[]string{
				"╭────────────────────────────────────────╮",
				"│ ✻ Welcome to Claude Code!              │",
				"╰────────────────────────────────────────╯",
				"",
				"> ",
			},
		},
	}

	adapter := &claudeAdapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, event := adapter.CheckAttention(tt.lines)
			if result != attention.Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != attention.NeedsInput {
				t.Errorf("expected NeedsInput, got %v", event.Type)
			}
			if event.Source != "heuristic" {
				t.Errorf("expected source 'heuristic', got %q", event.Source)
			}
		})
	}
}

func TestClaudeCode_SpinnerOverridesPrompt(t *testing.T) {
	// When both spinner and "> " are visible, spinner wins — Working.
	adapter := &claudeAdapter{}
	lines := []string{
		"> ",
		"✦ Thinking…",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working (spinner overrides prompt), got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestClaudeCode_NoPromptNoSpinnerReturnsNo(t *testing.T) {
	// No spinner, no prompt — returns No.
	adapter := &claudeAdapter{}
	lines := []string{"Building project..."}
	result, event := adapter.CheckAttention(lines)
	if result != attention.No {
		t.Errorf("expected No for no signal, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}
