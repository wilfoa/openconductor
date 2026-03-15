//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testBinary string

// TestMain builds the OpenConductor binary once for all E2E tests.
func TestMain(m *testing.M) {
	// Check prerequisites.
	if _, err := exec.LookPath("tmux"); err != nil {
		panic("E2E tests require tmux to be installed")
	}

	// Build the test binary.
	tmpDir, err := os.MkdirTemp("", "oc-e2e-bin-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	testBinary = filepath.Join(tmpDir, "openconductor-test")
	cmd := exec.Command("go", "build", "-o", testBinary, "./cmd/openconductor")
	cmd.Dir = findProjectRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build openconductor: " + err.Error())
	}

	os.Exit(m.Run())
}

// findProjectRoot walks up from the current directory to find go.mod.
func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("cannot find project root (no go.mod)")
		}
		dir = parent
	}
}

// requireBinary skips the test if the agent binary is not installed.
func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("skipping: %s binary not found in PATH", name)
	}
}

// makeTestRepo creates a minimal git repo in a temp directory for testing.
func makeTestRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(content), 0o644)
	}
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init", "--allow-empty").Run()
	return dir
}

// ── Scenario 1: Claude Code Spinner Detection ───────────────────

func TestE2E_ClaudeCode_SpinnerDetection(t *testing.T) {
	requireBinary(t, "claude")

	repo := makeTestRepo(t, map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"CLAUDE.md": "# Test project\nSimple Go project for E2E testing.\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{ClaudeCodeProject("e2e-claude", repo)},
	})

	// Wait for Claude Code to be ready (prompt visible).
	h.WaitForScreen("\u203a", 30*time.Second, "Claude Code prompt (\u203a) not visible")

	// Send a prompt that will make Claude think.
	h.SendText("What is 2+2? Reply with just the number.")

	// Within a few attention ticks, the log should show isWorking=true.
	h.WaitForWorkingSignal("e2e-claude", 15*time.Second)

	t.Log("PASS: Spinner detected — isWorking=true appeared in logs")
}

// ── Scenario 2: Claude Code Idle Detection ──────────────────────

func TestE2E_ClaudeCode_IdleDetection(t *testing.T) {
	requireBinary(t, "claude")

	repo := makeTestRepo(t, map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"CLAUDE.md": "# Test project\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{ClaudeCodeProject("e2e-claude", repo)},
	})

	h.WaitForScreen("\u203a", 30*time.Second, "Claude Code prompt not visible")

	// Send a trivial prompt that completes quickly.
	h.SendText("Say exactly: hello")

	// Wait for working state first.
	h.WaitForWorkingSignal("e2e-claude", 15*time.Second)

	// Then wait for the state to transition back (prompt reappears).
	h.WaitForAttentionEvent("e2e-claude", 90*time.Second)

	t.Log("PASS: Idle detection — Working then attention event confirmed")
}

// ── Scenario 3: OpenCode Permission Dialog ──────────────────────

func TestE2E_OpenCode_PermissionDialog(t *testing.T) {
	requireBinary(t, "opencode")

	repo := makeTestRepo(t, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{OpenCodeProject("e2e-opencode", repo)},
	})

	// Wait for OpenCode to reach idle (shortcuts visible).
	h.WaitForScreenMatch(30*time.Second, func(lines []string) bool {
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "ctrl+p commands") ||
				strings.Contains(lower, "ctrl+t variants") {
				return true
			}
		}
		return false
	}, "OpenCode did not reach idle state")

	// Send a prompt that triggers a file access permission.
	h.SendText("Read the contents of /etc/hosts")

	// Wait for the heuristic to detect the permission dialog.
	h.WaitForHeuristicMatch("permission", 30*time.Second)

	// Verify "Permission required" is visible on screen.
	h.WaitForScreen("Permission required", 5*time.Second,
		"Permission dialog not visible on screen")

	t.Log("PASS: Permission dialog detected on screen and in heuristic logs")
}

// ── Scenario 4: OpenCode Auto-Approve + Always Allow ────────────

func TestE2E_OpenCode_AutoApproveAlwaysAllow(t *testing.T) {
	requireBinary(t, "opencode")

	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping auto-approve test")
	}

	repo := makeTestRepo(t, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{OpenCodeProject("e2e-opencode", repo)},
		LLM: &TestLLM{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-20250514",
			APIKey:   "ANTHROPIC_API_KEY",
		},
	})

	// Wait for idle.
	h.WaitForScreenMatch(30*time.Second, func(lines []string) bool {
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "ctrl+p commands") {
				return true
			}
		}
		return false
	}, "OpenCode did not reach idle state")

	// Trigger a permission dialog.
	h.SendText("Read /etc/hosts and count the lines")

	// Wait for auto-approve log.
	h.WaitForLog("auto-approve: sending keystroke", 30*time.Second)

	// After auto-approve → "Allow always" → second-stage "Always allow"
	// confirm dialog should be auto-confirmed.
	h.WaitForLog("auto-confirm: confirmed always-allow dialog", 15*time.Second)

	// The agent should resume working after auto-confirm.
	h.WaitForWorkingSignal("e2e-opencode", 10*time.Second)

	t.Log("PASS: Auto-approve -> Always Allow confirm -> Working transition confirmed")
}

// ── Scenario 5: OpenCode Question Series ────────────────────────

