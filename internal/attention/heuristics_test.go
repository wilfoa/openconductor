// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import "testing"

// All tests use nil checker (generic patterns only, no agent-specific checks).
// Agent-specific attention tests live in agent/opencode_test.go and
// agent/claude_test.go alongside their adapter implementations.

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
			result, event := CheckHeuristics(lines, Running, nil)
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
			result, event := CheckHeuristics(lines, Running, nil)
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
			result, event := CheckHeuristics(lines, Running, nil)
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
	result, event := CheckHeuristics(lines, Running, nil)
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
	result, event := CheckHeuristics(lines, Exited, nil)
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
	result, event := CheckHeuristics(lines, Running, nil)
	if result != No {
		t.Errorf("expected No, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestCheckHeuristics_EmptyLines(t *testing.T) {
	lines := []string{"", "", ""}
	result, event := CheckHeuristics(lines, Running, nil)
	if result != No {
		t.Errorf("expected No, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestCheckHeuristics_ScansMultipleLines(t *testing.T) {
	// Error is on 2nd-to-last non-empty line, not the last.
	lines := []string{
		"Error: build failed",
		"see log for details",
	}
	result, event := CheckHeuristics(lines, Running, nil)
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
	result, event := CheckHeuristics(lines, Running, nil)
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
	result, event := CheckHeuristics(lines, Running, nil)
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event)
	}
}

// ── Checker Integration Tests ────────────────────────────────────

// mockChecker is a test AttentionChecker that returns configurable results.
type mockChecker struct {
	result HeuristicResult
	event  *AttentionEvent
}

func (m *mockChecker) CheckAttention(lastLines []string) (HeuristicResult, *AttentionEvent) {
	return m.result, m.event
}

func TestCheckHeuristics_CheckerWorkingSuppressesGeneric(t *testing.T) {
	// When a checker returns Working, generic error patterns are suppressed.
	checker := &mockChecker{result: Working, event: nil}
	lines := []string{"error: some compile error in output"}
	result, event := CheckHeuristics(lines, Running, checker)
	if result != Working {
		t.Errorf("expected Working, got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestCheckHeuristics_CheckerNoSkipsGeneric(t *testing.T) {
	// When a checker is provided but returns No, generic patterns are skipped.
	checker := &mockChecker{result: No, event: nil}
	lines := []string{"error: build failed"}
	result, event := CheckHeuristics(lines, Running, checker)
	if result != No {
		t.Errorf("expected No (generic patterns skipped), got %v", result)
	}
	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
}

func TestCheckHeuristics_ProcessExitedOverridesChecker(t *testing.T) {
	// Process state always wins — even if a checker would return Working.
	checker := &mockChecker{result: Working, event: nil}
	lines := []string{"✦ Sublimating…"}
	result, event := CheckHeuristics(lines, Exited, checker)
	if result != Certain {
		t.Errorf("expected Certain, got %v", result)
	}
	if event == nil || event.Type != NeedsReview {
		t.Errorf("expected NeedsReview, got %v", event)
	}
}
