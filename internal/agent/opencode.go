// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
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
// repo directory. When opts.Continue is true, --continue is passed to resume
// the previous conversation (used for restored tabs and first tab for a project).
func (a *opencodeAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	args := []string{}
	if opts.Continue {
		args = append(args, "--continue")
	}
	cmd := exec.Command("opencode", args...)
	cmd.Dir = strings.TrimRight(repoPath, "/")
	return cmd
}

// ApproveKeystroke sends Enter to confirm "Allow once" — the first (default-
// selected) option in OpenCode's permission dialog. The dialog is a selection-
// based widget: arrow keys navigate, Enter confirms.
func (a *opencodeAdapter) ApproveKeystroke() []byte { return []byte("\r") }

// ApproveSessionKeystroke navigates to "Allow always" (second option) and
// confirms. Right arrow moves from the default "Allow once" to "Allow always".
// Enter is sent separately by the handler after a SubmitDelay pause so the
// dialog processes the navigation before confirmation.
func (a *opencodeAdapter) ApproveSessionKeystroke() []byte { return []byte("\x1b[C") }

// DenyKeystroke navigates to "Reject" (third option) and confirms.
// Two right arrows move from "Allow once" past "Allow always" to "Reject".
// Enter is sent separately by the handler after a SubmitDelay pause.
func (a *opencodeAdapter) DenyKeystroke() []byte { return []byte("\x1b[C\x1b[C") }

// QuestionKeystroke returns down-arrow sequences to navigate OpenCode's
// vertical question dialog to the given option. Option 1 is already selected
// by default, so no navigation is needed (returns nil → caller sends Enter).
// For option N, (N-1) down arrows are sent. Enter is appended separately
// by the handler after SubmitDelay.
func (a *opencodeAdapter) QuestionKeystroke(optionNum int) []byte {
	if optionNum <= 1 {
		return nil
	}
	ks := make([]byte, 0, (optionNum-1)*3)
	for i := 1; i < optionNum; i++ {
		ks = append(ks, '\x1b', '[', 'B') // Down arrow: ESC [ B
	}
	return ks
}

// BootstrapFiles returns no bootstrap files for OpenCode.
func (a *opencodeAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}

// ChromeSkipRows returns the number of fixed TUI chrome rows at the top and
// bottom of OpenCode's alt-screen layout. Row 0 is the header bar, and the
// last 2 rows are the status bar and footer shortcuts.
func (a *opencodeAdapter) ChromeSkipRows() (top int, bottom int) {
	return 1, 2
}

// IsChromeLine returns true if the line is OpenCode TUI chrome that should
// be stripped from Telegram messages. This catches chrome that isn't covered
// by the fixed ChromeSkipRows values — specifically the status bar and model
// selector lines whose position shifts depending on the agent's state.
//
// Status bar format:   ▣  Build · claude-opus-4-6 · 22.6s
// Model selector:      Build  Claude Opus 4.6 Anthropic · max
// Shortcut hints:      ctrl+t variants  tab agents  ctrl+p commands
func (a *opencodeAdapter) IsChromeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(collapseSpaces(trimmed))

	// Status bar: starts with ▣ (U+25A3) followed by mode · model · time.
	if strings.HasPrefix(trimmed, "▣") {
		return true
	}
	// Status bar can also start with ▢ (U+25A2) or ■ (U+25A0) depending
	// on OpenCode version/theme.
	if strings.HasPrefix(trimmed, "▢") || strings.HasPrefix(trimmed, "■") {
		return true
	}

	// Model selector line: <mode> <model_name> <provider> [· <setting>]
	// Only when not part of a dialog (no "esc interrupt", "esc dismiss").
	if !strings.Contains(lower, "esc interrupt") &&
		!strings.Contains(lower, "esc dismiss") &&
		isModelSelectorLine(lower) {
		return true
	}

	// Idle shortcut hints: "ctrl+p commands", "ctrl+t variants", "tab agents".
	if strings.Contains(lower, "ctrl+p commands") ||
		strings.Contains(lower, "ctrl+t variants") ||
		strings.Contains(lower, "tab agents") {
		return true
	}

	// Working state footer: "esc interrupt" progress line.
	if strings.Contains(lower, "esc interrupt") {
		return true
	}

	return false
}

