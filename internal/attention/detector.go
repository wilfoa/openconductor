package attention

import (
	"context"
	"time"
)

const (
	defaultCheckInterval = 500 * time.Millisecond
	lastLinesCount       = 50
)

// Detector performs L1 heuristic-based attention detection on terminal
// output, with optional L2 LLM-based escalation for uncertain results.
//
// When a Classifier is configured, heuristic results with Uncertain
// confidence are forwarded to the LLM for a definitive classification.
type Detector struct {
	checkInterval time.Duration
	classifier    *Classifier
}

// NewDetector creates a Detector with default configuration and no L2
// classifier. Use SetClassifier to enable LLM escalation.
func NewDetector() *Detector {
	return &Detector{
		checkInterval: defaultCheckInterval,
	}
}

// SetClassifier configures the L2 LLM classifier for uncertain results.
// If nil, uncertain heuristic results are treated as no event.
func (d *Detector) SetClassifier(c *Classifier) {
	d.classifier = c
}

// CheckInterval returns the configured interval between attention checks.
func (d *Detector) CheckInterval() time.Duration {
	return d.checkInterval
}

// Check examines recent terminal output and process state to determine
// if the user's attention is needed.
//
// lastLines should be the most recent visible terminal lines (e.g. from
// session.GetScreenLines()). pid is the agent process ID for liveness
// checking. agentType identifies the coding agent (e.g. "claude-code",
// "opencode") for agent-specific pattern matching.
//
// Returns (event, isWorking):
//   - event != nil: attention is needed, isWorking is false.
//   - event == nil, isWorking == true: agent is actively working (positive
//     signal from agent-specific heuristics like spinner or progress bar).
//   - event == nil, isWorking == false: no signal detected; caller should
//     keep the current state (e.g. idle stays idle).
func (d *Detector) Check(ctx context.Context, projectName string, lastLines []string, pid int, agentType string) (event *AttentionEvent, isWorking bool) {
	processState := CheckProcess(pid)

	result, evt := CheckHeuristics(lastLines, processState, agentType)

	if result == Certain && evt != nil {
		evt.ProjectName = projectName
		return evt, false
	}

	// Working: agent-specific signal that the agent is active. No attention needed.
	if result == Working {
		return nil, true
	}

	// L2 escalation: use LLM to resolve uncertain heuristic results.
	if result == Uncertain && d.classifier != nil {
		return d.classifyUncertain(ctx, projectName, lastLines), false
	}

	return nil, false
}

// classifyUncertain calls the L2 LLM classifier and maps its response
// to an AttentionEvent. Returns nil if the LLM says WORKING or on error.
func (d *Detector) classifyUncertain(ctx context.Context, projectName string, lastLines []string) *AttentionEvent {
	state, err := d.classifier.Classify(ctx, projectName, lastLines)
	if err != nil {
		// On LLM error, treat as no event (fail open).
		return nil
	}

	return classifierStateToEvent(projectName, state)
}

// classifierStateToEvent maps a Classifier string result to an
// AttentionEvent. Returns nil for WORKING (no attention needed).
func classifierStateToEvent(projectName, state string) *AttentionEvent {
	switch state {
	case "WAITING_INPUT":
		return &AttentionEvent{
			ProjectName: projectName,
			Type:        NeedsInput,
			Detail:      "LLM detected agent waiting for input",
			Source:      "llm",
		}
	case "NEEDS_PERMISSION":
		return &AttentionEvent{
			ProjectName: projectName,
			Type:        NeedsPermission,
			Detail:      "LLM detected permission request",
			Source:      "llm",
		}
	case "DONE":
		return &AttentionEvent{
			ProjectName: projectName,
			Type:        NeedsReview,
			Detail:      "LLM detected task completion",
			Source:      "llm",
		}
	case "ERROR":
		return &AttentionEvent{
			ProjectName: projectName,
			Type:        HitError,
			Detail:      "LLM detected error state",
			Source:      "llm",
		}
	case "STUCK":
		return &AttentionEvent{
			ProjectName: projectName,
			Type:        Stuck,
			Detail:      "LLM detected agent appears stuck",
			Source:      "llm",
		}
	default:
		// WORKING or unrecognized — no attention needed.
		return nil
	}
}
