// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

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
	// Seed buffer with 10 unique lines (dedup rejects identical text).
	for i := 0; i < 10; i++ {
		tm.scrollback.Push(makeGlyphs("line %d", i))
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

func TestViewScrollbackOnly_AltScreen(t *testing.T) {
	// Alt-screen sessions should show ONLY scrollback content, no live
	// viewport mixing. This prevents duplicate content when scrollback
	// captures content that's still visible on the TUI's current screen.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 4))
	tm.vt = vt
	tm.width = 20
	tm.height = 4
	tm.altScreen = true // key difference from TestViewScrollbackMixedContent

	// Put content in the live viewport (simulating a TUI app).
	tm.vt.Write([]byte("HEADER\r\ncontent A\r\ncontent B\r\nFOOTER"))

	// Push scrollback lines — some overlap with viewport content.
	tm.scrollback.Push(makeGlyphsWidth("old line 1", 20))
	tm.scrollback.Push(makeGlyphsWidth("old line 2", 20))
	tm.scrollback.Push(makeGlyphsWidth("content A", 20))
	tm.scrollback.Push(makeGlyphsWidth("old line 3", 20))

	// Scroll up by 1 — should show ONLY scrollback, no viewport rows.
	tm.scrollOffset = 1
	view := tm.viewScrollback()
	lines := strings.Split(view, "\n")

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	// All 4 lines should come from scrollback, no HEADER or FOOTER.
	if strings.Contains(view, "HEADER") {
		t.Error("alt-screen scrollback should not contain live viewport HEADER")
	}
	if strings.Contains(view, "FOOTER") {
		t.Error("alt-screen scrollback should not contain live viewport FOOTER")
	}

	// The 4 scrollback lines should be visible (Len=4, height=4, offset=1).
	if !strings.Contains(lines[0], "old line 1") {
		t.Errorf("line 0 should be 'old line 1', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "old line 2") {
		t.Errorf("line 1 should be 'old line 2', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "content A") {
		t.Errorf("line 2 should be 'content A', got %q", lines[2])
	}
	if !strings.Contains(lines[3], "old line 3") {
		t.Errorf("line 3 should be 'old line 3', got %q", lines[3])
	}
}

func TestViewScrollbackOnly_ScrollFurther(t *testing.T) {
	// Verify that scrolling further into history works correctly.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 3))
	tm.vt = vt
	tm.width = 20
	tm.height = 3
	tm.altScreen = true

	for i := 0; i < 10; i++ {
		tm.scrollback.Push(makeGlyphsWidth(fmt.Sprintf("line %d", i), 20))
	}

	// Offset=1: newest page (lines 7, 8, 9).
	tm.scrollOffset = 1
	view := tm.viewScrollback()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[2], "line 9") {
		t.Errorf("offset=1 bottom should be 'line 9', got %q", lines[2])
	}

	// Offset=3: shifted 2 lines back (lines 5, 6, 7).
	tm.scrollOffset = 3
	view = tm.viewScrollback()
	lines = strings.Split(view, "\n")
	if !strings.Contains(lines[0], "line 5") {
		t.Errorf("offset=3 top should be 'line 5', got %q", lines[0])
	}
	if !strings.Contains(lines[2], "line 7") {
		t.Errorf("offset=3 bottom should be 'line 7', got %q", lines[2])
	}

	// Offset=10: at the very beginning (only line 0 visible at bottom).
	tm.scrollOffset = 10
	view = tm.viewScrollback()
	lines = strings.Split(view, "\n")
	if !strings.Contains(lines[2], "line 0") {
		t.Errorf("offset=10 bottom should be 'line 0', got %q", lines[2])
	}
	// Top two lines should be blank (no scrollback content).
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("offset=10 top should be blank, got %q", lines[0])
	}
}

// ── altScreen flag determines rendering path ────────────────────

func TestViewScrollback_MixedCausesDuplication(t *testing.T) {
	// Demonstrates that when altScreen is incorrectly false for a TUI session,
	// viewScrollbackMixed produces duplication — the most recent scrollback
	// rows at the top overlap with live viewport rows from the top. This is
	// the bug seen with mouse scroll.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(30, 5))
	tm.vt = vt
	tm.width = 30
	tm.height = 5
	tm.altScreen = false // BUG: should be true for TUI apps

	// Live viewport shows TUI content.
	tm.vt.Write([]byte("HEADER\r\nAlpha\r\nBravo\r\nCharlie\r\nFOOTER"))

	// Scrollback has content that partially overlaps the viewport.
	// The MOST RECENT scrollback lines (pushed last) will be shown at top
	// by the mixed renderer when scrolled up. Make them overlap with
	// viewport rows that will also be rendered from the live VT.
	tm.scrollback.Push(makeGlyphsWidth("old stuff", 30))
	tm.scrollback.Push(makeGlyphsWidth("HEADER", 30)) // matches viewport row 0
	tm.scrollback.Push(makeGlyphsWidth("Alpha", 30))  // matches viewport row 1

	// Scroll up by 2 with altScreen=false → mixed renderer.
	// Mixed shows: 2 most recent scrollback lines + top 3 viewport rows.
	// Scrollback: "HEADER", "Alpha" | Viewport: "HEADER", "Alpha", "Bravo"
	tm.scrollOffset = 2
	mixed := tm.viewScrollback()

	// Count how many times "HEADER" and "Alpha" appear — mixed renderer
	// will show each twice (once from scrollback, once from viewport).
	headerCount := strings.Count(mixed, "HEADER")
	alphaCount := strings.Count(mixed, "Alpha")
	if headerCount < 2 || alphaCount < 2 {
		t.Fatalf("expected mixed renderer to duplicate content: HEADER=%d Alpha=%d (want >=2 each)",
			headerCount, alphaCount)
	}

	// Now set altScreen=true and verify the scrollback-only renderer avoids it.
	tm.altScreen = true
	only := tm.viewScrollback()

	// The alt-screen view shows ONLY scrollback, no live viewport rows.
	onlyHeader := strings.Count(only, "HEADER")
	onlyAlpha := strings.Count(only, "Alpha")
	if onlyHeader > 1 {
		t.Errorf("alt-screen renderer should not duplicate 'HEADER', got %d", onlyHeader)
	}
	if onlyAlpha > 1 {
		t.Errorf("alt-screen renderer should not duplicate 'Alpha', got %d", onlyAlpha)
	}

	// The alt-screen view should NOT contain FOOTER from the live viewport.
	if strings.Contains(only, "FOOTER") {
		t.Error("alt-screen scrollback should not show live FOOTER")
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

// ── Alt-screen diff capture tests ───────────────────────────────

func TestPushAltScreenDiff_BasicCapture(t *testing.T) {
	// Simulate a TUI app full-screen repaint where content rows disappear.
	sb := newScrollbackBuffer(100)

	oldTexts := []string{
		"header", // row 0 — stays
		"old content line1",
		"old content line2",
		"old content line3",
		"old content line4",
		"footer", // row 5 — stays
	}
	oldGlyphs := make([]scrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header", // same as old
		"new content line1",
		"new content line2",
		"new content line3",
		"new content line4",
		"footer", // same as old
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0)

	// 4 old content rows disappeared (not in curTexts, not at same position).
	if pushed != 4 {
		t.Fatalf("expected 4 pushed rows, got %d", pushed)
	}
	if sb.Len() != 4 {
		t.Fatalf("expected scrollback len=4, got %d", sb.Len())
	}

	// Verify correct content was captured.
	for i := 0; i < 4; i++ {
		got := glyphsToText(sb.Line(i))
		expected := fmt.Sprintf("old content line%d", i+1)
		if got != expected {
			t.Errorf("line %d: got %q, want %q", i, got, expected)
		}
	}
}

func TestPushAltScreenDiff_SkipSmallDiff(t *testing.T) {
	// Only 2 rows changed — below minAltDiffRows threshold.
	sb := newScrollbackBuffer(100)

	oldTexts := []string{
		"header",
		"old content 1",
		"old content 2",
		"same row",
		"footer",
	}
	oldGlyphs := make([]scrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header",
		"new content 1",
		"new content 2",
		"same row",
		"footer",
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0)

	// Only 2 rows changed — below threshold of 3, so nothing pushed.
	if pushed != 0 {
		t.Fatalf("expected 0 pushed (below threshold), got %d", pushed)
	}
}

func TestPushAltScreenDiff_SkipRowsPresentElsewhere(t *testing.T) {
	// An old row that moved to a different position should NOT be pushed.
	sb := newScrollbackBuffer(100)

	oldTexts := []string{
		"header",
		"content A",
		"content B",
		"content C",
		"content D",
		"content E",
		"footer",
	}
	oldGlyphs := make([]scrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header",
		"content B", // moved up from row 2 to row 1
		"content C", // moved up from row 3 to row 2
		"content D", // moved up
		"content E", // moved up
		"content F", // new
		"footer",
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0)

	// Only "content A" truly disappeared (not anywhere in curTexts).
	// That's 1 row — below minAltDiffRows, so nothing pushed.
	if pushed != 0 {
		t.Fatalf("expected 0 pushed (only 1 unique disappeared row), got %d", pushed)
	}
}

func TestPushAltScreenDiff_BlankRowsIgnored(t *testing.T) {
	sb := newScrollbackBuffer(100)

	oldTexts := []string{
		"header",
		"", // blank — should be skipped
		"old line 1",
		"old line 2",
		"old line 3",
		"", // blank
		"footer",
	}
	oldGlyphs := make([]scrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header",
		"",
		"new line 1",
		"new line 2",
		"new line 3",
		"",
		"footer",
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0)

	// 3 old content rows disappeared (blank rows skipped).
	if pushed != 3 {
		t.Fatalf("expected 3 pushed rows, got %d", pushed)
	}
}

// ── Dedup tests ─────────────────────────────────────────────────

func TestScrollbackDedup_SkipsDuplicates(t *testing.T) {
	buf := newScrollbackBuffer(100)

	buf.Push(makeGlyphs("hello world"))
	buf.Push(makeGlyphs("hello world")) // duplicate — should be skipped
	buf.Push(makeGlyphs("hello world")) // duplicate — should be skipped

	if buf.Len() != 1 {
		t.Fatalf("expected len=1 after dedup, got %d", buf.Len())
	}

	got := glyphsToText(buf.Line(0))
	if got != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", got)
	}
}

func TestScrollbackDedup_AllowsUniqueContent(t *testing.T) {
	buf := newScrollbackBuffer(100)

	buf.Push(makeGlyphs("line A"))
	buf.Push(makeGlyphs("line B"))
	buf.Push(makeGlyphs("line C"))

	if buf.Len() != 3 {
		t.Fatalf("expected len=3, got %d", buf.Len())
	}
}

func TestScrollbackDedup_BlankLinesBypassDedup(t *testing.T) {
	buf := newScrollbackBuffer(100)

	// Blank lines (empty after trim) should always be pushed (not deduped).
	buf.Push(makeGlyphs(""))
	buf.Push(makeGlyphs(""))
	buf.Push(makeGlyphs(""))

	if buf.Len() != 3 {
		t.Fatalf("expected len=3 for blank lines, got %d", buf.Len())
	}
}

func TestScrollbackDedup_BufferEvictsOnWraparound(t *testing.T) {
	// With buffer-wide dedup, a line can only be re-pushed after it is
	// evicted from the ring buffer (i.e. overwritten by wraparound).
	buf := newScrollbackBuffer(5)

	buf.Push(makeGlyphs("original line"))
	if buf.Len() != 1 {
		t.Fatalf("expected len=1, got %d", buf.Len())
	}

	// "original line" is still in the buffer — re-push should be rejected.
	buf.Push(makeGlyphs("original line"))
	if buf.Len() != 1 {
		t.Fatalf("expected len=1 (dedup), got %d", buf.Len())
	}

	// Fill the buffer to capacity to evict "original line" via wraparound.
	buf.Push(makeGlyphs("filler 1"))
	buf.Push(makeGlyphs("filler 2"))
	buf.Push(makeGlyphs("filler 3"))
	buf.Push(makeGlyphs("filler 4")) // buffer full (cap=5)
	buf.Push(makeGlyphs("filler 5")) // overwrites "original line"

	if buf.Len() != 5 {
		t.Fatalf("expected len=5, got %d", buf.Len())
	}

	// Now "original line" has been evicted — re-push should succeed.
	buf.Push(makeGlyphs("original line"))
	newest := glyphsToText(buf.Line(buf.Len() - 1))
	if newest != "original line" {
		t.Fatalf("expected newest line to be %q after re-push, got %q", "original line", newest)
	}
}

func TestScrollbackDedup_NoEvictionWhileInBuffer(t *testing.T) {
	// Verify that a line cannot be re-pushed as long as it remains in the buffer,
	// even after many other unique lines have been pushed.
	buf := newScrollbackBuffer(100)

	buf.Push(makeGlyphs("persistent line"))

	// Push 50 unique lines — "persistent line" is still in the buffer.
	for i := 0; i < 50; i++ {
		buf.Push(makeGlyphs("other line %d", i))
	}

	beforeLen := buf.Len()
	buf.Push(makeGlyphs("persistent line")) // should be rejected — still in buffer
	if buf.Len() != beforeLen {
		t.Fatalf("expected len=%d (dedup), got %d", beforeLen, buf.Len())
	}
}

func TestScrollbackDedup_DuplicateNotCountedInLen(t *testing.T) {
	buf := newScrollbackBuffer(100)

	buf.Push(makeGlyphs("same content"))
	buf.Push(makeGlyphs("same content"))
	buf.Push(makeGlyphs("different content"))
	buf.Push(makeGlyphs("same content")) // still in dedup window

	if buf.Len() != 2 {
		t.Fatalf("expected len=2 (1 same + 1 different), got %d", buf.Len())
	}
}

// ── Chrome skipping tests ───────────────────────────────────────

func TestPushAltScreenDiff_ChromeSkipping(t *testing.T) {
	// With chrome skipping, header (row 0) and last 2 rows (status + footer)
	// should be excluded from candidates even if they changed.
	sb := newScrollbackBuffer(100)

	oldTexts := []string{
		"header v1.0",                        // row 0: header — skip
		"old content line1",                  // row 1: content
		"old content line2",                  // row 2: content
		"old content line3",                  // row 3: content
		"old content line4",                  // row 4: content
		"old content line5",                  // row 5: content
		"Build · claude-opus-4-6 · 5m 21s",   // row 6: status bar — skip
		"  > _                     esc exit", // row 7: footer — skip
	}
	oldGlyphs := make([]scrollbackLine, len(oldTexts))
	for i, txt := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", txt)
	}

	curTexts := []string{
		"header v1.0",       // same header
		"new content line1", // all content changed
		"new content line2",
		"new content line3",
		"new content line4",
		"new content line5",
		"Build · claude-opus-4-6 · 5m 22s",   // status bar changed (timer tick)
		"  > _                     esc exit", // footer same
	}

	// With chromeSkipFirst=1, chromeSkipLast=2: skip row 0 and rows 6-7.
	// Only rows 1-5 are candidates. All 5 old content rows disappeared.
	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 1, 2)

	if pushed != 5 {
		t.Fatalf("expected 5 pushed rows with chrome skipping, got %d", pushed)
	}
	if sb.Len() != 5 {
		t.Fatalf("expected scrollback len=5, got %d", sb.Len())
	}

	// Verify the status bar row was NOT pushed.
	for i := 0; i < sb.Len(); i++ {
		line := glyphsToText(sb.Line(i))
		if strings.Contains(line, "Build") {
			t.Errorf("status bar row should not be in scrollback, found at index %d: %q", i, line)
		}
	}
}

