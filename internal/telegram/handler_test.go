// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/session"
)

// ── ParseQuestionOptions ────────────────────────────────────────

// Helper to build a realistic OpenCode question dialog screen.
// The dialog has options above a footer line containing "enter submit  esc dismiss".
func questionDialog(conversation []string, options []string) []string {
	var lines []string
	lines = append(lines, conversation...)
	lines = append(lines, "") // gap before dialog
	lines = append(lines, "Which option would you like?")
	lines = append(lines, options...)
	lines = append(lines, "↕ select  enter submit  esc dismiss")
	return lines
}

func TestParseQuestionOptions_DotFormat(t *testing.T) {
	lines := questionDialog(
		[]string{"Some conversation above"},
		[]string{"1. Create a new file", "2. Edit existing file", "3. Delete file"},
	)
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options, got %d: %v", len(opts), opts)
	}
	if opts[0] != "1. Create a new file" {
		t.Fatalf("expected first option '1. Create a new file', got %q", opts[0])
	}
}

func TestParseQuestionOptions_ParenFormat(t *testing.T) {
	lines := questionDialog(
		[]string{"Select an action:"},
		[]string{"1) Continue", "2) Abort", "3) Retry"},
	)
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options with ')' format, got %d", len(opts))
	}
	if opts[0] != "1) Continue" {
		t.Fatalf("expected '1) Continue', got %q", opts[0])
	}
}

func TestParseQuestionOptions_MixedFormats(t *testing.T) {
	lines := questionDialog(nil, []string{"1. Dot format", "2) Paren format", "3. Another dot"})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options (mixed), got %d", len(opts))
	}
}

func TestParseQuestionOptions_NoDialogFooter(t *testing.T) {
	// Without a dialog footer, ParseQuestionOptions should return nil.
	lines := []string{"Just a regular question", "with no numbered options"}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 0 {
		t.Fatalf("expected 0 options (no footer), got %d", len(opts))
	}
}

func TestParseQuestionOptions_IndentedNumbers(t *testing.T) {
	lines := questionDialog(nil, []string{"  1. Indented option", "  2. Another one"})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (trimmed), got %d", len(opts))
	}
}

func TestParseQuestionOptions_ZeroNotMatched(t *testing.T) {
	lines := questionDialog(nil, []string{"0. This should not match", "1. This should match"})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (0 excluded), got %d", len(opts))
	}
}

func TestParseQuestionOptions_BlankLines(t *testing.T) {
	lines := questionDialog(nil, []string{"", "  ", "1. Only option", ""})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestParseQuestionOptions_ConfirmVariant(t *testing.T) {
	// Some dialogs use "enter confirm" instead of "enter submit".
	lines := []string{
		"Are you sure?",
		"1. Yes, proceed",
		"2. No, cancel",
		"↕ select  enter confirm  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options with 'confirm' footer, got %d", len(opts))
	}
}

func TestParseQuestionOptions_BoxDrawingBorders(t *testing.T) {
	// OpenCode wraps dialog options in box-drawing borders (│).
	lines := []string{
		"Some conversation content",
		"│ Which framework?",
		"│ 1. Jest",
		"│ 2. Vitest",
		"│ 3. Mocha",
		"↕ select  enter submit  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options with box borders, got %d: %v", len(opts), opts)
	}
	// Borders should be stripped from the option text.
	if opts[0] != "1. Jest" {
		t.Errorf("expected '1. Jest' (border stripped), got %q", opts[0])
	}
}

func TestParseQuestionOptions_IgnoresConversationNumbers(t *testing.T) {
	// Numbered items in conversation above the dialog should NOT be picked up.
	lines := []string{
		"Here are the steps:",
		"1. The scrollback is capturing OpenCode chrome",
		"2. We need to fix the sidebar filtering",
		"3. Add proper tests",
		"",
		"Which approach would you like?",
		"│ 1. Yes, add them",
		"│ 2. No, skip for now",
		"↕ select  enter submit  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (not conversation numbers), got %d: %v", len(opts), opts)
	}
	if opts[0] != "1. Yes, add them" {
		t.Errorf("expected '1. Yes, add them', got %q", opts[0])
	}
}

