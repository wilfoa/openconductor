// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openconductorhq/openconductor/internal/attention"
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

// ── Scrollbar-based sidebar detection ───────────────────────────

func TestFilterScreen_ScrollbarBorder_CropsSidebar(t *testing.T) {
	// Simulate OpenCode's layout where the conversation and sidebar are
	// separated by a scrollbar character (■ or █) instead of a box-drawing │.
	// This is the layout that OpenCode v1.2.x uses with OpenTUI.
	pad := func(s string, w int) string {
		runes := []rune(s)
		if len(runes) >= w {
			return string(runes[:w])
		}
		return s + strings.Repeat(" ", w-len(runes))
	}
	const convW = 80 // conversation panel width
	const sideW = 40 // sidebar width

	// ■ at column 80 acts as the scrollbar, sidebar starts at column 82.
	lines := []string{
		pad("  opencode v1.0", convW) + "■ " + pad("Project tab icon cleanup", sideW),
		pad("  │  # Build all packages", convW) + "■ " + pad("Context", sideW),
		pad("  │", convW) + "■ " + pad("95,326 tokens", sideW),
		pad("  │  $ go build ./... 2>&1", convW) + "■ " + pad("48% used", sideW),
		pad("", convW) + "■ " + pad("$0.00 spent", sideW),
		pad("  │  All tests pass.", convW) + "■ " + pad("", sideW),
		pad("  │", convW) + "■ " + pad("▼ MCP", sideW),
		pad("  │  Build clean.", convW) + "■ " + pad("• context7 Connected", sideW),
		pad("", convW) + "■ " + pad("• playwright Connected", sideW),
		pad("  > _                  ctrl+p commands", convW) + "■ " + pad("LSP", sideW),
	}

	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if strings.Contains(line, "Context") {
			t.Errorf("line %d contains sidebar 'Context': %q", i, line)
		}
		if strings.Contains(line, "tokens") {
			t.Errorf("line %d contains sidebar 'tokens': %q", i, line)
		}
		if strings.Contains(line, "MCP") {
			t.Errorf("line %d contains sidebar 'MCP': %q", i, line)
		}
		if strings.Contains(line, "playwright") {
			t.Errorf("line %d contains sidebar 'playwright': %q", i, line)
		}
	}

	// Conversation content should be preserved.
	found := false
	for _, line := range filtered {
		if strings.Contains(line, "Build all packages") {
			found = true
			break
		}
	}
	if !found {
		t.Error("filtered output missing conversation content 'Build all packages'")
	}
}

func TestFilterScreen_FullBlockScrollbar(t *testing.T) {
	// Test with █ (FULL BLOCK) as the scrollbar character.
	pad := func(s string, w int) string {
		runes := []rune(s)
		if len(runes) >= w {
			return string(runes[:w])
		}
		return s + strings.Repeat(" ", w-len(runes))
	}

	lines := make([]string, 12)
	for i := range lines {
		conv := pad(fmt.Sprintf("  Line %d of conversation", i), 70)
		sidebar := pad(fmt.Sprintf("sidebar %d", i), 30)
		lines[i] = conv + "█" + sidebar
	}

	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if strings.Contains(line, "sidebar") {
			t.Errorf("line %d contains sidebar content: %q", i, line)
		}
	}
}

// ── Content-based sidebar detection (fallback) ──────────────────

