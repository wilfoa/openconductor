// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	// When Claude Code shows its prompt without a spinner, it's idle — Certain.
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
		{
			"unicode prompt U+203A",
			[]string{"some output", "› "},
		},
		{
			"unicode prompt bare",
			[]string{"some output", "›"},
		},
		{
			"unicode prompt with trailing spaces",
			[]string{"some output", "›                                     "},
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

// ── Completion summary detection tests ──────────────────────────

func TestClaudeCode_CompletionSummaryIdle(t *testing.T) {
	// "* Worked for Ns" without an active spinner signals idle.
	tests := []struct {
		name  string
		lines []string
	}{
		{
			"worked for seconds",
			[]string{
				"Want me to add tests for any of these gaps?",
				"* Worked for 56s",
				"",
				"› ",
			},
		},
		{
			"worked for minutes",
			[]string{
				"Done refactoring the module.",
				"* Worked for 2m 30s",
				"› ",
			},
		},
		{
			"star prefix without prompt visible",
			[]string{
				"All changes applied.",
				"* Worked for 10s",
			},
		},
		{
			"diamond prefix",
			[]string{
				"Finished.",
				"✦ Worked for 120s",
			},
		},
		{
			"dot prefix",
			[]string{
				"Complete.",
				"· Worked for 5s",
			},
		},
		{
			"real screenshot scenario",
			[]string{
				"The biggest risk areas are the admin edit/delete operations.",
				"* Worked for 56s",
				"────────────────────────────────────────",
				"› ",
				"→ assign_it git:(main) | in: 1.5K out: 54.5K",
				"F2 rename  Ctrl+C exit",
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
		})
	}
}

func TestClaudeCode_CompletionNotMistokenAsSpinner(t *testing.T) {
	// "* Worked for 56s" must NOT be detected as a spinner.
	adapter := &claudeAdapter{}
	lines := []string{"* Worked for 56s"}
	result, _ := adapter.CheckAttention(lines)
	if result == attention.Working {
		t.Error("completion summary incorrectly detected as Working spinner")
	}
}

func TestClaudeCode_SpinnerOverridesCompletion(t *testing.T) {
	// If a spinner appears AFTER the completion line, spinner wins.
	adapter := &claudeAdapter{}
	lines := []string{
		"* Worked for 56s",
		"✦ Thinking…",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working (spinner overrides completion), got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

// ── Permission detection tests ──────────────────────────────────

func TestClaudeCode_PermissionYN(t *testing.T) {
	// "(y/n)" and "[y/n]" variants should be detected as NeedsPermission.
	tests := []struct {
		name string
		line string
	}{
		{"parens y/n", "Do you want to proceed? (y/n)"},
		{"brackets y/n", "Allow editing of src/main.go? [y/n]"},
		{"y/n in middle of line", "Some context (y/n) here"},
	}

	adapter := &claudeAdapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"some context", tt.line}
			result, event := adapter.CheckAttention(lines)
			if result != attention.Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != attention.NeedsPermission {
				t.Errorf("expected NeedsPermission, got %v", event.Type)
			}
			if event.Source != "heuristic" {
				t.Errorf("expected source 'heuristic', got %q", event.Source)
			}
		})
	}
}

func TestClaudeCode_PermissionYesNo(t *testing.T) {
	// "(yes/no)" and "[yes/no]" variants should be detected as NeedsPermission.
	tests := []struct {
		name string
		line string
	}{
		{"parens yes/no", "Continue with changes? (yes/no)"},
		{"brackets yes/no", "Overwrite existing file? [yes/no]"},
	}

	adapter := &claudeAdapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{tt.line}
			result, event := adapter.CheckAttention(lines)
			if result != attention.Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != attention.NeedsPermission {
				t.Errorf("expected NeedsPermission, got %v", event.Type)
			}
		})
	}
}

func TestClaudeCode_PermissionProceed(t *testing.T) {
	// "Do you want to proceed?" should be detected as NeedsPermission.
	adapter := &claudeAdapter{}
	lines := []string{
		"  Edit main.go",
		"  - if obj != nil {",
		"  + if obj == nil { return }",
		"",
		"  Do you want to proceed?",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != attention.NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
}

func TestClaudeCode_PermissionAllow(t *testing.T) {
	// "Allow running bash command: git status?" should be detected.
	tests := []struct {
		name string
		line string
	}{
		{"allow running", "Allow running bash command: git status?"},
		{"allow editing", "Allow editing of src/main.go?"},
		{"allow creating", "Allow creating file .env?"},
		{"allow reading", "Allow reading file secrets.txt?"},
	}

	adapter := &claudeAdapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{tt.line}
			result, event := adapter.CheckAttention(lines)
			if result != attention.Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != attention.NeedsPermission {
				t.Errorf("expected NeedsPermission, got %v", event.Type)
			}
		})
	}
}

