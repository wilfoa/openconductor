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
