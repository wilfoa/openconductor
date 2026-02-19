package agent

import (
	"os/exec"
	"strings"

	"github.com/amir/maestro/internal/config"
)

// claudeAdapter implements AgentAdapter for the Claude Code CLI.
type claudeAdapter struct{}

func init() {
	Register(&claudeAdapter{})
}

// Type returns config.AgentClaudeCode.
func (a *claudeAdapter) Type() config.AgentType {
	return config.AgentClaudeCode
}

// Command returns an *exec.Cmd that launches the "claude" CLI in the given
// repo directory.
func (a *claudeAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	args := []string{}
	if opts.Prompt != "" {
		args = append(args, "--prompt", opts.Prompt)
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = repoPath
	return cmd
}

// AttentionHints scans the last visible terminal lines for prompts that
// indicate Claude Code needs user interaction.
func (a *claudeAdapter) AttentionHints(lastLines []string) *AttentionHint {
	for i := len(lastLines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lastLines[i])
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		// Permission / confirmation prompts
		if strings.Contains(lower, "do you want to proceed") ||
			strings.Contains(lower, "allow") ||
			strings.Contains(lower, "yes/no") ||
			strings.Contains(lower, "(y/n)") {
			return &AttentionHint{
				Type:   "needs_permission",
				Detail: line,
			}
		}

		// Input prompt (Claude shows "> " when waiting for user input)
		if strings.HasSuffix(line, "> ") || line == ">" {
			return &AttentionHint{
				Type:   "needs_input",
				Detail: "Claude is waiting for input",
			}
		}

		// Error indicators
		if strings.Contains(lower, "error:") ||
			strings.Contains(lower, "fatal:") {
			return &AttentionHint{
				Type:   "error",
				Detail: line,
			}
		}

		// Done indicators
		if strings.Contains(lower, "task completed") ||
			strings.Contains(lower, "all done") {
			return &AttentionHint{
				Type:   "done",
				Detail: line,
			}
		}

		// Only check the last few non-empty lines
		break
	}
	return nil
}

// BootstrapFiles returns a placeholder CLAUDE.md for the repository.
func (a *claudeAdapter) BootstrapFiles() []BootstrapFile {
	return []BootstrapFile{
		{
			Path: "CLAUDE.md",
			Content: []byte(`# CLAUDE.md — Project Context for Claude Code

## Overview
This file provides context to Claude Code about the project.

## Guidelines
- Follow existing code style and conventions
- Write tests for new functionality
- Keep changes focused and minimal
`),
		},
	}
}
