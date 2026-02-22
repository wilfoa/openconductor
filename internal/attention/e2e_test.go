// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"context"
	"os"
	"testing"
)

// livePID returns the current process PID, which is guaranteed to be alive
// for the duration of the test. Using a real live PID ensures CheckProcess
// returns Running, not Exited.
func livePID() int { return os.Getpid() }

// E2E-style tests that simulate full Detector.Check() scenarios with
// realistic terminal output from real agents. These test the complete
// pipeline: agent-specific heuristics → generic patterns → L2 escalation
// without needing to run actual agents.

// ── Claude Code E2E Scenarios ───────────────────────────────────

func TestE2E_ClaudeCode_WelcomeScreen(t *testing.T) {
	d := NewDetector()
	lines := simulateClaudeCodeWelcome()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentClaudeCode)

	if isWorking {
		t.Error("expected isWorking=false for welcome screen")
	}
	if event == nil {
		t.Fatal("expected attention event for welcome screen prompt, got nil")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
	t.Logf("Welcome screen → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_ClaudeCode_Thinking(t *testing.T) {
	d := NewDetector()
	lines := simulateClaudeCodeThinking()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentClaudeCode)

	if event != nil {
		t.Errorf("expected nil event while thinking, got %v", event)
	}
	if !isWorking {
		t.Error("expected isWorking=true while spinner is visible")
	}
}

func TestE2E_ClaudeCode_FinishedAnswer(t *testing.T) {
	// Simulates: user asked "which date is 21 business days after Feb 3rd"
	// Claude Code answered and returned to prompt.
	d := NewDetector()
	lines := simulateClaudeCodeFinished()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentClaudeCode)

	if isWorking {
		t.Error("expected isWorking=false after answer")
	}
	if event == nil {
		t.Fatal("expected attention event after Claude Code finishes, got nil")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
	t.Logf("Finished answer → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_ClaudeCode_ToolUsePermission(t *testing.T) {
	d := NewDetector()
	lines := simulateClaudeCodePermission()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentClaudeCode)

	if isWorking {
		t.Error("expected isWorking=false for permission prompt")
	}
	if event == nil {
		t.Fatal("expected attention event for permission prompt, got nil")
	}
	// Could be NeedsPermission (from generic pattern) or NeedsInput (from
	// Claude Code prompt detection) — both are valid since the agent-specific
	// check catches "> " first.
	t.Logf("Permission → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_ClaudeCode_WorkingWithErrors(t *testing.T) {
	// Claude Code is actively fixing errors — spinner visible, error text in output.
	d := NewDetector()
	lines := simulateClaudeCodeFixingErrors()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentClaudeCode)

	if event != nil {
		t.Errorf("expected nil event (spinner suppresses errors), got %v", event)
	}
	if !isWorking {
		t.Error("expected isWorking=true while fixing errors")
	}
}

func TestE2E_ClaudeCode_StateTransitions(t *testing.T) {
	// Simulate the full lifecycle: idle → working → idle
	d := NewDetector()
	ctx := context.Background()

	// Phase 1: Welcome screen → NeedsInput
	event, isWorking := d.Check(ctx, "proj1", simulateClaudeCodeWelcome(), livePID(), AgentClaudeCode)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 1 (welcome): expected NeedsInput, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 1: expected isWorking=false")
	}

	// Phase 2: User types, agent starts thinking → Working
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeThinking(), livePID(), AgentClaudeCode)
	if event != nil {
		t.Errorf("phase 2 (thinking): expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("phase 2: expected isWorking=true")
	}

	// Phase 3: Agent finishes → NeedsInput
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeFinished(), livePID(), AgentClaudeCode)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 3 (finished): expected NeedsInput, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 3: expected isWorking=false")
	}
}

// ── OpenCode E2E Scenarios ──────────────────────────────────────

func TestE2E_OpenCode_Working(t *testing.T) {
	d := NewDetector()
	lines := simulateOpenCodeWorking()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentOpenCode)

	if event != nil {
		t.Errorf("expected nil event while working, got %v", event)
	}
	if !isWorking {
		t.Error("expected isWorking=true while esc interrupt is visible")
	}
}

