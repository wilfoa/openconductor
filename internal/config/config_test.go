// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAllowsZeroProjects(t *testing.T) {
	cfg := &Config{Projects: []Project{}}
	if err := cfg.validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateRejectsMissingName(t *testing.T) {
	cfg := &Config{Projects: []Project{{Name: "", Repo: "/tmp", Agent: AgentClaudeCode}}}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateRejectsMissingRepo(t *testing.T) {
	cfg := &Config{Projects: []Project{{Name: "test", Repo: "", Agent: AgentClaudeCode}}}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestValidateRejectsUnknownAgent(t *testing.T) {
	cfg := &Config{Projects: []Project{{Name: "test", Repo: "/tmp", Agent: "unknown"}}}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestLoadOrDefaultNoFile(t *testing.T) {
	cfg := LoadOrDefault(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(cfg.Projects))
	}
	if !cfg.Notifications.Enabled {
		t.Fatal("expected notifications enabled by default")
	}
}

func TestLoadOrDefaultValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `projects:
  - name: myproj
    repo: /tmp
    agent: claude-code
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadOrDefault(path)
	if len(cfg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0].Name != "myproj" {
		t.Fatalf("expected name 'myproj', got %q", cfg.Projects[0].Name)
	}
}

func TestSaveCreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "config.yaml")

	cfg := &Config{
		Projects: []Project{{Name: "p1", Repo: "/tmp", Agent: AgentGemini}},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{
		Projects: []Project{
			{Name: "alpha", Repo: "/tmp/alpha", Agent: AgentClaudeCode},
			{Name: "beta", Repo: "/tmp/beta", Agent: AgentCodex},
		},
		Notifications: NotificationConfig{Enabled: true, Cooldown: 60},
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded.Projects) != len(original.Projects) {
		t.Fatalf("expected %d projects, got %d", len(original.Projects), len(loaded.Projects))
	}
	for i, p := range loaded.Projects {
		o := original.Projects[i]
		if p.Name != o.Name || p.Repo != o.Repo || p.Agent != o.Agent {
			t.Fatalf("project %d mismatch: got %+v, want %+v", i, p, o)
		}
	}
}
