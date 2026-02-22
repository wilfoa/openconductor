// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package permission classifies and manages agent permission requests.
//
// Classification is two-layered:
//   - L1: fast regex pattern matching against known agent output formats.
//   - L2: LLM-based classification for uncertain or unrecognised patterns.
//
// The result is a universal Category that is independent of the agent type,
// allowing a single ApprovalLevel config value to apply across all agents.
package permission

import "github.com/openconductorhq/openconductor/internal/config"

// Category classifies the type of permission an agent is requesting.
type Category string

const (
	// FileRead covers reading or viewing file contents.
	FileRead Category = "file_read"
	// FileEdit covers modifying existing file contents.
	FileEdit Category = "file_edit"
	// FileCreate covers creating new files or directories.
	FileCreate Category = "file_create"
	// FileDelete covers deleting files or directories.
	FileDelete Category = "file_delete"
	// BashSafe covers execution of a curated set of low-risk shell commands
	// (git, ls, cat, npm, go, cargo, pip, make, etc.).
	BashSafe Category = "bash_safe"
	// BashAny covers execution of any shell command not in the safe list.
	BashAny Category = "bash_any"
	// MCPTools covers invocation of MCP (Model Context Protocol) tools.
	MCPTools Category = "mcp_tools"
	// Network covers outbound network requests (fetch, HTTP calls, etc.).
	Network Category = "network"
	// Unknown is returned when classification cannot determine the category.
	Unknown Category = "unknown"
)

// ParsedPermission is the result of classifying a permission request from
// raw terminal output.
type ParsedPermission struct {
	// Category is the universal permission category.
	Category Category
	// Description is a brief human-readable summary of what is being requested.
	Description string
	// Confidence is the classifier's confidence in the result, 0.0–1.0.
	// L1 pattern matches always return 1.0; L2 returns the LLM's stated confidence.
	Confidence float64
	// Source indicates how the result was produced: "pattern" (L1) or "llm" (L2).
	Source string
}

// safeLevelCategories are the categories auto-approved at ApprovalSafe.
var safeLevelCategories = map[Category]bool{
	FileRead:   true,
	FileEdit:   true,
	FileCreate: true,
	BashSafe:   true,
	MCPTools:   true,
}

// fullLevelCategories are the categories auto-approved at ApprovalFull.
var fullLevelCategories = map[Category]bool{
	FileRead:   true,
	FileEdit:   true,
	FileCreate: true,
	FileDelete: true,
	BashSafe:   true,
	BashAny:    true,
	MCPTools:   true,
	Network:    true,
}

// IsAllowed reports whether the given category is permitted under level.
func IsAllowed(level config.ApprovalLevel, cat Category) bool {
	switch level {
	case config.ApprovalFull:
		return fullLevelCategories[cat]
	case config.ApprovalSafe:
		return safeLevelCategories[cat]
	default:
		return false
	}
}

// confidenceThreshold is the minimum L2 confidence required to act on a
// classification. Results below this threshold fall back to notifying the user.
const confidenceThreshold = 0.8