func TestClaudeCode_SpinnerOverridesPermission(t *testing.T) {
	// When both spinner and old permission text are visible, spinner wins.
	// The permission text is stale from a prior prompt.
	adapter := &claudeAdapter{}
	lines := []string{
		"Do you want to proceed? (y/n)",
		"✦ Fixing…",
	}
	result, event := adapter.CheckAttention(lines)
	if result != attention.Working {
		t.Errorf("expected Working (spinner overrides permission), got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestClaudeCode_PermissionOverridesPrompt(t *testing.T) {
	// When both permission and "> " are visible, permission wins.
	// Claude Code always shows "> " on row 23 but the y/n prompt is
	// the one that matters.
	adapter := &claudeAdapter{}
	lines := make([]string, 24)
	lines[2] = "  Edit main.go"
	lines[3] = "  - if obj != nil {"
	lines[4] = "  + if obj == nil { return }"
	lines[6] = "  Do you want to proceed? (y/n)"
	for i := 7; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
	result, event := adapter.CheckAttention(lines)
	if result != attention.Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != attention.NeedsPermission {
		t.Errorf("expected NeedsPermission (not NeedsInput), got %v", event.Type)
	}
}

// ── Command() tests ─────────────────────────────────────────────

func TestClaudeCode_CommandContinue(t *testing.T) {
	adapter := &claudeAdapter{}
	cmd := adapter.Command("/tmp/repo", LaunchOptions{Continue: true})
	args := cmd.Args
	// args[0] is "claude", the rest are flags.
	found := false
	for _, a := range args {
		if a == "--continue" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --continue in args, got %v", args)
	}
}

func TestClaudeCode_CommandPromptAndContinue(t *testing.T) {
	adapter := &claudeAdapter{}
	cmd := adapter.Command("/tmp/repo", LaunchOptions{
		Continue: true,
		Prompt:   "fix the bug",
	})
	args := cmd.Args
	// Expect: ["claude", "--continue", "--prompt", "fix the bug"]
	continueIdx := -1
	promptIdx := -1
	for i, a := range args {
		if a == "--continue" {
			continueIdx = i
		}
		if a == "--prompt" {
			promptIdx = i
		}
	}
	if continueIdx < 0 {
		t.Fatalf("expected --continue in args, got %v", args)
	}
	if promptIdx < 0 {
		t.Fatalf("expected --prompt in args, got %v", args)
	}
	if continueIdx >= promptIdx {
		t.Errorf("expected --continue before --prompt, got continue=%d prompt=%d in %v",
			continueIdx, promptIdx, args)
	}
	// Verify prompt value follows --prompt flag.
	if promptIdx+1 >= len(args) || args[promptIdx+1] != "fix the bug" {
		t.Errorf("expected prompt value 'fix the bug', got %v", args)
	}
}

func TestClaudeCode_CommandNoFlags(t *testing.T) {
	adapter := &claudeAdapter{}
	cmd := adapter.Command("/tmp/repo", LaunchOptions{})
	// Just "claude" with no extra args.
	if len(cmd.Args) != 1 {
		t.Errorf("expected 1 arg (just claude), got %v", cmd.Args)
	}
	if cmd.Dir != "/tmp/repo" {
		t.Errorf("expected Dir=/tmp/repo, got %q", cmd.Dir)
	}
}

// ── LoadHistory tests ───────────────────────────────────────────

func TestClaudeCode_LoadHistory_NoProjectDir(t *testing.T) {
	// A repo path that has no corresponding Claude project dir.
	adapter := &claudeAdapter{}
	lines, err := adapter.LoadHistory("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if lines != nil {
		t.Errorf("expected nil lines, got %v", lines)
	}
}

func TestClaudeCode_LoadHistory_EmptyDir(t *testing.T) {
	// Create a temp dir that mimics a Claude project dir but has no sessions.
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, ".claude", "projects", "-tmp-test-repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// findLatestSession should return "", nil for empty directory.
	path, err := findLatestSession(projectDir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestClaudeCode_LoadHistory_ParsesMessages(t *testing.T) {
	// Create a minimal session JSONL with user and assistant messages.
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "test-session.jsonl")

	// Build JSONL content.
	userMsg := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": "fix the bug in main.go",
		},
	}
	assistantMsg := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "thinking", "thinking": "Let me analyze the bug..."},
				{"type": "text", "text": "I found the bug. Here's the fix:"},
			},
		},
	}
	// A progress message that should be skipped.
	progressMsg := map[string]interface{}{
		"type": "progress",
		"data": map[string]interface{}{
			"type": "tool_progress",
		},
	}

	var lines []string
	for _, msg := range []interface{}{userMsg, assistantMsg, progressMsg} {
		b, _ := json.Marshal(msg)
		lines = append(lines, string(b))
	}

	content := []byte(joinLines(lines))
	if err := os.WriteFile(sessionFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := parseSessionMessages(sessionFile)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have: "── You ──", user text, "", "── Assistant ──", assistant text.
	foundYou := false
	foundAssistant := false
	foundUserText := false
	foundAssistantText := false
	foundThinking := false

	for _, l := range result {
		if l == "── You ──" {
			foundYou = true
		}
		if l == "── Assistant ──" {
			foundAssistant = true
		}
		if l == "fix the bug in main.go" {
			foundUserText = true
		}
		if l == "I found the bug. Here's the fix:" {
			foundAssistantText = true
		}
		if l == "Let me analyze the bug..." {
			foundThinking = true
		}
	}

	if !foundYou {
		t.Error("expected '── You ──' header in output")
	}
	if !foundAssistant {
		t.Error("expected '── Assistant ──' header in output")
	}
	if !foundUserText {
		t.Error("expected user message text in output")
	}
	if !foundAssistantText {
		t.Error("expected assistant text in output")
	}
	if foundThinking {
		t.Error("thinking blocks should be excluded from output")
	}
}

