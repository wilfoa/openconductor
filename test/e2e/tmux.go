//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
)

// CapturePaneRaw returns the full tmux pane content as a single string.
func (h *Harness) CapturePaneRaw() string {
	h.t.Helper()
	out, err := exec.Command("tmux", "capture-pane",
		"-t", h.sessionID,
		"-p", // print to stdout
	).Output()
	if err != nil {
		h.t.Fatalf("tmux capture-pane failed: %v", err)
	}
	return string(out)
}

// CapturePane returns the tmux pane content as a slice of strings (one per row),
// trimming trailing empty lines.
func (h *Harness) CapturePane() []string {
	raw := h.CapturePaneRaw()
	lines := strings.Split(raw, "\n")

	// Trim trailing empty lines.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// SendKeys sends keys to the tmux pane. Keys can be plain text or special
// tmux key names: "Enter", "Tab", "Escape", "C-c", "C-s", "C-j", "C-k",
// "Up", "Down", "Left", "Right".
//
// Examples:
//
//	h.SendKeys("Hello world", "Enter")
//	h.SendKeys("C-s")
func (h *Harness) SendKeys(keys ...string) {
	h.t.Helper()
	args := append([]string{"send-keys", "-t", h.sessionID}, keys...)
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		h.t.Fatalf("tmux send-keys %v failed: %v\n%s", keys, err, out)
	}
}

// SendText types text into the active pane and presses Enter.
func (h *Harness) SendText(text string) {
	h.t.Helper()
	h.SendKeys(text, "Enter")
}

// SendRaw sends raw literal text to the tmux pane (no key interpretation).
func (h *Harness) SendRaw(data string) {
	h.t.Helper()
	out, err := exec.Command("tmux", "send-keys", "-t", h.sessionID, "-l", data).CombinedOutput()
	if err != nil {
		h.t.Fatalf("tmux send-keys -l failed: %v\n%s", err, out)
	}
}

// PaneSize returns the current pane width and height.
func (h *Harness) PaneSize() (width, height int) {
	h.t.Helper()
	out, err := exec.Command("tmux", "display-message",
		"-t", h.sessionID, "-p", "#{pane_width}x#{pane_height}",
	).Output()
	if err != nil {
		h.t.Fatalf("tmux display-message failed: %v", err)
	}
	fmt.Sscanf(strings.TrimSpace(string(out)), "%dx%d", &width, &height)
	return
}
