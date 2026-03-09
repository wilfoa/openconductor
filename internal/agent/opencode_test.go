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

// ── False positive prevention ───────────────────────────────────
// When conversation content discusses OpenCode's UI (e.g., an AI agent
// explaining dialog keywords), the heuristic must not false-positive.
// This requires a full-height screen so the discussion text is above
// the bottom zone where TUI chrome actually lives.

func TestOpenCode_ConversationMentioningQuestionKeywords_NoFalsePositive(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := make([]string, 24)
	// Conversation about dialogs in the upper part of screen.
	lines[2] = `The dialog footer says "enter submit  esc dismiss"`
	lines[3] = `It checks for "enter confirm" and "esc dismiss" patterns`
	// Bottom: idle model selector.
	lines[22] = "  Build  Claude Opus 4.6 Anthropic · max"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain (idle), got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event)
	}
}

func TestOpenCode_ConversationMentioningPermissionKeywords_NoFalsePositive(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := make([]string, 24)
	// Conversation discusses permission UI in the upper screen.
	lines[2] = `The permission dialog shows "Allow once" and "Reject"`
	lines[3] = `With "ctrl+f fullscreen" and "Permission required" header`
	lines[4] = `The hasAllowOnce flag is set when "Allow once" is found`
	// Bottom: idle model selector.
	lines[22] = "  Build  Claude Opus 4.6 Anthropic · max"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain (idle), got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event)
	}
}

func TestOpenCode_ConversationMentioningEscInterrupt_NoFalsePositive(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := make([]string, 24)
	// Conversation mentions "esc interrupt" in upper screen area.
	lines[2] = `The "esc interrupt" pattern signals working state`
	lines[3] = `It appears in the bottom progress bar`
	// Bottom: idle shortcut hints (no actual "esc interrupt" chrome).
	lines[23] = "  ctrl+t variants  tab agents  ctrl+p commands"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain (idle), got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event)
	}
}

func TestOpenCode_RealQuestionDialog_StillDetected(t *testing.T) {
	// Verify that actual question dialogs at the bottom are still detected
	// on a full-height terminal.
	adapter := &opencodeAdapter{}
	lines := make([]string, 24)
	lines[0] = "  Some conversation"
	lines[18] = "Which framework would you like?"
	lines[19] = "1. Jest"
	lines[20] = "2. Vitest"
	lines[21] = "3. Playwright"
	lines[22] = ""
	lines[23] = "↕ select  enter submit  esc dismiss"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsAnswer {
		t.Errorf("expected NeedsAnswer, got %v", event)
	}
}

func TestOpenCode_RealPermissionDialog_StillDetected(t *testing.T) {
	// Verify that actual permission dialogs are still detected
	// on a full-height terminal.
	adapter := &opencodeAdapter{}
	lines := make([]string, 24)
	lines[0] = "  Some conversation"
	lines[10] = "⚠ Permission required"
	lines[11] = "← Write to src/main.go"
	lines[20] = "Allow once   Allow always   Reject       ctrl+f fullscreen"
	lines[23] = ""

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event)
	}
}

func TestOpenCode_PermissionRequiredAlone_NotSufficient(t *testing.T) {
	// "Permission required" alone (without button signals) should not
	// trigger permission detection — it could be conversation text.
	adapter := &opencodeAdapter{}
	lines := make([]string, 24)
	lines[2] = `The "Permission required" header appears at the top`
	// Bottom: idle model selector.
	lines[22] = "  Build  Claude Opus 4.6 Anthropic · max"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain (idle), got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput (not NeedsPermission), got %v", event)
	}
}

// ── Soft question detection (free-text questions in conversation) ─

// buildSoftQuestionScreen constructs a realistic 30-row OpenCode screen
// with conversation content containing questions in the upper area and
// idle chrome at the bottom. This mirrors the layout shown in the
// screenshot: the agent finished a plan and listed "Questions for You"
// with numbered items, then became idle.
func buildSoftQuestionScreen(heading string, questions []string) []string {
	lines := make([]string, 30)
	lines[0] = "  opencode v1.0" // header chrome
	lines[2] = "  Key Design Decisions"
	lines[3] = "  1. Use existing E2E billing API..."
	lines[4] = "  2. Hard assertions via API calls..."
	lines[6] = "" // blank separator
	// Question heading + items start around the middle.
	row := 8
	if heading != "" {
		lines[row] = "  " + heading
		row++
	}
	for i, q := range questions {
		lines[row] = fmt.Sprintf("  %d. %s", i+1, q)
		row++
	}
	// Idle chrome at the bottom.
	lines[27] = "  ▣  Plan · claude-opus-4-5 · 2m 59s"
	lines[28] = "  Plan  Claude Opus 4.5 Anthropic · max"
	lines[29] = "  ctrl+t variants  tab agents  ctrl+p commands"
	return lines
}

