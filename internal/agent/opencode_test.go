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

// ── Gap-based sidebar detection (no vertical border) ────────────

// simulateGapScreen builds a screen where the sidebar is separated from the
// main content by a wide gap of spaces (no vertical border character).
func simulateGapScreen(contentWidth, gapWidth, sidebarWidth, height int) []string {
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		content := strings.Repeat("C", contentWidth-i%5) // vary content length
		gap := strings.Repeat(" ", gapWidth+i%5)         // gap fills the rest
		sidebar := strings.Repeat("S", sidebarWidth)
		lines[i] = content + gap + sidebar
	}
	return lines
}

func TestFilterScreen_GapDetection_CropsSidebar(t *testing.T) {
	// 60-char content + ~30-char gap + 30-char sidebar = ~120 total.
	lines := simulateGapScreen(60, 30, 30, 15)
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if strings.Contains(line, "S") {
			t.Errorf("line %d still contains sidebar content: %q", i, line)
		}
		if !strings.Contains(line, "C") {
			t.Errorf("line %d missing conversation content: %q", i, line)
		}
	}
}

func TestFilterScreen_GapDetection_RealisticOpenCode(t *testing.T) {
	// Realistic OpenCode layout WITHOUT a vertical border — just a space gap.
	pad := func(s string, w int) string {
		if len(s) >= w {
			return s[:w]
		}
		return s + strings.Repeat(" ", w-len(s))
	}
	const contentW = 120
	const sidebarW = 40

	lines := []string{
		pad("  Task title search", contentW) + pad("Dev data import script search", sidebarW),
		pad("  ┃", contentW) + pad("Context", sidebarW),
		pad("  ┃  # Commit all changes", contentW) + pad("98,441 tokens", sidebarW),
		pad("  ┃", contentW) + pad("49% used", sidebarW),
		pad("  ┃  $ git commit -m \"feat: refactor\"", contentW) + pad("$0.00 spent", sidebarW),
		pad("  ┃  Discount system:", contentW) + pad("▼ MCP", sidebarW),
		pad("  ┃  - Remove fixed-amount type", contentW) + pad("• context7 Connected", sidebarW),
		pad("  ┃  - Add duration field", contentW) + pad("• playwright Connected", sidebarW),
		pad("  ┃  - One-time discounts auto-deactivate", contentW) + pad("• polybugger Connected", sidebarW),
		pad("  ┃  - Alembic migration dc01disc0unt01", contentW) + pad("• sequential-thinking Connected", sidebarW),
		pad("  ┃", contentW) + pad("", sidebarW),
		pad("  ┃  Dev domain migration:", contentW) + pad("LSP", sidebarW),
		pad("  ┃  - Move dev to dev.ahavtz.org.il", contentW) + pad("LSPs will activate as files are read", sidebarW),
		pad("  ┃  - Update terraform, deploy scripts", contentW) + pad("", sidebarW),
		pad("  ┃", contentW) + pad("▼ Modified Files", sidebarW),
	}

	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	// Sidebar content should be removed.
	for i, line := range filtered {
		if strings.Contains(line, "context7") {
			t.Errorf("line %d contains sidebar 'context7': %q", i, line)
		}
		if strings.Contains(line, "tokens") {
			t.Errorf("line %d contains sidebar 'tokens': %q", i, line)
		}
		if strings.Contains(line, "playwright") {
			t.Errorf("line %d contains sidebar 'playwright': %q", i, line)
		}
		if strings.Contains(line, "Modified Files") {
			t.Errorf("line %d contains sidebar 'Modified Files': %q", i, line)
		}
	}

	// Conversation content should be preserved.
	foundCommit := false
	foundMigration := false
	for _, line := range filtered {
		if strings.Contains(line, "Commit all changes") {
			foundCommit = true
		}
		if strings.Contains(line, "domain migration") {
			foundMigration = true
		}
	}
	if !foundCommit {
		t.Error("filtered output missing 'Commit all changes'")
	}
	if !foundMigration {
		t.Error("filtered output missing 'domain migration'")
	}
}

func TestFilterScreen_GapDetection_NarrowGapIgnored(t *testing.T) {
	// A gap that's too narrow (< 8 columns) should not trigger detection.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "content" + "     " + "more content" + strings.Repeat(" ", 50)
	}
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	// Lines should be unchanged.
	for i, line := range filtered {
		if line != lines[i] {
			t.Errorf("line %d incorrectly cropped: got %q", i, line)
		}
	}
}

func TestFilterScreen_GapDetection_NoSidebarContent(t *testing.T) {
	// Content followed by trailing whitespace only (no sidebar) should
	// not trigger gap detection because there's no density rise after the gap.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "some content here" + strings.Repeat(" ", 100)
	}
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if line != lines[i] {
			t.Errorf("line %d incorrectly cropped: got %q", i, line)
		}
	}
}