func TestPushAltScreenDiff_ChromeSkipLargerThanScreen(t *testing.T) {
	// Edge case: skip values larger than screen height should not panic.
	sb := newScrollbackBuffer(100)

	oldTexts := []string{"only row"}
	oldGlyphs := []scrollbackLine{makeGlyphs("only row")}
	curTexts := []string{"changed"}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 5, 5)
	if pushed != 0 {
		t.Fatalf("expected 0 pushed for oversized skip, got %d", pushed)
	}
}

// ── textToGlyphs tests ─────────────────────────────────────────

func TestTextToGlyphs_BasicString(t *testing.T) {
	glyphs := textToGlyphs("hello")
	if len(glyphs) != 5 {
		t.Fatalf("expected 5 glyphs, got %d", len(glyphs))
	}
	got := glyphsToText(glyphs)
	if got != "hello" {
		t.Errorf("round-trip: expected %q, got %q", "hello", got)
	}
}

func TestTextToGlyphs_EmptyString(t *testing.T) {
	glyphs := textToGlyphs("")
	if len(glyphs) != 0 {
		t.Fatalf("expected 0 glyphs for empty string, got %d", len(glyphs))
	}
}

func TestTextToGlyphs_Unicode(t *testing.T) {
	glyphs := textToGlyphs("── You ──")
	got := glyphsToText(glyphs)
	if got != "── You ──" {
		t.Errorf("expected %q, got %q", "── You ──", got)
	}
}

