// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package agent defines the AgentAdapter interface and a registry for
// supported coding-agent CLIs.
package agent

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/openconductorhq/openconductor/internal/config"
)

// LaunchOptions holds parameters passed when launching an agent.
type LaunchOptions struct {
	Prompt string
	// Continue tells the agent to resume the previous conversation instead
	// of starting fresh. Used when restoring tabs from a previous session
	// or opening the first tab for a project (which already has history).
	Continue bool
}

// BootstrapFile represents a file that should be written into the repo before
// launching the agent.
type BootstrapFile struct {
	Path    string
	Content []byte
}

// AgentAdapter abstracts a coding-agent CLI (e.g. Claude Code, Codex, Gemini).
type AgentAdapter interface {
	// Type returns the config-level agent type identifier.
	Type() config.AgentType

	// Command builds an *exec.Cmd ready to be started in the given repo path.
	Command(repoPath string, opts LaunchOptions) *exec.Cmd

	// BootstrapFiles returns files that should be seeded into a repo before
	// the agent is launched for the first time.
	BootstrapFiles() []BootstrapFile

	// ApproveKeystroke returns the raw bytes to send to the PTY to approve a
	// permission request (e.g. "y\n" for Claude Code, "a" for OpenCode).
	ApproveKeystroke() []byte

	// ApproveSessionKeystroke returns bytes that approve the permission for the
	// entire session (e.g. "A" for OpenCode). Returns nil if the agent does not
	// support session-wide approval.
	ApproveSessionKeystroke() []byte

	// DenyKeystroke returns the raw bytes to send to the PTY to deny a
	// permission request.
	DenyKeystroke() []byte
}

// ScreenFilter is an optional interface that agents can implement to crop
// screen output before it is sent to Telegram. Agents with sidebar panels
// (e.g. OpenCode) implement this to extract only the conversation area.
type ScreenFilter interface {
	FilterScreen(lines []string) []string
}

// FilterScreen runs the adapter's screen filter if it implements ScreenFilter,
// otherwise returns lines unchanged.
func FilterScreen(agentType config.AgentType, lines []string) []string {
	a, err := Get(agentType)
	if err != nil {
		return lines
	}
	if f, ok := a.(ScreenFilter); ok {
		return f.FilterScreen(lines)
	}
	return lines
}

// ChromeLayout is an optional interface that agents can implement to describe
// fixed TUI chrome rows (header, footer, status bar) that should be excluded
// from scrollback capture. When pushing alt-screen diffs, rows in the chrome
// zones change frequently (timer ticks, token counters) and would pollute
// the scrollback buffer with noise.
type ChromeLayout interface {
	// ChromeSkipRows returns the number of rows to skip from the top and
	// bottom of the screen when capturing scrollback from alt-screen diffs.
	ChromeSkipRows() (top int, bottom int)
}

// ChromeSkipRows returns the chrome skip values for the given agent type.
// If the adapter does not implement ChromeLayout, returns (0, 0).
func ChromeSkipRows(agentType config.AgentType) (top int, bottom int) {
	a, err := Get(agentType)
	if err != nil {
		return 0, 0
	}
	if c, ok := a.(ChromeLayout); ok {
		return c.ChromeSkipRows()
	}
	return 0, 0
}

// SubmitDelay is an optional interface that agents can implement to specify
// the pause between writing text and sending Enter to the PTY. TUI apps
// with event-loop-based stdin processing (e.g. Bubble Tea) may need a delay
// so the text is committed before the submit key arrives.
type SubmitDelay interface {
	// SubmitDelay returns the duration to wait between writing text and
	// sending Enter (\r) to the PTY. Return 0 for no delay.
	SubmitDelay() time.Duration
}

// GetSubmitDelay returns the submit delay for the given agent type.
// If the adapter does not implement SubmitDelay, returns 0.
func GetSubmitDelay(agentType config.AgentType) time.Duration {
	a, err := Get(agentType)
	if err != nil {
		return 0
	}
	if d, ok := a.(SubmitDelay); ok {
		return d.SubmitDelay()
	}
	return 0
}

// HistoryProvider is an optional interface that agents can implement to supply
// previous conversation history for scrollback pre-population. When a session
// tab opens, the app calls LoadHistory to get text lines that are pushed into
// the scrollback buffer so the user can scroll up to see prior context.
type HistoryProvider interface {
	LoadHistory(repoPath string) ([]string, error)
}

// LoadHistory calls the adapter's LoadHistory if it implements HistoryProvider,
// otherwise returns nil.
func LoadHistory(agentType config.AgentType, repoPath string) ([]string, error) {
	a, err := Get(agentType)
	if err != nil {
		return nil, nil
	}
	if h, ok := a.(HistoryProvider); ok {
		return h.LoadHistory(repoPath)
	}
	return nil, nil
}

// registry maps agent type identifiers to their adapter implementations.
var registry = map[config.AgentType]AgentAdapter{}

// Register adds an adapter to the global registry.
func Register(a AgentAdapter) {
	registry[a.Type()] = a
}

// Get returns the adapter for the given agent type, or an error if none is
// registered.
func Get(agentType config.AgentType) (AgentAdapter, error) {
	a, ok := registry[agentType]
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %q", agentType)
	}
	return a, nil
}
