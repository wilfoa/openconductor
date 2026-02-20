package attention

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── buildPrompt tests ───────────────────────────────────────────

func TestBuildPromptContainsLines(t *testing.T) {
	lines := []string{"line one", "line two", "line three"}
	prompt := buildPrompt(lines)

	for _, line := range lines {
		if !strings.Contains(prompt, line) {
			t.Errorf("expected prompt to contain %q", line)
		}
	}
}

func TestBuildPromptContainsAllStates(t *testing.T) {
	prompt := buildPrompt([]string{"test"})

	for state := range validStates {
		if !strings.Contains(prompt, state) {
			t.Errorf("expected prompt to mention state %q", state)
		}
	}
}

func TestBuildPromptEmpty(t *testing.T) {
	prompt := buildPrompt(nil)
	// Should not panic and should still contain the instruction text.
	if !strings.Contains(prompt, "Classification:") {
		t.Error("expected prompt to contain 'Classification:'")
	}
}

// ── parseState tests ────────────────────────────────────────────

func TestParseStateExactMatch(t *testing.T) {
	for state := range validStates {
		got := parseState(state)
		if got != state {
			t.Errorf("parseState(%q) = %q, want %q", state, got, state)
		}
	}
}

func TestParseStateCaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"waiting_input", "WAITING_INPUT"},
		{"Working", "WORKING"},
		{"done", "DONE"},
		{"  ERROR  ", "ERROR"},
	}
	for _, tt := range tests {
		got := parseState(tt.input)
		if got != tt.want {
			t.Errorf("parseState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseStateEmbedded(t *testing.T) {
	// LLM might return extra text around the state.
	got := parseState("The agent is WAITING_INPUT for the user")
	if got != "WAITING_INPUT" {
		t.Errorf("parseState with extra text = %q, want WAITING_INPUT", got)
	}
}

func TestParseStateUnknownDefaultsToWorking(t *testing.T) {
	got := parseState("I don't know what the agent is doing")
	if got != "WORKING" {
		t.Errorf("parseState(unknown) = %q, want WORKING", got)
	}
}

// ── Classifier throttling tests ─────────────────────────────────

func TestClassifierThrottleUnchangedBuffer(t *testing.T) {
	mock := &mockLLMClient{response: "DONE"}
	c := NewClassifier(mock)

	ctx := context.Background()
	lines := []string{"Task completed."}

	// First call — hits the LLM.
	result, err := c.Classify(ctx, "proj1", lines)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if result != "DONE" {
		t.Fatalf("first call: got %q, want DONE", result)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", mock.calls)
	}

	// Second call with same buffer — should return cached, no new LLM call.
	result, err = c.Classify(ctx, "proj1", lines)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if result != "DONE" {
		t.Fatalf("second call: got %q, want DONE", result)
	}
	if mock.calls != 1 {
		t.Fatalf("expected still 1 LLM call after unchanged buffer, got %d", mock.calls)
	}
}

func TestClassifierThrottleMinInterval(t *testing.T) {
	mock := &mockLLMClient{response: "DONE"}
	c := NewClassifier(mock)
	// Shorten interval for test speed.
	c.minInterval = 50 * time.Millisecond

	ctx := context.Background()

	// First call.
	_, _ = c.Classify(ctx, "proj1", []string{"line A"})
	if mock.calls != 1 {
		t.Fatalf("expected 1 call, got %d", mock.calls)
	}

	// Immediate second call with different buffer — throttled.
	_, _ = c.Classify(ctx, "proj1", []string{"line B"})
	if mock.calls != 1 {
		t.Fatalf("expected still 1 call (throttled), got %d", mock.calls)
	}

	// Wait for interval to pass, then call again.
	time.Sleep(60 * time.Millisecond)
	_, _ = c.Classify(ctx, "proj1", []string{"line C"})
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls after interval, got %d", mock.calls)
	}
}

func TestClassifierWorkingBackoff(t *testing.T) {
	mock := &mockLLMClient{response: "WORKING"}
	c := NewClassifier(mock)
	c.minInterval = 10 * time.Millisecond
	c.workingBackoff = 80 * time.Millisecond

	ctx := context.Background()

	// First call — returns WORKING.
	result, _ := c.Classify(ctx, "proj1", []string{"line A"})
	if result != "WORKING" {
		t.Fatalf("expected WORKING, got %q", result)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 call, got %d", mock.calls)
	}

	// Wait past minInterval but NOT past workingBackoff.
	time.Sleep(30 * time.Millisecond)
	_, _ = c.Classify(ctx, "proj1", []string{"line B"})
	if mock.calls != 1 {
		t.Fatalf("expected still 1 call (working backoff), got %d", mock.calls)
	}

	// Wait past workingBackoff.
	time.Sleep(60 * time.Millisecond)
	_, _ = c.Classify(ctx, "proj1", []string{"line C"})
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls after backoff, got %d", mock.calls)
	}
}

func TestClassifierDifferentSessions(t *testing.T) {
	mock := &mockLLMClient{response: "DONE"}
	c := NewClassifier(mock)

	ctx := context.Background()

	// Two different sessions — no throttling between them.
	_, _ = c.Classify(ctx, "proj1", []string{"output A"})
	_, _ = c.Classify(ctx, "proj2", []string{"output B"})

	if mock.calls != 2 {
		t.Fatalf("expected 2 LLM calls for different sessions, got %d", mock.calls)
	}
}

func TestClassifierLLMError(t *testing.T) {
	mock := &mockLLMClient{err: fmt.Errorf("connection refused")}
	c := NewClassifier(mock)

	ctx := context.Background()
	_, err := c.Classify(ctx, "proj1", []string{"output"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected error to contain 'connection refused', got %q", err.Error())
	}
}