func TestFilterScreen_ContentDetection_NoSeparator(t *testing.T) {
	// Simulate an OpenCode layout where there is NO visible separator between
	// conversation and sidebar — just a thin space gap of ~2 characters.
	// Neither border detection nor gap detection should work, but content
	// detection should catch the known sidebar patterns.
	pad := func(s string, w int) string {
		runes := []rune(s)
		if len(runes) >= w {
			return string(runes[:w])
		}
		return s + strings.Repeat(" ", w-len(runes))
	}
	const convW = 90
	const sideW = 30

	lines := []string{
		pad("  opencode v1.0", convW) + pad("Dev data import script", sideW),
		pad("  │  # Commit all changes", convW) + pad("Context", sideW),
		pad("  │", convW) + pad("98,441 tokens", sideW),
		pad("  │  $ git commit -m \"feat\"", convW) + pad("49% used", sideW),
		pad("  │  Discount system:", convW) + pad("$0.00 spent", sideW),
		pad("  │  - Remove fixed-amount", convW) + pad("▼ MCP", sideW),
		pad("  │  - Add duration field", convW) + pad("• context7 Connected", sideW),
		pad("  │  - One-time auto-deact", convW) + pad("• playwright Connected", sideW),
		pad("  │", convW) + pad("• polybugger Connected", sideW),
		pad("  │  Dev domain migration:", convW) + pad("LSP", sideW),
		pad("  │  - Move dev to dev.ahavtz", convW) + pad("LSPs will activate", sideW),
		pad("  > _ ctrl+p commands ctrl+t", convW) + pad("▼ Modified Files", sideW),
	}

	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if strings.Contains(line, "tokens") {
			t.Errorf("line %d contains sidebar 'tokens': %q", i, line)
		}
		if strings.Contains(line, "MCP") {
			t.Errorf("line %d contains sidebar 'MCP': %q", i, line)
		}
		if strings.Contains(line, "Modified Files") {
			t.Errorf("line %d contains sidebar 'Modified Files': %q", i, line)
		}
	}

	// Conversation should be preserved.
	found := false
	for _, line := range filtered {
		if strings.Contains(line, "Commit all changes") {
			found = true
			break
		}
	}
	if !found {
		t.Error("filtered output missing conversation 'Commit all changes'")
	}
}

func TestFilterScreen_ContentDetection_FewPatterns_NotEnough(t *testing.T) {
	// Only 1 sidebar pattern — not enough to trigger content detection.
	// The word "Context" in conversation should not cause false cropping.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("  Line %d. Context is important.%s", i, strings.Repeat(" ", 50))
	}
	adapter := &opencodeAdapter{}
	filtered := adapter.FilterScreen(lines)

	for i, line := range filtered {
		if !strings.Contains(line, "Context is important") {
			t.Errorf("line %d lost conversation content: %q", i, line)
		}
	}
}

// ── History loading ─────────────────────────────────────────────

func TestFormatHistory_BasicConversation(t *testing.T) {
	parts := []struct {
		Role string `json:"role"`
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{Role: "user", Type: "text", Text: "Fix the login bug"},
		{Role: "assistant", Type: "text", Text: "I'll help you fix the login bug.\nLet me look at the code."},
		{Role: "user", Type: "text", Text: "Thanks, also fix the signup page"},
	}

	lines := formatHistory(parts)
	if len(lines) == 0 {
		t.Fatal("expected non-empty history")
	}

	// Check role headers exist.
	foundYou := false
	foundAssistant := false
	for _, line := range lines {
		if strings.Contains(line, "── You") {
			foundYou = true
		}
		if strings.Contains(line, "── Assistant") {
			foundAssistant = true
		}
	}
	if !foundYou {
		t.Error("expected '── You' header in history")
	}
	if !foundAssistant {
		t.Error("expected '── Assistant' header in history")
	}

	// Check content is present.
	fullText := strings.Join(lines, "\n")
	if !strings.Contains(fullText, "Fix the login bug") {
		t.Error("expected user message content in history")
	}
	if !strings.Contains(fullText, "I'll help you") {
		t.Error("expected assistant message content in history")
	}
}

func TestFormatHistory_EmptyParts(t *testing.T) {
	parts := []struct {
		Role string `json:"role"`
		Type string `json:"type"`
		Text string `json:"text"`
	}{}
	lines := formatHistory(parts)
	if lines != nil {
		t.Errorf("expected nil for empty parts, got %v", lines)
	}
}

