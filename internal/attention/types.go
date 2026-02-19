package attention

// AttentionType categorizes the kind of attention a project needs from the user.
type AttentionType int

const (
	// NeedsInput indicates the process is waiting for user input.
	NeedsInput AttentionType = iota
	// NeedsPermission indicates the process is requesting a permission decision.
	NeedsPermission
	// NeedsReview indicates the process has finished and output should be reviewed.
	NeedsReview
	// HitError indicates the process has encountered an error.
	HitError
	// Stuck indicates the process appears to be stuck or unresponsive.
	Stuck
)

// String returns a human-readable representation of the AttentionType.
func (a AttentionType) String() string {
	switch a {
	case NeedsInput:
		return "NeedsInput"
	case NeedsPermission:
		return "NeedsPermission"
	case NeedsReview:
		return "NeedsReview"
	case HitError:
		return "HitError"
	case Stuck:
		return "Stuck"
	default:
		return "Unknown"
	}
}

// ProcessState represents the current state of a monitored process.
type ProcessState int

const (
	// Running indicates the process is actively executing.
	Running ProcessState = iota
	// BlockedOnRead indicates the process is blocked waiting for stdin input.
	BlockedOnRead
	// Exited indicates the process has terminated.
	Exited
	// Unknown indicates the process state could not be determined.
	Unknown
)

// HeuristicResult indicates the confidence level of a heuristic check.
type HeuristicResult int

const (
	// Certain indicates the heuristic is confident in its determination.
	Certain HeuristicResult = iota
	// Uncertain indicates the heuristic has a possible but unconfirmed match.
	Uncertain
	// No indicates no attention-worthy pattern was detected.
	No
)

// AttentionEvent represents a detected event that requires user attention.
type AttentionEvent struct {
	// ProjectName is the name of the project that needs attention.
	ProjectName string
	// Type categorizes the kind of attention needed.
	Type AttentionType
	// Detail provides a human-readable description of what triggered the event.
	Detail string
	// Source indicates how the event was detected: "heuristic" or "llm".
	Source string
}
