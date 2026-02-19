package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type AgentType string

const (
	AgentClaudeCode AgentType = "claude-code"
	AgentCodex      AgentType = "codex"
	AgentGemini     AgentType = "gemini"
	AgentOpenCode   AgentType = "opencode"
)

type Project struct {
	Name  string    `yaml:"name"`
	Repo  string    `yaml:"repo"`
	Agent AgentType `yaml:"agent"`
}

type LLMConfig struct {
	Provider string `yaml:"provider"` // "anthropic", "openai", "google"
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key_env"` // env var name containing the key
}

type NotificationConfig struct {
	Enabled  bool `yaml:"enabled"`
	Cooldown int  `yaml:"cooldown_seconds"`
}

type Config struct {
	Projects      []Project          `yaml:"projects"`
	LLM           LLMConfig          `yaml:"llm"`
	Notifications NotificationConfig `yaml:"notifications"`
}

func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".maestro"
	}
	return filepath.Join(home, ".maestro")
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	for i, p := range c.Projects {
		if p.Name == "" {
			return fmt.Errorf("project %d: missing name", i)
		}
		if p.Repo == "" {
			return fmt.Errorf("project %q: missing repo path", p.Name)
		}
		switch p.Agent {
		case AgentClaudeCode, AgentCodex, AgentGemini, AgentOpenCode:
			// valid
		default:
			return fmt.Errorf("project %q: unknown agent type %q", p.Name, p.Agent)
		}
	}
	return nil
}

func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		return &Config{
			Projects: []Project{},
			Notifications: NotificationConfig{
				Enabled:  true,
				Cooldown: 30,
			},
		}
	}
	return cfg
}

// Save writes the config to the given path, creating directories as needed.
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
