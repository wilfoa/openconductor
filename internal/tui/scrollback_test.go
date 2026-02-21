package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

// ── Ring buffer tests ───────────────────────────────────────────

func TestScrollbackBufferPushAndLen(t *testing.T) {
	buf := newScrollbackBuffer(5)

	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got len=%d", buf.Len())
	}

	for i := 0; i < 3; i++ {
		buf.Push(makeGlyphs("line %d", i))
	}

	if buf.Len() != 3 {
		t.Fatalf("expected len=3, got %d", buf.Len())
	}
}

func TestScrollbackBufferLineOrder(t *testing.T) {
	buf := newScrollbackBuffer(10)
	buf.Push(makeGlyphs("first"))
	buf.Push(makeGlyphs("second"))
	buf.Push(makeGlyphs("third"))

	got0 := glyphsToText(buf.Line(0))
	got2 := glyphsToText(buf.Line(2))

	if got0 != "first" {
		t.Fatalf("Line(0): expected %q, got %q", "first", got0)
	}
	if got2 != "third" {
		t.Fatalf("Line(2): expected %q, got %q", "third", got2)
	}
}

func TestScrollbackBufferWraparound(t *testing.T) {
	buf := newScrollbackBuffer(3)

	buf.Push(makeGlyphs("a"))
	buf.Push(makeGlyphs("b"))
	buf.Push(makeGlyphs("c"))
	buf.Push(makeGlyphs("d")) // overwrites "a"
	buf.Push(makeGlyphs("e")) // overwrites "b"

	if buf.Len() != 3 {
		t.Fatalf("expected len=3 after wraparound, got %d", buf.Len())
	}

	// Oldest should now be "c", newest "e".
	oldest := glyphsToText(buf.Line(0))
	newest := glyphsToText(buf.Line(2))

	if oldest != "c" {
		t.Fatalf("oldest: expected %q, got %q", "c", oldest)
	}
	if newest != "e" {
		t.Fatalf("newest: expected %q, got %q", "e", newest)
	}
}

func TestScrollbackBufferOutOfRange(t *testing.T) {
	buf := newScrollbackBuffer(5)
	buf.Push(makeGlyphs("x"))

	if buf.Line(-1) != nil {
		t.Error("Line(-1) should return nil")
	}
	if buf.Line(1) != nil {
		t.Error("Line(1) should return nil for buffer with 1 element")
	}
}

func TestScrollbackBufferClear(t *testing.T) {
	buf := newScrollbackBuffer(5)
	buf.Push(makeGlyphs("a"))
	buf.Push(makeGlyphs("b"))
	buf.Clear()

	if buf.Len() != 0 {
		t.Fatalf("expected len=0 after clear, got %d", buf.Len())
	}
}

// ── Shift detection tests ───────────────────────────────────────

func TestDetectScrollShiftSingleLine(t *testing.T) {
	old := []string{"line A", "line B", "line C", "line D"}
	// After 1 line scrolled off: old[1:] shifted to new[0:]
	new := []string{"line B", "line C", "line D", "new stuff"}

	shift := detectScrollShift(old, new)
	if shift != 1 {
		t.Fatalf("expected shift=1, got %d", shift)
	}
}

func TestDetectScrollShiftMultipleLines(t *testing.T) {
	old := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	new := []string{"C", "D", "E", "F", "G", "H", "new1", "new2"}

	shift := detectScrollShift(old, new)
	if shift != 2 {
		t.Fatalf("expected shift=2, got %d", shift)
	}
}

func TestDetectScrollShiftNoScroll(t *testing.T) {
	old := []string{"A", "B", "C"}
	new := []string{"A", "B", "C"}

	shift := detectScrollShift(old, new)
	if shift != 0 {
		t.Fatalf("expected shift=0, got %d", shift)
	}
}

func TestDetectScrollShiftInPlaceEdit(t *testing.T) {
	// Content changed but didn't scroll — row 0 was modified in place.
	old := []string{"line 1", "line 2", "line 3"}
	new := []string{"CHANGED", "line 2", "line 3"}

	shift := detectScrollShift(old, new)
	if shift != 0 {
		t.Fatalf("expected shift=0 for in-place edit, got %d", shift)
	}
}

func TestDetectScrollShiftEmptyRows(t *testing.T) {
	// Empty rows should not produce false matches.
	old := []string{"content", "", "", ""}
	new := []string{"", "", "", "new"}

	shift := detectScrollShift(old, new)
	// Old[1] is empty, should be skipped. No valid match.
	if shift != 0 {
		t.Fatalf("expected shift=0 for empty row match, got %d", shift)
	}
}

func TestDetectScrollShiftEmpty(t *testing.T) {
	if detectScrollShift(nil, []string{"a"}) != 0 {
		t.Error("nil old should return 0")
	}
	if detectScrollShift([]string{"a"}, nil) != 0 {
		t.Error("nil new should return 0")
	}
}