func TestTextToGlyphs_PushedToScrollback(t *testing.T) {
	// Verify that textToGlyphs output works with the scrollback buffer.
	buf := newScrollbackBuffer(100)
	lines := []string{
		"── You ──────────────",
		"Fix the login bug",
		"",
		"── Assistant ─────────",
		"I'll help you fix it.",
	}
	for _, line := range lines {
		buf.Push(textToGlyphs(line))
	}

	if buf.Len() != 5 {
		t.Fatalf("expected 5 lines in scrollback, got %d", buf.Len())
	}

	got := glyphsToText(buf.Line(0))
	if !strings.HasPrefix(got, "── You") {
		t.Errorf("first line should be You header, got %q", got)
	}

	got = glyphsToText(buf.Line(1))
	if got != "Fix the login bug" {
		t.Errorf("second line: expected %q, got %q", "Fix the login bug", got)
	}
}

// ── E2E: alt-screen scroll-up duplication test ──────────────────

// TestAltScreenScrollNoDuplication is the key test for the scroll duplication
// bug. It simulates the full app-level flow:
//
//  1. A TUI app (OpenCode) renders multiple frames via cursor-home + full rewrite
//  2. pushAltScreenDiff captures disappeared rows into scrollback (like runScrollCheck)
//  3. User scrolls up → viewScrollbackOnly renders from scrollback
//  4. Check that NO line in the scrollback view appears on the live viewport
//
// This exercises the exact code path the user triggers: alt-screen TUI session,
// scrollback captured via snapshot diffs, rendering via viewScrollbackOnly.
func TestAltScreenScrollNoDuplication(t *testing.T) {
	const width = 60
	const height = 14
	const chromeTop = 1
	const chromeBottom = 2

	// Set up terminal model with vt10x and scrollback.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(width, height))
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = newScrollbackBuffer(1000)

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"

	// --- Phase 1: Simulate TUI app rendering frames that scroll by 5 lines ---
	// Each frame has: header (row 0), content rows (1-11), status bar (12), footer (13).
	// In the real app, 100ms of output scrolls content by several lines at once.
	contentRows := height - chromeTop - chromeBottom // 11 content rows

	// Track snapshots for pushAltScreenDiff (simulates checkScrollback).
	var prevTexts []string
	var prevGlyphs []scrollbackLine

	scrollPerFrame := 5
	for frame := 0; frame < 50; frame++ {
		frameStart := 1 + frame*scrollPerFrame
		// Build and render the frame.
		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, frameStart, frameStart+contentRows-1)
		vt.Write([]byte(frameData))

		// Snapshot current screen.
		curTexts := make([]string, height)
		curGlyphs := make([]scrollbackLine, height)
		for row := 0; row < height; row++ {
			glyphs := make(scrollbackLine, width)
			for col := 0; col < width; col++ {
				glyphs[col] = vt.Cell(col, row)
			}
			curGlyphs[row] = glyphs
			curTexts[row] = glyphsToText(glyphs)
		}

		// Run the same diff logic as checkScrollback for alt-screen.
		if prevTexts != nil {
			pushAltScreenDiff(tm.scrollback, prevTexts, prevGlyphs, curTexts, chromeTop, chromeBottom)
		}

		prevTexts = curTexts
		prevGlyphs = curGlyphs
	}

	t.Logf("scrollback has %d lines after 50 frames", tm.scrollback.Len())

	if tm.scrollback.Len() == 0 {
		t.Fatal("expected scrollback to have captured lines from TUI redraws")
	}

	// --- Phase 2: Get the live viewport text ---
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := tm.viewportRowText(row)
		if text != "" {
			liveLines[text] = true
		}
	}

	t.Logf("live viewport has %d non-empty unique lines", len(liveLines))

	// --- Phase 3: Scroll up by various amounts and check for duplication ---
	for _, scrollDelta := range []int{1, 3, height / 2, height, tm.scrollback.Len()} {
		tm.scrollOffset = scrollDelta
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("DUPLICATION at scrollOffset=%d row %d: %q appears in both scrollback view and live viewport",
					scrollDelta, i, trimmed)
			}
		}
	}

	// --- Phase 4: Verify scrollback content is ordered (oldest first) ---
	// Lines should be roughly Line 1, Line 2, ... (with possible gaps from
	// the minAltDiffRows threshold skipping small diffs).
	sbTexts := make([]string, tm.scrollback.Len())
	for i := 0; i < tm.scrollback.Len(); i++ {
		sbTexts[i] = glyphsToText(tm.scrollback.Line(i))
	}
	t.Logf("scrollback content (first 10): %v", sbTexts[:min(10, len(sbTexts))])
	t.Logf("scrollback content (last 10): %v", sbTexts[max(0, len(sbTexts)-10):])

	// --- Phase 5: Verify no internal duplicates in the scrollback view ---
	for _, scrollDelta := range []int{height / 2, height} {
		tm.scrollOffset = scrollDelta
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		seen := make(map[string]int)
		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if prevIdx, exists := seen[trimmed]; exists {
				t.Errorf("INTERNAL DUPLICATION at scrollOffset=%d: %q appears at rows %d and %d",
					scrollDelta, trimmed, prevIdx, i)
			}
			seen[trimmed] = i
		}
	}
}

