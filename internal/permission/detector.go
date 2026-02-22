// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package permission

import (
	"context"

	"github.com/openconductorhq/openconductor/internal/config"
)

// DetectionMode controls whether L1 pattern matching runs before L2 or
// whether the L2 classifier is always used.
type DetectionMode int

const (
	// ModeL1First tries pattern matching and only escalates to L2 on miss.
	// Best for agents with well-documented, stable permission prompt formats.
	ModeL1First DetectionMode = iota
	// ModeL2First skips L1 and always uses the LLM classifier.
	// Best for agents with TUI modals or variable permission text.
	ModeL2First
)

// DetectionModeFor returns the appropriate detection mode for an agent type.
// All supported agents use L1-first: pattern matching is tried before the LLM
// classifier. If L1 misses (e.g. an action phrase not yet in the pattern table)
// the call falls through to L2 when a classifier is configured.
func DetectionModeFor(_ config.AgentType) DetectionMode {
	return ModeL1First
}

// Detector combines L1 pattern matching and an optional L2 LLM classifier to
// produce a ParsedPermission from raw terminal output.
type Detector struct {
	classifier *Classifier // nil when no LLM is configured
}

// NewDetector creates a Detector. classifier may be nil; in that case only L1
// pattern matching is used and L2 escalation is skipped.
func NewDetector(classifier *Classifier) *Detector {
	return &Detector{classifier: classifier}
}

// Detect classifies a permission request from terminal output.
//
// Returns nil when:
//   - The detection mode is L1-first and no L1 pattern matched AND no
//     classifier is configured.
//   - The detection mode is L2-first and no classifier is configured.
//   - The L2 classifier returned a result below confidenceThreshold.
//
// A nil result means "cannot classify; notify the user".
func (d *Detector) Detect(ctx context.Context, sessionName string, agentType config.AgentType, lines []string) *ParsedPermission {
	mode := DetectionModeFor(agentType)

	if mode == ModeL1First {
		if p := TryMatch(agentType, lines); p != nil {
			return p
		}
		// L1 missed — fall through to L2 if available.
	}

	if d.classifier == nil {
		return nil
	}

	p, err := d.classifier.Classify(ctx, sessionName, agentType, lines)
	if err != nil || p == nil {
		return nil
	}

	// Reject low-confidence L2 results.
	if p.Confidence < confidenceThreshold {
		return nil
	}

	return p
}