// ── TUI full-screen redraw shift detection ──────────────────────

func TestDetectScrollShiftTUIWithHeader(t *testing.T) {
	// Simulates a TUI app (like OpenCode) that has a fixed header at row 0,
	// a fixed footer at the last row, and content that scrolls in the middle.
	// The header/footer DON'T shift, but the content area does.
	old := []string{
		"  opencode v1.0",         // row 0: header (fixed)
		"  Response from claude:", // row 1: content
		"  Line 1 of response",    // row 2
		"  Line 2 of response",    // row 3
		"  Line 3 of response",    // row 4
		"  Line 4 of response",    // row 5
		"  Line 5 of response",    // row 6
		"  Line 6 of response",    // row 7
		"  Line 7 of response",    // row 8
		"  Line 8 of response",    // row 9
		"  Line 9 of response",    // row 10
		"  Line 10 of response",   // row 11
		"",                        // row 12
		"  > _         esc exit",  // row 13: footer (fixed)
	}
	new := []string{
		"  opencode v1.0",        // row 0: header (same)
		"  Line 1 of response",   // row 1: was row 2 (shifted up by 1)
		"  Line 2 of response",   // row 2: was row 3
		"  Line 3 of response",   // row 3: was row 4
		"  Line 4 of response",   // row 4: was row 5
		"  Line 5 of response",   // row 5: was row 6
		"  Line 6 of response",   // row 6: was row 7
		"  Line 7 of response",   // row 7: was row 8
		"  Line 8 of response",   // row 8: was row 9
		"  Line 9 of response",   // row 9: was row 10
		"  Line 10 of response",  // row 10: was row 11
		"  Line 11 of response",  // row 11: NEW content
		"",                       // row 12
		"  > _         esc exit", // row 13: footer (same)
	}

	shift := detectScrollShift(old, new)
	if shift != 1 {
		t.Fatalf("expected shift=1 for TUI with fixed header, got %d", shift)
	}
}

func TestDetectScrollShiftTUIMultiLineScroll(t *testing.T) {
	// TUI scrolls 2 lines at once (e.g., a large chunk of output arrived).
	old := []string{
		"  header",
		"  Line A",
		"  Line B",
		"  Line C",
		"  Line D",
		"  Line E",
		"  Line F",
		"  Line G",
		"  Line H",
		"  footer",
	}
	new := []string{
		"  header", // fixed
		"  Line C", // was row 3 (shift=2)
		"  Line D",
		"  Line E",
		"  Line F",
		"  Line G",
		"  Line H",
		"  Line I", // new
		"  Line J", // new
		"  footer", // fixed
	}

	shift := detectScrollShift(old, new)
	if shift != 2 {
		t.Fatalf("expected shift=2 for TUI multi-line scroll, got %d", shift)
	}
}

func TestDetectScrollShiftFullScreenNoScroll(t *testing.T) {
	// TUI redraws without scrolling — same content.
	rows := []string{
		"  header",
		"  Line 1",
		"  Line 2",
		"  Line 3",
		"  Line 4",
		"  Line 5",
		"  Line 6",
		"  footer",
	}

	shift := detectScrollShift(rows, rows)
	if shift != 0 {
		t.Fatalf("expected shift=0 for identical redraw, got %d", shift)
	}
}

// ── Scroll navigation tests ────────────────────────────────────

func TestScrollByClamps(t *testing.T) {
	tm := newTerminalModel()
	// Seed buffer with 10 lines.
	for i := 0; i < 10; i++ {
		tm.scrollback.Push(makeGlyphs("line"))
	}

	// Scroll up by 5.
	tm.ScrollBy(5)
	if tm.scrollOffset != 5 {
		t.Fatalf("expected offset=5, got %d", tm.scrollOffset)
	}

	// Scroll up past max — should clamp.
	tm.ScrollBy(100)
	if tm.scrollOffset != 10 {
		t.Fatalf("expected offset clamped to 10, got %d", tm.scrollOffset)
	}

	// Scroll down past 0 — should clamp.
	tm.ScrollBy(-100)
	if tm.scrollOffset != 0 {
		t.Fatalf("expected offset clamped to 0, got %d", tm.scrollOffset)
	}
}

func TestScrollToBottom(t *testing.T) {
	tm := newTerminalModel()
	tm.scrollOffset = 42
	tm.ScrollToBottom()

	if tm.scrollOffset != 0 {
		t.Fatalf("expected offset=0, got %d", tm.scrollOffset)
	}
}

func TestInScrollMode(t *testing.T) {
	tm := newTerminalModel()

	if tm.InScrollMode() {
		t.Error("should not be in scroll mode initially")
	}

	tm.scrollOffset = 1
	if !tm.InScrollMode() {
		t.Error("should be in scroll mode with offset > 0")
	}
}