func TestClaudeCode_LoadHistory_ToolResultSkipped(t *testing.T) {
	// User messages with tool_result content (array) should be skipped.
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "test-session.jsonl")

	toolResultMsg := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role": "user",
			"content": []map[string]interface{}{
				{
					"type":        "tool_result",
					"tool_use_id": "toolu_123",
					"content":     "file contents here",
				},
			},
		},
	}
	b, _ := json.Marshal(toolResultMsg)
	if err := os.WriteFile(sessionFile, b, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := parseSessionMessages(sessionFile)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for tool_result-only messages, got %v", result)
	}
}

func TestClaudeCode_ClaudeProjectDir(t *testing.T) {
	// Verify the path encoding logic.
	dir := claudeProjectDir("/nonexistent/path/that/will/not/exist")
	if dir != "" {
		t.Errorf("expected empty string for nonexistent path, got %q", dir)
	}
}

// joinLines joins string slices with newline separators.
func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

// ── OutputFilter (CSI filter tests) ─────────────────────────────────────────

func TestClaudeCode_OutputFilterInterface(t *testing.T) {
	adapter := &claudeAdapter{}
	f := adapter.NewOutputFilter()
	if f == nil {
		t.Fatal("NewOutputFilter() returned nil")
	}
}

func TestClaudeCode_FilterPassthrough(t *testing.T) {
	// Data with no ESC bytes should pass through unchanged.
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("Hello, world! Normal terminal output.\r\n")
	got := f(input)
	if string(got) != string(input) {
		t.Errorf("expected passthrough, got %q", got)
	}
}

func TestClaudeCode_FilterStripKittyKeyboard(t *testing.T) {
	// CSI > 1 u (kitty keyboard enable) must be stripped.
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("before\x1b[>1uafter")
	got := f(input)
	if string(got) != "beforeafter" {
		t.Errorf("expected %q, got %q", "beforeafter", got)
	}
}

func TestClaudeCode_FilterStripKittyKeyboardDisable(t *testing.T) {
	// CSI < u (kitty keyboard disable/pop) must be stripped.
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("before\x1b[<uafter")
	got := f(input)
	if string(got) != "beforeafter" {
		t.Errorf("expected %q, got %q", "beforeafter", got)
	}
}

func TestClaudeCode_FilterStripExtendedSGR(t *testing.T) {
	// CSI > 4 ; 2 m (extended SGR) must be stripped.
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("before\x1b[>4;2mafter")
	got := f(input)
	if string(got) != "beforeafter" {
		t.Errorf("expected %q, got %q", "beforeafter", got)
	}
}

func TestClaudeCode_FilterStripEqualPrefix(t *testing.T) {
	// CSI = 1 c (DA3 tertiary device attributes) must be stripped.
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("before\x1b[=1cafter")
	got := f(input)
	if string(got) != "beforeafter" {
		t.Errorf("expected %q, got %q", "beforeafter", got)
	}
}

func TestClaudeCode_FilterPreserveNormalCSI(t *testing.T) {
	// Normal CSI sequences (no extended prefix) must be preserved.
	tests := []struct {
		name  string
		input string
	}{
		{"SGR bold", "hello\x1b[1mworld"},
		{"cursor move", "\x1b[10;20Htext"},
		{"erase line", "\x1b[2Kline"},
		{"private mode set", "\x1b[?25hcursor"},
		{"private mode reset", "\x1b[?25lcursor"},
		{"SGR color", "\x1b[38;5;196mred"},
	}
	f := (&claudeAdapter{}).NewOutputFilter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f([]byte(tt.input))
			if string(got) != tt.input {
				t.Errorf("expected %q preserved, got %q", tt.input, got)
			}
		})
	}
}