func TestOpenCode_SoftQuestion_QuestionsForYouHeading(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := buildSoftQuestionScreen("Questions for You", []string{
		"Scope: Should this cover daycare/summer camp payments too?",
		"Refund testing: Do you want me to verify refund actually hits CreditGuard UAT?",
		"Failed changes dashboard: Is there a dedicated admin page for managing failed charges?",
		"Priority: Given time constraints, would you prefer Option A or Option B?",
	})

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event for soft question, got nil")
	}
	if event.Type != attention.NeedsAnswer {
		t.Errorf("expected NeedsAnswer for soft questions, got %v", event.Type)
	}
	if !strings.Contains(event.Detail, "free text") && !strings.Contains(event.Detail, "question") {
		t.Errorf("expected detail mentioning questions, got %q", event.Detail)
	}
}

func TestOpenCode_SoftQuestion_QuestionsForYou_LowerCase(t *testing.T) {
	adapter := &opencodeAdapter{}
	lines := buildSoftQuestionScreen("Questions for you", []string{
		"Should we use the new API?",
		"Which database do you prefer?",
	})

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsAnswer {
		t.Errorf("expected NeedsAnswer, got %v", event)
	}
}

func TestOpenCode_SoftQuestion_MultipleQuestionMarks(t *testing.T) {
	// No "Questions for You" heading, but 3+ lines ending with "?"
	// should still trigger soft question detection.
	adapter := &opencodeAdapter{}
	lines := make([]string, 30)
	lines[2] = "  Before I proceed, I need some clarification:"
	lines[3] = "  1. Should the tests cover edge cases?"
	lines[4] = "  2. Do you want integration tests?"
	lines[5] = "  3. Should I mock the external API?"
	// Idle chrome at bottom.
	lines[28] = "  Plan  Claude Opus 4.5 Anthropic · max"
	lines[29] = "  ctrl+t variants  tab agents  ctrl+p commands"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsAnswer {
		t.Errorf("expected NeedsAnswer for multiple questions, got %v", event)
	}
}

func TestOpenCode_SoftQuestion_TwoQuestionMarks_NotEnough(t *testing.T) {
	// Only 2 lines with "?" — not enough to trigger soft question.
	adapter := &opencodeAdapter{}
	lines := make([]string, 30)
	lines[2] = "  I fixed the bug. Does it look correct?"
	lines[3] = "  Should I also fix the other file?"
	// Idle chrome at bottom.
	lines[28] = "  Plan  Claude Opus 4.5 Anthropic · max"
	lines[29] = "  ctrl+t variants  tab agents  ctrl+p commands"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	// Should be NeedsInput (plain idle), NOT NeedsAnswer.
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput (too few questions), got %v", event)
	}
}

func TestOpenCode_SoftQuestion_AgentWorking_NoDetection(t *testing.T) {
	// Agent is working (esc interrupt visible) — even if conversation has
	// questions, working state must take priority.
	adapter := &opencodeAdapter{}
	lines := make([]string, 30)
	lines[2] = "  Questions for You"
	lines[3] = "  1. Should I use React or Vue?"
	lines[4] = "  2. Do you prefer TypeScript?"
	lines[5] = "  3. What about testing framework?"
	// Working chrome at bottom (no idle shortcuts).
	lines[29] = "  · · · · ■ ■  esc interrupt"

	result, _ := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working (agent busy), got %v", result)
	}
}

func TestOpenCode_SoftQuestion_NoQuestionsInContent_JustIdle(t *testing.T) {
	// Agent is idle with normal conversation content (no questions).
	adapter := &opencodeAdapter{}
	lines := make([]string, 30)
	lines[2] = "  I've fixed the bug in main.go."
	lines[3] = "  All tests pass."
	lines[4] = "  Build clean."
	// Idle chrome.
	lines[28] = "  Plan  Claude Opus 4.5 Anthropic · max"
	lines[29] = "  ctrl+t variants  tab agents  ctrl+p commands"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsInput {
		t.Errorf("expected NeedsInput (no questions), got %v", event)
	}
}

func TestOpenCode_SoftQuestion_HardDialogStillWins(t *testing.T) {
	// When a real question dialog overlay is active (with footer), that
	// should still win over soft question detection.
	adapter := &opencodeAdapter{}
	lines := make([]string, 30)
	lines[2] = "  Questions for You"
	lines[3] = "  1. Should I use React?"
	// Dialog overlay at the bottom.
	lines[24] = "Which framework?"
	lines[25] = "1. Jest"
	lines[26] = "2. Vitest"
	lines[27] = "3. Playwright"
	lines[29] = "↕ select  enter submit  esc dismiss"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsAnswer {
		t.Errorf("expected NeedsAnswer, got %v", event)
	}
	// The detail should indicate it's a dialog (hard) question, not soft.
	if strings.Contains(event.Detail, "free text") {
		t.Errorf("dialog question should not say 'free text', got %q", event.Detail)
	}
}