func TestE2E_OpenCode_Idle(t *testing.T) {
	d := NewDetector()
	lines := simulateOpenCodeIdle()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentOpenCode)

	if isWorking {
		t.Error("expected isWorking=false when idle")
	}
	if event == nil {
		t.Fatal("expected attention event for idle OpenCode, got nil")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
	t.Logf("OpenCode idle → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_OpenCode_PermissionDialog(t *testing.T) {
	d := NewDetector()
	lines := simulateOpenCodeExternalDirPermission()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentOpenCode)

	if isWorking {
		t.Error("expected isWorking=false for permission dialog")
	}
	if event == nil {
		t.Fatal("expected attention event for permission dialog, got nil")
	}
	if event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
	t.Logf("OpenCode permission dialog → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_OpenCode_QuestionDialog(t *testing.T) {
	d := NewDetector()
	lines := simulateOpenCodeQuestion()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), AgentOpenCode)

	if isWorking {
		t.Error("expected isWorking=false for question dialog")
	}
	if event == nil {
		t.Fatal("expected attention event for question dialog, got nil")
	}
	if event.Type != NeedsAnswer {
		t.Errorf("expected NeedsAnswer, got %v", event.Type)
	}
	t.Logf("OpenCode question dialog → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_OpenCode_StateTransitions(t *testing.T) {
	d := NewDetector()
	ctx := context.Background()

	// Phase 1: Idle → NeedsInput
	event, _ := d.Check(ctx, "proj1", simulateOpenCodeIdle(), livePID(), AgentOpenCode)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 1 (idle): expected NeedsInput, got %v", event)
	}

	// Phase 2: Working → isWorking=true
	event, isWorking := d.Check(ctx, "proj1", simulateOpenCodeWorking(), livePID(), AgentOpenCode)
	if event != nil {
		t.Errorf("phase 2 (working): expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("phase 2: expected isWorking=true")
	}

	// Phase 3: Permission dialog → NeedsPermission
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeExternalDirPermission(), livePID(), AgentOpenCode)
	if event == nil || event.Type != NeedsPermission {
		t.Fatalf("phase 3 (permission): expected NeedsPermission, got %v", event)
	}

	// Phase 4: Question dialog → NeedsAnswer
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeQuestion(), livePID(), AgentOpenCode)
	if event == nil || event.Type != NeedsAnswer {
		t.Fatalf("phase 4 (question): expected NeedsAnswer, got %v", event)
	}

	// Phase 5: Back to idle
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeIdle(), livePID(), AgentOpenCode)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 5 (idle again): expected NeedsInput, got %v", event)
	}
}

// ── Process Exit Scenarios ──────────────────────────────────────

func TestE2E_ProcessExited(t *testing.T) {
	d := NewDetector()
	lines := []string{"some output"}

	// Use a PID that almost certainly doesn't exist.
	// CheckProcess returns Exited for non-existent PIDs.
	deadPID := 99999999
	event, isWorking := d.Check(context.Background(), "proj1", lines, deadPID, AgentClaudeCode)

	if isWorking {
		t.Error("expected isWorking=false for exited process")
	}
	if event == nil {
		t.Fatal("expected event for exited process")
	}
	if event.Type != NeedsReview {
		t.Errorf("expected NeedsReview, got %v", event.Type)
	}
}

// ── Simulated Screen Outputs ────────────────────────────────────

func simulateClaudeCodeWelcome() []string {
	// Real Claude Code welcome screen (simplified).
	lines := make([]string, 24)
	lines[0] = ""
	lines[1] = "╭────────────────────────────────────────────────╮"
	lines[2] = "│         ✻ Welcome to Claude Code!              │"
	lines[3] = "│                                                │"
	lines[4] = "│   /help for help                               │"
	lines[5] = "╰────────────────────────────────────────────────╯"
	lines[6] = ""
	lines[7] = "  Tips:"
	lines[8] = "  - Use /clear to clear context"
	lines[9] = "  - Use /config to view settings"
	lines[10] = ""
	for i := 11; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
	return lines
}

func simulateClaudeCodeThinking() []string {
	lines := make([]string, 24)
	lines[0] = "> which date is 21 business days after Feb 3rd"
	lines[1] = ""
	lines[2] = "✦ Thinking…"
	for i := 3; i < 24; i++ {
		lines[i] = ""
	}
	return lines
}

func simulateClaudeCodeFinished() []string {
	lines := make([]string, 24)
	lines[0] = "> which date is 21 business days after Feb 3rd"
	lines[1] = ""
	lines[2] = "  Counting 21 business days (weekdays only) from February 3rd, 2026:"
	lines[3] = ""
	lines[4] = "  February 3-27 = 19 business days"
	lines[5] = "  March 2-3 = 2 more business days"
	lines[6] = ""
	lines[7] = "  **March 4th, 2026** is 21 business days after February 3rd."
	lines[8] = ""
	for i := 9; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
	return lines
}

func simulateClaudeCodePermission() []string {
	lines := make([]string, 24)
	lines[0] = "> fix the bug in main.go"
	lines[1] = ""
	lines[2] = "  I'd like to edit main.go to fix the null pointer:"
	lines[3] = ""
	lines[4] = "  Edit main.go"
	lines[5] = "  - if obj != nil {"
	lines[6] = "  + if obj == nil { return }"
	lines[7] = ""
	lines[8] = "  Do you want to proceed? (y/n)"
	for i := 9; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
	return lines
}