func TestParseQuestionOptions_SelectDismissFooter(t *testing.T) {
	// Broader dialog footer: just "select" + "dismiss".
	lines := []string{
		"Choose a file:",
		"1. main.go",
		"2. handler.go",
		"⌘ select  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options with select/dismiss footer, got %d", len(opts))
	}
}

// ── extractLeadingNumber ────────────────────────────────────────

func TestExtractLeadingNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1. Foo", "1"},
		{"12) Bar", "12"},
		{"3.thing", "3"},
		{"99 whatever", "99"},
		{"abc", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractLeadingNumber(tt.input)
		if got != tt.want {
			t.Errorf("extractLeadingNumber(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── FormatCallbackData ──────────────────────────────────────────

func TestFormatCallbackData_Short(t *testing.T) {
	data := FormatCallbackData("perm", "my-project", "allow")
	expected := "perm:my-project:allow"
	if data != expected {
		t.Fatalf("expected %q, got %q", expected, data)
	}
}

func TestFormatCallbackData_Truncates(t *testing.T) {
	longName := "this-is-a-very-long-project-name-that-exceeds-the-telegram-limit"
	data := FormatCallbackData("perm", longName, "allow")
	if len(data) > 64 {
		t.Fatalf("expected callback data <= 64 bytes, got %d", len(data))
	}
	if data[0:5] != "perm:" {
		t.Fatalf("expected prefix 'perm:', got %q", data[:5])
	}
}

func TestFormatCallbackData_PreservesShortData(t *testing.T) {
	data := FormatCallbackData("opt", "p", "1")
	if data != "opt:p:1" {
		t.Fatalf("expected 'opt:p:1', got %q", data)
	}
}

// ── Callback data round-trip ────────────────────────────────────
// Verify that the callback data produced by keyboards can be parsed back
// into the correct kind/project/action triple.

func TestCallbackRoundTrip_Permission(t *testing.T) {
	project := "my-proj"
	for _, action := range []string{"allow", "allowall", "deny"} {
		data := FormatCallbackData("perm", project, action)
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts, got %d from %q", len(parts), data)
		}
		if parts[0] != "perm" {
			t.Errorf("kind: expected 'perm', got %q", parts[0])
		}
		if parts[1] != project {
			t.Errorf("project: expected %q, got %q", project, parts[1])
		}
		if parts[2] != action {
			t.Errorf("action: expected %q, got %q", action, parts[2])
		}
	}
}

func TestCallbackRoundTrip_QuestionOption(t *testing.T) {
	project := "stocks"
	for _, num := range []string{"1", "2", "3"} {
		data := FormatCallbackData("opt", project, num)
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts from %q", data)
		}
		if parts[0] != "opt" || parts[1] != project || parts[2] != num {
			t.Errorf("round-trip failed: %q → [%s, %s, %s]", data, parts[0], parts[1], parts[2])
		}
	}
}

// ── Keyboard structure ──────────────────────────────────────────

func TestPermissionKeyboard_HasThreeButtons(t *testing.T) {
	kb := PermissionKeyboard("test-project")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(row))
	}

	// Verify button labels (emoji-prefixed).
	labels := []string{row[0].Text, row[1].Text, row[2].Text}
	if labels[0] != "🟢 Allow Once" || labels[1] != "🟡 Allow Always" || labels[2] != "🔴 Deny" {
		t.Fatalf("unexpected button labels: %v", labels)
	}

	// Verify callback data is parseable.
	for _, btn := range row {
		parts := strings.SplitN(*btn.CallbackData, ":", 3)
		if len(parts) != 3 {
			t.Errorf("button %q: callback data %q not parseable", btn.Text, *btn.CallbackData)
		}
		if parts[0] != "perm" {
			t.Errorf("button %q: expected kind 'perm', got %q", btn.Text, parts[0])
		}
		if parts[1] != "test-project" {
			t.Errorf("button %q: expected project 'test-project', got %q", btn.Text, parts[1])
		}
	}

	// Verify specific actions.
	if !strings.HasSuffix(*row[0].CallbackData, ":allow") {
		t.Errorf("Allow Once button: expected ':allow' suffix, got %q", *row[0].CallbackData)
	}
	if !strings.HasSuffix(*row[1].CallbackData, ":allowall") {
		t.Errorf("Allow Always button: expected ':allowall' suffix, got %q", *row[1].CallbackData)
	}
	if !strings.HasSuffix(*row[2].CallbackData, ":deny") {
		t.Errorf("Deny button: expected ':deny' suffix, got %q", *row[2].CallbackData)
	}
}