// TestAltScreenScrollSmallBuffer tests scroll behavior when the scrollback
// buffer has fewer lines than the viewport height — the user should see
// partial content, not garbage or duplication.
func TestAltScreenScrollSmallBuffer(t *testing.T) {
	const width = 40
	const height = 10

	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(width, height))
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = newScrollbackBuffer(100)

	// Only push 3 lines to scrollback (less than height).
	tm.scrollback.Push(makeGlyphsWidth("Line A", width))
	tm.scrollback.Push(makeGlyphsWidth("Line B", width))
	tm.scrollback.Push(makeGlyphsWidth("Line C", width))

	// Scroll up by 1.
	tm.scrollOffset = 1
	view := tm.viewScrollbackOnly()
	lines := strings.Split(view, "\n")

	nonBlank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonBlank++
		}
	}
	t.Logf("small buffer scroll: %d non-blank lines out of %d total", nonBlank, len(lines))

	// Should not panic and should have at most 3 non-blank lines.
	if nonBlank > 3 {
		t.Errorf("expected at most 3 non-blank lines, got %d", nonBlank)
	}
}

// TestAltScreenScrollHalfPageNoDuplication tests the half-page jump logic
// specifically — this is what the mouse handler uses for alt-screen sessions.
func TestAltScreenScrollHalfPageNoDuplication(t *testing.T) {
	const width = 60
	const height = 20
	const chromeTop = 1
	const chromeBottom = 2

	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(width, height))
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = newScrollbackBuffer(1000)

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"

	// Simulate 100 frames of TUI output, scrolling 5 lines per frame.
	var prevTexts []string
	var prevGlyphs []scrollbackLine
	contentRows := height - chromeTop - chromeBottom // 17 content rows
	scrollPerFrame := 5

	for frame := 0; frame < 100; frame++ {
		frameStart := 1 + frame*scrollPerFrame
		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, frameStart, frameStart+contentRows-1)
		vt.Write([]byte(frameData))

		curTexts := make([]string, height)
		curGlyphs := make([]scrollbackLine, height)
		for row := 0; row < height; row++ {
			glyphs := make(scrollbackLine, width)
			for col := 0; col < width; col++ {
				glyphs[col] = vt.Cell(col, row)
			}
			curGlyphs[row] = glyphs
			curTexts[row] = glyphsToText(glyphs)
		}

		if prevTexts != nil {
			pushAltScreenDiff(tm.scrollback, prevTexts, prevGlyphs, curTexts, chromeTop, chromeBottom)
		}
		prevTexts = curTexts
		prevGlyphs = curGlyphs
	}

	t.Logf("scrollback has %d lines after 100 frames", tm.scrollback.Len())

	// Get live viewport.
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := tm.viewportRowText(row)
		if text != "" {
			liveLines[text] = true
		}
	}

	// Simulate exact mouse handler behavior: half-page jump.
	halfPage := height / 2

	// Scroll up multiple ticks and check each.
	tm.scrollOffset = 0
	for tick := 1; tick <= 5; tick++ {
		tm.ScrollBy(halfPage)
		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("tick %d (offset=%d) row %d: %q duplicates live viewport",
					tick, tm.scrollOffset, i, trimmed)
			}
		}
	}
}

