// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"strings"

	"github.com/hinshun/vt10x"
)

// defaultScrollbackCapacity is the maximum number of lines retained in the
// scrollback buffer before the oldest lines are overwritten.
const defaultScrollbackCapacity = 10_000

// scrollbackLine is a snapshot of one terminal row's glyph data, preserved
// before vt10x destroys it during a scroll-up operation.
type scrollbackLine []vt10x.Glyph

// scrollbackBuffer is a ring buffer that captures terminal rows as they scroll
// off the top of the vt10x viewport. vt10x has no built-in scrollback — its
// scrollUp() clears lines before rotating them — so we snapshot rows before
// each Write() and push any that scrolled off into this buffer.
//
// The buffer maintains a dedup set covering ALL lines currently stored. When
// a line is overwritten (ring wraps), its text is removed from the set. This
// prevents the same content from being stored repeatedly — critical for
// alt-screen TUI apps (like OpenCode) that do full-screen repaints every
// ~100ms, which would otherwise flood the buffer with duplicate conversation
// rows.
type scrollbackBuffer struct {
	lines []scrollbackLine
	cap   int // max capacity
	start int // index of the oldest line
	len   int // number of lines currently stored

	// lineTexts stores the text content of each ring slot, parallel to lines.
	// Used to maintain textSet: when a slot is overwritten, its old text is
	// decremented/removed from textSet before the new text is added.
	lineTexts []string
	textSet   map[string]int // text → count of occurrences in buffer
}

func newScrollbackBuffer(capacity int) *scrollbackBuffer {
	if capacity <= 0 {
		capacity = defaultScrollbackCapacity
	}
	return &scrollbackBuffer{
		lines:     make([]scrollbackLine, capacity),
		cap:       capacity,
		lineTexts: make([]string, capacity),
		textSet:   make(map[string]int, capacity),
	}
}

// Push appends a glyph row to the buffer, taking ownership of the slice.
// Rows whose text content already exists in the buffer are skipped to
// prevent duplicate content from alt-screen TUI repaints.
// When full, the oldest line is overwritten (and its text removed from
// the dedup set).
func (b *scrollbackBuffer) Push(glyphs scrollbackLine) {
	text := glyphsToText(glyphs)

	// Dedup: skip if this exact text is already in the buffer.
	// Blank lines bypass dedup so that multiple blank separators are preserved.
	if text != "" {
		if b.textSet[text] > 0 {
			return
		}
	}

	// Compute the ring slot to write into.
	idx := (b.start + b.len) % b.cap

	// If the ring is full, we're overwriting the oldest line — remove its
	// text from the dedup set so that content evicted from scrollback can
	// be re-added if it appears again later.
	if b.len == b.cap {
		old := b.lineTexts[idx]
		if old != "" {
			if c := b.textSet[old]; c <= 1 {
				delete(b.textSet, old)
			} else {
				b.textSet[old] = c - 1
			}
		}
		b.start = (b.start + 1) % b.cap
	} else {
		b.len++
	}

	// Store the line and update the dedup set.
	b.lines[idx] = glyphs
	b.lineTexts[idx] = text
	if text != "" {
		b.textSet[text]++
	}
}

// Len returns the number of lines currently in the buffer.
func (b *scrollbackBuffer) Len() int {
	return b.len
}

// Line returns the i-th line where i=0 is the oldest and i=Len()-1 is the
// newest. Returns nil for out-of-range indices.
func (b *scrollbackBuffer) Line(i int) scrollbackLine {
	if i < 0 || i >= b.len {
		return nil
	}
	return b.lines[(b.start+i)%b.cap]
}

// Clear discards all stored lines and resets the dedup set.
func (b *scrollbackBuffer) Clear() {
	b.start = 0
	b.len = 0
	b.textSet = make(map[string]int, b.cap)
	for i := range b.lineTexts {
		b.lineTexts[i] = ""
	}
}

// ── Scroll shift detection ──────────────────────────────────────

// detectScrollShift compares row text content before and after a vt10x Write
// to determine how many lines scrolled off the top.
//
// It uses vote-based matching: for each candidate shift, it counts how many
// row pairs (old[i+shift], new[i]) are equal, skipping empty rows. This
// handles both normal terminal scrolling and TUI apps that do full-screen
// redraws (cursor-home + rewrite) with fixed headers/footers that don't shift.
//
// Returns 0 if no scroll was detected.
func detectScrollShift(oldTexts, newTexts []string) int {
	height := len(oldTexts)
	if height == 0 || len(newTexts) == 0 {
		return 0
	}

	// Limit search to half the screen height — larger shifts in a single
	// Write() are rare and more likely to produce false matches.
	maxShift := height / 2
	if maxShift < 1 {
		maxShift = 1
	}

	for shift := 1; shift <= maxShift; shift++ {
		comparisons := height - shift
		if comparisons > len(newTexts) {
			comparisons = len(newTexts)
		}
		if comparisons <= 0 {
			break
		}

		matches := 0
		nonEmpty := 0
		for i := 0; i < comparisons; i++ {
			old := oldTexts[i+shift]
			new := newTexts[i]
			if old == "" && new == "" {
				continue // skip empty-to-empty — not informative
			}
			nonEmpty++
			if old == new {
				matches++
			}
		}

		// Need at least 3 non-empty matching rows and >50% match rate.
		// The threshold is intentionally permissive to handle TUI apps
		// where a few rows (header, footer, new content at bottom) won't
		// match, while the content area in the middle does.
		minMatches := 3
		if height <= 6 {
			minMatches = 2
		}
		if nonEmpty >= minMatches && matches*2 > nonEmpty {
			return shift
		}
	}

	return 0
}

// ── Helpers ─────────────────────────────────────────────────────

// textToGlyphs converts a plain text string to a glyph slice with default
// colors. Used for pre-populating scrollback with history lines.
func textToGlyphs(text string) scrollbackLine {
	runes := []rune(text)
	glyphs := make(scrollbackLine, len(runes))
	for i, r := range runes {
		glyphs[i] = vt10x.Glyph{
			Char: r,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}

// glyphsToText converts a glyph slice to its text content, trimming trailing
// spaces. Used for scroll shift detection.
func glyphsToText(glyphs []vt10x.Glyph) string {
	var sb strings.Builder
	sb.Grow(len(glyphs))
	for _, g := range glyphs {
		if g.Char == 0 {
			sb.WriteRune(' ')
		} else {
			sb.WriteRune(g.Char)
		}
	}
	return strings.TrimRight(sb.String(), " ")
}