func TestQuestionKeyboard_MatchesOptions(t *testing.T) {
	options := []string{"1. Create file", "2. Edit file", "3. Delete file"}
	kb := QuestionKeyboard("proj", options)

	// QuestionKeyboard puts all buttons in one row.
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(row))
	}

	// Button labels should be the full option text with emoji prefix.
	for i, btn := range row {
		want := "🟣 " + options[i]
		if btn.Text != want {
			t.Errorf("button %d: expected label %q, got %q", i, want, btn.Text)
		}
	}

	// Callback data should contain just the number.
	for i, btn := range row {
		parts := strings.SplitN(*btn.CallbackData, ":", 3)
		expectedNum := extractLeadingNumber(options[i])
		if parts[2] != expectedNum {
			t.Errorf("button %d: expected action %q, got %q", i, expectedNum, parts[2])
		}
	}
}

func TestQuestionKeyboard_ParenFormat(t *testing.T) {
	options := []string{"1) Yes", "2) No"}
	kb := QuestionKeyboard("proj", options)
	row := kb.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(row))
	}
	// "1) Yes" → number should be "1".
	parts := strings.SplitN(*row[0].CallbackData, ":", 3)
	if parts[2] != "1" {
		t.Errorf("expected action '1', got %q", parts[2])
	}
}

// ── AttentionKeyboard ───────────────────────────────────────────

func TestAttentionKeyboard_HasFourButtons(t *testing.T) {
	kb := AttentionKeyboard("test-project")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 4 {
		t.Fatalf("expected 4 buttons, got %d", len(row))
	}

	// Verify button labels (emoji-prefixed).
	expected := []string{"🟡 yes", "🟡 no", "🟡 continue", "⏭ skip"}
	for i, btn := range row {
		if btn.Text != expected[i] {
			t.Errorf("button %d: expected label %q, got %q", i, expected[i], btn.Text)
		}
	}

	// Verify all callback data uses "reply" kind.
	for _, btn := range row {
		parts := strings.SplitN(*btn.CallbackData, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("button %q: callback data %q not parseable", btn.Text, *btn.CallbackData)
		}
		if parts[0] != "reply" {
			t.Errorf("button %q: expected kind 'reply', got %q", btn.Text, parts[0])
		}
		if parts[1] != "test-project" {
			t.Errorf("button %q: expected project 'test-project', got %q", btn.Text, parts[1])
		}
	}
}

// ── ErrorKeyboard ───────────────────────────────────────────────

func TestErrorKeyboard_HasThreeButtons(t *testing.T) {
	kb := ErrorKeyboard("test-project")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(row))
	}

	// Verify button labels (emoji-prefixed).
	expected := []string{"🔄 retry", "⏭ skip", "🔴 abort"}
	for i, btn := range row {
		if btn.Text != expected[i] {
			t.Errorf("button %d: expected label %q, got %q", i, expected[i], btn.Text)
		}
	}

	// Verify all callback data uses "reply" kind.
	for _, btn := range row {
		parts := strings.SplitN(*btn.CallbackData, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("button %q: callback data not parseable", btn.Text)
		}
		if parts[0] != "reply" {
			t.Errorf("button %q: expected kind 'reply', got %q", btn.Text, parts[0])
		}
	}
}

// ── Callback round-trip: reply ──────────────────────────────────

func TestCallbackRoundTrip_Reply(t *testing.T) {
	project := "my-proj"
	for _, action := range []string{"yes", "no", "continue", "skip", "retry", "abort"} {
		data := FormatCallbackData("reply", project, action)
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts, got %d from %q", len(parts), data)
		}
		if parts[0] != "reply" {
			t.Errorf("kind: expected 'reply', got %q", parts[0])
		}
		if parts[1] != project {
			t.Errorf("project: expected %q, got %q", project, parts[1])
		}
		if parts[2] != action {
			t.Errorf("action: expected %q, got %q", action, parts[2])
		}
	}
}

// ── Handler project-by-topic lookup ─────────────────────────────

