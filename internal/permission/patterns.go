// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package permission

import (
	"regexp"
	"strings"

	"github.com/openconductorhq/openconductor/internal/config"
)

// bashSafeCommands is the set of shell commands considered low-risk.
// This list is used by both L1 pattern matching and the L2 classifier prompt.
var bashSafeCommands = []string{
	"git", "ls", "cat", "head", "tail", "grep", "find",
	"npm", "yarn", "pnpm", "node",
	"go", "cargo", "pip", "pip3", "pytest", "python", "python3",
	"ruby", "java", "make", "echo", "pwd", "mkdir", "which",
	"env", "printenv", "source", "export",
}

// BashSafeCommandSet returns the bash-safe command list as a set for O(1) lookup.
func BashSafeCommandSet() map[string]bool {
	m := make(map[string]bool, len(bashSafeCommands))
	for _, c := range bashSafeCommands {
		m[c] = true
	}
	return m
}

// BashSafeCommandList returns the bash-safe command list as a comma-separated
// string, suitable for embedding in LLM prompts.
func BashSafeCommandList() string {
	return strings.Join(bashSafeCommands, ", ")
}

// patternRule maps a compiled regex to a permission category.
// The regex is matched case-insensitively against each terminal line.
type patternRule struct {
	re       *regexp.Regexp
	category Category
}

// ── Claude Code L1 patterns ─────────────────────────────────────────────────
//
// Claude Code renders permission requests as natural-language lines such as:
//   "Allow editing of src/main.go? [y/n]"
//   "Allow running bash command: git status? [y/n]"
//   "Allow creating file .env? [y/n]"
//   "Allow deleting file tmp/old.log? [y/n]"
//   "Allow reading file secrets.txt? [y/n]"
//   "Allow fetching https://api.example.com? [y/n]"

var claudeCodePatterns = []patternRule{
	{regexp.MustCompile(`(?i)\ballow\s+(reading|viewing)\b`), FileRead},
	{regexp.MustCompile(`(?i)\ballow\s+(editing|modifying|writing\s+to|updating)\b`), FileEdit},
	{regexp.MustCompile(`(?i)\ballow\s+(creating|making)\s+(file|directory|dir|folder)\b`), FileCreate},
	{regexp.MustCompile(`(?i)\ballow\s+(deleting|removing|delete)\b`), FileDelete},
	{regexp.MustCompile(`(?i)\ballow\s+(fetching|downloading|requesting)\b`), Network},
	{regexp.MustCompile(`(?i)\ballow\s+running\b`), BashAny}, // refined below
	{regexp.MustCompile(`(?i)\ballow\s+calling\s+(mcp|tool)\b`), MCPTools},
}

// ── OpenCode L1 patterns ─────────────────────────────────────────────────────
//
// OpenCode renders a TUI permission modal with the following structure
// (confirmed from real terminal capture):
//
//   ⚠ Permission required
//   ← Access external directory ~/Downloads/some/path
//   Patterns
//   - /path/to/glob/*
//   Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm
//
// The action description appears after the "←" (U+2190 LEFTWARDS ARROW) and
// uses natural-language phrases. Patterns below match that description line.
// The `←` arrow combined with the action verb is the primary classification
// signal; the button row ("Allow once" / "Allow always") confirms the dialog.
//
// Known action phrases observed / reasonably inferred from OpenCode source:
//   "Access external directory ..."  → FileRead
//   "Read file ..."                  → FileRead
//   "Write file ..."                 → FileCreate
//   "Edit file ..."                  → FileEdit
//   "Delete file ..."                → FileDelete
//   "Execute command ..."            → BashAny (refined to BashSafe)
//   "Run command ..."                → BashAny (refined to BashSafe)
//   "Fetch URL ..."                  → Network
//   "HTTP request ..."               → Network
//   "mcp_<tool> ..."                 → MCPTools

