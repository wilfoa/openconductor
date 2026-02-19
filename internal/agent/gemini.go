package agent

import (
	"os/exec"
	"strings"

	"github.com/amir/maestro/internal/config"
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

// AttentionHints scans the last visible terminal lines for prompts that
// indicate the Gemini CLI needs user interaction.
func (a *geminiAdapter) AttentionHints(lastLines []string) *AttentionHint {
	for i := len(lastLines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lastLines[i])
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		// Permission / confirmation prompts
		if strings.Contains(lower, "confirm") ||
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
				Detail: "Gemini is waiting for input",
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

// BootstrapFiles returns no bootstrap files for Gemini.
func (a *geminiAdapter) BootstrapFiles() []BootstrapFile {
	return nil
}
