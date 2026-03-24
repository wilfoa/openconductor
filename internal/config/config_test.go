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
		Projects: []Project{{Name: "p1", Repo: "/tmp", Agent: AgentOpenCode}},
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
			{Name: "beta", Repo: "/tmp/beta", Agent: AgentOpenCode},
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
		OpenTabs: []TabState{
			{Project: "ProjectA"},
			{Project: "ProjectB", Label: "PB custom"},
			{Project: "ProjectC"},
		},
		ActiveTab: "ProjectB",
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded := LoadState(path)
	if len(loaded.OpenTabs) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(loaded.OpenTabs))
	}
	for i, ts := range original.OpenTabs {
		if loaded.OpenTabs[i].Project != ts.Project {
			t.Errorf("tab %d project: got %q, want %q", i, loaded.OpenTabs[i].Project, ts.Project)
		}
		if loaded.OpenTabs[i].Label != ts.Label {
			t.Errorf("tab %d label: got %q, want %q", i, loaded.OpenTabs[i].Label, ts.Label)
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

func TestFilterValidTabs(t *testing.T) {
	projects := []Project{
		{Name: "Alpha", Repo: "/a", Agent: AgentOpenCode},
		{Name: "Beta", Repo: "/b", Agent: AgentOpenCode},
		{Name: "Gamma", Repo: "/g", Agent: AgentClaudeCode},
	}

	// Tab list includes a deleted project "Deleted".
	tabs := []TabState{
		{Project: "Beta"},
		{Project: "Deleted", Label: "old label"},
		{Project: "Alpha", Label: "A custom"},
	}
	result := FilterValidTabs(tabs, projects)

	if len(result) != 2 {
		t.Fatalf("expected 2 valid tabs, got %d: %v", len(result), result)
	}
	if result[0].Project != "Beta" || result[1].Project != "Alpha" {
		t.Errorf("expected [Beta Alpha], got %v", result)
	}
	// Verify labels are preserved.
	if result[1].Label != "A custom" {
		t.Errorf("expected label %q, got %q", "A custom", result[1].Label)
	}
}

func TestFilterValidTabs_AllDeleted(t *testing.T) {
	projects := []Project{
		{Name: "Alpha", Repo: "/a", Agent: AgentOpenCode},
	}
	tabs := []TabState{{Project: "Gone1"}, {Project: "Gone2"}}
	result := FilterValidTabs(tabs, projects)

	if len(result) != 0 {
		t.Fatalf("expected 0 valid tabs, got %d", len(result))
	}
}

func TestFilterValidTabs_EmptyTabs(t *testing.T) {
	projects := []Project{{Name: "A", Repo: "/a", Agent: AgentOpenCode}}
	result := FilterValidTabs(nil, projects)
	if len(result) != 0 {
		t.Fatalf("expected 0 tabs for nil input, got %d", len(result))
	}
}

func TestTabProjectNames(t *testing.T) {
	tabs := []TabState{
		{Project: "A", Label: "custom"},
		{Project: "B"},
	}
	names := TabProjectNames(tabs)
	if len(names) != 2 || names[0] != "A" || names[1] != "B" {
		t.Errorf("expected [A B], got %v", names)
	}
}

func TestSaveState_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "state.json")

	err := SaveState(path, AppState{OpenTabs: []TabState{{Project: "A"}}})
	if err != nil {
		t.Fatalf("SaveState should create nested dirs: %v", err)
	}

	loaded := LoadState(path)
	if len(loaded.OpenTabs) != 1 || loaded.OpenTabs[0].Project != "A" {
		t.Fatalf("round-trip failed: %+v", loaded)
	}
}

// ── Persona-related tests ───────────────────────────────────────

func TestValidateAcceptsPersonaField(t *testing.T) {
	cfg := &Config{Projects: []Project{
		{Name: "proj", Repo: "/tmp", Agent: AgentClaudeCode, Persona: PersonaVibe},
	}}
	if err := cfg.validate(); err != nil {
		t.Fatalf("expected no error for project with persona 'vibe', got %v", err)
	}
}

func TestValidateAcceptsEmptyPersona(t *testing.T) {
	cfg := &Config{Projects: []Project{
		{Name: "proj", Repo: "/tmp", Agent: AgentClaudeCode, Persona: ""},
	}}
	if err := cfg.validate(); err != nil {
		t.Fatalf("expected no error for project with empty persona, got %v", err)
	}
}

func TestValidatePersonaRefBuiltin(t *testing.T) {
	cfg := &Config{}
	for _, p := range []PersonaType{PersonaVibe, PersonaPOC, PersonaScale} {
		if err := cfg.ValidatePersonaRef(p); err != nil {
			t.Fatalf("expected builtin persona %q to be accepted, got %v", p, err)
		}
	}
}

