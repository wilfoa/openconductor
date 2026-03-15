//go:build e2e

package e2e

import (
	"strings"
	"time"
)

// ── Screen Assertions ───────────────────────────────────────────

// WaitForScreen polls tmux capture-pane until the screen contains the
// given substring, or fails after timeout.
func (h *Harness) WaitForScreen(substr string, timeout time.Duration, failMsg string) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screen := h.CapturePaneRaw()
		if strings.Contains(screen, substr) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	screen := h.CapturePaneRaw()
	h.t.Fatalf("%s\nsubstring %q not found in screen:\n%s", failMsg, substr, screen)
}

// WaitForScreenGone polls until the screen no longer contains the substring.
func (h *Harness) WaitForScreenGone(substr string, timeout time.Duration, failMsg string) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screen := h.CapturePaneRaw()
		if !strings.Contains(screen, substr) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	screen := h.CapturePaneRaw()
	h.t.Fatalf("%s\nsubstring %q still present in screen:\n%s", failMsg, substr, screen)
}

// WaitForScreenMatch polls until the predicate returns true for the screen lines.
func (h *Harness) WaitForScreenMatch(timeout time.Duration, pred func(lines []string) bool, failMsg string) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		lines := h.CapturePane()
		if pred(lines) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	screen := h.CapturePaneRaw()
	h.t.Fatalf("%s\nscreen:\n%s", failMsg, screen)
}

// ScreenContains returns true if the current screen contains the substring.
func (h *Harness) ScreenContains(substr string) bool {
	return strings.Contains(h.CapturePaneRaw(), substr)
}

// ── Log Assertions ──────────────────────────────────────────────

// WaitForLog waits for a log entry with the given message substring.
func (h *Harness) WaitForLog(msgSubstr string, timeout time.Duration) LogEntry {
	h.t.Helper()
	return h.logWatcher.WaitForEntry(timeout, func(e LogEntry) bool {
		return strings.Contains(e.Msg, msgSubstr)
	}, "log message containing: "+msgSubstr)
}

// WaitForLogField waits for a log entry where msg contains msgSubstr AND
// the given field has the specified string value.
func (h *Harness) WaitForLogField(msgSubstr, field, value string, timeout time.Duration) LogEntry {
	h.t.Helper()
	return h.logWatcher.WaitForEntry(timeout, func(e LogEntry) bool {
		return strings.Contains(e.Msg, msgSubstr) && e.GetString(field) == value
	}, "log '"+msgSubstr+"' with "+field+"="+value)
}

// ── State Assertions ────────────────────────────────────────────

// WaitForWorkingSignal waits for the attention check log to report isWorking=true
// for the given project.
func (h *Harness) WaitForWorkingSignal(project string, timeout time.Duration) {
	h.t.Helper()
	h.logWatcher.WaitForEntry(timeout, func(e LogEntry) bool {
		return e.Msg == "attention check" &&
			e.GetString("project") == project &&
			e.GetBool("isWorking")
	}, "isWorking=true for project "+project)
}

// WaitForAttentionEvent waits for the attention check to report hasEvent=true
// for the given project.
func (h *Harness) WaitForAttentionEvent(project string, timeout time.Duration) {
	h.t.Helper()
	h.logWatcher.WaitForEntry(timeout, func(e LogEntry) bool {
		return e.Msg == "attention check" &&
			e.GetString("project") == project &&
			e.GetBool("hasEvent")
	}, "hasEvent=true for project "+project)
}

// WaitForStateTransition waits for an attention state transition log entry
// for the given project transitioning to the specified state.
func (h *Harness) WaitForStateTransition(project, toState string, timeout time.Duration) {
	h.t.Helper()
	h.logWatcher.WaitForEntry(timeout, func(e LogEntry) bool {
		return e.Msg == "attention state transition" &&
			e.GetString("project") == project &&
			e.GetString("to") == toState
	}, "state transition to "+toState+" for "+project)
}

// WaitForHeuristicMatch waits for a specific heuristic log message.
func (h *Harness) WaitForHeuristicMatch(msgSubstr string, timeout time.Duration) LogEntry {
	h.t.Helper()
	return h.logWatcher.WaitForEntry(timeout, func(e LogEntry) bool {
		return strings.Contains(e.Msg, "heuristic") && strings.Contains(e.Msg, msgSubstr)
	}, "heuristic log containing: "+msgSubstr)
}

// ── Sidebar Assertions ──────────────────────────────────────────

// WaitForSidebarState waits for the sidebar to show a specific state text
// next to the project name (e.g., "working", "permission", "idle", "asking").
func (h *Harness) WaitForSidebarState(project, state string, timeout time.Duration) {
	h.t.Helper()
	h.WaitForScreenMatch(timeout, func(lines []string) bool {
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, strings.ToLower(project)) &&
				strings.Contains(lower, state) {
				return true
			}
		}
		return false
	}, "sidebar state '"+state+"' for project "+project)
}
