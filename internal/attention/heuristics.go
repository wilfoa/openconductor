package attention

import "strings"

// permissionPatterns are strings that indicate the process is asking for a
// permission decision from the user.
var permissionPatterns = []string{
	"[y/N]",
	"[Y/n]",
	"(yes/no)",
	"Allow?",
	"Approve?",
	"Do you want to proceed?",
	"Permission denied",
}

// errorPatterns are strings that indicate the process has encountered an error.
var errorPatterns = []string{
	"Error:",
	"error:",
	"FAILED",
	"panic:",
	"fatal:",
}

// promptSuffixes are line endings that suggest an interactive prompt is
// waiting for input, though with less certainty than explicit patterns.
var promptSuffixes = []string{
	"> ",
	"? ",
	"$ ",
	">>> ",
}

// CheckHeuristics applies L1 heuristic checks against recent terminal output
// and process state to determine if the project needs user attention.
//
// It returns a HeuristicResult indicating confidence level and, when the
// result is Certain or Uncertain, an AttentionEvent describing what was found.
func CheckHeuristics(lastLines []string, processState ProcessState) (HeuristicResult, *AttentionEvent) {
	// Check process state first -- these are the highest confidence signals.
	if processState == Exited {
		return Certain, &AttentionEvent{
			Type:   NeedsReview,
			Detail: "process has exited",
			Source: "heuristic",
		}
	}

	if processState == BlockedOnRead {
		return Certain, &AttentionEvent{
			Type:   NeedsInput,
			Detail: "process is waiting for input",
			Source: "heuristic",
		}
	}

	// Scan lines from bottom to top for the most recent relevant output.
	for i := len(lastLines) - 1; i >= 0; i-- {
		line := lastLines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check permission patterns.
		for _, pattern := range permissionPatterns {
			if strings.Contains(line, pattern) {
				return Certain, &AttentionEvent{
					Type:   NeedsPermission,
					Detail: "permission prompt detected: " + pattern,
					Source: "heuristic",
				}
			}
		}

		// Check error patterns.
		for _, pattern := range errorPatterns {
			if strings.Contains(line, pattern) {
				return Certain, &AttentionEvent{
					Type:   HitError,
					Detail: "error detected: " + trimmed,
					Source: "heuristic",
				}
			}
		}

		// Check prompt suffixes (uncertain -- could be normal output).
		for _, suffix := range promptSuffixes {
			if strings.HasSuffix(line, suffix) {
				return Uncertain, &AttentionEvent{
					Type:   NeedsInput,
					Detail: "possible prompt detected: " + trimmed,
					Source: "heuristic",
				}
			}
		}

		// Only inspect the last few non-empty lines to avoid false positives
		// from older output further up in the buffer.
		break
	}

	return No, nil
}
