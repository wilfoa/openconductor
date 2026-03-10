// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"context"
	"os"
	"strings"
	"testing"
)

// livePID returns the current process PID, which is guaranteed to be alive
// for the duration of the test. Using a real live PID ensures CheckProcess
// returns Running, not Exited.
func livePID() int { return os.Getpid() }

// testOpenCodeChecker mimics the OpenCode adapter's CheckAttention for e2e tests.
type testOpenCodeChecker struct{}

func (c *testOpenCodeChecker) CheckAttention(lastLines []string) (HeuristicResult, *AttentionEvent) {
	hasEscInterrupt := false
	hasIdleShortcuts := false
	hasPermissionRequired := false
	hasAllowOnce := false
	hasQuestionDialog := false

	for i := len(lastLines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lastLines[i])
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "esc interrupt") {
			hasEscInterrupt = true
		}
		if strings.Contains(lower, "ctrl+p commands") || strings.Contains(lower, "ctrl+t variants") {
			hasIdleShortcuts = true
		}
		// Model selector footer: "Build  Claude Opus 4.6 Anthropic · max"
		if strings.Contains(lower, " · ") {
			for _, p := range []string{"anthropic", "openai", "google", "groq", "bedrock", "openrouter", "copilot", "local", "vertexai"} {
				if strings.Contains(lower, p) {
					hasIdleShortcuts = true
					break
				}
			}
		}
		if strings.Contains(lower, "permission required") {
			hasPermissionRequired = true
		}
		if strings.Contains(lower, "allow once") || strings.Contains(lower, "allow always") {
			hasAllowOnce = true
		}
		if (strings.Contains(lower, "enter submit") || strings.Contains(lower, "enter confirm")) && strings.Contains(lower, "esc dismiss") {
			hasQuestionDialog = true
		}
	}

	// Permission and question dialogs take priority over working signal.
	if hasPermissionRequired || hasAllowOnce {
		return Certain, &AttentionEvent{Type: NeedsPermission, Detail: "opencode permission dialog detected", Source: "heuristic"}
	}
	if hasQuestionDialog {
		return Certain, &AttentionEvent{Type: NeedsAnswer, Detail: "opencode question dialog detected", Source: "heuristic"}
	}
	if hasEscInterrupt {
		return Working, nil
	}
	if hasIdleShortcuts {
		return Certain, &AttentionEvent{Type: NeedsInput, Detail: "opencode is idle, waiting for prompt", Source: "heuristic"}
	}
	return No, nil
}

// e2eClaudeChecker and e2eOpenCodeChecker are used by all e2e tests.
var e2eClaudeChecker AttentionChecker = &testClaudeChecker{}
var e2eOpenCodeChecker AttentionChecker = &testOpenCodeChecker{}

// E2E-style tests that simulate full Detector.Check() scenarios with
// realistic terminal output from real agents. These test the complete
// pipeline: agent-specific heuristics → generic patterns → L2 escalation
// without needing to run actual agents.

// ── Claude Code E2E Scenarios ───────────────────────────────────

func TestE2E_ClaudeCode_WelcomeScreen(t *testing.T) {
	d := NewDetector()
	lines := simulateClaudeCodeWelcome()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

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

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

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

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

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

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

	if isWorking {
		t.Error("expected isWorking=false for permission prompt")
	}
	if event == nil {
		t.Fatal("expected attention event for permission prompt, got nil")
	}
	// Permission prompt must be detected as NeedsPermission, not NeedsInput.
	// The "(y/n)" pattern takes priority over the "> " prompt on row 23.
	if event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
	t.Logf("Permission → %s: %s (source: %s)", event.Type, event.Detail, event.Source)
}

func TestE2E_ClaudeCode_WorkingWithErrors(t *testing.T) {
	// Claude Code is actively fixing errors — spinner visible, error text in output.
	d := NewDetector()
	lines := simulateClaudeCodeFixingErrors()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

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
	event, isWorking := d.Check(ctx, "proj1", simulateClaudeCodeWelcome(), livePID(), e2eClaudeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 1 (welcome): expected NeedsInput, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 1: expected isWorking=false")
	}

	// Phase 2: User types, agent starts thinking → Working
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeThinking(), livePID(), e2eClaudeChecker)
	if event != nil {
		t.Errorf("phase 2 (thinking): expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("phase 2: expected isWorking=true")
	}

	// Phase 3: Agent finishes → NeedsInput
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeFinished(), livePID(), e2eClaudeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 3 (finished): expected NeedsInput, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 3: expected isWorking=false")
	}
}

