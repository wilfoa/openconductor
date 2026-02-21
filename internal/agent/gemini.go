package agent

import (
	"os/exec"

	"github.com/openconductorhq/openconductor/internal/config"
)

// geminiAdapter implements AgentAdapter for the Gemini CLI.
type geminiAdapter struct{}

func init() {
	Register(&geminiAdapter{})
}

// Type returns config.AgentGemini.
func (a *geminiAdapter) Type() config.AgentType {
	return config.AgentGemini
}

// Command returns an *exec.Cmd that launches the "gemini" CLI in the given
// repo directory.
func (a *geminiAdapter) Command(repoPath string, opts LaunchOptions) *exec.Cmd {
	args := []string{}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	cmd := exec.Command("gemini", args...)
	cmd.Dir = repoPath
	return cmd
}

// BootstrapFiles returns no bootstrap files for Gemini.
func (a *geminiAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}