func TestFormatHistory_SkipsEmptyText(t *testing.T) {
	parts := []struct {
		Role string `json:"role"`
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{Role: "user", Type: "text", Text: "Hello"},
		{Role: "assistant", Type: "text", Text: ""},
		{Role: "assistant", Type: "text", Text: "World"},
	}
	lines := formatHistory(parts)
	// Empty text part should be skipped, not create an extra blank.
	fullText := strings.Join(lines, "\n")
	if !strings.Contains(fullText, "Hello") || !strings.Contains(fullText, "World") {
		t.Errorf("expected content, got %q", fullText)
	}
}

func TestFormatHistory_ConsecutiveSameRole(t *testing.T) {
	// Multiple text parts from the same role should NOT repeat the header.
	parts := []struct {
		Role string `json:"role"`
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{Role: "assistant", Type: "text", Text: "Part 1"},
		{Role: "assistant", Type: "text", Text: "Part 2"},
	}
	lines := formatHistory(parts)
	headerCount := 0
	for _, line := range lines {
		if strings.Contains(line, "── Assistant") {
			headerCount++
		}
	}
	if headerCount != 1 {
		t.Errorf("expected 1 Assistant header for consecutive parts, got %d", headerCount)
	}
}

func TestSqliteEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"it's", "it''s"},
		{"a'b'c", "a''b''c"},
		{"no quotes", "no quotes"},
	}
	for _, tt := range tests {
		got := sqliteEscape(tt.input)
		if got != tt.expected {
			t.Errorf("sqliteEscape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLoadHistory_TopLevel_UnknownAgent(t *testing.T) {
	// Unknown agent should return nil, nil.
	lines, err := LoadHistory("unknown-agent", "/some/path")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if lines != nil {
		t.Errorf("expected nil lines, got %v", lines)
	}
}

func TestLoadHistory_TopLevel_ClaudeCode(t *testing.T) {
	// Claude Code doesn't implement HistoryProvider, should return nil.
	lines, err := LoadHistory("claude-code", "/some/path")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if lines != nil {
		t.Errorf("expected nil lines for claude-code, got %v", lines)
	}
}

func TestRoleLabel(t *testing.T) {
	tests := []struct {
		role     string
		expected string
	}{
		{"user", "You"},
		{"assistant", "Assistant"},
		{"system", "System"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := roleLabel(tt.role)
		if got != tt.expected {
			t.Errorf("roleLabel(%q) = %q, want %q", tt.role, got, tt.expected)
		}
	}
}

// ── CheckAttention (opencode attention heuristics) ──────────────

func TestOpenCode_EscInterruptWorking(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := []string{
		"· · · · ■ ■  esc interrupt",
		"",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestOpenCode_IdleWithShortcuts(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := []string{
		"   ┃  Build  Claude Opus 4.5 (latest) Anthropic · max",
		"   ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
		"                                ctrl+t variants  tab agents  ctrl+p commands",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
}

func TestOpenCode_PermissionOverridesEscInterrupt(t *testing.T) {
	// When a permission modal overlays the screen, "esc interrupt" from the
	// underlying progress bar can remain in the vt10x buffer. Permission
	// must take priority — the agent cannot continue until the user responds.
	adapter := &opencodeAdapter{}
	lines := []string{
		"some output",
		"· · · · ■ ■  esc interrupt",
		"",
		"⚠ Permission required",
		"← Access external directory ~/.pub-cache/hosted/pub.dev/mapbox_maps_flutter",
		"Patterns",
		"- /Users/amir/.pub-cache/hosted/pub.dev/mapbox_maps_flutter-2.18.0/lib/src/*",
		"Allow once   Allow always   Reject       ctrl+f  fullscreen  s select  enter confirm",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected permission event, got nil")
	}
	if event.Type != attention.NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
}

func TestOpenCode_QuestionOverridesEscInterrupt(t *testing.T) {
	// Same overlay issue: question dialog must override working signal.
	adapter := &opencodeAdapter{}
	lines := []string{
		"· · · · ■ ■  esc interrupt",
		"Which framework?",
		"1. Jest",
		"2. Vitest",
		"↕ select  enter submit  esc dismiss",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected question event, got nil")
	}
	if event.Type != attention.NeedsAnswer {
		t.Errorf("expected NeedsAnswer, got %v", event.Type)
	}
}

func TestOpenCode_EscInterruptSuppressesGenericError(t *testing.T) {
	// When OpenCode is working (esc interrupt visible), error content
	// in the output should be suppressed.
	adapter := &opencodeAdapter{}
	lines := []string{
		"error: build failed",
		"· · · · ■ ■  esc interrupt",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestOpenCode_ProviderNameInConversation_NotIdle(t *testing.T) {
	// "Anthropic" appearing in conversation text (without " · " separator)
	// should NOT trigger idle detection.
	adapter := &opencodeAdapter{}
	lines := []string{
		"  I'm using the Anthropic API to call Claude.",
		"  The OpenAI client is similar.",
	}
	result, _ := adapter.CheckAttention(lines)
	if result != attention.No {
		t.Errorf("expected No for provider name in conversation, got %v", result)
	}
}

func TestOpenCode_NoAgentSignals(t *testing.T) {
	// OpenCode output without specific signals returns No.
	adapter := &opencodeAdapter{}
	lines := []string{"some random output"}
	result, event := adapter.CheckAttention(lines)
	if result != attention.No {
		t.Errorf("expected No for no signal, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestOpenCode_ModelSelectorIdle(t *testing.T) {
	// When OpenCode is idle, the footer shows the model selector instead of
	// "ctrl+p commands". The provider + " · " pattern should trigger idle.
	adapter := &opencodeAdapter{}
	lines := []string{
		"  I've fixed the bug in main.go.",
		"",
		"",
		"  Build  Claude Opus 4.6 Anthropic · max",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event)
	}
}

func TestOpenCode_ModelSelectorVariants(t *testing.T) {
	// Test multiple provider formats in the model selector.
	adapter := &opencodeAdapter{}
	cases := []string{
		"  Build  GPT-4o OpenAI · max",
		"  Build  Gemini 2.5 Google · high",
		"  Build  Llama 4 Groq · medium",
		"  Chat  Claude Sonnet 4 Anthropic · max",
	}
	for _, footer := range cases {
		lines := []string{"output", "", footer}
		result, event := adapter.CheckAttention(lines)
		if result != attention.Certain {
			t.Errorf("footer %q: expected Certain, got %v", footer, result)
		}
		if event == nil || event.Type != attention.NeedsInput {
			t.Errorf("footer %q: expected NeedsInput, got %v", footer, event)
		}
	}
}

func TestOpenCode_ModelSelectorIgnoredDuringWorking(t *testing.T) {
	// If "esc interrupt" is also present (e.g., modal overlay didn't cover
	// the model line), working should still win because no permission or
	// question dialog is active.
	adapter := &opencodeAdapter{}
	lines := []string{
		"  Build  Claude Opus 4.6 Anthropic · max",
		"· · · · ■ ■  esc interrupt",
	}
	result, _ := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working when esc interrupt is present, got %v", result)
	}
}

func TestOpenCode_CtrlPCommandsAlone(t *testing.T) {
	// Just ctrl+p commands without esc interrupt → idle.
	adapter := &opencodeAdapter{}
	lines := []string{
		"ctrl+p commands",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event)
	}
}

// ── ChromeSkipRows ──────────────────────────────────────────────

func TestOpenCode_ChromeSkipRows(t *testing.T) {
	adapter := &opencodeAdapter{}
	top, bottom := adapter.ChromeSkipRows()
	if top != 1 {
		t.Errorf("expected top=1, got %d", top)
	}
	if bottom != 2 {
		t.Errorf("expected bottom=2, got %d", bottom)
	}
}

// ── SubmitDelay ─────────────────────────────────────────────────

func TestOpenCode_SubmitDelay(t *testing.T) {
	adapter := &opencodeAdapter{}
	delay := adapter.SubmitDelay()
	if delay != 50*1_000_000 { // 50ms in nanoseconds
		t.Errorf("expected 50ms, got %v", delay)
	}
}
