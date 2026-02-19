package agent

import (
	"os/exec"
	"strings"

	"github.com/amir/maestro/internal/config"
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

// AttentionHints scans the last visible terminal lines for prompts that
// indicate OpenCode needs user interaction.
func (a *opencodeAdapter) AttentionHints(lastLines []string) *AttentionHint {
	for i := len(lastLines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lastLines[i])
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		// Permission / confirmation prompts
		if strings.Contains(lower, "approve") ||
			strings.Contains(lower, "allow") ||
			strings.Contains(lower, "yes/no") ||
			strings.Contains(lower, "(y/n)") {
			return &AttentionHint{
				Type:   "needs_permission",
				Detail: line,
			}
		}

		// Input prompt
		if strings.HasSuffix(line, "> ") || line == ">" {
			return &AttentionHint{
				Type:   "needs_input",
				Detail: "OpenCode is waiting for input",
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
		if strings.Contains(lower, "completed") ||
			strings.Contains(lower, "finished") {
			return &AttentionHint{
				Type:   "done",
				Detail: line,
			}
		}

		break
	}
	return nil
}

// BootstrapFiles returns no bootstrap files for OpenCode.
func (a *opencodeAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}
