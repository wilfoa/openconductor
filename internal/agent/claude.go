// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"os/exec"

	"github.com/openconductorhq/openconductor/internal/config"
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

// ApproveKeystroke returns "y\n" — Claude Code uses y/n prompts.
func (a *claudeAdapter) ApproveKeystroke() []byte { return []byte("y\n") }

// ApproveSessionKeystroke returns nil — Claude Code has no session-wide approval.
func (a *claudeAdapter) ApproveSessionKeystroke() []byte { return nil }

// DenyKeystroke returns "n\n".
func (a *claudeAdapter) DenyKeystroke() []byte { return []byte("n\n") }

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