// FormatImageInput returns a prompt message for an image sent via Telegram.
// OpenCode passes the prompt text directly to the LLM which can reference the
// file path for context.
func (a *opencodeAdapter) FormatImageInput(imagePath string, caption string) string {
	if caption != "" {
		return caption + "\n\n[Image attached: " + imagePath + "]"
	}
	return "Look at the image at " + imagePath
}

// SubmitDelay returns the pause between writing text and Enter for OpenCode.
// OpenCode's Bubble Tea event loop needs ~50ms to process the input text
// before receiving the submit key.
func (a *opencodeAdapter) SubmitDelay() time.Duration {
	return 50 * time.Millisecond
}

// CheckAttention detects OpenCode's working/idle/permission/question state
// from its terminal output.
//
// Working: OpenCode shows a progress bar with "esc interrupt" at the bottom
// while an LLM is actively generating a response.
//
// Permission dialog: OpenCode overlays a modal with the header
// "Permission required" (with a ⚠ prefix) and buttons
// "Allow once  Allow always  Reject" at the bottom. The action being
// requested appears as "← <Action> <detail>" inside the modal.
// Example screenshot content:
//
//	⚠ Permission required
//	← Access external directory ~/Downloads/...
//	Patterns
//	- /path/to/glob/*
//	Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm
//
// Question dialog: OpenCode shows a multi-option question modal when the agent
// needs a decision from the user. The footer is uniquely:
//
//	↕ select  enter submit  esc dismiss
//
// Idle/done: The progress bar disappears and the bottom shows either
// keyboard shortcuts like "ctrl+t variants  tab agents  ctrl+p commands"
// or the model selector "Build  Claude Opus 4.6 Anthropic · max"
// without "esc interrupt". This means the agent has finished and is
// waiting for the user's next prompt.
func (a *opencodeAdapter) CheckAttention(lastLines []string) (attention.HeuristicResult, *attention.AttentionEvent) {
	hasEscInterrupt := false
	hasIdleShortcuts := false
	hasPermissionRequired := false
	hasAllowOnce := false
	hasReject := false
	hasQuestionDialog := false
	hasFullscreen := false      // "ctrl+f fullscreen" appears in permission dialog footer
	hasConfirmTab := false      // question series "Confirm" review tab
	hasAlwaysAllowText := false // "Always allow" text in dialog header
	hasConfirmCancel := false   // "Confirm" + "Cancel" button pair

	// TUI chrome signals (dialog buttons, status bar, keyboard shortcuts)
	// always appear in the bottom portion of the screen. We restrict most
	// signal detection to the bottom zone to prevent false positives when
	// conversation content discusses these keywords — e.g. an AI agent
	// explaining OpenCode's UI will mention "esc interrupt", "Allow once",
	// "enter submit  esc dismiss" etc. in flowing text.
	//
	// The only exception is "Permission required" which is a dialog header
	// that may appear higher up. It requires co-occurrence with button
	// signals from the bottom zone to trigger a detection.
	const maxChromeScanRows = 12
	bottomBound := len(lastLines) - maxChromeScanRows

	for i := len(lastLines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lastLines[i])
		if trimmed == "" {
			continue
		}

		// Collapse runs of whitespace to single spaces so VT grid padding
		// between button labels doesn't break substring matching.
		// e.g. "Allow once    Allow always    Reject" → "allow once allow always reject"
		lower := strings.ToLower(collapseSpaces(trimmed))

		// "Permission required" header — may be prefixed with Unicode symbols
		// (△ U+25B3, ⚠ U+26A0) that the VT reader splits into separate cells.
		// Scanned across all rows because the header can appear in the
		// middle of the dialog overlay. Alone it is not sufficient — it
		// must co-occur with button signals from the bottom zone.
		if strings.Contains(lower, "permission required") {
			hasPermissionRequired = true
		}

		// All remaining signals are TUI chrome or dialog footer elements
		// that live in the bottom portion of the screen. Skip lines above
		// the bottom zone to avoid false matches on conversation content.
		if i < bottomBound {
			continue
		}

		if strings.Contains(lower, "esc interrupt") {
			hasEscInterrupt = true
		}
		if strings.Contains(lower, "ctrl+p commands") || strings.Contains(lower, "ctrl+t variants") {
			hasIdleShortcuts = true
		}
		// Model selector footer visible when idle: "Build · Claude Opus 4.6 Anthropic · max".
		// Only match when the line does NOT contain "esc interrupt" — the model
		// selector row is always visible but should only signal idle when the
		// agent is not working.
		if !strings.Contains(lower, "esc interrupt") && isModelSelectorLine(lower) {
			hasIdleShortcuts = true
		}
		// "Allow once" / "Allow always" in the button row.
		if strings.Contains(lower, "allow once") || strings.Contains(lower, "allow always") {
			hasAllowOnce = true
		}
		// "Reject" button alongside Allow buttons.
		if strings.Contains(lower, "reject") {
			hasReject = true
		}
		// "ctrl+f fullscreen" appears only in the permission dialog footer.
		if strings.Contains(lower, "ctrl+f fullscreen") {
			hasFullscreen = true
		}
		// Question dialog footer: "↕ select  enter submit  esc dismiss".
		// Require "select" alongside ("enter submit"|"enter confirm") and
		// "esc dismiss" all on the same line — this three-keyword match
		// avoids false positives from conversation text that might mention
		// a subset of these keywords.
		if strings.Contains(lower, "select") &&
			(strings.Contains(lower, "enter submit") || strings.Contains(lower, "enter confirm")) &&
			strings.Contains(lower, "esc dismiss") {
			hasQuestionDialog = true
		}
		// Question series Confirm tab footer: "⇆ tab  enter submit  esc dismiss".
		// This appears after all questions in a series have been answered.
		// Unlike intermediate question tabs, it has NO "select" — the
		// Confirm tab shows a review screen, not selectable options.
		if strings.Contains(lower, "tab") &&
			strings.Contains(lower, "enter submit") &&
			strings.Contains(lower, "esc dismiss") &&
			!strings.Contains(lower, "select") {
			hasConfirmTab = true
		}
		// "Always allow" second-stage confirmation dialog. After selecting
		// "Allow always" on a permission, OpenCode shows:
		//   △ Always allow
		//   This will allow read until OpenCode is restarted.
		//   [Confirm]  Cancel          ⇆ select  enter confirm
		// "Confirm" is already highlighted — just press Enter.
		// Require both "always allow" text AND "confirm"+"cancel" buttons
		// to avoid false positives from conversation content.
		if strings.Contains(lower, "always allow") {
			hasAlwaysAllowText = true
		}
		if strings.Contains(lower, "confirm") && strings.Contains(lower, "cancel") {
			hasConfirmCancel = true
		}
	}

	logging.Debug("heuristic: opencode scan result",
		"escInterrupt", hasEscInterrupt,
		"idleShortcuts", hasIdleShortcuts,
		"permissionRequired", hasPermissionRequired,
		"allowOnce", hasAllowOnce,
		"reject", hasReject,
		"fullscreen", hasFullscreen,
		"questionDialog", hasQuestionDialog,
		"confirmTab", hasConfirmTab,
		"alwaysAllowText", hasAlwaysAllowText,
		"confirmCancel", hasConfirmCancel,
	)

	// "Always allow" second-stage confirmation — auto-confirm by pressing
	// Enter. This dialog appears after the user (or auto-approve) selected
	// "Allow always" on a permission. "Confirm" is already highlighted.
	// Handled before regular permission detection so it doesn't get stuck
	// waiting for user input.
	if hasAlwaysAllowText && hasConfirmCancel {
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsPermission,
			Detail: "opencode always-allow auto-confirm",
			Source: "heuristic",
		}
	}

	// Permission and question dialogs take priority over the working
	// signal. When OpenCode renders a modal overlay it only redraws the
	// dialog cells — the underlying "esc interrupt" progress text can
	// remain in the vt10x buffer. The agent cannot continue until the
	// user responds, so these must win.
	//
	// Permission dialog detection — require at least TWO signals to avoid
	// false positives when conversation content mentions individual keywords
	// (e.g. discussing "Allow once" or "Permission required" in text):
	//   - header + any button signal
	//   - two different button signals
	isPermission := hasPermissionRequired && (hasAllowOnce || hasReject || hasFullscreen) ||
		hasAllowOnce && hasReject ||
		hasFullscreen && hasReject
	if isPermission {
		// Permission dialog is visible.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsPermission,
			Detail: "opencode permission dialog detected",
			Source: "heuristic",
		}
	}

	if hasQuestionDialog {
		// Question dialog is visible — agent is asking for a decision.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsAnswer,
			Detail: "opencode question dialog detected",
			Source: "heuristic",
		}
	}

	if hasConfirmTab {
		// Question series Confirm tab — all questions answered, review screen
		// is showing. The user already provided answers via Telegram buttons;
		// the attention loop auto-submits this by pressing Enter.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsAnswer,
			Detail: "opencode question confirm tab",
			Source: "heuristic",
		}
	}

	if hasEscInterrupt {
		// Agent is actively working — suppress generic patterns.
		return attention.Working, nil
	}

	if hasIdleShortcuts {
		// Agent is idle. Check if the conversation contains free-text
		// questions that the agent is waiting for the user to answer.
		// This is a "soft" question — the agent printed questions in its
		// response text (not a dialog overlay) and is now idle at the
		// prompt. Examples: "Questions for You", numbered items ending
		// with "?", etc.
		if hasSoftQuestions(lastLines, bottomBound) {
			return attention.Certain, &attention.AttentionEvent{
				Type:   attention.NeedsAnswer,
				Detail: "agent is asking questions (free text)",
				Source: "heuristic",
			}
		}
		// Agent is idle, waiting for user input.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsInput,
			Detail: "opencode is idle, waiting for prompt",
			Source: "heuristic",
		}
	}

	return attention.No, nil
}

