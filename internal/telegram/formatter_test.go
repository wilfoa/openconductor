// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"strings"
	"testing"
)

// ── cleanScreen ─────────────────────────────────────────────────

func TestCleanScreen_TrimsLeadingTrailingBlanks(t *testing.T) {
	lines := []string{"", "  ", "hello", "world", "", ""}
	got := cleanScreen(lines)
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("expected content, got %q", got)
	}
	// Should not start or end with newline.
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(got, "\n") {
		t.Fatalf("expected no leading/trailing newlines, got %q", got)
	}
}

func TestCleanScreen_AllBlankReturnsEmpty(t *testing.T) {
	lines := []string{"", "  ", "   "}
	got := cleanScreen(lines)
	if got != "" {
		t.Fatalf("expected empty string for all-blank input, got %q", got)
	}
}

func TestCleanScreen_HTMLEscapes(t *testing.T) {
	lines := []string{"<script>alert('xss')</script>"}
	got := cleanScreen(lines)
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected HTML-escaped output, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped tags, got %q", got)
	}
}

func TestCleanScreen_PreservesMiddleBlanks(t *testing.T) {
	lines := []string{"line1", "", "line3"}
	got := cleanScreen(lines)
	parts := strings.Split(got, "\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 lines (with middle blank), got %d: %q", len(parts), got)
	}
}

// ── splitMessage ────────────────────────────────────────────────

func TestSplitMessage_ShortFitsInOne(t *testing.T) {
	header := "<b>proj</b>\n\n"
	body := "short body"
	msgs := splitMessage(header, body, true)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "<b>proj</b>") {
		t.Fatal("expected header in message")
	}
	if !strings.Contains(msgs[0], "<pre>") {
		t.Fatal("expected <pre> tag in code block mode")
	}

	// Without code block mode, no <pre> tags.
	plain := splitMessage(header, body, false)
	if len(plain) != 1 {
		t.Fatalf("expected 1 message, got %d", len(plain))
	}
	if strings.Contains(plain[0], "<pre>") {
		t.Fatal("plain mode should not contain <pre> tag")
	}
	if !strings.Contains(plain[0], "short body") {
		t.Fatal("expected body in plain message")
	}
}

func TestSplitMessage_LongSplitsAcrossMessages(t *testing.T) {
	header := "<b>proj</b>\n\n"
	// Build body that exceeds maxMessageLen.
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("This is a line of output that takes up space in the message body\n")
	}
	body := sb.String()

	msgs := splitMessage(header, body, true)
	if len(msgs) < 2 {
		t.Fatalf("expected multiple messages, got %d", len(msgs))
	}

	// First message should have the header.
	if !strings.Contains(msgs[0], "<b>proj</b>") {
		t.Fatal("expected header in first message")
	}

	// Subsequent messages should NOT have the header.
	for i := 1; i < len(msgs); i++ {
		if strings.Contains(msgs[i], "<b>proj</b>") {
			t.Fatalf("message %d should not contain header", i)
		}
	}

	// All messages should be within the limit.
	for i, msg := range msgs {
		if len(msg) > maxMessageLen+100 { // small tolerance for tag overhead
			t.Fatalf("message %d exceeds limit: %d bytes", i, len(msg))
		}
	}

	// Every message should have matching <pre>...</pre> tags in code block mode.
	for i, msg := range msgs {
		if !strings.Contains(msg, "<pre>") || !strings.Contains(msg, "</pre>") {
			t.Fatalf("message %d missing <pre> tags", i)
		}
	}
}

// ── Format functions ────────────────────────────────────────────

func TestFormatResponse_NilForBlankScreen(t *testing.T) {
	msgs := FormatResponse("proj", []string{"", "  "})
	if msgs != nil {
		t.Fatalf("expected nil for blank screen, got %v", msgs)
	}
}

