// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

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
	AgentOpenCode   AgentType = "opencode"
)

// ApprovalLevel controls which permission requests are auto-approved for a
// project without requiring user interaction.
type ApprovalLevel string

const (
	// ApprovalOff disables auto-approve; all permission requests notify the user.
	ApprovalOff ApprovalLevel = "off"
	// ApprovalSafe auto-approves low-to-medium risk operations: file reads,
	// file edits, file creation, safe shell commands, and MCP tool calls.
	ApprovalSafe ApprovalLevel = "safe"
	// ApprovalFull auto-approves all operations including file deletion, any
	// shell command, and network requests. Use with caution.
	ApprovalFull ApprovalLevel = "full"
)

// PersonaType identifies a behavioral preset that writes agent-specific
// instructions into the project's instruction file. Built-in personas
// use well-known slugs; custom personas reference names from Config.Personas.
type PersonaType string

const (
	// PersonaNone is the zero value -- no persona instructions are written.
	PersonaNone PersonaType = ""
	// PersonaVibe optimises for velocity: skip tests, auto-approve, move fast.
	PersonaVibe PersonaType = "vibe"
	// PersonaPOC builds working prototypes with reasonable quality.
	PersonaPOC PersonaType = "poc"
	// PersonaScale targets production-grade engineering with TDD and thorough review.
	PersonaScale PersonaType = "scale"
)

// BuiltinPersonaNames is the set of built-in persona slugs. Exported so
// the persona package can check for name collisions during custom persona
// creation without importing a list of constants.
var BuiltinPersonaNames = map[PersonaType]bool{
	PersonaVibe:  true,
	PersonaPOC:   true,
	PersonaScale: true,
}

// CustomPersona defines a user-created persona stored in the top-level
// config under the "personas" key.
type CustomPersona struct {
	// Name is a slug identifier used in Project.Persona.
	Name string `yaml:"name"`
	// Label is a human-readable display name shown in the TUI.
	Label string `yaml:"label"`
	// Instructions is the markdown text injected into the agent's
	// instruction file between the persona markers.
	Instructions string `yaml:"instructions"`
	// AutoApprove is the suggested default approval level when this
	// persona is selected.
	AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
}

type Project struct {
	Name        string        `yaml:"name"`
	Repo        string        `yaml:"repo"`
	Agent       AgentType     `yaml:"agent"`
	Persona     PersonaType   `yaml:"persona,omitempty"`
	AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
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

// TelegramConfig controls the Telegram bot bridge for remote agent
// interaction via Forum Topics.
type TelegramConfig struct {
	Enabled     bool   `yaml:"enabled"`
	BotTokenEnv string `yaml:"bot_token_env"` // env var name containing the bot token
	ChatID      int64  `yaml:"chat_id"`       // supergroup ID (with Forum Topics enabled)
}

type Config struct {
	Projects      []Project          `yaml:"projects"`
	Personas      []CustomPersona    `yaml:"personas,omitempty"`
	LLM           LLMConfig          `yaml:"llm"`
	Notifications NotificationConfig `yaml:"notifications"`
	Telegram      TelegramConfig     `yaml:"telegram"`
}

func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".openconductor"
	}
	return filepath.Join(home, ".openconductor")
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
		case AgentClaudeCode, AgentOpenCode:
			// valid
		default:
			return fmt.Errorf("project %q: unknown agent type %q", p.Name, p.Agent)
		}
		switch p.AutoApprove {
		case ApprovalOff, ApprovalSafe, ApprovalFull, "":
			// valid (empty defaults to off)
		default:
			return fmt.Errorf("project %q: unknown auto_approve level %q", p.Name, p.AutoApprove)
		}
	}

	// Validate custom persona definitions.
	personaNames := make(map[string]bool)
	for i, cp := range c.Personas {
		if cp.Name == "" {
			return fmt.Errorf("persona %d: missing name", i)
		}
		if personaNames[cp.Name] {
			return fmt.Errorf("persona %q: duplicate name", cp.Name)
		}
		personaNames[cp.Name] = true
		if cp.Label == "" {
			return fmt.Errorf("persona %q: missing label", cp.Name)
		}
		if cp.Instructions == "" {
			return fmt.Errorf("persona %q: missing instructions", cp.Name)
		}
		switch cp.AutoApprove {
		case ApprovalOff, ApprovalSafe, ApprovalFull, "":
			// valid
		default:
			return fmt.Errorf("persona %q: unknown auto_approve level %q",
				cp.Name, cp.AutoApprove)
		}
	}

	return nil
}

// ValidatePersonaRef checks whether a persona reference is valid against
// the built-in names and the config's custom personas. This is NOT called
// from validate()/Load() so that a deleted custom persona does not prevent
// config loading.
func (c *Config) ValidatePersonaRef(persona PersonaType) error {
	if persona == PersonaNone {
		return nil
	}
	if BuiltinPersonaNames[persona] {
		return nil
	}
	for _, cp := range c.Personas {
		if PersonaType(cp.Name) == persona {
			return nil
		}
	}
	return fmt.Errorf("unknown persona %q", persona)
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