// knownProviders are lowercase LLM provider names that appear in the
// OpenCode model selector footer (e.g., "Build  Claude Opus 4.6 Anthropic · max").
var knownProviders = []string{
	"anthropic", "openai", "google", "groq", "bedrock",
	"openrouter", "copilot", "local", "vertexai",
}

// knownModes are lowercase mode names that appear at the start of the
// OpenCode model selector line. Matching mode + provider makes the
// detection more robust than provider alone.
var knownModes = []string{
	"build", "plan", "code", "ask", "debug", "edit", "chat",
}

// isModelSelectorLine returns true if the line matches OpenCode's idle-state
// model selector footer. The format is:
//
//	<mode>  <model_name> <provider> [· <setting>]
//
// We require a known mode name AND a known provider name on the same line.
// The middle-dot separator " · " (U+00B7) may or may not be present
// depending on the OpenCode version.
//
// IMPORTANT: The caller must exclude lines that contain "esc interrupt",
// because the model selector row is always visible even during working state.
func isModelSelectorLine(lower string) bool {
	hasProvider := false
	for _, p := range knownProviders {
		if strings.Contains(lower, p) {
			hasProvider = true
			break
		}
	}
	if !hasProvider {
		return false
	}
	for _, m := range knownModes {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// collapseSpaces replaces runs of consecutive spaces with a single space.
// This normalises VT grid-padded text (e.g. button rows where labels are
// separated by many spaces) so substring matching works reliably.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prev := false
	for _, r := range s {
		if r == ' ' {
			if !prev {
				b.WriteRune(r)
			}
			prev = true
		} else {
			b.WriteRune(r)
			prev = false
		}
	}
	return b.String()
}

// softQuestionHeadings are patterns that agents use when asking the user
// questions in conversation text (not via a dialog overlay). These are
// "soft" questions — the agent printed them in its response and is now idle,
// waiting for a free-text reply.
var softQuestionHeadings = []string{
	"questions for you",
	"i have a few questions",
	"i have some questions",
	"before i begin, a few questions",
	"before i proceed, a few questions",
	"before i start, a few questions",
	"a few questions",
	"some questions",
}

// hasSoftQuestions scans the conversation area (above the chrome zone) for
// question patterns. Returns true if the agent appears to be asking the user
// free-text questions. Two detection strategies:
//
//  1. Heading match: a known question heading (e.g. "Questions for You")
//     appears in the conversation area.
//  2. Question density: 3+ lines ending with "?" in the conversation area
//     (heuristic for numbered questions without a known heading).
func hasSoftQuestions(lines []string, bottomBound int) bool {
	// Only scan the conversation area — lines above the bottom chrome zone.
	limit := len(lines)
	if bottomBound > 0 && bottomBound < limit {
		limit = bottomBound
	}

	questionLineCount := 0

	for i := 0; i < limit; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Strategy 1: known question heading.
		for _, heading := range softQuestionHeadings {
			if strings.Contains(lower, heading) {
				return true
			}
		}

		// Strategy 2: count lines ending with "?".
		if strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "?)") {
			questionLineCount++
		}
	}

	// 3+ question-ending lines is a strong signal.
	return questionLineCount >= 3
}

