package attention

import (
	"time"

	"github.com/amir/maestro/internal/terminal"
)

const (
	defaultCheckInterval = 500 * time.Millisecond
	lastLinesCount       = 50
)

// Detector performs L1 heuristic-based attention detection on terminal
// output. It examines the most recent terminal lines and process state
// to determine whether a project needs user attention.
//
// L2 LLM-based detection is not yet wired and will be added in a future
// sprint.
type Detector struct {
	checkInterval time.Duration
}

// NewDetector creates a Detector with default configuration.
func NewDetector() *Detector {
	return &Detector{
		checkInterval: defaultCheckInterval,
	}
}

// CheckInterval returns the configured interval between attention checks.
func (d *Detector) CheckInterval() time.Duration {
	return d.checkInterval
}

// Check examines a project's terminal buffer and process state to determine
// if the user's attention is needed.
//
// It retrieves the last 50 lines from the buffer, checks the process state,
// and runs L1 heuristic analysis. An AttentionEvent is returned only when
// the heuristic result is Certain. Uncertain and No results return nil
// (L2 LLM analysis for uncertain cases will be added later).
func (d *Detector) Check(projectName string, buffer *terminal.Buffer, pid int) *AttentionEvent {
	lastLines := buffer.LastLines(lastLinesCount)
	processState := CheckProcess(pid)

	result, event := CheckHeuristics(lastLines, processState)

	if result == Certain && event != nil {
		event.ProjectName = projectName
		return event
	}

	// L2 LLM-based analysis for Uncertain results will be added in a
	// future sprint. For now, uncertain signals are treated as no event.
	return nil
}