// ── Capture and write integration ───────────────────────────────

func TestCaptureAndWriteDetectsScroll(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(40, 5))
	tm.vt = vt
	tm.width = 40
	tm.height = 5

	// Fill the terminal with 5 lines.
	for i := 0; i < 5; i++ {
		tm.vt.Write([]byte("line " + string(rune('A'+i)) + "\r\n"))
	}

	// Now write a new line which should cause the top line to scroll off.
	tm.captureAndWrite([]byte("new line\r\n"))

	if tm.scrollback.Len() == 0 {
		t.Fatal("expected scrollback to capture at least 1 line")
	}

	// "line A" was already destroyed by the initial vt.Write() calls (which
	// bypass captureAndWrite). The captureAndWrite call saves "line B" —
	// the top row at the time of the scrolling write.
	oldest := glyphsToText(tm.scrollback.Line(0))
	if !strings.HasPrefix(oldest, "line B") {
		t.Fatalf("expected oldest scrollback to start with 'line B', got %q", oldest)
	}
}

func TestCaptureAndWriteNoScrollNoPush(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(40, 5))
	tm.vt = vt
	tm.width = 40
	tm.height = 5

	// Write text that fits without scrolling.
	tm.captureAndWrite([]byte("hello"))

	if tm.scrollback.Len() != 0 {
		t.Fatalf("expected empty scrollback for non-scrolling write, got %d", tm.scrollback.Len())
	}
}

// ── Render tests ────────────────────────────────────────────────

func TestRenderGlyphRowPreservesColor(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 10

	// Create a glyph row with a non-default foreground.
	glyphs := make(scrollbackLine, 10)
	for i := range glyphs {
		glyphs[i] = vt10x.Glyph{
			Char: 'A',
			FG:   vt10x.Color(1), // red
			BG:   vt10x.DefaultBG,
		}
	}

	var sb strings.Builder
	tm.renderGlyphRow(&sb, glyphs)
	result := sb.String()

	// Should contain an SGR sequence for red foreground.
	if !strings.Contains(result, "\x1b[") {
		t.Error("expected SGR escape sequences in rendered output")
	}
	if !strings.Contains(result, "A") {
		t.Error("expected character 'A' in rendered output")
	}
}

func TestRenderGlyphRowPadsShorterLine(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 20

	// Create a 10-char wide glyph row — should be padded to 20.
	glyphs := make(scrollbackLine, 10)
	for i := range glyphs {
		glyphs[i] = vt10x.Glyph{Char: 'X', FG: vt10x.DefaultFG, BG: vt10x.DefaultBG}
	}

	var sb strings.Builder
	tm.renderGlyphRow(&sb, glyphs)
	result := sb.String()

	// Count visible characters (X + spaces for padding).
	visible := strings.Count(result, "X") + strings.Count(result, " ")
	if visible < 20 {
		t.Fatalf("expected at least 20 visible chars (10 X + 10 space), got %d", visible)
	}
}

func TestViewScrollbackMixedContent(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 4))
	tm.vt = vt
	tm.width = 20
	tm.height = 4

	// Put content in the live viewport.
	tm.vt.Write([]byte("live row 0\r\nlive row 1\r\nlive row 2\r\nlive row 3"))

	// Push some scrollback lines.
	tm.scrollback.Push(makeGlyphsWidth("scrollback 1", 20))
	tm.scrollback.Push(makeGlyphsWidth("scrollback 2", 20))

	// Scroll up by 2 — should show 2 scrollback + 2 viewport rows.
	tm.scrollOffset = 2
	view := tm.viewScrollback()

	lines := strings.Split(view, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines in view, got %d", len(lines))
	}

	// First two lines should contain scrollback content.
	if !strings.Contains(lines[0], "scrollback 1") {
		t.Errorf("line 0 should contain 'scrollback 1', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "scrollback 2") {
		t.Errorf("line 1 should contain 'scrollback 2', got %q", lines[1])
	}

	// Last two lines should contain live viewport content.
	if !strings.Contains(lines[2], "live row 0") {
		t.Errorf("line 2 should contain 'live row 0', got %q", lines[2])
	}
	if !strings.Contains(lines[3], "live row 1") {
		t.Errorf("line 3 should contain 'live row 1', got %q", lines[3])
	}
}

// ── E2E: TUI-style full-screen rewrite via vt10x ────────────────