func TestE2E_ClaudeCode_PermissionYN(t *testing.T) {
	// Claude Code shows a "(y/n)" permission prompt — must detect NeedsPermission.
	d := NewDetector()
	lines := simulateClaudeCodePermissionYN()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

	if isWorking {
		t.Error("expected isWorking=false for permission prompt")
	}
	if event == nil {
		t.Fatal("expected attention event for (y/n) permission, got nil")
	}
	if event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
}

func TestE2E_ClaudeCode_PermissionBashAllow(t *testing.T) {
	// Claude Code shows "Allow running bash command: git status?" — must detect.
	d := NewDetector()
	lines := simulateClaudeCodePermissionBashAllow()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

	if isWorking {
		t.Error("expected isWorking=false for bash allow prompt")
	}
	if event == nil {
		t.Fatal("expected attention event for bash allow prompt, got nil")
	}
	if event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
}

func TestE2E_ClaudeCode_SpinnerOverridesPermission(t *testing.T) {
	// Active spinner + stale permission text visible → Working (not Permission).
	d := NewDetector()
	lines := simulateClaudeCodeSpinnerWithStalePermission()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

	if event != nil {
		t.Errorf("expected nil event (spinner overrides stale permission), got %v", event)
	}
	if !isWorking {
		t.Error("expected isWorking=true while spinner active")
	}
}

func TestE2E_ClaudeCode_PermissionWithPromptVisible(t *testing.T) {
	// Permission + "> " prompt both visible → Permission wins.
	d := NewDetector()
	lines := simulateClaudeCodePermissionWithPrompt()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eClaudeChecker)

	if isWorking {
		t.Error("expected isWorking=false for permission prompt")
	}
	if event == nil {
		t.Fatal("expected attention event, got nil")
	}
	if event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission (not NeedsInput), got %v", event.Type)
	}
}

func TestE2E_ClaudeCode_FullLifecycle(t *testing.T) {
	// Full lifecycle: idle → working → permission → working → idle
	d := NewDetector()
	ctx := context.Background()

	// Phase 1: Welcome screen → NeedsInput
	event, isWorking := d.Check(ctx, "proj1", simulateClaudeCodeWelcome(), livePID(), e2eClaudeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 1: expected NeedsInput, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 1: expected isWorking=false")
	}

	// Phase 2: User types, agent starts thinking → Working
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeThinking(), livePID(), e2eClaudeChecker)
	if event != nil {
		t.Errorf("phase 2: expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("phase 2: expected isWorking=true")
	}

	// Phase 3: Agent hits permission prompt → NeedsPermission
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodePermission(), livePID(), e2eClaudeChecker)
	if event == nil || event.Type != NeedsPermission {
		t.Fatalf("phase 3: expected NeedsPermission, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 3: expected isWorking=false")
	}

	// Phase 4: User approves, agent resumes working → Working
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeThinking(), livePID(), e2eClaudeChecker)
	if event != nil {
		t.Errorf("phase 4: expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("phase 4: expected isWorking=true")
	}

	// Phase 5: Agent finishes → NeedsInput
	event, isWorking = d.Check(ctx, "proj1", simulateClaudeCodeFinished(), livePID(), e2eClaudeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 5: expected NeedsInput, got event=%v", event)
	}
	if isWorking {
		t.Error("phase 5: expected isWorking=false")
	}
}

// ── OpenCode E2E Scenarios ──────────────────────────────────────

