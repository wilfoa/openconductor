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

// ── State persistence tests ─────────────────────────────────────

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := AppState{
		OpenTabs:  []string{"ProjectA", "ProjectB", "ProjectC"},
		ActiveTab: "ProjectB",
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded := LoadState(path)
	if len(loaded.OpenTabs) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(loaded.OpenTabs))
	}
	for i, name := range original.OpenTabs {
		if loaded.OpenTabs[i] != name {
			t.Errorf("tab %d: got %q, want %q", i, loaded.OpenTabs[i], name)
		}
	}
	if loaded.ActiveTab != "ProjectB" {
		t.Errorf("active tab: got %q, want %q", loaded.ActiveTab, "ProjectB")
	}
}

func TestLoadState_MissingFile(t *testing.T) {
	state := LoadState("/nonexistent/path/state.json")
	if len(state.OpenTabs) != 0 {
		t.Fatalf("expected empty tabs for missing file, got %d", len(state.OpenTabs))
	}
	if state.ActiveTab != "" {
		t.Fatalf("expected empty active tab, got %q", state.ActiveTab)
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte("{invalid json"), 0o644)

	state := LoadState(path)
	if len(state.OpenTabs) != 0 {
		t.Fatalf("expected empty tabs for invalid JSON, got %d", len(state.OpenTabs))
	}
}

func TestFilterValidProjects(t *testing.T) {
	projects := []Project{
		{Name: "Alpha", Repo: "/a", Agent: AgentOpenCode},
		{Name: "Beta", Repo: "/b", Agent: AgentOpenCode},
		{Name: "Gamma", Repo: "/g", Agent: AgentClaudeCode},
	}

	// Tab list includes a deleted project "Deleted".
	tabs := []string{"Beta", "Deleted", "Alpha"}
	result := FilterValidProjects(tabs, projects)

	if len(result) != 2 {
		t.Fatalf("expected 2 valid tabs, got %d: %v", len(result), result)
	}
	if result[0] != "Beta" || result[1] != "Alpha" {
		t.Errorf("expected [Beta Alpha], got %v", result)
	}
}

func TestFilterValidProjects_AllDeleted(t *testing.T) {
	projects := []Project{
		{Name: "Alpha", Repo: "/a", Agent: AgentOpenCode},
	}
	tabs := []string{"Gone1", "Gone2"}
	result := FilterValidProjects(tabs, projects)

	if len(result) != 0 {
		t.Fatalf("expected 0 valid tabs, got %d", len(result))
	}
}

func TestFilterValidProjects_EmptyTabs(t *testing.T) {
	projects := []Project{{Name: "A", Repo: "/a", Agent: AgentOpenCode}}
	result := FilterValidProjects(nil, projects)
	if len(result) != 0 {
		t.Fatalf("expected 0 tabs for nil input, got %d", len(result))
	}
}

func TestSaveState_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "state.json")

	err := SaveState(path, AppState{OpenTabs: []string{"A"}})
	if err != nil {
		t.Fatalf("SaveState should create nested dirs: %v", err)
	}

	loaded := LoadState(path)
	if len(loaded.OpenTabs) != 1 || loaded.OpenTabs[0] != "A" {
		t.Fatalf("round-trip failed: %+v", loaded)
	}
}
