package agent

import (
	"os/exec"

	"github.com/amir/maestro/internal/config"
)

// codexAdapter implements AgentAdapter for the Codex CLI.
type codexAdapter struct{}

func init() {
	Register(&codexAdapter{})
}

// Type returns config.AgentCodex.
func (a *codexAdapter) Type() config.AgentType {
	return config.AgentCodex
}

// Command returns an *exec.Cmd that launches the "codex" CLI in the given
// repo directory.
func (a *codexAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	args := []string{}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	cmd := exec.Command("codex", args...)
	cmd.Dir = repoPath
	return cmd
}

// BootstrapFiles returns no bootstrap files for Codex.
func (a *codexAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}