func simulateClaudeCodeFixingErrors() []string {
	lines := make([]string, 24)
	lines[0] = "  Error: main.go:42: undefined variable 'foo'"
	lines[1] = "  Error: main.go:58: too many arguments"
	lines[2] = ""
	lines[3] = "✦ Fixing…"
	for i := 4; i < 24; i++ {
		lines[i] = ""
	}
	return lines
}

// ── OpenCode Permission Scenario ────────────────────────────────────────────
//
// Reconstructed from a real terminal capture (screenshot confirmed):
//
//	⚠ Permission required
//	← Access external directory ~/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots
//	Patterns
//	- /Users/amir/Downloads/.../rebuilding-bots/*
//	Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm
//
// The modal overlays existing output. The full visible screen looks like:
//
//	lines 0-4:  previous bash output (cp commands)
//	line  5:    ■ Build · claude-opus-4-6
//	line  6:    (blank)
//	line  7:    ⚠ Permission required
//	line  8:    ← Access external directory ~/Downloads/.../rebuilding-bots
//	line  9:    (blank)
//	line 10:    Patterns
//	line 11:    - /Users/amir/Downloads/.../rebuilding-bots/*
//	line 12:    (blank)
//	line 13:    Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm

func simulateOpenCodeExternalDirPermission() []string {
	lines := make([]string, 24)
	// Prior output still visible above the dialog.
	lines[0] = "Found 5 .env files. Let me copy each one to the corresponding location in your current project:"
	lines[1] = "$ cp /Users/amir/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots/.env.sample /Users/amir/Development/parlibot/rebuilding-bots/.env.sample"
	lines[2] = "$ cp /Users/amir/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots/backend/es/.env /Users/amir/Development/parlibot/rebuilding-bots/backend/es/.env"
	lines[3] = ""
	lines[4] = "■ Build · claude-opus-4-6"
	lines[5] = ""
	// Permission dialog modal.
	lines[6] = "⚠ Permission required"
	lines[7] = "← Access external directory ~/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots"
	lines[8] = ""
	lines[9] = "Patterns"
	lines[10] = "- /Users/amir/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots/*"
	lines[11] = ""
	lines[12] = "Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm"
	for i := 13; i < 24; i++ {
		lines[i] = ""
	}
	return lines
}

func simulateOpenCodeWorking() []string {
	lines := make([]string, 24)
	lines[0] = "   ┃  Analyzing the codebase for potential issues..."
	lines[1] = "   ┃"
	lines[2] = "   ┃  Looking at main.go, config.go, and server.go"
	for i := 3; i < 22; i++ {
		lines[i] = ""
	}
	lines[22] = "   · · · · ■ ■  esc interrupt"
	lines[23] = ""
	return lines
}

func simulateOpenCodeIdle() []string {
	lines := make([]string, 24)
	lines[0] = "   ┃  Done! All changes applied."
	lines[1] = "   ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀"
	for i := 2; i < 22; i++ {
		lines[i] = ""
	}
	lines[22] = "                                ctrl+t variants  tab agents  ctrl+p commands"
	lines[23] = ""
	return lines
}

// ── OpenCode Question Dialog Scenario ──────────────────────────────────────
//
// Reconstructed from a real terminal capture (screenshot confirmed):
//
//	■ Build · claude-opus-4-6
//
//	Docker Desktop currently has only ~2GB of RAM, which isn't enough for
//	the Vite build. You'll need to increase it to at least 6GB. Go to
//	Docker Desktop > Settings > Resources > Memory and increase it.
//	Should I wait while you do that, or would you prefer I try another approach?
//
//	1. I'll increase Docker memory
//	   I'll go to Docker Desktop settings and increase RAM to 6GB+, then tell you to retry
//	2. Try with swap/workaround
//	   Attempt to build with swap or reduced Vite concurrency as a workaround
//	3. Type your own answer
//
//	↕ select  enter submit  esc dismiss

func simulateOpenCodeQuestion() []string {
	lines := make([]string, 24)
	lines[0] = "■ Build · claude-opus-4-6"
	lines[1] = ""
	lines[2] = "Docker Desktop currently has only ~2GB of RAM, which isn't enough for the Vite build."
	lines[3] = "You'll need to increase it to at least 6GB. Go to Docker Desktop > Settings > Resources"
	lines[4] = "> Memory and increase it. Should I wait while you do that, or would you prefer I try"
	lines[5] = "another approach?"
	lines[6] = ""
	lines[7] = "1. I'll increase Docker memory"
	lines[8] = "   I'll go to Docker Desktop settings and increase RAM to 6GB+, then tell you to retry"
	lines[9] = "2. Try with swap/workaround"
	lines[10] = "   Attempt to build with swap or reduced Vite concurrency as a workaround"
	lines[11] = "3. Type your own answer"
	lines[12] = ""
	lines[13] = "↕ select  enter submit  esc dismiss"
	for i := 14; i < 24; i++ {
		lines[i] = ""
	}
	return lines
}
