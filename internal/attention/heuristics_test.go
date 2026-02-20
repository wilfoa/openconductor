package attention

import "testing"

func TestCheckHeuristics_PermissionPatterns(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"y/n lowercase", "Do you want to proceed? [y/n]"},
		{"Y/n uppercase", "Allow this action? [Y/n]"},
		{"yes/no", "Continue? (yes/no)"},
		{"allow", "Allow?"},
		{"approve", "Approve?"},
		{"do you want to proceed", "Do you want to proceed with this change?"},
		{"mixed case", "do YOU want to PROCEED?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"", "", tt.line}
			result, event := CheckHeuristics(lines, Running, "")
			if result != Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != NeedsPermission {
				t.Errorf("expected NeedsPermission, got %v", event.Type)
			}
		})
	}
}

func TestCheckHeuristics_ErrorPatterns(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"error colon", "Error: something went wrong"},
		{"fatal", "fatal: not a git repository"},
		{"panic", "panic: runtime error"},
		{"failed", "Build FAILED with 3 errors"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"", tt.line}
			result, event := CheckHeuristics(lines, Running, "")
			if result != Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != HitError {
				t.Errorf("expected HitError, got %v", event.Type)
			}
		})
	}
}

func TestCheckHeuristics_DonePatterns(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"task completed", "Task completed successfully"},
		{"all done", "All done!"},
		{"completed", "Operation completed"},
		{"finished", "Build finished"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"", tt.line}
			result, event := CheckHeuristics(lines, Running, "")
			if result != Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != NeedsReview {
				t.Errorf("expected NeedsReview, got %v", event.Type)
			}
		})
	}
}

func TestCheckHeuristics_PromptSuffix_Uncertain(t *testing.T) {
	lines := []string{"", "Enter your choice> "}
	result, event := CheckHeuristics(lines, Running, "")
	if result != Uncertain {
		t.Errorf("expected Uncertain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
}

func TestCheckHeuristics_ProcessExited(t *testing.T) {
	lines := []string{"some normal output"}
	result, event := CheckHeuristics(lines, Exited, "")
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != NeedsReview {
		t.Errorf("expected NeedsReview, got %v", event.Type)
	}
}

func TestCheckHeuristics_NoMatch(t *testing.T) {
	lines := []string{"Building project...", "Compiling main.go", "Running tests"}
	result, event := CheckHeuristics(lines, Running, "")
	if result != No {
		t.Errorf("expected No, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestCheckHeuristics_EmptyLines(t *testing.T) {
	lines := []string{"", "", ""}
	result, event := CheckHeuristics(lines, Running, "")
	if result != No {
		t.Errorf("expected No, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestCheckHeuristics_ScansMultipleLines(t *testing.T) {
	// Error is on 2nd-to-last non-empty line, not the last.
	// Old code only scanned 1 line and would miss this.
	lines := []string{
		"Error: build failed",
		"see log for details",
	}
	result, event := CheckHeuristics(lines, Running, "")
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != HitError {
		t.Errorf("expected HitError, got %v", event.Type)
	}
}

func TestCheckHeuristics_CaseInsensitive(t *testing.T) {
	lines := []string{"", "ERROR: Something Bad"}
	result, event := CheckHeuristics(lines, Running, "")
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != HitError {
		t.Errorf("expected HitError event, got %v", event)
	}
}

func TestCheckHeuristics_PermissionPriorityOverError(t *testing.T) {
	// Permission on the last line, error above. Permission should win
	// because it's scanned first (bottom-up).
	lines := []string{
		"Error: something failed",
		"Do you want to proceed? [y/n]",
	}
	result, event := CheckHeuristics(lines, Running, "")
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event)
	}
}

// ── Claude Code Agent-Specific Tests ─────────────────────────────

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"some output above", "", tt.line, ""}
			result, event := CheckHeuristics(lines, Running, AgentClaudeCode)
			if result != Working {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{tt.line}
			result, _ := CheckHeuristics(lines, Running, AgentClaudeCode)
			if result == Working {
				t.Errorf("did not expect Working for line %q", tt.line)
			}
		})
	}
}

func TestClaudeCode_SpinnerSuppressesGenericError(t *testing.T) {
	// When Claude Code shows a spinner AND output contains "error:",
	// the spinner takes priority — agent is working, not stuck on error.
	lines := []string{
		"error: some compile error in output",
		"✦ Fixing…",
	}
	result, event := CheckHeuristics(lines, Running, AgentClaudeCode)
	if result != Working {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, event := CheckHeuristics(tt.lines, Running, AgentClaudeCode)
			if result != Certain {
				t.Errorf("expected Certain, got %v", result)
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != NeedsInput {
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
	lines := []string{
		"> ",
		"✦ Thinking…",
	}
	result, event := CheckHeuristics(lines, Running, AgentClaudeCode)
	if result != Working {
		t.Errorf("expected Working (spinner overrides prompt), got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestClaudeCode_NoPromptNoSpinnerFallsThrough(t *testing.T) {
	// No spinner, no prompt — falls through to generic patterns.
	lines := []string{"Building project..."}
	result, _ := CheckHeuristics(lines, Running, AgentClaudeCode)
	// Should not be Working or Certain from agent-specific check.
	if result == Working {
		t.Error("expected not Working for output without spinner or prompt")
	}
}

func TestClaudeCode_ProcessExitedOverridesSpinner(t *testing.T) {
	// Process state always wins — even if spinner text is visible, exited
	// means exited.
	lines := []string{"✦ Sublimating…"}
	result, event := CheckHeuristics(lines, Exited, AgentClaudeCode)
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != NeedsReview {
		t.Errorf("expected NeedsReview, got %v", event)
	}
}

// ── OpenCode Agent-Specific Tests ────────────────────────────────

func TestOpenCode_EscInterruptWorking(t *testing.T) {
	lines := []string{
		"· · · · ■ ■  esc interrupt",
		"",
	}
	result, event := CheckHeuristics(lines, Running, AgentOpenCode)
	if result != Working {
		t.Errorf("expected Working, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestOpenCode_IdleWithShortcuts(t *testing.T) {
	lines := []string{
		"   ┃  Build  Claude Opus 4.5 (latest) Anthropic · max",
		"   ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
		"                                ctrl+t variants  tab agents  ctrl+p commands",
	}
	result, event := CheckHeuristics(lines, Running, AgentOpenCode)
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
}

func TestOpenCode_EscInterruptSuppressesGenericError(t *testing.T) {
	// When OpenCode is working (esc interrupt visible), generic error
	// patterns in the output should be suppressed.
	lines := []string{
		"error: build failed",
		"· · · · ■ ■  esc interrupt",
	}
	result, event := CheckHeuristics(lines, Running, AgentOpenCode)
	if result != Working {
		t.Errorf("expected Working, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestOpenCode_NoAgentSignals(t *testing.T) {
	// OpenCode output without specific signals falls through to generic.
	lines := []string{"some random output"}
	result, _ := CheckHeuristics(lines, Running, AgentOpenCode)
	if result == Working || result == Certain {
		t.Errorf("expected No or Uncertain for unrecognized output, got %v", result)
	}
}

func TestOpenCode_CtrlPCommandsAlone(t *testing.T) {
	// Just ctrl+p commands without esc interrupt → idle.
	lines := []string{
		"ctrl+p commands",
	}
	result, event := CheckHeuristics(lines, Running, AgentOpenCode)
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event)
	}
}
