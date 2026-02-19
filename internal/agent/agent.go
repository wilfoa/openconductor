// Package agent defines the AgentAdapter interface and a registry for
// supported coding-agent CLIs.
package agent

import (
	"fmt"
	"os/exec"

	"github.com/amir/maestro/internal/config"
)

// LaunchOptions holds parameters passed when launching an agent.
type LaunchOptions struct {
	Prompt string
}

// AttentionHint describes a detected attention condition from an agent's
// terminal output.
type AttentionHint struct {
	Type   string // "needs_input", "needs_permission", "done", "error"
	Detail string
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

	// AttentionHints inspects the last visible terminal lines and returns an
	// AttentionHint if the agent appears to need user interaction, or nil
	// otherwise.
	AttentionHints(lastLines []string) *AttentionHint

	// BootstrapFiles returns files that should be seeded into a repo before
	// the agent is launched for the first time.
	BootstrapFiles() []BootstrapFile
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