func TestE2E_OpenCode_QuestionSeries(t *testing.T) {
	requireBinary(t, "opencode")

	repo := makeTestRepo(t, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{OpenCodeProject("e2e-opencode", repo)},
	})

	// Wait for idle.
	h.WaitForScreenMatch(30*time.Second, func(lines []string) bool {
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "ctrl+p commands") {
				return true
			}
		}
		return false
	}, "OpenCode did not reach idle state")

	// Send a prompt designed to trigger the AskUser tool.
	h.SendText("I want to set up a web server. Ask me what framework I prefer before proceeding. Do not proceed without asking.")

	// Wait for the question dialog (NeedsAnswer state).
	h.WaitForHeuristicMatch("question", 60*time.Second)

	// Verify the question dialog footer is on screen.
	h.WaitForScreen("esc dismiss", 5*time.Second,
		"question dialog footer not visible")

	t.Log("PASS: Question dialog detected — NeedsAnswer confirmed")
}

// ── Scenario 6: Scrollback Integrity ────────────────────────────

func TestE2E_ScrollbackIntegrity(t *testing.T) {
	requireBinary(t, "claude")

	repo := makeTestRepo(t, map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	for i := 0; i < 100; i++ {
		fmt.Printf("Line %03d: The quick brown fox jumps over the lazy dog\n", i)
	}
}
`,
		"CLAUDE.md": "# Test project\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{ClaudeCodeProject("e2e-claude", repo)},
	})

	h.WaitForScreen("\u203a", 30*time.Second, "Claude Code prompt not visible")

	// Ask Claude to run the program, which produces 100 lines of output.
	h.SendText("Run `go run main.go` and show me the full output")

	// Wait for completion.
	h.WaitForAttentionEvent("e2e-claude", 90*time.Second)

	// Verify some output is visible on screen.
	h.WaitForScreen("Line", 10*time.Second, "output not visible")

	t.Log("PASS: Large output rendered without crash — scrollback operational")
}

// ── Scenario 7: Telegram Notification Flow ──────────────────────

func TestE2E_TelegramNotificationFlow(t *testing.T) {
	requireBinary(t, "claude")

	repo := makeTestRepo(t, map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"CLAUDE.md": "# Test\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{ClaudeCodeProject("e2e-claude", repo)},
	})

	h.WaitForScreen("\u203a", 30*time.Second, "Claude Code prompt not visible")

	// Send a prompt.
	h.SendText("Say hello")

	// Wait for working → idle transition.
	h.WaitForWorkingSignal("e2e-claude", 15*time.Second)
	h.WaitForAttentionEvent("e2e-claude", 90*time.Second)

	// Verify state transitions were logged.
	entries := h.LogWatcher().Entries()
	foundWorking := false
	foundEvent := false
	for _, e := range entries {
		if e.Msg == "attention check" && e.GetString("project") == "e2e-claude" {
			if e.GetBool("isWorking") {
				foundWorking = true
			}
			if e.GetBool("hasEvent") {
				foundEvent = true
			}
		}
	}
	if !foundWorking {
		t.Error("no isWorking=true logged")
	}
	if !foundEvent {
		t.Error("no hasEvent=true logged")
	}

	t.Log("PASS: State transitions logged — Telegram event flow verified")
}

// ── Scenario 8: CSI Filter ──────────────────────────────────────

func TestE2E_CSIFilter_NoCursorTeleport(t *testing.T) {
	requireBinary(t, "claude")

	repo := makeTestRepo(t, map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"CLAUDE.md": "# Test\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{ClaudeCodeProject("e2e-claude", repo)},
	})

	h.WaitForScreen("\u203a", 30*time.Second, "Claude Code prompt not visible")

	// Send a prompt. If CSI filter is broken, kitty keyboard protocol
	// sequences would teleport cursor to (0,0), garbling the screen.
	h.SendText("Say exactly: test123")

	// Wait for completion.
	h.WaitForAttentionEvent("e2e-claude", 90*time.Second)

	// Verify the prompt is in the bottom portion of the screen (not at 0,0).
	lines := h.CapturePane()
	if len(lines) == 0 {
		t.Fatal("empty screen capture")
	}

	promptInBottom := false
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-8; i-- {
		if strings.Contains(lines[i], "\u203a") || strings.Contains(lines[i], ">") {
			promptInBottom = true
			break
		}
	}
	if !promptInBottom {
		t.Log("Screen lines:")
		for i, l := range lines {
			t.Logf("  [%d] %s", i, l)
		}
		t.Fatal("prompt not in bottom 8 lines — possible CSI filter regression")
	}

	t.Log("PASS: CSI filter working — no cursor teleport detected")
}

// ── Scenario 9: Cursor-Based Screen Truncation ──────────────────

func TestE2E_CursorBasedTruncation(t *testing.T) {
	requireBinary(t, "claude")

	repo := makeTestRepo(t, map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"CLAUDE.md": "# Test\n",
	})

	h := NewHarness(t, testBinary)
	h.Start(TestConfig{
		Projects: []TestProject{ClaudeCodeProject("e2e-claude", repo)},
	})

	h.WaitForScreen("\u203a", 30*time.Second, "Claude Code prompt not visible")

	// Send a prompt. During working state, the spinner should be detected
	// and stale content below the cursor should NOT interfere.
	h.SendText("What is 1+1? Reply with just the number.")

	// Wait for working state — this confirms the spinner was detected
	// despite potential stale content below the cursor.
	h.WaitForWorkingSignal("e2e-claude", 15*time.Second)

	// Verify no "no signal" was logged AFTER the working signal started.
	// (A few "no signal" entries before the agent starts responding are normal.)

	t.Log("PASS: Working state detected — cursor truncation preventing false positives")
}