// buildTUIFrameWithChrome builds a cursor-home + full-screen rewrite frame
// with header (row 0), content rows, status bar (row height-2), footer (row height-1).
func buildTUIFrameWithChrome(width int, header, statusBar, footer string, startLine, endLine int) string {
	// Calculate total rows: header + content + status + footer.
	// Content rows fill the middle.
	var sb strings.Builder
	sb.WriteString("\x1b[H") // cursor home

	// Row 0: header
	sb.WriteString(padRight(header, width))
	sb.WriteString("\r\n")

	// Content rows (between header and status bar).
	for line := startLine; line <= endLine; line++ {
		content := "  Line " + itoa(line) + " of response"
		sb.WriteString(padRight(content, width))
		sb.WriteString("\r\n")
	}

	// Status bar (second to last row).
	sb.WriteString(padRight(statusBar, width))
	sb.WriteString("\r\n")

	// Footer (last row, no trailing newline).
	sb.WriteString(padRight(footer, width))

	return sb.String()
}

// TestAltScreenTraditionalScrollPathNoDuplication tests the scenario where
// detectScrollShift succeeds for an alt-screen TUI app (content area scrolls
// while header/footer stay fixed). This exercises the shift > 0 branch in
// checkScrollback, NOT the pushAltScreenDiff branch.
func TestAltScreenTraditionalScrollPathNoDuplication(t *testing.T) {
	const width = 60
	const height = 14
	const chromeTop = 1
	const chromeBottom = 2

	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(width, height))
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = newScrollbackBuffer(1000)

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"
	contentRows := height - chromeTop - chromeBottom // 11

	// Track snapshots like checkScrollback does.
	var prevTexts []string
	var prevGlyphs []scrollbackLine

	scrollPerFrame := 3 // enough for shift detection but less than content rows

	for frame := 0; frame < 60; frame++ {
		frameStart := 1 + frame*scrollPerFrame
		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, frameStart, frameStart+contentRows-1)
		vt.Write([]byte(frameData))

		curTexts := make([]string, height)
		curGlyphs := make([]scrollbackLine, height)
		for row := 0; row < height; row++ {
			glyphs := make(scrollbackLine, width)
			for col := 0; col < width; col++ {
				glyphs[col] = vt.Cell(col, row)
			}
			curGlyphs[row] = glyphs
			curTexts[row] = glyphsToText(glyphs)
		}

		if prevTexts != nil {
			// Use the EXACT same logic as checkScrollback.
			shift := detectScrollShift(prevTexts, curTexts)
			if shift > 0 {
				firstDiff := 0
				for firstDiff < len(prevTexts) && firstDiff < len(curTexts) && prevTexts[firstDiff] == curTexts[firstDiff] {
					firstDiff++
				}
				end := firstDiff + shift
				if end > len(prevGlyphs) {
					end = len(prevGlyphs)
				}
				for i := firstDiff; i < end; i++ {
					tm.scrollback.Push(prevGlyphs[i])
				}
				t.Logf("frame %d: shift=%d, pushed rows %d-%d", frame, shift, firstDiff, end-1)
			} else {
				// Alt-screen diff fallback.
				pushed := pushAltScreenDiff(tm.scrollback, prevTexts, prevGlyphs, curTexts, chromeTop, chromeBottom)
				if pushed > 0 {
					t.Logf("frame %d: pushAltScreenDiff pushed %d", frame, pushed)
				}
			}
		}

		prevTexts = curTexts
		prevGlyphs = curGlyphs
	}

	t.Logf("scrollback has %d lines after 60 frames", tm.scrollback.Len())

	// Get live viewport text.
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := tm.viewportRowText(row)
		if text != "" {
			liveLines[text] = true
		}
	}

	// Scroll up and check for duplication.
	for _, offset := range []int{1, 3, height / 2, height} {
		tm.scrollOffset = offset
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("offset=%d row %d: %q duplicates live viewport", offset, i, trimmed)
			}
		}

		// Check internal duplicates too.
		seen := make(map[string]int)
		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if prevIdx, exists := seen[trimmed]; exists {
				t.Errorf("offset=%d: %q appears at rows %d and %d", offset, trimmed, prevIdx, i)
			}
			seen[trimmed] = i
		}
	}
}