func TestOpenCode_SoftQuestion_PermissionStillWins(t *testing.T) {
	// Permission dialog must still take priority over soft questions.
	adapter := &opencodeAdapter{}
	lines := make([]string, 30)
	lines[2] = "  Questions for You"
	lines[3] = "  1. Should I proceed?"
	lines[10] = "⚠ Permission required"
	lines[11] = "← Write to src/main.go"
	lines[25] = "Allow once   Allow always   Reject       ctrl+f fullscreen"

	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != attention.NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event)
	}
}

func TestOpenCode_SoftQuestion_QuestionHeadingVariants(t *testing.T) {
	// Various headings that agents use when asking questions.
	adapter := &opencodeAdapter{}
	headings := []string{
		"Questions for You",
		"Questions for you",
		"Questions for you:",
		"I have a few questions:",
		"I have some questions before proceeding:",
		"Before I begin, a few questions:",
		"A few questions:",
		"Some questions:",
	}
	for _, heading := range headings {
		lines := buildSoftQuestionScreen(heading, []string{
			"Should I include tests?",
			"Do you want TypeScript?",
			"What about documentation?",
		})
		result, event := adapter.CheckAttention(lines)
		if result != attention.Certain {
			t.Errorf("heading %q: expected Certain, got %v", heading, result)
		}
		if event == nil || event.Type != attention.NeedsAnswer {
			t.Errorf("heading %q: expected NeedsAnswer, got %v", heading, event)
		}
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

// ── IsChromeLine ────────────────────────────────────────────────

func TestOpenCode_IsChromeLine_StatusBar(t *testing.T) {
	adapter := &opencodeAdapter{}
	tests := []struct {
		line string
		want bool
	}{
		{"▣  Build · claude-opus-4-6 · 22.6s", true},
		{"▣  Build · claude-opus-4-6 · 1m32s", true},
		{"▢  Plan · gpt-4o · 5.0s", true},
		{"■  Code · gemini-2.5 · 10s", true},
		{"  ▣  Build · claude-opus-4-6 · 22.6s  ", true}, // with whitespace
	}
	for _, tt := range tests {
		got := adapter.IsChromeLine(tt.line)
		if got != tt.want {
			t.Errorf("IsChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestOpenCode_IsChromeLine_ModelSelector(t *testing.T) {
	adapter := &opencodeAdapter{}
	tests := []struct {
		line string
		want bool
	}{
		{"Build  Claude Opus 4.6 Anthropic · max", true},
		{"  Build  Claude Opus 4.6 Anthropic · max  ", true},
		{"Chat  GPT-4o OpenAI · high", true},
		{"Code  Gemini 2.5 Google · medium", true},
		{"Plan  Llama 4 Groq · max", true},
	}
	for _, tt := range tests {
		got := adapter.IsChromeLine(tt.line)
		if got != tt.want {
			t.Errorf("IsChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestOpenCode_IsChromeLine_ShortcutHints(t *testing.T) {
	adapter := &opencodeAdapter{}
	tests := []struct {
		line string
		want bool
	}{
		{"ctrl+t variants  tab agents  ctrl+p commands", true},
		{"  ctrl+p commands  ", true},
		{"  tab agents  ctrl+t variants  ", true},
	}
	for _, tt := range tests {
		got := adapter.IsChromeLine(tt.line)
		if got != tt.want {
			t.Errorf("IsChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestOpenCode_IsChromeLine_WorkingFooter(t *testing.T) {
	adapter := &opencodeAdapter{}
	tests := []struct {
		line string
		want bool
	}{
		{"· · · · ■ ■  esc interrupt", true},
		{"  ■ ■ ■  esc interrupt  ", true},
	}
	for _, tt := range tests {
		got := adapter.IsChromeLine(tt.line)
		if got != tt.want {
			t.Errorf("IsChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestOpenCode_IsChromeLine_ConversationContent(t *testing.T) {
	adapter := &opencodeAdapter{}
	tests := []struct {
		line string
		want bool
	}{
		{"I'll help you fix the bug.", false},
		{"  src/main.go", false},
		{"  + func main() {", false},
		{"  Build clean and all tests pass.", false},
		{"", false}, // blank line
		{"  Here's the Anthropic API key you need.", false}, // contains provider but no mode
	}
	for _, tt := range tests {
		got := adapter.IsChromeLine(tt.line)
		if got != tt.want {
			t.Errorf("IsChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestOpenCode_IsChromeLine_DialogFooterNotFiltered(t *testing.T) {
	// Dialog footers with "esc dismiss" should NOT be filtered — they are
	// part of the question/permission dialog content, not idle chrome.
	adapter := &opencodeAdapter{}
	tests := []struct {
		line string
		want bool
	}{
		{"↕ select  enter submit  esc dismiss", false},
		{"Allow once  Allow always  Reject  ctrl+f fullscreen  enter confirm", false},
	}
	for _, tt := range tests {
		got := adapter.IsChromeLine(tt.line)
		if got != tt.want {
			t.Errorf("IsChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestFilterChromeLines_TopLevel_OpenCode(t *testing.T) {
	lines := []string{
		"▣  Build · claude-opus-4-6 · 22.6s",
		"I'll help you fix the bug.",
		"  src/main.go",
		"Build  Claude Opus 4.6 Anthropic · max",
		"ctrl+t variants  tab agents  ctrl+p commands",
	}
	filtered := FilterChromeLines("opencode", lines)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 non-chrome lines, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "I'll help you fix the bug." {
		t.Errorf("expected conversation content, got %q", filtered[0])
	}
	if filtered[1] != "  src/main.go" {
		t.Errorf("expected file path, got %q", filtered[1])
	}
}

func TestFilterChromeLines_TopLevel_UnknownAgent(t *testing.T) {
	lines := []string{"line 1", "line 2"}
	filtered := FilterChromeLines("unknown-agent", lines)
	if len(filtered) != 2 {
		t.Fatalf("expected unchanged for unknown agent, got %d", len(filtered))
	}
}

// ── FormatImageInput ────────────────────────────────────────────

func TestOpenCode_FormatImageInput_WithCaption(t *testing.T) {
	adapter := &opencodeAdapter{}
	got := adapter.FormatImageInput(".openconductor/images/photo.jpg", "Fix this bug")
	if !strings.Contains(got, "Fix this bug") {
		t.Errorf("expected caption in output, got %q", got)
	}
	if !strings.Contains(got, ".openconductor/images/photo.jpg") {
		t.Errorf("expected path in output, got %q", got)
	}
}

func TestOpenCode_FormatImageInput_NoCaption(t *testing.T) {
	adapter := &opencodeAdapter{}
	got := adapter.FormatImageInput(".openconductor/images/photo.jpg", "")
	if !strings.Contains(got, ".openconductor/images/photo.jpg") {
		t.Errorf("expected path in output, got %q", got)
	}
}

func TestFormatImageInput_TopLevel_OpenCode(t *testing.T) {
	got := FormatImageInput("opencode", "img.png", "caption text")
	if !strings.Contains(got, "caption text") {
		t.Errorf("expected caption, got %q", got)
	}
	if !strings.Contains(got, "img.png") {
		t.Errorf("expected path, got %q", got)
	}
}

func TestFormatImageInput_TopLevel_UnknownAgent(t *testing.T) {
	// Unknown agent falls back to default format.
	got := FormatImageInput("unknown-agent", "img.png", "hello")
	if !strings.Contains(got, "hello") || !strings.Contains(got, "img.png") {
		t.Errorf("expected default format, got %q", got)
	}
}

func TestFormatImageInput_TopLevel_NoCaption(t *testing.T) {
	got := FormatImageInput("unknown-agent", "img.png", "")
	if !strings.Contains(got, "img.png") {
		t.Errorf("expected path in default format, got %q", got)
	}
	if strings.Contains(got, "null") || strings.Contains(got, "nil") {
		t.Errorf("unexpected null/nil in output: %q", got)
	}
}

// ── Permission keystrokes ───────────────────────────────────────

func TestOpenCode_ApproveKeystroke_IsEnter(t *testing.T) {
	// "Allow once" is the default selection — just Enter to confirm.
	adapter := &opencodeAdapter{}
	ks := adapter.ApproveKeystroke()
	if string(ks) != "\r" {
		t.Errorf("expected \\r, got %q", ks)
	}
}

func TestOpenCode_ApproveSessionKeystroke_NavigatesRight(t *testing.T) {
	// "Allow always" is the second option — Right arrow to navigate.
	// Enter is sent separately by the handler after SubmitDelay.
	adapter := &opencodeAdapter{}
	ks := adapter.ApproveSessionKeystroke()
	if string(ks) != "\x1b[C" {
		t.Errorf("expected Right arrow (\\x1b[C), got %q", ks)
	}
}

func TestOpenCode_DenyKeystroke_NavigatesRightTwice(t *testing.T) {
	// "Reject" is the third option — two Right arrows.
	adapter := &opencodeAdapter{}
	ks := adapter.DenyKeystroke()
	if string(ks) != "\x1b[C\x1b[C" {
		t.Errorf("expected two Right arrows, got %q", ks)
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