// FilterScreen extracts the conversation panel from the OpenCode TUI by
// detecting and removing the right sidebar (Context, MCP, LSP, Todo panels).
//
// Three detection strategies are tried in order:
//  1. Vertical border: scan for │/┃/█/■ characters appearing on ≥50% of lines.
//  2. Whitespace gap: find a sustained drop in per-column text density
//     (the gap between content and sidebar) followed by a rise (sidebar).
//  3. Sidebar content: detect known sidebar patterns at consistent column
//     offsets when the boundary has no visible separator.
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

	// Strategy 1: vertical border or scrollbar character.
	dividerCol := findVerticalBorder(runeLines, maxWidth, len(lines))
	strategy := "border"

	// Strategy 2: whitespace gap between content and sidebar.
	if dividerCol < 0 {
		dividerCol = findContentGap(runeLines, maxWidth, len(lines))
		strategy = "gap"
	}

	// Strategy 3: detect known sidebar content patterns.
	if dividerCol < 0 {
		dividerCol = findSidebarContent(lines, maxWidth)
		strategy = "content"
	}

	if dividerCol < 0 {
		logging.Debug("filterscreen: no sidebar detected",
			"lines", len(lines),
			"maxWidth", maxWidth,
		)
		return lines
	}

	logging.Debug("filterscreen: sidebar detected",
		"strategy", strategy,
		"dividerCol", dividerCol,
		"maxWidth", maxWidth,
	)

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