func TestE2E_OpenCode_Working(t *testing.T) {
	d := NewDetector()
	lines := simulateOpenCodeWorking()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

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

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

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

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

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

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

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

func TestE2E_OpenCode_QuestionDialogEnterConfirm(t *testing.T) {
	d := NewDetector()
	// Some OpenCode versions use "enter confirm" instead of "enter submit".
	lines := simulateOpenCodeQuestion()
	lines[13] = "↕ select  enter confirm  esc dismiss"

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

	if isWorking {
		t.Error("expected isWorking=false for question dialog (enter confirm)")
	}
	if event == nil {
		t.Fatal("expected attention event for question dialog (enter confirm), got nil")
	}
	if event.Type != NeedsAnswer {
		t.Errorf("expected NeedsAnswer, got %v", event.Type)
	}
}

func TestE2E_OpenCode_PermissionWithEscInterruptOverlay(t *testing.T) {
	// When a permission modal overlays the screen, "esc interrupt" from the
	// underlying progress bar can remain in the vt10x buffer. Permission
	// must take priority.
	d := NewDetector()
	lines := simulateOpenCodePermissionWithOverlay()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

	if isWorking {
		t.Error("expected isWorking=false for permission dialog (even with esc interrupt)")
	}
	if event == nil {
		t.Fatal("expected attention event for permission dialog, got nil")
	}
	if event.Type != NeedsPermission {
		t.Errorf("expected NeedsPermission, got %v", event.Type)
	}
}

func TestE2E_OpenCode_ModelSelectorIdle(t *testing.T) {
	// When OpenCode shows the model selector footer without "ctrl+p commands",
	// it should still be detected as idle.
	d := NewDetector()
	lines := simulateOpenCodeModelSelectorIdle()

	event, isWorking := d.Check(context.Background(), "proj1", lines, livePID(), e2eOpenCodeChecker)

	if isWorking {
		t.Error("expected isWorking=false for model selector idle")
	}
	if event == nil {
		t.Fatal("expected attention event for model selector idle, got nil")
	}
	if event.Type != NeedsInput {
		t.Errorf("expected NeedsInput, got %v", event.Type)
	}
}

func TestE2E_OpenCode_StateTransitions(t *testing.T) {
	d := NewDetector()
	ctx := context.Background()

	// Phase 1: Idle → NeedsInput
	event, _ := d.Check(ctx, "proj1", simulateOpenCodeIdle(), livePID(), e2eOpenCodeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 1 (idle): expected NeedsInput, got %v", event)
	}

	// Phase 2: Working → isWorking=true
	event, isWorking := d.Check(ctx, "proj1", simulateOpenCodeWorking(), livePID(), e2eOpenCodeChecker)
	if event != nil {
		t.Errorf("phase 2 (working): expected nil event, got %v", event)
	}
	if !isWorking {
		t.Error("phase 2: expected isWorking=true")
	}

	// Phase 3: Permission dialog → NeedsPermission
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeExternalDirPermission(), livePID(), e2eOpenCodeChecker)
	if event == nil || event.Type != NeedsPermission {
		t.Fatalf("phase 3 (permission): expected NeedsPermission, got %v", event)
	}

	// Phase 4: Question dialog → NeedsAnswer
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeQuestion(), livePID(), e2eOpenCodeChecker)
	if event == nil || event.Type != NeedsAnswer {
		t.Fatalf("phase 4 (question): expected NeedsAnswer, got %v", event)
	}

	// Phase 5: Permission with overlay — should still detect permission
	event, _ = d.Check(ctx, "proj1", simulateOpenCodePermissionWithOverlay(), livePID(), e2eOpenCodeChecker)
	if event == nil || event.Type != NeedsPermission {
		t.Fatalf("phase 5 (permission+overlay): expected NeedsPermission, got %v", event)
	}

	// Phase 6: Model selector idle
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeModelSelectorIdle(), livePID(), e2eOpenCodeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 6 (model selector idle): expected NeedsInput, got %v", event)
	}

	// Phase 7: Back to classic idle (ctrl+p commands)
	event, _ = d.Check(ctx, "proj1", simulateOpenCodeIdle(), livePID(), e2eOpenCodeChecker)
	if event == nil || event.Type != NeedsInput {
		t.Fatalf("phase 7 (idle again): expected NeedsInput, got %v", event)
	}
}

// ── Process Exit Scenarios ──────────────────────────────────────

