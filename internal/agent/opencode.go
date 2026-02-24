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
// Strategy: scan from the right for the first column where a vertical
// box-drawing character (│) appears on a majority of lines — that column
// is the sidebar divider. Crop every line to that boundary.
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

	// Scan from the right (within the rightmost 40% of the screen) looking
	// for the first column where │ appears on ≥50% of non-empty lines.
	threshold := len(lines) / 2
	if threshold < 3 {
		threshold = 3
	}
	startCol := maxWidth * 60 / 100 // only look in the right 40%

	dividerCol := -1
	for col := maxWidth - 1; col >= startCol; col-- {
		count := 0
		for _, rl := range runeLines {
			if col < len(rl) && isVerticalBorder(rl[col]) {
				count++
			}
		}
		if count >= threshold {
			dividerCol = col
			break
		}
	}

	if dividerCol < 0 {
		// No sidebar detected — return lines unchanged.
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

// isVerticalBorder returns true for vertical box-drawing characters used as
// panel dividers.
func isVerticalBorder(r rune) bool {
	switch r {
	case '│', '┃', '║', '╎', '╏', '┆', '┇', '┊', '┋':
		return true
	}
	return false
}