func TestHandler_ProjectByTopic(t *testing.T) {
	state := newTopicState()
	state.Set("alpha", 100)
	state.Set("beta", 200)

	h := &handler{state: state, projects: []config.Project{
		{Name: "alpha"},
		{Name: "beta"},
	}}

	if got := h.projectByTopic(100); got != "alpha" {
		t.Errorf("expected 'alpha', got %q", got)
	}
	if got := h.projectByTopic(200); got != "beta" {
		t.Errorf("expected 'beta', got %q", got)
	}
	if got := h.projectByTopic(999); got != "" {
		t.Errorf("expected empty for unknown topic, got %q", got)
	}
}

// ── HandleInboundMedia ──────────────────────────────────────────

func TestHandleInboundMedia_NoThread(t *testing.T) {
	h := &handler{state: newTopicState()}
	ok := h.HandleInboundMedia(nil, nil, "", 0, nil)
	if ok {
		t.Fatal("expected false for threadID=0")
	}
}

func TestHandleInboundMedia_UnknownTopic(t *testing.T) {
	h := &handler{state: newTopicState()}
	ok := h.HandleInboundMedia([]rawPhotoSize{{FileID: "x"}}, nil, "", 999, nil)
	if ok {
		t.Fatal("expected false for unknown topic")
	}
}

func TestHandleInboundMedia_NoFileID(t *testing.T) {
	state := newTopicState()
	state.Set("proj", 100)
	mgr := session.NewManager()
	h := &handler{mgr: mgr, state: state, projects: []config.Project{{Name: "proj", Repo: "/tmp/test"}}}
	// No active sessions → returns false before checking file_id.
	ok := h.HandleInboundMedia(nil, nil, "caption", 100, nil)
	if ok {
		t.Fatal("expected false when no sessions")
	}
}

// ── ParseQuestionOptions with realistic filtered screen ─────────

func TestParseQuestionOptions_FullScreenWithChromeFiltered(t *testing.T) {
	// Simulate a realistic 30-row OpenCode screen with a question dialog.
	// The agent adapter's FilterChromeLines removes status bar and shortcut
	// hints, but the bottom skip from ChromeSkipRows must NOT strip the
	// dialog footer — otherwise ParseQuestionOptions can't find it and
	// returns nil (no inline keyboard buttons).
	//
	// This is a regression test: ChromeSkipRows(1, 2) previously stripped
	// the bottom 2 rows in sendTelegramEvent, removing dialog footers.
	screen := make([]string, 30)
	screen[0] = "  Some conversation content"
	screen[1] = "  More conversation..."
	// ...blank lines in the middle...
	screen[24] = "Which framework would you like to use?"
	screen[25] = "1. Jest"
	screen[26] = "2. Vitest"
	screen[27] = "3. Playwright"
	screen[28] = ""
	screen[29] = "↕ select  enter submit  esc dismiss"

	// After filtering (header row stripped, chrome lines filtered), the
	// dialog footer must still be present for ParseQuestionOptions to work.
	opts := ParseQuestionOptions(screen)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options from full screen, got %d: %v", len(opts), opts)
	}
	if opts[0] != "1. Jest" || opts[1] != "2. Vitest" || opts[2] != "3. Playwright" {
		t.Errorf("unexpected options: %v", opts)
	}
}

func TestParseQuestionOptions_FooterInSecondToLastRow(t *testing.T) {
	// Dialog footer not in the very last row — there may be an empty line
	// after it. ParseQuestionOptions should still find it.
	screen := make([]string, 30)
	screen[25] = "1. Option A"
	screen[26] = "2. Option B"
	screen[27] = "↕ select  enter submit  esc dismiss"
	screen[28] = ""
	screen[29] = ""

	opts := ParseQuestionOptions(screen)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d: %v", len(opts), opts)
	}
}

// ── writePermKeystroke ───────────────────────────────────────────

// pipeSession creates a session with a pipe as the PTY fd. The reader
// end collects what writePermKeystroke sends. Close reader when done.
func pipeSession(t *testing.T, agentType config.AgentType) (*session.Session, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	s := &session.Session{
		ID:       "test",
		Instance: 1,
		Project:  config.Project{Name: "test", Agent: agentType},
	}
	s.Ptmx = w
	s.State = session.StateRunning
	return s, r
}

func readAll(r *os.File) []byte {
	// Read with a small timeout — the pipe should have all data by now.
	r.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	return buf[:n]
}

