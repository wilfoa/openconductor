//go:build e2e

package e2e

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TestConfig generates an OpenConductor config for a test scenario.
type TestConfig struct {
	Projects []TestProject `yaml:"projects"`
	LLM      *TestLLM      `yaml:"llm,omitempty"`
	Telegram *TestTelegram `yaml:"telegram,omitempty"`
}

// TestProject defines a project in the test config.
type TestProject struct {
	Name  string `yaml:"name"`
	Repo  string `yaml:"repo"`
	Agent string `yaml:"agent"`
}

// TestLLM defines the LLM config for auto-approve tests.
type TestLLM struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
}

// TestTelegram defines the Telegram config (usually disabled for tests).
type TestTelegram struct {
	Enabled bool `yaml:"enabled"`
}

// WriteTo writes the config to the given path.
func (c TestConfig) WriteTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ClaudeCodeProject returns a TestProject configured for Claude Code.
func ClaudeCodeProject(name, repoPath string) TestProject {
	return TestProject{
		Name:  name,
		Repo:  repoPath,
		Agent: "claude-code",
	}
}

// OpenCodeProject returns a TestProject configured for OpenCode.
func OpenCodeProject(name, repoPath string) TestProject {
	return TestProject{
		Name:  name,
		Repo:  repoPath,
		Agent: "opencode",
	}
}