func TestCaptureScrollbackTUIFullRedraw(t *testing.T) {
	// Simulates how a Bubble Tea TUI app renders: cursor home (\x1b[H) then
	// rewrite all rows. Content scrolls in a content area while a header and
	// footer stay fixed. This is the OpenCode scenario the user reported.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(40, 10))
	tm.vt = vt
	tm.width = 40
	tm.height = 10

	// Frame 1: initial render.
	frame1 := "\x1b[H" + // cursor home
		"  opencode v1.0                         \r\n" +
		"  Line 1 of response                    \r\n" +
		"  Line 2 of response                    \r\n" +
		"  Line 3 of response                    \r\n" +
		"  Line 4 of response                    \r\n" +
		"  Line 5 of response                    \r\n" +
		"  Line 6 of response                    \r\n" +
		"  Line 7 of response                    \r\n" +
		"                                        \r\n" +
		"  > _                       esc exit    "
	tm.vt.Write([]byte(frame1))

	// Frame 2: content scrolls by 1, header/footer fixed.
	frame2 := "\x1b[H" +
		"  opencode v1.0                         \r\n" +
		"  Line 2 of response                    \r\n" +
		"  Line 3 of response                    \r\n" +
		"  Line 4 of response                    \r\n" +
		"  Line 5 of response                    \r\n" +
		"  Line 6 of response                    \r\n" +
		"  Line 7 of response                    \r\n" +
		"  Line 8 of response                    \r\n" +
		"                                        \r\n" +
		"  > _                       esc exit    "

	tm.captureAndWrite([]byte(frame2))

	if tm.scrollback.Len() == 0 {
		t.Fatal("expected scrollback to capture a line from TUI full-screen redraw")
	}

	captured := glyphsToText(tm.scrollback.Line(tm.scrollback.Len() - 1))
	// The captured line should be one of the content rows that scrolled off.
	// With shift=1, row 1 ("Line 1 of response") should be captured.
	if !strings.Contains(captured, "Line 1") {
		t.Fatalf("expected captured line to contain 'Line 1', got %q", captured)
	}
}

func TestCaptureScrollbackTUI200Lines(t *testing.T) {
	// Simulates the user's scenario: OpenCode prints 200+ lines. We send
	// multiple full-screen redraws, each scrolling the content by 1 line.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(60, 14))
	tm.vt = vt
	tm.width = 60
	tm.height = 14

	header := "  opencode v1.0"
	footer := "  > _                                       esc exit"

	// Render initial frame (lines 1-12).
	renderTUIFrame(tm.vt, tm.width, header, footer, 1, 12)

	// Simulate 200 lines of output, scrolling 1 line at a time.
	capturedCount := 0
	for startLine := 2; startLine <= 190; startLine++ {
		frame := buildTUIFrame(tm.width, header, footer, startLine, startLine+11)
		tm.captureAndWrite([]byte(frame))
		if tm.scrollback.Len() > capturedCount {
			capturedCount = tm.scrollback.Len()
		}
	}

	// We should have captured a significant number of lines.
	// Not all frames will trigger detection (some may fail the matching
	// threshold), but the majority should succeed.
	if tm.scrollback.Len() < 100 {
		t.Fatalf("expected at least 100 scrollback lines from 189 scroll events, got %d", tm.scrollback.Len())
	}

	t.Logf("captured %d scrollback lines from 189 scroll events", tm.scrollback.Len())
}

// renderTUIFrame writes a full TUI frame directly to vt (bypasses scrollback).
func renderTUIFrame(vt vt10x.Terminal, width int, header, footer string, startLine, endLine int) {
	vt.Write([]byte(buildTUIFrame(width, header, footer, startLine, endLine)))
}

// buildTUIFrame builds a cursor-home + full-screen rewrite string mimicking
// a TUI app with a fixed header (row 0), content area, and footer (last row).
func buildTUIFrame(width int, header, footer string, startLine, endLine int) string {
	var sb strings.Builder
	sb.WriteString("\x1b[H") // cursor home

	// Row 0: header
	sb.WriteString(padRight(header, width))
	sb.WriteString("\r\n")

	// Content rows
	for line := startLine; line <= endLine; line++ {
		content := "  Line " + strings.Repeat("", 0)
		content = "  Line " + itoa(line) + " of response"
		sb.WriteString(padRight(content, width))
		sb.WriteString("\r\n")
	}

	// Footer (last row, no trailing newline)
	sb.WriteString(padRight(footer, width))

	return sb.String()
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// ── Helpers ─────────────────────────────────────────────────────

// makeGlyphs creates a scrollbackLine from a format string with default colors.
func makeGlyphs(format string, args ...any) scrollbackLine {
	text := fmt.Sprintf(format, args...)

	glyphs := make(scrollbackLine, len(text))
	for i, ch := range text {
		glyphs[i] = vt10x.Glyph{
			Char: ch,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}

// makeGlyphsWidth creates a scrollbackLine padded/truncated to the given width.
func makeGlyphsWidth(text string, width int) scrollbackLine {
	glyphs := make(scrollbackLine, width)
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(text) {
			ch = rune(text[i])
		}
		glyphs[i] = vt10x.Glyph{
			Char: ch,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}
