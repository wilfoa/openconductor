// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"context"
	"strings"
	"testing"
)

// mockLLMClient is a test double for llm.Client.
type mockLLMClient struct {
	response string
	err      error
	calls    int
}

func (m *mockLLMClient) Classify(_ context.Context, _ string) (string, error) {
	m.calls++
	return m.response, m.err
}

// testClaudeChecker mimics the Claude Code adapter's CheckAttention for testing.
type testClaudeChecker struct{}

func (c *testClaudeChecker) CheckAttention(lastLines []string) (HeuristicResult, *AttentionEvent) {
	hasSpinner := false
	hasPrompt := false
	scanned := 0
	for i := len(lastLines) - 1; i >= 0 && scanned < 5; i-- {
		trimmed := strings.TrimSpace(lastLines[i])
		if trimmed == "" {
			continue
		}
		scanned++
		// Simple spinner detection for test purposes.
		if len(trimmed) > 2 && (trimmed[0] == '*' || strings.HasPrefix(trimmed, "✦ ") || strings.HasPrefix(trimmed, "· ")) {
			if strings.HasSuffix(trimmed, "…") || strings.HasSuffix(trimmed, "...") {
				hasSpinner = true
				break
			}
		}
		if !hasPrompt && (strings.HasSuffix(lastLines[i], "> ") || trimmed == ">") {
			hasPrompt = true
		}
	}
	if hasSpinner {
		return Working, nil
	}
	if hasPrompt {
		return Certain, &AttentionEvent{Type: NeedsInput, Detail: "claude code idle", Source: "heuristic"}
	}
	return No, nil
}

func TestDetector_Check_CertainHeuristic(t *testing.T) {
	d := NewDetector()

	lines := []string{"", "Error: build failed"}
	event, isWorking := d.Check(context.Background(), "proj1", lines, 0, nil)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if isWorking {
		t.Error("expected isWorking=false for attention event")
	}
	if event.ProjectName != "proj1" {
		t.Errorf("expected ProjectName 'proj1', got %q", event.ProjectName)
	}
	if event.Type != HitError {
		t.Errorf("expected HitError, got %v", event.Type)
	}
	if event.Source != "heuristic" {
		t.Errorf("expected source 'heuristic', got %q", event.Source)
	}
}

func TestDetector_Check_NoEvent(t *testing.T) {
	d := NewDetector()

	lines := []string{"Building...", "Compiling..."}
	event, isWorking := d.Check(context.Background(), "proj1", lines, 0, nil)

	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
	if isWorking {
		t.Error("expected isWorking=false when no pattern matched")
	}
}

func TestDetector_Check_UncertainWithoutClassifier(t *testing.T) {
	d := NewDetector()

	// Prompt suffix triggers Uncertain, but no classifier is set.
	lines := []string{"", "Enter choice> "}
	event, isWorking := d.Check(context.Background(), "proj1", lines, 0, nil)

	if event != nil {
		t.Errorf("expected nil event without classifier, got %v", event)
	}
	if isWorking {
		t.Error("expected isWorking=false for uncertain without classifier")
	}
}

func TestDetector_Check_UncertainWithClassifier(t *testing.T) {
	mock := &mockLLMClient{response: "WAITING_INPUT"}
	d := NewDetector()
	d.SetClassifier(NewClassifier(mock))

	// Prompt suffix triggers Uncertain → escalates to LLM.
	lines := []string{"", "Enter choice> "}
	event, isWorking := d.Check(context.Background(), "proj1", lines, 0, nil)

	if event == nil {
		t.Fatal("expected event from classifier, got nil")
	}
	if isWorking {
		t.Error("expected isWorking=false for attention event")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
	if event.Source != "llm" {
		t.Errorf("expected source 'llm', got %q", event.Source)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", mock.calls)
	}
}

func TestDetector_Check_ClassifierSaysWorking(t *testing.T) {
	mock := &mockLLMClient{response: "WORKING"}
	d := NewDetector()
	d.SetClassifier(NewClassifier(mock))

	lines := []string{"", "Enter choice> "}
	event, isWorking := d.Check(context.Background(), "proj1", lines, 0, nil)

	// WORKING means no attention needed → nil event, not isWorking
	// (the classifier said WORKING, but that's LLM-level — isWorking
	// is only for agent-specific heuristic signals).
	if event != nil {
		t.Errorf("expected nil event for WORKING, got %v", event)
	}
	if isWorking {
		t.Error("expected isWorking=false for classifier WORKING response")
	}
}

func TestDetector_Check_AgentWorking(t *testing.T) {
	d := NewDetector()

	// Claude Code spinner → positive working signal via checker.
	lines := []string{"✦ Thinking…"}
	checker := &testClaudeChecker{}
	event, isWorking := d.Check(context.Background(), "proj1", lines, 0, checker)

	if event != nil {
		t.Errorf("expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("expected isWorking=true for agent-specific working signal")
	}
}

func TestClassifierStateToEvent(t *testing.T) {
	tests := []struct {
		state    string
		wantType AttentionType
		wantNil  bool
	}{
		{"WAITING_INPUT", NeedsInput, false},
		{"NEEDS_PERMISSION", NeedsPermission, false},
		{"DONE", NeedsReview, false},
		{"ERROR", HitError, false},
		{"STUCK", Stuck, false},
		{"WORKING", 0, true},
		{"unknown", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			event := classifierStateToEvent("proj1", tt.state)
			if tt.wantNil {
				if event != nil {
					t.Errorf("expected nil, got %v", event)
				}
				return
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != tt.wantType {
				t.Errorf("expected %v, got %v", tt.wantType, event.Type)
			}
		})
	}
}
