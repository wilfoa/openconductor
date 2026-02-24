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

// dedupWindow is the number of recently pushed lines whose text content is
// tracked for deduplication. Alt-screen TUI apps (like OpenCode) repaint the
// entire screen every ~100ms, causing the same conversation rows to be pushed
// repeatedly across successive repaints. The dedup window prevents this.
const dedupWindow = 200

// scrollbackBuffer is a ring buffer that captures terminal rows as they scroll
// off the top of the vt10x viewport. vt10x has no built-in scrollback — its
// scrollUp() clears lines before rotating them — so we snapshot rows before
// each Write() and push any that scrolled off into this buffer.
//
// The buffer maintains a dedup set of recently pushed line texts to prevent
// the same content from being stored repeatedly (common with alt-screen TUI
// apps that do full-screen repaints).
type scrollbackBuffer struct {
	lines []scrollbackLine
	cap   int // max capacity
	start int // index of the oldest line
	len   int // number of lines currently stored

	// recentTexts is a ring of text content for the last dedupWindow pushes,
	// used to evict stale entries from recentSet.
	recentTexts []string
	recentSet   map[string]int // text → count in recentTexts ring
	recentIdx   int            // next write position in recentTexts
}

func newScrollbackBuffer(capacity int) *scrollbackBuffer {
	if capacity <= 0 {
		capacity = defaultScrollbackCapacity
	}
	return &scrollbackBuffer{
		lines:       make([]scrollbackLine, capacity),
		cap:         capacity,
		recentTexts: make([]string, dedupWindow),
		recentSet:   make(map[string]int, dedupWindow),
	}
}

// Push appends a glyph row to the buffer, taking ownership of the slice.
// Rows whose text content matches a recently pushed line are skipped to
// prevent duplicate content from alt-screen TUI repaints.
// When full, the oldest line is overwritten.
func (b *scrollbackBuffer) Push(glyphs scrollbackLine) {
	text := glyphsToText(glyphs)

	// Dedup: skip if this exact text was pushed recently.
	if text != "" {
		if b.recentSet[text] > 0 {
			return
		}
	}

	// Record in dedup ring.
	if text != "" {
		// Evict the old entry at this ring position.
		old := b.recentTexts[b.recentIdx]
		if old != "" {
			if c := b.recentSet[old]; c <= 1 {
				delete(b.recentSet, old)
			} else {
				b.recentSet[old] = c - 1
			}
		}
		b.recentTexts[b.recentIdx] = text
		b.recentSet[text]++
		b.recentIdx = (b.recentIdx + 1) % dedupWindow
	}

	// Append to ring buffer.
	idx := (b.start + b.len) % b.cap
	b.lines[idx] = glyphs
	if b.len == b.cap {
		b.start = (b.start + 1) % b.cap
	} else {
		b.len++
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

// Clear discards all stored lines.
func (b *scrollbackBuffer) Clear() {
	b.start = 0
	b.len = 0
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