// sidebarPatterns are substrings that appear exclusively in OpenCode's right
// sidebar panel. When these are found at consistent column offsets and no
// border/gap was detected, we use the leftmost match offset as the crop point.
var sidebarPatterns = []string{
	"Context",
	"tokens",
	"% used",
	"spent",
	"▼ MCP",
	"▼ Todo",
	"▼ Modified Files",
	"Connected",
	"LSPs will activate",
}

// findSidebarContent detects the sidebar by looking for known sidebar text
// patterns. It finds the leftmost column where a sidebar pattern starts and
// uses that (minus a small margin) as the crop point. Returns -1 if fewer
// than 2 patterns are found or they start in the left half of the screen.
func findSidebarContent(lines []string, maxWidth int) int {
	minCol := maxWidth // leftmost column where a sidebar pattern was found
	matches := 0

	for _, line := range lines {
		for _, pat := range sidebarPatterns {
			idx := strings.Index(line, pat)
			if idx < 0 {
				continue
			}
			// Convert byte offset to rune offset.
			runeIdx := utf8.RuneCountInString(line[:idx])
			// Only count if in the right 50% of the screen.
			if runeIdx >= maxWidth/2 {
				matches++
				if runeIdx < minCol {
					minCol = runeIdx
				}
			}
		}
	}

	// Need at least 2 sidebar patterns to be confident.
	if matches < 2 || minCol >= maxWidth {
		return -1
	}

	// Crop 1 column before the first sidebar pattern to remove any
	// separator space.
	if minCol > 0 {
		minCol--
	}
	return minCol
}

// isVerticalBorder returns true for characters commonly used as panel dividers
// or scrollbars in terminal UIs. This includes box-drawing verticals, block
// elements (used for scrollbar tracks/thumbs), and geometric shapes (used for
// scrollbar indicators in TUI frameworks like OpenTUI).
func isVerticalBorder(r rune) bool {
	// Box-drawing vertical characters.
	switch r {
	case '│', '┃', '║', '╎', '╏', '┆', '┇', '┊', '┋':
		return true
	}
	// U+2580–U+259F: Block Elements (▀▁▂▃▄▅▆▇█▉▊▋▌▍▎▏▐░▒▓).
	// Used for scrollbar tracks (░▒) and thumbs (█▓▌▐).
	if r >= 0x2580 && r <= 0x259F {
		return true
	}
	// Geometric shapes commonly used as scrollbar indicators.
	switch r {
	case '■', '▮': // U+25A0 BLACK SQUARE, U+25AE BLACK VERTICAL RECTANGLE
		return true
	}
	return false
}