func TestE2E_ProcessExited(t *testing.T) {
	d := NewDetector()
	lines := []string{"some output"}

	// Use a PID that almost certainly doesn't exist.
	// CheckProcess returns Exited for non-existent PIDs.
	deadPID := 99999999
	event, isWorking := d.Check(context.Background(), "proj1", lines, deadPID, e2eClaudeChecker)

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

func simulateClaudeCodePermissionYN() []string {
	// Permission prompt with explicit (y/n) indicator.
	lines := make([]string, 24)
	lines[0] = "> create a new config file"
	lines[1] = ""
	lines[2] = "  I'll create a new config.yaml file:"
	lines[3] = ""
	lines[4] = "  Allow creating file config.yaml? (y/n)"
	for i := 5; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
	return lines
}

func simulateClaudeCodePermissionBashAllow() []string {
	// "Allow running bash command: ..." permission prompt.
	lines := make([]string, 24)
	lines[0] = "> check the git status"
	lines[1] = ""
	lines[2] = "  I'll check the repository status:"
	lines[3] = ""
	lines[4] = "  Allow running bash command: git status?"
	for i := 5; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
	return lines
}

func simulateClaudeCodeSpinnerWithStalePermission() []string {
	// Spinner is active, but old permission text from a prior prompt is
	// still visible in the vt10x buffer above. The spinner should win.
	lines := make([]string, 24)
	lines[0] = "> fix the bug"
	lines[1] = ""
	lines[2] = "  Do you want to proceed? (y/n)"
	lines[3] = ""
	lines[4] = "✦ Applying…"
	for i := 5; i < 24; i++ {
		lines[i] = ""
	}
	return lines
}

func simulateClaudeCodePermissionWithPrompt() []string {
	// Permission prompt visible along with "> " on row 23.
	// Permission should take priority over the idle prompt.
	lines := make([]string, 24)
	lines[0] = "> deploy the changes"
	lines[1] = ""
	lines[2] = "  I'd like to run the deploy script:"
	lines[3] = ""
	lines[4] = "  Allow running bash command: ./deploy.sh?"
	for i := 5; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "> "
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

// ── OpenCode Permission With Overlay Scenario ───────────────────────────────
//
// Reconstructed from a real terminal capture (screenshot confirmed).
// When OpenCode renders a permission modal, it only redraws the dialog
// cells. The underlying "esc interrupt" progress text from the actively
// generating model can remain in the vt10x buffer on a line not covered
// by the modal overlay.

func simulateOpenCodePermissionWithOverlay() []string {
	lines := make([]string, 24)
	lines[0] = "  It's mapbox_maps_flutter v2.x (version 85.0.0). Let me find the API:"
	lines[1] = ""
	lines[2] = "  # Find mapbox package cache location"
	lines[3] = "  $ find /Users/amir/.pub-cache -type d -name \"mapbox_maps_flutter*\""
	lines[4] = "  /Users/amir/.pub-cache/hosted/pub.dev/mapbox_maps_flutter-2.18.0"
	lines[5] = ""
	lines[6] = "  ● Read settings.dart [offset=640, limit=50]"
	lines[7] = "  ● Grep \"scaleBarSettings\" in .../lib/src"
	// The "esc interrupt" from the underlying progress bar is still in the buffer.
	lines[8] = "  · · · · ■ ■  esc interrupt"
	lines[9] = ""
	lines[10] = "  ■ Build · claude-opus-4-6"
	lines[11] = ""
	// Permission dialog overlaid on top.
	lines[12] = "  ⚠ Permission required"
	lines[13] = "  ← Access external directory ~/.pub-cache/hosted/pub.dev/mapbox_maps_flutter-2.18.0/lib/src"
	lines[14] = ""
	lines[15] = "  Patterns"
	lines[16] = "  - /Users/amir/.pub-cache/hosted/pub.dev/mapbox_maps_flutter-2.18.0/lib/src/*"
	lines[17] = ""
	lines[18] = "  Allow once   Allow always   Reject"
	lines[19] = ""
	for i := 20; i < 23; i++ {
		lines[i] = ""
	}
	lines[23] = "  ctrl+f  fullscreen  s select  enter confirm"
	return lines
}

// ── OpenCode Model Selector Idle Scenario ───────────────────────────────────
//
// Reconstructed from a real terminal capture (screenshot confirmed).
// When OpenCode is idle, some versions show the model selector footer
// ("Build  Claude Opus 4.6 Anthropic · max") instead of the keyboard
// shortcuts ("ctrl+p commands").

func simulateOpenCodeModelSelectorIdle() []string {
	lines := make([]string, 24)
	lines[0] = "  All tests pass. Here's a summary:"
	lines[1] = ""
	lines[2] = "  Bug 1 — Permission dialog not detected"
	lines[3] = "  Bug 2 — Stale session index after closing all tabs"
	lines[4] = ""
	lines[5] = "  Want me to commit these?"
	lines[6] = ""
	lines[7] = "  ■ Build · claude-opus-4-6 · 5m 20s"
	lines[8] = ""
	for i := 9; i < 22; i++ {
		lines[i] = ""
	}
	// Model selector footer — no "ctrl+p commands" or "esc interrupt".
	lines[22] = "  Build  Claude Opus 4.6 Anthropic · max"
	lines[23] = ""
	return lines
}