func TestFormatResponse_IncludesProjectName(t *testing.T) {
	msgs := FormatResponse("my-project", []string{"output line"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	if !strings.Contains(msgs[0], "my-project") {
		t.Fatalf("expected project name in output, got %q", msgs[0])
	}
}

func TestFormatPermission_HasLockEmoji(t *testing.T) {
	msgs := FormatPermission("proj", "write file.txt", []string{"Allow?"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	// Lock emoji is \xF0\x9F\x94\x92.
	if !strings.Contains(msgs[0], "\xF0\x9F\x94\x92") {
		t.Fatal("expected lock emoji in permission message")
	}
	if !strings.Contains(msgs[0], "write file.txt") {
		t.Fatal("expected detail in permission message")
	}
}

func TestFormatPermission_NoDetail(t *testing.T) {
	msgs := FormatPermission("proj", "", []string{"Allow?"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	if strings.Contains(msgs[0], "<code>") {
		t.Fatal("expected no <code> block when detail is empty")
	}
}

func TestFormatQuestion_HasQuestionEmoji(t *testing.T) {
	msgs := FormatQuestion("proj", []string{"Which option?"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	// Question emoji is \xe2\x9d\x93.
	if !strings.Contains(msgs[0], "\xe2\x9d\x93") {
		t.Fatal("expected question emoji")
	}
}

func TestFormatError_HasRedCircle(t *testing.T) {
	msgs := FormatError("proj", "connection failed", []string{"Error: timeout"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	// Red circle is \xF0\x9F\x94\xB4.
	if !strings.Contains(msgs[0], "\xF0\x9F\x94\xB4") {
		t.Fatal("expected red circle emoji in error message")
	}
}

func TestFormatDone_HasCheckmark(t *testing.T) {
	msgs := FormatDone("proj", []string{"Completed successfully"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	// Checkmark is \xe2\x9c\x85.
	if !strings.Contains(msgs[0], "\xe2\x9c\x85") {
		t.Fatal("expected checkmark emoji in done message")
	}
}

func TestFormatAttention_HasWarningEmoji(t *testing.T) {
	msgs := FormatAttention("proj", "needs input", []string{"Waiting..."})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	// Warning is \xe2\x9a\xa0.
	if !strings.Contains(msgs[0], "\xe2\x9a\xa0") {
		t.Fatal("expected warning emoji in attention message")
	}
}

// ── FormatActionTaken ───────────────────────────────────────────

func TestFormatActionTaken_WithUser(t *testing.T) {
	result := FormatActionTaken("original text", "Allowed once", "Alice")
	if !strings.Contains(result, "original text") {
		t.Fatal("expected original text preserved")
	}
	if !strings.Contains(result, "Allowed once") {
		t.Fatal("expected action label")
	}
	if !strings.Contains(result, "Alice") {
		t.Fatal("expected user name")
	}
}

func TestFormatActionTaken_WithoutUser(t *testing.T) {
	result := FormatActionTaken("original", "Denied", "")
	if !strings.Contains(result, "Denied") {
		t.Fatal("expected action label")
	}
	if strings.Contains(result, " by ") {
		t.Fatal("expected no 'by' when user is empty")
	}
}

func TestFormatActionTaken_HTMLEscapes(t *testing.T) {
	result := FormatActionTaken("text", "<b>bold</b>", "<Alice>")
	if strings.Contains(result, "<b>bold</b>") {
		t.Fatal("expected HTML-escaped action")
	}
	if strings.Contains(result, "<Alice>") {
		t.Fatal("expected HTML-escaped user")
	}
}

// ── Display quality: each attention type is distinct ────────────
// These tests verify that a realistic screen produces a clear, informative
// message with the correct structure for each attention type.

func realisticScreen() []string {
	return []string{
		"  opencode v1.0",
		"",
		"  I'll create the config file for you.",
		"",
		"  src/config.ts",
		"  + export const config = {",
		"  +   port: 3000,",
		"  +   host: 'localhost',",
		"  + }",
		"",
		"  > _                               esc exit",
	}
}

func TestFormatDisplay_Permission_Structure(t *testing.T) {
	screen := []string{
		"  Claude wants to write to src/main.go",
		"",
		"  Allow this action?",
		"  (y)es / (n)o / (a)lways",
	}
	msgs := FormatPermission("my-api", "Write to src/main.go", screen)
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := msgs[0]

	// Must have: project name in bold, lock emoji, detail in code block, screen in pre.
	checks := []struct {
		contains string
		label    string
	}{
		{"<b>my-api</b>", "bold project name"},
		{"\xF0\x9F\x94\x92", "lock emoji"},
		{"<code>Write to src/main.go</code>", "detail in code block"},
		{"<pre>", "screen content in pre block"},
		{"Allow this action?", "screen content preserved"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("permission message missing %s: %q not found in:\n%s", c.label, c.contains, msg)
		}
	}
}

func TestFormatDisplay_Question_Structure(t *testing.T) {
	screen := []string{
		"  Which database would you like to use?",
		"",
		"  1. PostgreSQL",
		"  2. MySQL",
		"  3. SQLite",
	}
	msgs := FormatQuestion("backend", screen)
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := msgs[0]

	checks := []struct {
		contains string
		label    string
	}{
		{"<b>backend</b>", "bold project name"},
		{"\xe2\x9d\x93", "question emoji"},
		{"<pre>", "pre block"},
		{"PostgreSQL", "option content"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("question message missing %s", c.label)
		}
	}
}

func TestFormatDisplay_Attention_Structure(t *testing.T) {
	msgs := FormatAttention("frontend", "waiting for input", realisticScreen())
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := msgs[0]

	checks := []struct {
		contains string
		label    string
	}{
		{"<b>frontend</b>", "bold project name"},
		{"\xe2\x9a\xa0", "warning emoji"},
		{"waiting for input", "detail text"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("attention message missing %s", c.label)
		}
	}
}

func TestFormatDisplay_Error_Structure(t *testing.T) {
	screen := []string{
		"  Error: ENOENT: no such file or directory",
		"  at /app/src/index.ts:42",
	}
	msgs := FormatError("backend", "build failed", screen)
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := msgs[0]

	checks := []struct {
		contains string
		label    string
	}{
		{"<b>backend</b>", "bold project name"},
		{"\xF0\x9F\x94\xB4", "red circle emoji"},
		{"build failed", "detail text"},
		{"ENOENT", "error content"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("error message missing %s", c.label)
		}
	}
}

func TestFormatDisplay_Done_Structure(t *testing.T) {
	msgs := FormatDone("my-api", realisticScreen())
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := msgs[0]

	checks := []struct {
		contains string
		label    string
	}{
		{"<b>my-api</b>", "bold project name"},
		{"\xe2\x9c\x85", "checkmark emoji"},
		{"config.ts", "screen content"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("done message missing %s", c.label)
		}
	}
}

func TestFormatDisplay_Response_Structure(t *testing.T) {
	msgs := FormatResponse("my-api", realisticScreen())
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := msgs[0]

	// Response uses plain text (no <pre>) for natural language readability.
	if !strings.Contains(msg, "<b>my-api</b>") {
		t.Error("response missing bold project name")
	}
	if strings.Contains(msg, "<pre>") {
		t.Error("response should not contain <pre> tag (plain text mode)")
	}
	if !strings.Contains(msg, "config.ts") {
		t.Error("response missing screen content")
	}
}

// ── All types produce distinct output ───────────────────────────

func TestFormatDisplay_AllTypesDistinct(t *testing.T) {
	screen := realisticScreen()
	project := "proj"
	detail := "some detail"

	response := FormatResponse(project, screen)
	permission := FormatPermission(project, detail, screen)
	question := FormatQuestion(project, screen)
	attention := FormatAttention(project, detail, screen)
	errMsg := FormatError(project, detail, screen)
	done := FormatDone(project, screen)

	all := [][]string{response, permission, question, attention, errMsg, done}
	names := []string{"response", "permission", "question", "attention", "error", "done"}

	// Each type should produce at least one non-empty message.
	for i, msgs := range all {
		if len(msgs) == 0 {
			t.Errorf("%s: produced no messages", names[i])
			continue
		}
		if msgs[0] == "" {
			t.Errorf("%s: first message is empty", names[i])
		}
	}

	// No two types should produce identical first messages.
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if len(all[i]) > 0 && len(all[j]) > 0 && all[i][0] == all[j][0] {
				t.Errorf("%s and %s produced identical messages", names[i], names[j])
			}
		}
	}
}

// ── Edge cases ──────────────────────────────────────────────────

func TestFormatPermission_BlankScreen_StillShowsHeader(t *testing.T) {
	msgs := FormatPermission("proj", "write file", []string{"", ""})
	if len(msgs) == 0 {
		t.Fatal("permission with blank screen should still produce a message")
	}
	if !strings.Contains(msgs[0], "proj") {
		t.Error("expected project name even with blank screen")
	}
}

func TestFormatAttention_BlankScreen_StillShowsHeader(t *testing.T) {
	msgs := FormatAttention("proj", "needs input", []string{"", ""})
	if len(msgs) == 0 {
		t.Fatal("attention with blank screen should still produce a message")
	}
}

func TestFormatError_BlankScreen_StillShowsHeader(t *testing.T) {
	msgs := FormatError("proj", "crash", []string{"", ""})
	if len(msgs) == 0 {
		t.Fatal("error with blank screen should still produce a message")
	}
}

func TestFormatDone_BlankScreen_StillShowsHeader(t *testing.T) {
	msgs := FormatDone("proj", []string{"", ""})
	if len(msgs) == 0 {
		t.Fatal("done with blank screen should still produce a message")
	}
}

// ── Reply hints ─────────────────────────────────────────────────
// Verify that actionable messages include the reply hint footer,
// and non-actionable types (Permission, Question) do not.

func TestFormatResponse_HasReplyHint(t *testing.T) {
	msgs := FormatResponse("proj", []string{"output"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "Reply in this thread") {
		t.Error("response should include reply hint")
	}
}

func TestFormatAttention_HasReplyHint(t *testing.T) {
	msgs := FormatAttention("proj", "waiting", []string{"prompt"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "Reply in this thread") {
		t.Error("attention should include reply hint")
	}
}

func TestFormatError_HasReplyHint(t *testing.T) {
	msgs := FormatError("proj", "failed", []string{"Error: boom"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "Reply in this thread") {
		t.Error("error should include reply hint")
	}
}

func TestFormatDone_HasReplyHint(t *testing.T) {
	msgs := FormatDone("proj", []string{"done"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "Reply in this thread") {
		t.Error("done should include reply hint")
	}
}

func TestFormatPermission_NoReplyHint(t *testing.T) {
	msgs := FormatPermission("proj", "write file", []string{"allow?"})
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	for _, m := range msgs {
		if strings.Contains(m, "Reply in this thread") {
			t.Error("permission should NOT include reply hint (has its own keyboard)")
		}
	}
}

// ── Decorative line filtering ───────────────────────────────────

func TestCleanScreen_SkipsBoxDrawingBorders(t *testing.T) {
	lines := []string{
		"  hello world",
		"  ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
		"  goodbye",
	}
	got := cleanScreen(lines)
	if strings.Contains(got, "▀") {
		t.Errorf("decorative border line should be filtered out, got %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "goodbye") {
		t.Errorf("text content should be preserved, got %q", got)
	}
}

func TestCleanScreen_SkipsHorizontalDividers(t *testing.T) {
	lines := []string{
		"content above",
		"  ─────────────────────────────────",
		"  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		"  ═══════════════════════════════════",
		"content below",
	}
	got := cleanScreen(lines)
	if strings.Contains(got, "─") || strings.Contains(got, "━") || strings.Contains(got, "═") {
		t.Errorf("horizontal dividers should be filtered out, got %q", got)
	}
	if !strings.Contains(got, "content above") || !strings.Contains(got, "content below") {
		t.Errorf("text content should be preserved, got %q", got)
	}
}

func TestCleanScreen_KeepsMixedTextAndBoxChars(t *testing.T) {
	// Lines with box-drawing AND real text should be kept.
	lines := []string{
		"│ Status: running │",
		"├── file1.go",
	}
	got := cleanScreen(lines)
	if !strings.Contains(got, "Status") {
		t.Errorf("lines mixing box chars and text should be kept, got %q", got)
	}
	if !strings.Contains(got, "file1.go") {
		t.Errorf("tree-style lines should be kept, got %q", got)
	}
}

func TestIsDecorativeLine(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{"  ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀", true},
		{"╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀", true},
		{"───────────────────────", true},
		{"━━━━━━━━━━━━━━━━━━━━━━━", true},
		{"  ═══════════════════  ", true},
		{"╭──────────────────────╮", true},
		{"╰──────────────────────╯", true},
		{"│ some text │", false}, // has text
		{"hello world", false},   // no box chars
		{"├── file.go", false},   // has text
		{"   ", false},           // all whitespace, no box chars
		{"", false},              // empty
		{"  █████████████████  ", true}, // block elements only
		{"░░░░░░░░░░░░░░░░░░░░", true},  // light shade blocks
	}
	for _, tt := range tests {
		got := isDecorativeLine(tt.line)
		if got != tt.expected {
			t.Errorf("isDecorativeLine(%q) = %v, want %v", tt.line, got, tt.expected)
		}
	}
}