// ── History loading ─────────────────────────────────────────────

// openCodeDBPath returns the path to the OpenCode SQLite database.
func openCodeDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

// LoadHistory reads the most recent OpenCode session for the given repo
// directory and returns the conversation as text lines for scrollback
// pre-population.
//
// It shells out to the system sqlite3 CLI (available by default on macOS)
// to avoid adding a Go SQLite dependency.
func (a *opencodeAdapter) LoadHistory(repoPath string) ([]string, error) {
	dbPath := openCodeDBPath()
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil // no DB → no history
	}

	sqlite3, err := exec.LookPath("sqlite3")
	if err != nil {
		logging.Debug("opencode: sqlite3 not found, skipping history load")
		return nil, nil
	}

	// Find the most recent session for this directory.
	// OpenCode stores absolute paths without trailing slashes.
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		absRepo = repoPath
	}
	absRepo = strings.TrimRight(absRepo, "/")

	sessionQuery := `SELECT id FROM session WHERE directory = '` +
		sqliteEscape(absRepo) +
		`' AND parent_id IS NULL ORDER BY time_updated DESC LIMIT 1`

	cmd := exec.Command(sqlite3, "-json", dbPath, sessionQuery)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, nil
	}

	var sessions []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &sessions); err != nil || len(sessions) == 0 {
		return nil, nil
	}
	sessionID := sessions[0].ID

	// Load all text parts for this session, ordered by message and part time.
	partsQuery := `SELECT ` +
		`json_extract(m.data, '$.role') as role, ` +
		`json_extract(p.data, '$.type') as type, ` +
		`json_extract(p.data, '$.text') as text ` +
		`FROM message m JOIN part p ON p.message_id = m.id ` +
		`WHERE m.session_id = '` + sqliteEscape(sessionID) + `' ` +
		`AND json_extract(p.data, '$.type') = 'text' ` +
		`ORDER BY m.time_created, p.time_created`

	cmd = exec.Command(sqlite3, "-json", dbPath, partsQuery)
	out, err = cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, nil
	}

	var parts []struct {
		Role string `json:"role"`
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out, &parts); err != nil {
		logging.Debug("opencode: failed to parse history parts", "err", err)
		return nil, nil
	}

	return formatHistory(parts), nil
}

// historyPart is a text part from the OpenCode DB used by formatHistory.
type historyPart struct {
	Role string
	Text string
}

// formatHistory converts a sequence of role+text parts into readable text
// lines with role headers.
func formatHistory(parts []struct {
	Role string `json:"role"`
	Type string `json:"type"`
	Text string `json:"text"`
}) []string {
	if len(parts) == 0 {
		return nil
	}

	const headerWidth = 60

	var lines []string
	lastRole := ""

	for _, p := range parts {
		if p.Text == "" {
			continue
		}

		// Insert a role header when the role changes.
		if p.Role != lastRole {
			if len(lines) > 0 {
				lines = append(lines, "") // blank separator
			}
			label := roleLabel(p.Role)
			pad := headerWidth - len(label) - 4 // "── " + label + " " + padding
			if pad < 2 {
				pad = 2
			}
			header := "── " + label + " " + strings.Repeat("─", pad)
			lines = append(lines, header)
			lastRole = p.Role
		}

		// Wrap the text content into lines.
		for _, line := range strings.Split(p.Text, "\n") {
			lines = append(lines, line)
		}
	}

	return lines
}

// roleLabel returns a human-readable label for a message role.
func roleLabel(role string) string {
	switch role {
	case "user":
		return "You"
	case "assistant":
		return "Assistant"
	case "system":
		return "System"
	default:
		return role
	}
}

// sqliteEscape escapes single quotes for use in SQLite string literals.
func sqliteEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
