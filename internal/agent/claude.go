// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
)

// claudeAdapter implements AgentAdapter for the Claude Code CLI.
type claudeAdapter struct{}

func init() {
	Register(&claudeAdapter{})
}

// Type returns config.AgentClaudeCode.
func (a *claudeAdapter) Type() config.AgentType {
	return config.AgentClaudeCode
}

// Command returns an *exec.Cmd that launches the "claude" CLI in the given
// repo directory. Supports --continue to resume the last conversation and
// --prompt to send an initial message.
func (a *claudeAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	args := []string{}
	if opts.Continue {
		args = append(args, "--continue")
	}
	if opts.Prompt != "" {
		args = append(args, "--prompt", opts.Prompt)
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = repoPath
	return cmd
}

// ApproveKeystroke returns "y\n" — Claude Code uses y/n prompts.
func (a *claudeAdapter) ApproveKeystroke() []byte { return []byte("y\n") }

// ApproveSessionKeystroke returns nil — Claude Code has no session-wide approval.
func (a *claudeAdapter) ApproveSessionKeystroke() []byte { return nil }

// DenyKeystroke returns "n\n".
func (a *claudeAdapter) DenyKeystroke() []byte { return []byte("n\n") }

// maxScanLines limits how many non-empty lines from the bottom of the
// screen we inspect. Scanning too far up risks false positives from
// stale output.
const maxScanLines = 5

// claudePermissionPatterns match inline permission prompts that Claude Code
// renders when requesting approval for tool use, file edits, or bash commands.
// These are compiled once at package init and reused for every check.
var claudePermissionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\(y/n\)`),               // "(y/n)"
	regexp.MustCompile(`\[y/n\]`),               // "[y/n]"
	regexp.MustCompile(`\(yes/no\)`),            // "(yes/no)"
	regexp.MustCompile(`\[yes/no\]`),            // "[yes/no]"
	regexp.MustCompile(`(?i)\bproceed\?\s*$`),   // "Do you want to proceed?"
	regexp.MustCompile(`(?i)\ballow\b.*\?\s*$`), // "Allow running bash command: git status?"
}

// CheckAttention detects Claude Code's working/idle/permission state from its
// terminal output.
//
// The priority order is:
//
//  1. Working (spinner) — highest priority, agent is actively processing.
//  2. Permission prompt — agent is blocked waiting for y/n approval.
//  3. Idle (prompt) — agent is waiting for the next user message.
//  4. No signal — nothing detected.
//
// Working: Claude Code shows an animated spinner line like "✦ Sublimating…"
// or "· Sublimating…" — the prefix alternates between ✦ (U+2726) and ·
// (U+00B7) while a verb + ellipsis describes the current activity.
//
// Permission: Claude Code shows inline permission prompts such as
// "Do you want to proceed? (y/n)" or "Allow running bash command: git status?"
// When detected, the agent needs user approval before it can continue.
//
// Idle: When neither spinner nor permission is found, Claude Code shows "> "
// as its input prompt. The absence of a spinner + presence of "> " is a
// definitive signal that the agent is waiting for input.
func (a *claudeAdapter) CheckAttention(lastLines []string) (attention.HeuristicResult, *attention.AttentionEvent) {
	hasSpinner := false
	hasPermission := false
	hasPrompt := false
	scanned := 0

	for i := len(lastLines) - 1; i >= 0 && scanned < maxScanLines; i-- {
		trimmed := strings.TrimSpace(lastLines[i])
		if trimmed == "" {
			continue
		}
		scanned++

		// Spinner detection — breaks immediately. If the agent is spinning,
		// any permission text visible is stale (left over in the vt10x buffer
		// from a prior prompt that was already answered).
		if isClaudeCodeSpinner(trimmed) {
			hasSpinner = true
			logging.Debug("heuristic: claude-code spinner detected",
				"line", trimmed,
			)
			break
		}

		// Permission detection — check every scanned line against all
		// permission patterns. Does not break; we continue scanning to
		// also look for prompt signals.
		if !hasPermission {
			for _, re := range claudePermissionPatterns {
				if re.MatchString(trimmed) {
					hasPermission = true
					logging.Debug("heuristic: claude-code permission detected",
						"line", trimmed,
						"pattern", re.String(),
					)
					break
				}
			}
		}

		// Claude Code's prompt: line ends with "> " (with the trailing
		// space) or the trimmed content is exactly ">".
		if !hasPrompt && (strings.HasSuffix(lastLines[i], "> ") || trimmed == ">") {
			hasPrompt = true
			logging.Debug("heuristic: claude-code prompt detected",
				"line", trimmed,
			)
		}
	}

	// Priority: Spinner > Permission > Prompt > Nothing.
	if hasSpinner {
		// Agent is actively working — suppress everything else.
		return attention.Working, nil
	}

	if hasPermission {
		// Agent is blocked waiting for permission approval.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsPermission,
			Detail: "claude code permission prompt detected",
			Source: "heuristic",
		}
	}

	if hasPrompt {
		// No spinner, no permission + prompt visible = agent is idle.
		return attention.Certain, &attention.AttentionEvent{
			Type:   attention.NeedsInput,
			Detail: "claude code is idle, waiting for prompt",
			Source: "heuristic",
		}
	}

	logging.Debug("heuristic: claude-code no signal",
		"scanned", scanned,
	)
	return attention.No, nil
}

// isClaudeCodeSpinner returns true if the line matches Claude Code's animated
// status pattern: a prefix character (✦ or ·) followed by a space and a
// capitalized verb ending in "…".
//
// Examples:
//
//	"✦ Sublimating…"  → true
//	"· Thinking…"     → true
//	"· some output"   → false (no trailing …)
//	"normal text"     → false (no prefix)
func isClaudeCodeSpinner(line string) bool {
	// Check for ✦ (U+2726 four-pointed star) prefix.
	if rest, ok := strings.CutPrefix(line, "✦ "); ok {
		return isVerbEllipsis(rest)
	}
	// Check for · (U+00B7 middle dot) prefix.
	if rest, ok := strings.CutPrefix(line, "· "); ok {
		return isVerbEllipsis(rest)
	}
	// Check for * (ASCII asterisk, sometimes seen in plain captures).
	if rest, ok := strings.CutPrefix(line, "* "); ok {
		return isVerbEllipsis(rest)
	}
	return false
}

// isVerbEllipsis returns true if s starts with an uppercase letter and ends
// with "…" (U+2026 horizontal ellipsis) or "..." (three ASCII dots).
func isVerbEllipsis(s string) bool {
	if s == "" {
		return false
	}
	runes := []rune(s)
	if !unicode.IsUpper(runes[0]) {
		return false
	}
	return strings.HasSuffix(s, "…") || strings.HasSuffix(s, "...")
}

// BootstrapFiles returns a placeholder CLAUDE.md for the repository.
func (a *claudeAdapter) BootstrapFiles() []BootstrapFile {
	return []BootstrapFile{
		{
			Path: "CLAUDE.md",
			Content: []byte(`# CLAUDE.md — Project Context for Claude Code

## Overview
This file provides context to Claude Code about the project.

## Guidelines
- Follow existing code style and conventions
- Write tests for new functionality
- Keep changes focused and minimal
`),
		},
	}
}

// ── OutputFilter ────────────────────────────────────────────────────────────

// NewOutputFilter returns a per-session filter that strips CSI escape sequences
// containing prefix bytes (>, <, =) that vt10x cannot parse. Without this
// filter, sequences like CSI > 1 u (kitty keyboard protocol) are misparsed as
// CSI u (cursor restore), teleporting the cursor to position (0,0).
//
// The returned function is stateful — it tracks partial escape sequences that
// may span PTY read boundaries — and must only be used by a single session.
func (a *claudeAdapter) NewOutputFilter() func([]byte) []byte {
	f := &csiFilter{}
	return f.filter
}

// csiFilter strips CSI sequences with extended prefix bytes (>, <, =) from
// raw terminal output. These prefix bytes are part of ECMA-48 but vt10x only
// handles the ? prefix, causing sequences with other prefixes to be misparsed.
//
// CSI sequence format: ESC [ (prefix)? (params)* (intermediate)* (final)
//   - Prefix bytes:       0x3C-0x3F (includes < = > ?)
//   - Parameter bytes:    0x30-0x3F (digits, semicolons, etc.)
//   - Intermediate bytes: 0x20-0x2F
//   - Final byte:         0x40-0x7E
type csiFilter struct {
	state   int    // parser state
	pending []byte // buffered bytes from an incomplete ESC or ESC [ at chunk end
}

// Parser states.
const (
	csiStateNormal = iota // passing through bytes unchanged
	csiStateEsc           // saw ESC, waiting for next byte
	csiStateCSI           // saw ESC [, need to check for prefix byte
	csiStateSkip          // inside an extended CSI, discarding until final byte
)

// filter processes a chunk of raw PTY data and returns the data with extended
// CSI sequences removed. It maintains state across calls to handle sequences
// that span chunk boundaries.
func (f *csiFilter) filter(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// Fast path: if no ESC in the data and we're in normal state, return as-is.
	if f.state == csiStateNormal && !containsByte(data, 0x1B) {
		return data
	}

	out := make([]byte, 0, len(data))

	for _, b := range data {
		switch f.state {
		case csiStateNormal:
			if b == 0x1B {
				// Start of a potential escape sequence. Buffer the ESC
				// in case we need to pass it through (normal CSI).
				f.pending = append(f.pending[:0], b)
				f.state = csiStateEsc
			} else {
				out = append(out, b)
			}

		case csiStateEsc:
			if b == '[' {
				// ESC [ — could be a CSI we need to filter.
				f.pending = append(f.pending, b)
				f.state = csiStateCSI
			} else {
				// Not a CSI (e.g. ESC O for SS3, ESC ] for OSC).
				// Flush the buffered ESC and this byte.
				out = append(out, f.pending...)
				out = append(out, b)
				f.pending = f.pending[:0]
				f.state = csiStateNormal
			}

		case csiStateCSI:
			// We've seen ESC [. Check if the next byte is an extended prefix.
			if b == '>' || b == '<' || b == '=' {
				// Extended CSI — discard the entire sequence.
				f.pending = f.pending[:0]
				f.state = csiStateSkip
			} else {
				// Normal CSI (no prefix, or ? prefix which vt10x handles).
				// Flush the buffered ESC [ and this byte.
				out = append(out, f.pending...)
				out = append(out, b)
				f.pending = f.pending[:0]
				f.state = csiStateNormal
			}

		case csiStateSkip:
			// Inside an extended CSI. Discard bytes until the final byte.
			if b >= 0x40 && b <= 0x7E {
				// Final byte — sequence is complete, return to normal.
				f.state = csiStateNormal
			}
			// All bytes (parameter, intermediate, and final) are discarded.
		}
	}

	return out
}

// containsByte returns true if b is present in data. Used for the fast path
// check to avoid allocation when no filtering is needed.
func containsByte(data []byte, b byte) bool {
	for _, v := range data {
		if v == b {
			return true
		}
	}
	return false
}

// ── HistoryProvider ─────────────────────────────────────────────────────────

// LoadHistory reads the most recent Claude Code conversation for the given
// repo and returns formatted text lines suitable for scrollback display.
//
// Claude Code stores conversations in ~/.claude/projects/<encoded-path>/
// where the encoded path replaces "/" with "-". Each session is a JSONL file
// named <session-uuid>.jsonl containing one JSON object per message/event.
func (a *claudeAdapter) LoadHistory(repoPath string) ([]string, error) {
	dir := claudeProjectDir(repoPath)
	if dir == "" {
		return nil, nil
	}

	sessionFile, err := findLatestSession(dir)
	if err != nil || sessionFile == "" {
		return nil, nil
	}

	return parseSessionMessages(sessionFile)
}

// claudeProjectDir returns the Claude Code project directory for the given
// repo path, or "" if it does not exist. Claude Code encodes paths by
// replacing "/" with "-", e.g. "/Users/amir/foo" → "-Users-amir-foo".
func claudeProjectDir(repoPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Resolve symlinks and clean the path for consistent encoding.
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}
	absPath = filepath.Clean(absPath)

	encoded := strings.ReplaceAll(absPath, string(filepath.Separator), "-")
	dir := filepath.Join(home, ".claude", "projects", encoded)

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// findLatestSession returns the path to the most recently modified .jsonl
// session file in the given directory, or "" if none exist.
func findLatestSession(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	type candidate struct {
		path    string
		modTime int64
	}
	var candidates []candidate

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime().UnixNano(),
		})
	}

	if len(candidates) == 0 {
		return "", nil
	}

	// Sort by modification time descending — most recent first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})

	return candidates[0].path, nil
}

// claudeMessage is the minimal structure needed to parse Claude Code's
// session JSONL entries for history display.
type claudeMessage struct {
	Type    string `json:"type"`
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// claudeContentBlock represents a single block in an assistant message's
// content array.
type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// parseSessionMessages reads a Claude Code session JSONL file and extracts
// user and assistant messages, formatting them with role headers for
// scrollback display.
func parseSessionMessages(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lines []string
	seenContent := false

	for _, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		var msg claudeMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue // skip malformed lines
		}

		switch msg.Type {
		case "user":
			text := extractUserContent(msg.Message.Content)
			if text == "" {
				continue
			}
			if seenContent {
				lines = append(lines, "")
			}
			lines = append(lines, "── You ──")
			lines = append(lines, text)
			seenContent = true

		case "assistant":
			text := extractAssistantText(msg.Message.Content)
			if text == "" {
				continue
			}
			if seenContent {
				lines = append(lines, "")
			}
			lines = append(lines, "── Assistant ──")
			lines = append(lines, text)
			seenContent = true
		}
	}

	if len(lines) == 0 {
		return nil, nil
	}
	return lines, nil
}

// extractUserContent extracts the text from a user message's content field.
// User messages can have content as either a plain string or an array of
// tool_result blocks (for tool responses).
func extractUserContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as a plain string first (most common for user messages).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Not a string — likely a tool_result array. Skip these for history
	// display since they are internal plumbing, not user-authored text.
	return ""
}

// extractAssistantText extracts displayable text from an assistant message's
// content array. It concatenates all "text" type blocks, skipping "thinking"
// and "tool_use" blocks.
func extractAssistantText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var blocks []claudeContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		// Content might be a single string (unusual for assistant).
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	}

	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}
