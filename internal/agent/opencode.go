// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"os/exec"

	"github.com/openconductorhq/openconductor/internal/config"
)

// opencodeAdapter implements AgentAdapter for the OpenCode CLI.
type opencodeAdapter struct{}

func init() {
	Register(&opencodeAdapter{})
}

// Type returns config.AgentOpenCode.
func (a *opencodeAdapter) Type() config.AgentType {
	return config.AgentOpenCode
}

// Command returns an *exec.Cmd that launches the "opencode" CLI in the given
// repo directory.
func (a *opencodeAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	cmd := exec.Command("opencode")
	cmd.Dir = repoPath
	return cmd
}

// ApproveKeystroke returns "a" — OpenCode uses single-key permission dialog.
func (a *opencodeAdapter) ApproveKeystroke() []byte { return []byte("a") }

// ApproveSessionKeystroke returns "A" — OpenCode supports session-wide approval.
func (a *opencodeAdapter) ApproveSessionKeystroke() []byte { return []byte("A") }

// DenyKeystroke returns "d".
func (a *opencodeAdapter) DenyKeystroke() []byte { return []byte("d") }

// BootstrapFiles returns no bootstrap files for OpenCode.
func (a *opencodeAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}
