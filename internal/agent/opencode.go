// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/openconductorhq/openconductor/internal/config"
)

// opencodeAdapter implements AgentAdapter for the OpenCode CLI.
type opencodeAdapter struct{}

func init() {
	Register(&opencodeAdapter{})
}

// Type returns config.AgentOpenCode.
func (a *opencodeAdapter) Type() config.AgentType {
	return config.AgentOpenCode
}

// Command returns an *exec.Cmd that launches the "opencode" CLI in the given
// repo directory. The --continue flag resumes the previous conversation.
func (a *opencodeAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	cmd := exec.Command("opencode", "--continue")
	cmd.Dir = repoPath
	return cmd
}

// ApproveKeystroke returns "a" — OpenCode uses single-key permission dialog.
func (a *opencodeAdapter) ApproveKeystroke() []byte { return []byte("a") }

// ApproveSessionKeystroke returns "A" — OpenCode supports session-wide approval.
func (a *opencodeAdapter) ApproveSessionKeystroke() []byte { return []byte("A") }

// DenyKeystroke returns "d".
func (a *opencodeAdapter) DenyKeystroke() []byte { return []byte("d") }

// BootstrapFiles returns no bootstrap files for OpenCode.
func (a *opencodeAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}

// FilterScreen extracts the conversation panel from the OpenCode TUI by
// detecting and removing the right sidebar (Context, MCP, LSP, Todo panels).
//
// Two detection strategies are tried in order:
//  1. Vertical border: scan for │/┃ characters appearing on ≥50% of lines.
//  2. Whitespace gap: find a sustained drop in per-column text density
//     (the gap between content and sidebar) followed by a rise (sidebar).
//
// Crop every line to the detected boundary.
func (a *opencodeAdapter) FilterScreen(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}

	// Find the width of the widest line (in runes, not bytes).
	maxWidth := 0
	for _, line := range lines {
		if w := utf8.RuneCountInString(line); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth == 0 {
		return lines
	}

	// Convert lines to rune slices for column-level scanning.
	runeLines := make([][]rune, len(lines))
	for i, line := range lines {
		runeLines[i] = []rune(line)
	}

	// Strategy 1: vertical border character.
	dividerCol := findVerticalBorder(runeLines, maxWidth, len(lines))

	// Strategy 2: whitespace gap between content and sidebar.
	if dividerCol < 0 {
		dividerCol = findContentGap(runeLines, maxWidth, len(lines))
	}

	if dividerCol < 0 {
		return lines
	}

	// Crop each line to the divider column (exclusive).
	result := make([]string, len(lines))
	for i, rl := range runeLines {
		if dividerCol < len(rl) {
			result[i] = strings.TrimRight(string(rl[:dividerCol]), " ")
		} else {
			result[i] = strings.TrimRight(string(rl), " ")
		}
	}
	return result
}

// findVerticalBorder scans from the right (within the rightmost 40% of the
// screen) for a column where a vertical box-drawing character appears on
// ≥50% of lines. Returns the column index, or -1 if not found.
func findVerticalBorder(runeLines [][]rune, maxWidth, lineCount int) int {
	threshold := lineCount / 2
	if threshold < 3 {
		threshold = 3
	}
	startCol := maxWidth * 60 / 100

	for col := maxWidth - 1; col >= startCol; col-- {
		count := 0
		for _, rl := range runeLines {
			if col < len(rl) && isVerticalBorder(rl[col]) {
				count++
			}
		}
		if count >= threshold {
			return col
		}
	}
	return -1
}

// findContentGap detects the sidebar by looking for a sustained drop in
// per-column text density (a "gap" of whitespace columns) followed by a
// rise (sidebar content). This handles layouts where the sidebar has no
// visible vertical border — just a wide space gap.
//
// Returns the column at the start of the gap (the crop point), or -1.
func findContentGap(runeLines [][]rune, maxWidth, lineCount int) int {
	const (
		minGapWidth      = 8 // gap must be at least this many columns wide
		gapDensityCeil   = 2 // gap columns have at most this many lines with content
		sidebarDensityLo = 3 // sidebar must have content on at least this many lines
	)

	// Count non-blank lines.
	nonBlank := 0
	for _, rl := range runeLines {
		for _, r := range rl {
			if r != ' ' && r != 0 {
				nonBlank++
				break
			}
		}
	}
	if nonBlank < 4 {
		return -1
	}

	// Build per-column density (number of lines with a non-space character).
	density := make([]int, maxWidth)
	for _, rl := range runeLines {
		for col := 0; col < len(rl) && col < maxWidth; col++ {
			if rl[col] != ' ' && rl[col] != 0 {
				density[col]++
			}
		}
	}

	// Scan from the 40% mark rightward looking for: content → gap → sidebar.
	startSearch := maxWidth * 40 / 100

	gapStart := -1
	gapLen := 0
	for col := startSearch; col < maxWidth; col++ {
		if density[col] <= gapDensityCeil {
			if gapStart < 0 {
				gapStart = col
			}
			gapLen++
		} else {
			// Hit content after a gap — is this the sidebar?
			if gapLen >= minGapWidth && density[col] >= sidebarDensityLo {
				return gapStart
			}
			gapStart = -1
			gapLen = 0
		}
	}

	return -1
}

// isVerticalBorder returns true for vertical box-drawing characters used as
// panel dividers.
func isVerticalBorder(r rune) bool {
	switch r {
	case '│', '┃', '║', '╎', '╏', '┆', '┇', '┊', '┋':
		return true
	}
	return false
}