func TestClaudeCode_FilterPreserveOtherEsc(t *testing.T) {
	// Non-CSI escape sequences (OSC, SS3, etc.) must be preserved.
	tests := []struct {
		name  string
		input string
	}{
		{"OSC title", "\x1b]0;title\x07"},
		{"SS3 F1", "\x1bOP"},
		{"RIS reset", "\x1bc"},
	}
	f := (&claudeAdapter{}).NewOutputFilter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f([]byte(tt.input))
			if string(got) != tt.input {
				t.Errorf("expected %q preserved, got %q", tt.input, got)
			}
		})
	}
}

func TestClaudeCode_FilterMultipleSequences(t *testing.T) {
	// Multiple extended CSI sequences in one chunk should all be stripped.
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("a\x1b[>1ub\x1b[<uc\x1b[>4;2md")
	got := f(input)
	if string(got) != "abcd" {
		t.Errorf("expected %q, got %q", "abcd", got)
	}
}

func TestClaudeCode_FilterMixedSequences(t *testing.T) {
	// Mix of normal CSI (preserved) and extended CSI (stripped).
	f := (&claudeAdapter{}).NewOutputFilter()
	input := []byte("\x1b[1m\x1b[>1uhello\x1b[0m")
	got := f(input)
	expected := "\x1b[1mhello\x1b[0m"
	if string(got) != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestClaudeCode_FilterCrossBoundary_EscAtEnd(t *testing.T) {
	// ESC at end of chunk 1, [ > 1 u at start of chunk 2.
	f := (&claudeAdapter{}).NewOutputFilter()

	chunk1 := []byte("hello\x1b")
	chunk2 := []byte("[>1uworld")

	got1 := f(chunk1)
	got2 := f(chunk2)
	combined := string(got1) + string(got2)

	if combined != "helloworld" {
		t.Errorf("cross-boundary: expected %q, got %q", "helloworld", combined)
	}
}

func TestClaudeCode_FilterCrossBoundary_EscBracketAtEnd(t *testing.T) {
	// ESC [ at end of chunk 1, > 1 u at start of chunk 2.
	f := (&claudeAdapter{}).NewOutputFilter()

	chunk1 := []byte("hello\x1b[")
	chunk2 := []byte(">1uworld")

	got1 := f(chunk1)
	got2 := f(chunk2)
	combined := string(got1) + string(got2)

	if combined != "helloworld" {
		t.Errorf("cross-boundary: expected %q, got %q", "helloworld", combined)
	}
}

func TestClaudeCode_FilterCrossBoundary_NormalCSI(t *testing.T) {
	// ESC [ at end of chunk 1, normal params at start of chunk 2.
	// The normal CSI must be preserved.
	f := (&claudeAdapter{}).NewOutputFilter()

	chunk1 := []byte("hello\x1b[")
	chunk2 := []byte("1mworld")

	got1 := f(chunk1)
	got2 := f(chunk2)
	combined := string(got1) + string(got2)

	expected := "hello\x1b[1mworld"
	if combined != expected {
		t.Errorf("cross-boundary normal CSI: expected %q, got %q", expected, combined)
	}
}

func TestClaudeCode_FilterCrossBoundary_ExtendedSplit(t *testing.T) {
	// Extended CSI split in the middle of params: ESC [ > 1 in chunk 1, u in chunk 2.
	f := (&claudeAdapter{}).NewOutputFilter()

	chunk1 := []byte("hello\x1b[>1")
	chunk2 := []byte("uworld")

	got1 := f(chunk1)
	got2 := f(chunk2)
	combined := string(got1) + string(got2)

	if combined != "helloworld" {
		t.Errorf("cross-boundary extended split: expected %q, got %q", "helloworld", combined)
	}
}

func TestClaudeCode_FilterEmpty(t *testing.T) {
	f := (&claudeAdapter{}).NewOutputFilter()
	got := f([]byte{})
	if len(got) != 0 {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestClaudeCode_FilterIndependentSessions(t *testing.T) {
	// Two filters must maintain independent state.
	adapter := &claudeAdapter{}
	f1 := adapter.NewOutputFilter()
	f2 := adapter.NewOutputFilter()

	// Leave f1 in mid-sequence state.
	f1([]byte("hello\x1b["))

	// f2 should work independently — no cross-contamination.
	got := f2([]byte("normal text"))
	if string(got) != "normal text" {
		t.Errorf("f2 contaminated by f1 state: got %q", got)
	}

	// f1 should still strip the extended CSI when completed.
	got1 := f1([]byte(">1uworld"))
	if string(got1) != "world" {
		t.Errorf("f1 should strip extended CSI: got %q", got1)
	}
}
