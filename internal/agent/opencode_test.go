// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"strings"
	"testing"
)

// ── FilterScreen (opencode sidebar cropping) ────────────────────

// simulateOpenCodeScreen builds a fake OpenCode TUI screen with a
// conversation area on the left and a sidebar separated by │ on the right.
func simulateOpenCodeScreen(convWidth, sidebarWidth, height int) []string {
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		conv := strings.Repeat("C", convWidth)
		sidebar := strings.Repeat("S", sidebarWidth)
		lines[i] = conv + "│" + sidebar
	}
	return lines
}

func TestFilterScreen_CropsSidebar(t *testing.T) {
	// 80-char conversation + │ + 30-char sidebar = 111 total.
	lines := simulateOpenCodeScreen(80, 30, 20)
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if strings.Contains(line, "│") {
			t.Errorf("line %d still contains sidebar divider: %q", i, line)
		}
		if strings.Contains(line, "S") {
			t.Errorf("line %d still contains sidebar content: %q", i, line)
		}
		if !strings.Contains(line, "C") {
			t.Errorf("line %d missing conversation content: %q", i, line)
		}
	}
}

func TestFilterScreen_NoSidebar_ReturnsUnchanged(t *testing.T) {
	// No vertical border → no cropping.
	lines := []string{
		"Hello world",
		"This is a conversation",
		"No sidebar here",
	}
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if line != lines[i] {
			t.Errorf("line %d changed: got %q, want %q", i, line, lines[i])
		}
	}
}

func TestFilterScreen_EmptyLines(t *testing.T) {
	adapter := &opencodeAdapter{}
	result := adapter.FilterScreen(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}

	result = adapter.FilterScreen([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty for empty input, got %v", result)
	}
}

func TestFilterScreen_TrimsTrailingSpaces(t *testing.T) {
	// Conversation area with trailing spaces before the divider.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "content   " + strings.Repeat(" ", 20) + "│sidebar info"
	}
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line %d has trailing spaces: %q", i, line)
		}
	}
}

func TestFilterScreen_RealisticOpenCodeLayout(t *testing.T) {
	// Simulate a realistic OpenCode screen with mixed content.
	lines := []string{
		"  opencode v1.0                                    │  Project info        ",
		"                                                   │  Context             ",
		"  I'll help you fix the bug.                       │  102,431 tokens      ",
		"                                                   │  51% used            ",
		"  src/main.go                                      │                      ",
		"  + func main() {                                  │  ▼ MCP               ",
		"  +     fmt.Println(\"hello\")                       │  ● context7 Connected",
		"  + }                                              │  ● playwright Connect",
		"                                                   │                      ",
		"  Build clean and all tests pass.                  │  ▼ Todo              ",
		"                                                   │  [✓] Fix bug         ",
		"  > _                                              │  [✓] Run tests       ",
	}

	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	// Should not contain any sidebar content.
	for i, line := range filtered {
		if strings.Contains(line, "MCP") {
			t.Errorf("line %d contains sidebar 'MCP': %q", i, line)
		}
		if strings.Contains(line, "context7") {
			t.Errorf("line %d contains sidebar 'context7': %q", i, line)
		}
		if strings.Contains(line, "Todo") {
			t.Errorf("line %d contains sidebar 'Todo': %q", i, line)
		}
		if strings.Contains(line, "tokens") {
			t.Errorf("line %d contains sidebar 'tokens': %q", i, line)
		}
	}

	// Should contain conversation content.
	found := false
	for _, line := range filtered {
		if strings.Contains(line, "fix the bug") {
			found = true
			break
		}
	}
	if !found {
		t.Error("filtered output missing conversation content 'fix the bug'")
	}
}

func TestFilterScreen_DividerInLeftHalf_Ignored(t *testing.T) {
	// A │ that appears in the LEFT portion of the screen (e.g., inside a
	// diff view) should NOT be treated as the sidebar divider.
	lines := make([]string, 10)
	for i := range lines {
		// │ at column 5 (left 5% of a 100-char line) — should be ignored.
		lines[i] = "left │ content" + strings.Repeat(" ", 86)
	}
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	// Lines should be returned unchanged (no cropping at the left │).
	for i, line := range filtered {
		if line != lines[i] {
			t.Errorf("line %d incorrectly cropped: got %q, want %q", i, line, lines[i])
		}
	}
}

// ── FilterScreen top-level function ─────────────────────────────

func TestFilterScreen_TopLevel_OpenCode(t *testing.T) {
	lines := simulateOpenCodeScreen(80, 30, 20)
	filtered := FilterScreen("opencode", lines)

	for i, line := range filtered {
		if strings.Contains(line, "S") {
			t.Errorf("line %d still contains sidebar content: %q", i, line)
		}
	}
}

func TestFilterScreen_TopLevel_ClaudeCode_NoFilter(t *testing.T) {
	lines := []string{"line 1", "line 2", "line 3"}
	filtered := FilterScreen("claude-code", lines)

	for i, line := range filtered {
		if line != lines[i] {
			t.Errorf("line %d changed for claude-code: got %q, want %q", i, line, lines[i])
		}
	}
}

func TestFilterScreen_TopLevel_UnknownAgent(t *testing.T) {
	lines := []string{"hello"}
	filtered := FilterScreen("unknown-agent", lines)
	if filtered[0] != "hello" {
		t.Errorf("expected unchanged for unknown agent, got %q", filtered[0])
	}
}