func TestValidatePersonaRefNone(t *testing.T) {
	cfg := &Config{}
	if err := cfg.ValidatePersonaRef(PersonaNone); err != nil {
		t.Fatalf("expected empty persona ref to be accepted, got %v", err)
	}
}

func TestValidatePersonaRefCustom(t *testing.T) {
	cfg := &Config{
		Personas: []CustomPersona{
			{Name: "my-custom", Label: "My Custom", Instructions: "do stuff"},
		},
	}
	if err := cfg.ValidatePersonaRef(PersonaType("my-custom")); err != nil {
		t.Fatalf("expected custom persona 'my-custom' to be accepted, got %v", err)
	}
}

func TestValidatePersonaRefUnknown(t *testing.T) {
	cfg := &Config{}
	if err := cfg.ValidatePersonaRef(PersonaType("nonexistent")); err == nil {
		t.Fatal("expected error for unknown persona ref 'nonexistent'")
	}
}

func TestValidateCustomPersonaMissingName(t *testing.T) {
	cfg := &Config{
		Personas: []CustomPersona{
			{Name: "", Label: "Bad", Instructions: "text"},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for custom persona with empty name")
	}
}

func TestValidateCustomPersonaDuplicateName(t *testing.T) {
	cfg := &Config{
		Personas: []CustomPersona{
			{Name: "dupe", Label: "First", Instructions: "text1"},
			{Name: "dupe", Label: "Second", Instructions: "text2"},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for duplicate custom persona name")
	}
}

func TestValidateCustomPersonaMissingLabel(t *testing.T) {
	cfg := &Config{
		Personas: []CustomPersona{
			{Name: "nolabel", Label: "", Instructions: "text"},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for custom persona with empty label")
	}
}

func TestValidateCustomPersonaMissingInstructions(t *testing.T) {
	cfg := &Config{
		Personas: []CustomPersona{
			{Name: "noinstr", Label: "No Instr", Instructions: ""},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for custom persona with empty instructions")
	}
}

func TestValidateCustomPersonaInvalidApproval(t *testing.T) {
	cfg := &Config{
		Personas: []CustomPersona{
			{Name: "badapproval", Label: "Bad", Instructions: "text", AutoApprove: ApprovalLevel("yolo")},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for custom persona with invalid auto_approve")
	}
}

func TestSaveLoadRoundTripWithPersona(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{
		Projects: []Project{
			{Name: "proj1", Repo: "/tmp/proj1", Agent: AgentClaudeCode, Persona: PersonaVibe},
			{Name: "proj2", Repo: "/tmp/proj2", Agent: AgentOpenCode, Persona: PersonaType("my-custom")},
		},
		Personas: []CustomPersona{
			{Name: "my-custom", Label: "My Custom Persona", Instructions: "be creative", AutoApprove: ApprovalSafe},
		},
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(loaded.Projects))
	}
	if loaded.Projects[0].Persona != PersonaVibe {
		t.Fatalf("project 0 persona: got %q, want %q", loaded.Projects[0].Persona, PersonaVibe)
	}
	if loaded.Projects[1].Persona != PersonaType("my-custom") {
		t.Fatalf("project 1 persona: got %q, want %q", loaded.Projects[1].Persona, "my-custom")
	}

	if len(loaded.Personas) != 1 {
		t.Fatalf("expected 1 custom persona, got %d", len(loaded.Personas))
	}
	cp := loaded.Personas[0]
	if cp.Name != "my-custom" {
		t.Errorf("custom persona name: got %q, want %q", cp.Name, "my-custom")
	}
	if cp.Label != "My Custom Persona" {
		t.Errorf("custom persona label: got %q, want %q", cp.Label, "My Custom Persona")
	}
	if cp.Instructions != "be creative" {
		t.Errorf("custom persona instructions: got %q, want %q", cp.Instructions, "be creative")
	}
	if cp.AutoApprove != ApprovalSafe {
		t.Errorf("custom persona auto_approve: got %q, want %q", cp.AutoApprove, ApprovalSafe)
	}
}

func TestLoadConfigWithoutPersonaField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `projects:
  - name: legacy
    repo: /tmp/legacy
    agent: claude-code
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed for YAML without persona field: %v", err)
	}
	if cfg.Projects[0].Persona != PersonaNone {
		t.Fatalf("expected empty persona for legacy config, got %q", cfg.Projects[0].Persona)
	}
	if len(cfg.Personas) != 0 {
		t.Fatalf("expected 0 custom personas for legacy config, got %d", len(cfg.Personas))
	}
}