var openCodePatterns = []patternRule{
	// Action description lines — the "←" arrow prefix is the key signal.
	// U+2190 "←" is matched as a literal rune; the (?i) flag covers the verb.
	{regexp.MustCompile(`←\s*(?i)(read|view)\s`), FileRead},
	{regexp.MustCompile(`←\s*(?i)access\s+(external\s+)?(directory|dir|folder)\s`), FileRead},
	{regexp.MustCompile(`←\s*(?i)(write|create)\s+(file|directory|dir)?\s`), FileCreate},
	{regexp.MustCompile(`←\s*(?i)(edit|modify|patch)\s+(file)?\s`), FileEdit},
	{regexp.MustCompile(`←\s*(?i)(delete|remove)\s+(file|directory|dir)?\s`), FileDelete},
	{regexp.MustCompile(`←\s*(?i)(fetch|http)\s`), Network},
	{regexp.MustCompile(`←\s*(?i)mcp_\w+`), MCPTools},
	// Execute / run command — category refined to BashSafe via command inspection.
	{regexp.MustCompile(`←\s*(?i)(execute|run)\s+(command\s+)?`), BashAny},
}

// ── Generic fallback patterns ────────────────────────────────────────────────
//
// Applied when no agent-specific rules match; these are intentionally broad.

var genericPatterns = []patternRule{
	{regexp.MustCompile(`(?i)\b(read|view|cat|head|tail)\s+\S+\.(go|ts|js|py|rs|md|json|yaml|yml|toml|txt)\b`), FileRead},
	{regexp.MustCompile(`(?i)\b(edit|modify|write|patch|update)\b`), FileEdit},
	{regexp.MustCompile(`(?i)\b(create|mkdir)\b`), FileCreate},
	{regexp.MustCompile(`(?i)\b(delete|remove|rm)\b`), FileDelete},
	{regexp.MustCompile(`(?i)\b(fetch|curl|wget|http)\b`), Network},
	{regexp.MustCompile(`(?i)\bmcp_`), MCPTools},
}

// TryMatch performs L1 pattern matching against terminal lines and returns a
// ParsedPermission if a pattern matches with certainty, or nil if the content
// should be escalated to L2.
//
// For BashAny matches, the function inspects the command name to decide whether
// it should be downgraded to BashSafe.
func TryMatch(agentType config.AgentType, lines []string) *ParsedPermission {
	rules := rulesFor(agentType)
	text := strings.Join(lines, "\n")

	for _, rule := range rules {
		if rule.re.MatchString(text) {
			cat := rule.category
			// Refine BashAny → BashSafe if the command is on the safe list.
			if cat == BashAny {
				cat = refineBashCategory(text)
			}
			return &ParsedPermission{
				Category:    cat,
				Description: extractDescription(text),
				Confidence:  1.0,
				Source:      "pattern",
			}
		}
	}
	return nil
}

// rulesFor returns the pattern rules appropriate for the given agent type,
// falling back to generic rules for unknown agents.
func rulesFor(agentType config.AgentType) []patternRule {
	switch agentType {
	case config.AgentClaudeCode:
		return claudeCodePatterns
	case config.AgentOpenCode:
		return openCodePatterns
	default:
		return genericPatterns
	}
}

// refineBashCategory inspects the terminal text for the actual command name
// being executed and returns BashSafe if it is on the safe list, otherwise
// BashAny.
//
// It tries patterns in priority order — most specific first — so that
// "Allow running bash command: git status" resolves to "git" (via
// `command:\s+(\w+)`) rather than "bash" (via `running\s+(\w+)`).
func refineBashCategory(text string) Category {
	safe := BashSafeCommandSet()

	// Patterns tried in order: most specific wins.
	// Each captures the first meaningful token after the keyword.
	cmdPatterns := []*regexp.Regexp{
		// OpenCode: "← Execute command npm install" / "← Run command git status"
		regexp.MustCompile(`(?i)(?:execute|run)\s+command\s+(\w+)`),
		// Claude Code: "Allow running bash command: git status"
		regexp.MustCompile(`(?i)command:\s+(\w+)`),
		regexp.MustCompile(`(?i)bash:\s+(\w+)`),
		regexp.MustCompile(`(?i)running\s+(\w+)`),
	}

	for _, re := range cmdPatterns {
		m := re.FindStringSubmatch(text)
		if len(m) < 2 {
			continue
		}
		cmd := strings.ToLower(m[1])
		// Skip shell names — they are the executor, not the command.
		if cmd == "bash" || cmd == "sh" || cmd == "zsh" || cmd == "fish" {
			continue
		}
		if safe[cmd] {
			return BashSafe
		}
		return BashAny
	}
	return BashAny
}

// extractDescription returns a short human-readable description from the
// raw terminal text. It uses the first non-empty line as a summary.
func extractDescription(text string) string {
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && len(t) > 3 {
			if len(t) > 120 {
				return t[:120] + "…"
			}
			return t
		}
	}
	return "permission request"
}