// TestAltScreenScrollMixedCapturePaths tests the scenario where the shift
// detection alternates between finding shifts and not (due to varying output
// rates). This creates a mix of traditional scroll captures and alt-screen
// diff captures, which is the most realistic scenario.
func TestAltScreenScrollMixedCapturePaths(t *testing.T) {
	const width = 60
	const height = 14
	const chromeTop = 1
	const chromeBottom = 2

	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(width, height))
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = newScrollbackBuffer(1000)

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"
	contentRows := height - chromeTop - chromeBottom // 11

	var prevTexts []string
	var prevGlyphs []scrollbackLine

	shiftCaptures := 0
	diffCaptures := 0
	currentLine := 1

	for frame := 0; frame < 80; frame++ {
		// Alternate between small shifts (1-2 lines, too few for alt diff)
		// and larger shifts (5+ lines, detectable by shift detection).
		var scrollAmount int
		if frame%3 == 0 {
			scrollAmount = 5 // large scroll — shift detection path
		} else {
			scrollAmount = 1 // small scroll — may fail shift detection, try alt diff
		}
		currentLine += scrollAmount

		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, currentLine, currentLine+contentRows-1)
		vt.Write([]byte(frameData))

		curTexts := make([]string, height)
		curGlyphs := make([]scrollbackLine, height)
		for row := 0; row < height; row++ {
			glyphs := make(scrollbackLine, width)
			for col := 0; col < width; col++ {
				glyphs[col] = vt.Cell(col, row)
			}
			curGlyphs[row] = glyphs
			curTexts[row] = glyphsToText(glyphs)
		}

		if prevTexts != nil {
			shift := detectScrollShift(prevTexts, curTexts)
			if shift > 0 {
				firstDiff := 0
				for firstDiff < len(prevTexts) && firstDiff < len(curTexts) && prevTexts[firstDiff] == curTexts[firstDiff] {
					firstDiff++
				}
				end := firstDiff + shift
				if end > len(prevGlyphs) {
					end = len(prevGlyphs)
				}
				for i := firstDiff; i < end; i++ {
					tm.scrollback.Push(prevGlyphs[i])
				}
				shiftCaptures++
			} else {
				pushed := pushAltScreenDiff(tm.scrollback, prevTexts, prevGlyphs, curTexts, chromeTop, chromeBottom)
				if pushed > 0 {
					diffCaptures++
				}
			}
		}

		prevTexts = curTexts
		prevGlyphs = curGlyphs
	}

	t.Logf("scrollback: %d lines, shift captures: %d, diff captures: %d",
		tm.scrollback.Len(), shiftCaptures, diffCaptures)

	// Get live viewport.
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := tm.viewportRowText(row)
		if text != "" {
			liveLines[text] = true
		}
	}

	// Scroll up and check.
	for _, offset := range []int{1, 3, height / 2, height, height * 2} {
		tm.scrollOffset = offset
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("offset=%d row %d: %q duplicates live viewport", offset, i, trimmed)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
