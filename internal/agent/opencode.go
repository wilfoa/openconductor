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

// ChromeSkipRows returns the number of fixed TUI chrome rows at the top and
// bottom of OpenCode's alt-screen layout. Row 0 is the header bar, and the
// last 2 rows are the status bar and footer shortcuts.
func (a *opencodeAdapter) ChromeSkipRows() (top int, bottom int) {
	return 1, 2
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
	hasQuestionDialog := false

	// Scan all visible lines (not just a few) because the permission
	// and question dialogs span multiple rows and header text may appear
	// higher up the screen while the button row is at the bottom.
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
		// Model selector footer visible when idle: "Build  Claude Opus 4.6 Anthropic · max".
		// The middle-dot separator + provider name is unique to the model selector
		// and does not appear during working state (which shows "esc interrupt").
		if isModelSelectorLine(lower) {
			hasIdleShortcuts = true
		}
		if strings.Contains(lower, "permission required") {
			hasPermissionRequired = true
		}
		// "Allow once" appears in the button row of the permission dialog.
		if strings.Contains(lower, "allow once") || strings.Contains(lower, "allow always") {
			hasAllowOnce = true
		}
		// "enter submit  esc dismiss" or "enter confirm  esc dismiss" is
		// the footer of OpenCode's question/selection dialog. Match either
		// variant paired with "esc dismiss".
		if (strings.Contains(lower, "enter submit") || strings.Contains(lower, "enter confirm")) && strings.Contains(lower, "esc dismiss") {
			hasQuestionDialog = true
		}
	}

	logging.Debug("heuristic: opencode scan result",
		"escInterrupt", hasEscInterrupt,
		"idleShortcuts", hasIdleShortcuts,
		"permissionRequired", hasPermissionRequired,
		"allowOnce", hasAllowOnce,
		"questionDialog", hasQuestionDialog,
	)

	// Permission and question dialogs take priority over the working
	// signal. When OpenCode renders a modal overlay it only redraws the
	// dialog cells — the underlying "esc interrupt" progress text can
	// remain in the vt10x buffer. The agent cannot continue until the
	// user responds, so these must win.
	if hasPermissionRequired || hasAllowOnce {
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

	if hasEscInterrupt {
		// Agent is actively working — suppress generic patterns.
		return attention.Working, nil
	}

	if hasIdleShortcuts {
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

// isModelSelectorLine returns true if the line matches OpenCode's idle-state
// model selector footer. The format is:
//
//	<mode>  <model_name> <provider> · <setting>
//
// We match by looking for a known provider name followed by " · " on the
// same line. This pattern does not appear during working state (which shows
// "esc interrupt") or in modal dialogs.
func isModelSelectorLine(lower string) bool {
	// The middle-dot separator " · " (U+00B7) is the key distinguisher.
	if !strings.Contains(lower, " · ") {
		return false
	}
	for _, p := range knownProviders {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
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
	// OpenCode stores absolute paths in session.directory.
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		absRepo = repoPath
	}

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
