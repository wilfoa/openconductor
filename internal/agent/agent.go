// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package agent defines the AgentAdapter interface and a registry for
// supported coding-agent CLIs.
package agent

import (
	"fmt"
	"os/exec"

	"github.com/openconductorhq/openconductor/internal/config"
)

// LaunchOptions holds parameters passed when launching an agent.
type LaunchOptions struct {
	Prompt string
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
