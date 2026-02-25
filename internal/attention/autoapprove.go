// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"context"

	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
	"github.com/openconductorhq/openconductor/internal/permission"
)

// AutoApprover decides whether a detected permission request should be
// automatically approved based on the project's configured ApprovalLevel, and
// returns the keystroke bytes to send to the PTY if so.
type AutoApprover struct {
	permDetector *permission.Detector
}

// NewAutoApprover creates an AutoApprover. The provided permission.Detector
// wraps both the L1 pattern matcher and the optional L2 LLM classifier.
func NewAutoApprover(d *permission.Detector) *AutoApprover {
	return &AutoApprover{permDetector: d}
}

// AutoApproveResult is returned by CheckAndApprove.
type AutoApproveResult struct {
	// ShouldApprove is true when the permission is within the project's
	// configured level and the detector has sufficient confidence.
	ShouldApprove bool
	// Keystroke is the raw bytes to write to the session PTY to approve the
	// permission. Non-nil only when ShouldApprove is true.
	Keystroke []byte
	// Parsed is the classified permission, populated whenever the detector
	// produced a result (regardless of ShouldApprove).
	Parsed *permission.ParsedPermission
}

// ApprovalKeystrokes holds the raw bytes needed to approve a permission
// request via the agent's PTY. The caller extracts these from the agent
// adapter so this package doesn't depend on the agent package.
type ApprovalKeystrokes struct {
	// Approve is sent for a single approval (e.g. "y\n" for Claude Code).
	Approve []byte
	// ApproveSession is sent for session-wide approval (e.g. "A" for
	// OpenCode). Nil if the agent does not support session-wide approval.
	ApproveSession []byte
}

// CheckAndApprove classifies the permission being requested and decides
// whether to auto-approve it.
//
//   - project: the project configuration (provides ApprovalLevel and AgentType).
//   - lines: the most recent visible terminal lines used for classification.
//   - keystrokes: the agent's approval keystrokes (extracted from the adapter
//     by the caller to avoid an import cycle with the agent package).
//
// Returns an AutoApproveResult. When ShouldApprove is false the caller should
// notify the user normally.
func (a *AutoApprover) CheckAndApprove(
	ctx context.Context,
	project config.Project,
	lines []string,
	keystrokes ApprovalKeystrokes,
) AutoApproveResult {
	// Fast exit: auto-approve is disabled for this project.
	if project.AutoApprove == config.ApprovalOff || project.AutoApprove == "" {
		return AutoApproveResult{}
	}

	// Classify the permission.
	parsed := a.permDetector.Detect(ctx, project.Name, project.Agent, lines)
	if parsed == nil {
		// Classifier returned nothing actionable — notify the user.
		return AutoApproveResult{}
	}

	result := AutoApproveResult{Parsed: parsed}

	// Check whether the category falls within the project's approval level.
	if !permission.IsAllowed(project.AutoApprove, parsed.Category) {
		logging.Info("auto-approve: category not in level",
			"project", project.Name,
			"category", string(parsed.Category),
			"level", string(project.AutoApprove),
		)
		return result
	}

	// Use session-wide approval when the agent supports it, so subsequent
	// prompts of the same type are also handled without OpenConductor
	// needing to classify them again.
	keystroke := keystrokes.ApproveSession
	if keystroke == nil {
		keystroke = keystrokes.Approve
	}

	logging.Info("auto-approve: approved",
		"project", project.Name,
		"category", string(parsed.Category),
		"description", parsed.Description,
		"confidence", parsed.Confidence,
		"source", parsed.Source,
		"level", string(project.AutoApprove),
	)

	result.ShouldApprove = true
	result.Keystroke = keystroke
	return result
}
