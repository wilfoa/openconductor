//go:build e2e

// Package e2e provides end-to-end testing infrastructure for OpenConductor.
// Tests run real agent binaries (claude, opencode) inside tmux sessions and
// assert on terminal output, structured logs, and state transitions.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	// tmuxSessionPrefix is the prefix for tmux session names.
	tmuxSessionPrefix = "oc-e2e"

	// defaultWidth and defaultHeight are the terminal dimensions for the
	// tmux pane. Large enough for OpenCode's sidebar + conversation area.
	defaultWidth  = 200
	defaultHeight = 50

	// startupTimeout is how long to wait for OpenConductor to render.
	startupTimeout = 20 * time.Second

	// attentionTickWait is the time to wait for at least one attention
	// check cycle to complete (tick is 2s in production).
	attentionTickWait = 3 * time.Second
)

// Harness manages the lifecycle of an OpenConductor instance running inside
// tmux for E2E testing. Each test gets its own Harness with isolated config,
// log file, and state files.
type Harness struct {
	t          *testing.T
	sessionID  string
	tmpDir     string
	configPath string
	logPath    string
	statePath  string
	binaryPath string
	logWatcher *LogWatcher
	width      int
	height     int
}

// HarnessOption configures a Harness.
type HarnessOption func(*Harness)

// WithDimensions sets the tmux pane dimensions.
func WithDimensions(w, h int) HarnessOption {
	return func(hr *Harness) {
		hr.width = w
		hr.height = h
	}
}

// NewHarness creates a test harness. The binaryPath should point to a
// pre-built OpenConductor binary (built once in TestMain).
func NewHarness(t *testing.T, binaryPath string, opts ...HarnessOption) *Harness {
	t.Helper()

	// Use os.MkdirTemp instead of t.TempDir() because t.TempDir() is
	// cleaned up when the test ends — but the tmux session and its
	// subprocesses may still be reading these files during cleanup.
	tmpDir, err := os.MkdirTemp("", "oc-e2e-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	sessionID := fmt.Sprintf("%s-%d", tmuxSessionPrefix, time.Now().UnixNano()%1000000)

	h := &Harness{
		t:          t,
		sessionID:  sessionID,
		tmpDir:     tmpDir,
		configPath: filepath.Join(tmpDir, "config.yaml"),
		logPath:    filepath.Join(tmpDir, "openconductor.log"),
		statePath:  filepath.Join(tmpDir, "state.json"),
		binaryPath: binaryPath,
		width:      defaultWidth,
		height:     defaultHeight,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Start writes the config, launches OpenConductor in a tmux session, and
// waits for the TUI to render.
func (h *Harness) Start(cfg TestConfig) {
	h.t.Helper()

	// Write the test config.
	if err := cfg.WriteTo(h.configPath); err != nil {
		h.t.Fatalf("writing test config: %v", err)
	}

	// Kill any stale tmux session with this name.
	exec.Command("tmux", "kill-session", "-t", h.sessionID).Run()

	// Create a new tmux session running OpenConductor.
	// We use /bin/sh -c with exported env vars to ensure they're passed
	// to the OpenConductor process correctly.
	shellCmd := fmt.Sprintf(
		"export OC_CONFIG_PATH='%s' OC_LOG_DIR='%s' OC_STATE_PATH='%s'; exec '%s' --debug",
		h.configPath, h.tmpDir, h.statePath, h.binaryPath,
	)

	cmd := exec.Command("tmux",
		"new-session", "-d",
		"-s", h.sessionID,
		"-x", fmt.Sprintf("%d", h.width),
		"-y", fmt.Sprintf("%d", h.height),
		"/bin/sh", "-c", shellCmd,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		h.t.Fatalf("tmux new-session failed: %v\n%s", err, out)
	}

	// Start watching the log file.
	h.logWatcher = NewLogWatcher(h.t, h.logPath)

	// Wait for OpenConductor to start (log marker).
	h.logWatcher.WaitForEntry(startupTimeout, func(e LogEntry) bool {
		return e.Msg == "openconductor starting"
	}, "OpenConductor did not write startup log")

	// Wait for the agent session to actually start. This is an async
	// operation — OpenConductor starts the agent binary in a goroutine.
	// Wait for the session-started log or the first attention check.
	h.logWatcher.WaitForEntry(30*time.Second, func(e LogEntry) bool {
		return e.Msg == "attention check" || e.Msg == "session started"
	}, "agent session did not start")

	// Repeatedly handle interactive prompts that agents show on first run.
	// Claude Code shows "trust this folder?" and may also show other
	// first-run dialogs. We check every second for 15 seconds.
	for i := 0; i < 15; i++ {
		screen := h.CapturePaneRaw()
		if strings.Contains(screen, "trust this folder") ||
			strings.Contains(screen, "Yes, I trust") ||
			strings.Contains(screen, "Enter to confirm") {
			h.SendKeys("Enter")
			time.Sleep(2 * time.Second)
			continue
		}
		// If the agent prompt or idle state is visible, we're ready.
		if strings.Contains(screen, "\u203a") || // Claude Code prompt
			strings.Contains(screen, "ctrl+p commands") || // OpenCode idle
			strings.Contains(screen, "ctrl+t variants") || // OpenCode idle
			strings.Contains(screen, "esc interrupt") { // OpenCode working
			break
		}
		time.Sleep(1 * time.Second)
	}

	h.t.Cleanup(h.cleanup)
}

// cleanup kills the tmux session, stops the log watcher, and removes temp files.
func (h *Harness) cleanup() {
	// On test failure, dump the last screen and log entries for debugging.
	if h.t.Failed() {
		h.t.Log("=== SCREEN DUMP ON FAILURE ===")
		screen, _ := exec.Command("tmux", "capture-pane", "-t", h.sessionID, "-p").Output()
		h.t.Log(string(screen))
		h.t.Log("=== LAST LOG ENTRIES ===")
		if h.logWatcher != nil {
			entries := h.logWatcher.Entries()
			start := 0
			if len(entries) > 30 {
				start = len(entries) - 30
			}
			for _, e := range entries[start:] {
				h.t.Logf("  [%s] %s %s", e.Level, e.Msg, e.Session)
			}
		}
		h.t.Logf("=== CONFIG: %s ===", h.configPath)
		cfgData, _ := os.ReadFile(h.configPath)
		h.t.Log(string(cfgData))
	}

	if h.logWatcher != nil {
		h.logWatcher.Stop()
	}
	exec.Command("tmux", "kill-session", "-t", h.sessionID).Run()
	// Keep tmpDir on failure for post-mortem investigation.
	if !h.t.Failed() {
		os.RemoveAll(h.tmpDir)
	} else {
		h.t.Logf("Temp dir preserved for debugging: %s", h.tmpDir)
	}
}

// LogWatcher returns the harness's log watcher.
func (h *Harness) LogWatcher() *LogWatcher {
	return h.logWatcher
}

// TmpDir returns the harness's temp directory.
func (h *Harness) TmpDir() string {
	return h.tmpDir
}

// LogPath returns the path to the log file.
func (h *Harness) LogPath() string {
	return h.logPath
}