func TestWritePermKeystroke_NavigationAddsEnter(t *testing.T) {
	// OpenCode "Allow always": navigation-only keystroke (Right arrow).
	// writePermKeystroke should append Enter after SubmitDelay.
	s, r := pipeSession(t, "opencode")
	writePermKeystroke(s, []byte("\x1b[C"))
	got := readAll(r)
	// Expect: Right arrow + Enter.
	want := "\x1b[C\r"
	if string(got) != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestWritePermKeystroke_AlreadyHasLF_NoExtra(t *testing.T) {
	// Claude Code "deny": keystroke already ends with \n.
	// writePermKeystroke should NOT send an extra Enter.
	s, r := pipeSession(t, "claude")
	writePermKeystroke(s, []byte("n\n"))
	got := readAll(r)
	if string(got) != "n\n" {
		t.Errorf("expected %q, got %q", "n\n", got)
	}
}

func TestWritePermKeystroke_EndsWithCR_NoExtra(t *testing.T) {
	// OpenCode "Allow once": keystroke is just \r.
	s, r := pipeSession(t, "opencode")
	writePermKeystroke(s, []byte("\r"))
	got := readAll(r)
	if string(got) != "\r" {
		t.Errorf("expected %q, got %q", "\r", got)
	}
}

// ── QuestionKeystroke (OpenCode) ─────────────────────────────────

func TestQuestionKeystroke_Option1_JustEnter(t *testing.T) {
	// Option 1 is already selected — QuestionKeystroke returns nil,
	// writePermKeystroke sends delay + Enter.
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	writePermKeystroke(s, qr.QuestionKeystroke(1))
	got := readAll(r)
	if string(got) != "\r" {
		t.Errorf("option 1: expected %q, got %q", "\r", got)
	}
}

func TestQuestionKeystroke_Option2_OneDownArrow(t *testing.T) {
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	writePermKeystroke(s, qr.QuestionKeystroke(2))
	got := readAll(r)
	// Expect: 1 down arrow + Enter.
	want := "\x1b[B\r"
	if string(got) != want {
		t.Errorf("option 2: expected %q, got %q", want, got)
	}
}

func TestQuestionKeystroke_Option3_TwoDownArrows(t *testing.T) {
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	writePermKeystroke(s, qr.QuestionKeystroke(3))
	got := readAll(r)
	// Expect: 2 down arrows + Enter.
	want := "\x1b[B\x1b[B\r"
	if string(got) != want {
		t.Errorf("option 3: expected %q, got %q", want, got)
	}
}

func TestQuestionKeystroke_Option0_FallsBackToEnter(t *testing.T) {
	// Edge case: option 0 (e.g. failed Atoi) → nil → just Enter.
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	writePermKeystroke(s, qr.QuestionKeystroke(0))
	got := readAll(r)
	if string(got) != "\r" {
		t.Errorf("option 0: expected %q, got %q", "\r", got)
	}
}

func TestClaudeCode_ImplementsQuestionResponder(t *testing.T) {
	// Claude Code implements QuestionResponder for AskUserQuestion dialogs.
	a, _ := agent.Get("claude-code")
	qr, ok := a.(agent.QuestionResponder)
	if !ok {
		t.Fatal("claude adapter should implement QuestionResponder")
	}
	// Option 1 → nil (default, just Enter).
	if ks := qr.QuestionKeystroke(1); ks != nil {
		t.Errorf("option 1: expected nil, got %q", ks)
	}
	// Option 2 → one down arrow.
	if ks := qr.QuestionKeystroke(2); string(ks) != "\x1b[B" {
		t.Errorf("option 2: expected down arrow, got %q", ks)
	}
}

// ── ensureGitignore ─────────────────────────────────────────────

func TestEnsureGitignore_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	ensureGitignore(dir)

	gi := dir + "/.gitignore"
	data, err := os.ReadFile(gi)
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}
	if !strings.Contains(string(data), "*") {
		t.Errorf("expected '*' in .gitignore, got %q", string(data))
	}
}

func TestEnsureGitignore_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	gi := dir + "/.gitignore"
	os.WriteFile(gi, []byte("custom\n"), 0o644)

	ensureGitignore(dir)

	data, _ := os.ReadFile(gi)
	if string(data) != "custom\n" {
		t.Errorf("expected existing .gitignore preserved, got %q", string(data))
	}
}
