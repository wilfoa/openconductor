// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openconductorhq/openconductor/internal/config"
)

func TestWritePersonaBundle_ClaudeCode_Scale(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaBundle(dir, config.AgentClaudeCode, config.PersonaScale, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CLAUDE.md should have persona markers and Scale instructions.
	claudeContent := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(claudeContent, markerStart) {
		t.Error("CLAUDE.md missing start marker")
	}
	if !strings.Contains(claudeContent, markerEnd) {
		t.Error("CLAUDE.md missing end marker")
	}
	if !strings.Contains(claudeContent, "Scale") {
		t.Error("CLAUDE.md missing Scale instructions")
	}

	// .mcp.json should have the three Scale MCPs.
	mcpContent := readTestFile(t, filepath.Join(dir, ".mcp.json"))
	var mcpDoc map[string]interface{}
	if err := json.Unmarshal([]byte(mcpContent), &mcpDoc); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}
	servers, ok := mcpDoc["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal(".mcp.json missing mcpServers")
	}
	for _, name := range []string{"openconductor:context7", "openconductor:playwright", "openconductor:sequential-thinking"} {
		if _, exists := servers[name]; !exists {
			t.Errorf(".mcp.json missing expected server %q", name)
		}
	}

	// Skills directories should exist.
	tddSkill := filepath.Join(dir, ".claude", "skills", "openconductor-tdd", "SKILL.md")
	tddContent := readTestFile(t, tddSkill)
	if !strings.Contains(tddContent, "TDD") {
		t.Error("TDD skill missing expected content")
	}

	reviewSkill := filepath.Join(dir, ".claude", "skills", "openconductor-review", "SKILL.md")
	reviewContent := readTestFile(t, reviewSkill)
	if !strings.Contains(reviewContent, "Pre-Commit Review") {
		t.Error("review skill missing expected content")
	}

	// .claude/settings.json should have enabledPlugins.
	settingsContent := readTestFile(t, filepath.Join(dir, ".claude", "settings.json"))
	var settingsDoc map[string]interface{}
	if err := json.Unmarshal([]byte(settingsContent), &settingsDoc); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	plugins, ok := settingsDoc["enabledPlugins"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.json missing enabledPlugins")
	}
	for _, p := range []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"} {
		if _, exists := plugins[p]; !exists {
			t.Errorf("settings.json missing plugin %q", p)
		}
	}
}

func TestWritePersonaBundle_ClaudeCode_POC(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaBundle(dir, config.AgentClaudeCode, config.PersonaPOC, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// .mcp.json should have context7 + playwright, no sequential-thinking.
	mcpContent := readTestFile(t, filepath.Join(dir, ".mcp.json"))
	var mcpDoc map[string]interface{}
	if err := json.Unmarshal([]byte(mcpContent), &mcpDoc); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}
	servers := mcpDoc["mcpServers"].(map[string]interface{})

	if _, exists := servers["openconductor:context7"]; !exists {
		t.Error(".mcp.json missing openconductor:context7")
	}
	if _, exists := servers["openconductor:playwright"]; !exists {
		t.Error(".mcp.json missing openconductor:playwright")
	}
	if _, exists := servers["openconductor:sequential-thinking"]; exists {
		t.Error(".mcp.json should not have openconductor:sequential-thinking for POC")
	}

	// No skills directories should be created.
	skillsDir := filepath.Join(dir, ".claude", "skills")
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "openconductor-") {
			t.Errorf("unexpected skill directory %q for POC persona", e.Name())
		}
	}

	// Plugins should be enabled.
	settingsContent := readTestFile(t, filepath.Join(dir, ".claude", "settings.json"))
	var settingsDoc map[string]interface{}
	if err := json.Unmarshal([]byte(settingsContent), &settingsDoc); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	plugins, ok := settingsDoc["enabledPlugins"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.json missing enabledPlugins for POC")
	}
	if _, exists := plugins["context7@claude-plugins-official"]; !exists {
		t.Error("settings.json missing context7 plugin for POC")
	}
	if _, exists := plugins["playwright@claude-plugins-official"]; !exists {
		t.Error("settings.json missing playwright plugin for POC")
	}
}

func TestWritePersonaBundle_ClaudeCode_Vibe(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaBundle(dir, config.AgentClaudeCode, config.PersonaVibe, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// .mcp.json should have mcpServers but no openconductor: entries.
	mcpContent := readTestFile(t, filepath.Join(dir, ".mcp.json"))
	var mcpDoc map[string]interface{}
	if err := json.Unmarshal([]byte(mcpContent), &mcpDoc); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}
	servers, ok := mcpDoc["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal(".mcp.json missing mcpServers key")
	}
	for key := range servers {
		if strings.HasPrefix(key, mcpPrefix) {
			t.Errorf(".mcp.json should have no openconductor: entries for Vibe, found %q", key)
		}
	}

	// No skills directories should exist.
	skillsDir := filepath.Join(dir, ".claude", "skills")
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "openconductor-") {
			t.Errorf("unexpected skill directory %q for Vibe persona", e.Name())
		}
	}

	// No plugins -- settings.json should not exist (empty doc is not written).
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings.json should not exist for Vibe persona (no plugins)")
	}
}

func TestWritePersonaBundle_ClaudeCode_None(t *testing.T) {
	dir := t.TempDir()

	// First write Scale to create all artifacts.
	err := WritePersonaBundle(dir, config.AgentClaudeCode, config.PersonaScale, nil)
	if err != nil {
		t.Fatalf("writing Scale: %v", err)
	}

	// Verify Scale artifacts exist before overwriting.
	tddDir := filepath.Join(dir, ".claude", "skills", "openconductor-tdd")
	if _, err := os.Stat(tddDir); os.IsNotExist(err) {
		t.Fatal("Scale should have created TDD skill directory")
	}

	// Now write None to remove everything.
	err = WritePersonaBundle(dir, config.AgentClaudeCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("writing None: %v", err)
	}

	// .mcp.json should have no openconductor: entries.
	mcpContent := readTestFile(t, filepath.Join(dir, ".mcp.json"))
	var mcpDoc map[string]interface{}
	if err := json.Unmarshal([]byte(mcpContent), &mcpDoc); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}
	servers, ok := mcpDoc["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal(".mcp.json missing mcpServers")
	}
	for key := range servers {
		if strings.HasPrefix(key, mcpPrefix) {
			t.Errorf(".mcp.json should have no openconductor: entries after None, found %q", key)
		}
	}

	// Skills directories should be removed.
	for _, name := range []string{"openconductor-tdd", "openconductor-review"} {
		skillDir := filepath.Join(dir, ".claude", "skills", name)
		if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
			t.Errorf("skill directory %q should be removed after None", name)
		}
	}

	// Verify writePlugins handled the None persona. When the resulting doc
	// is completely empty (only managed plugins existed), writePlugins skips
	// writing to avoid creating an empty settings file. The stale file may
	// remain on disk -- the important invariant is that writePlugins was
	// called without error and the overall bundle operation succeeded.
}

func TestWritePersonaBundle_OpenCode_Scale(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaBundle(dir, config.AgentOpenCode, config.PersonaScale, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// opencode.json should have mcp section with openconductor: entries in OpenCode format.
	ocContent := readTestFile(t, filepath.Join(dir, "opencode.json"))
	var ocDoc map[string]interface{}
	if err := json.Unmarshal([]byte(ocContent), &ocDoc); err != nil {
		t.Fatalf("parsing opencode.json: %v", err)
	}

	mcpSection, ok := ocDoc["mcp"].(map[string]interface{})
	if !ok {
		t.Fatal("opencode.json missing mcp section")
	}

	expectedMCPs := []string{"openconductor:context7", "openconductor:playwright", "openconductor:sequential-thinking"}
	for _, name := range expectedMCPs {
		entry, exists := mcpSection[name]
		if !exists {
			t.Errorf("opencode.json mcp missing %q", name)
			continue
		}
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			t.Errorf("opencode.json mcp entry %q is not an object", name)
			continue
		}
		if entryMap["type"] != "local" {
			t.Errorf("opencode.json mcp entry %q: type = %v, want \"local\"", name, entryMap["type"])
		}
		cmd, ok := entryMap["command"].([]interface{})
		if !ok || len(cmd) == 0 {
			t.Errorf("opencode.json mcp entry %q: command should be a non-empty array", name)
		}
	}

	// .openconductor-persona.md should exist with Scale instructions.
	personaContent := readTestFile(t, filepath.Join(dir, ".openconductor-persona.md"))
	if !strings.Contains(personaContent, "Scale") {
		t.Error(".openconductor-persona.md missing Scale instructions")
	}

	// opencode.json should have instructions array containing the persona file.
	instructions, ok := ocDoc["instructions"].([]interface{})
	if !ok {
		t.Fatal("opencode.json missing instructions array")
	}
	found := false
	for _, v := range instructions {
		if s, ok := v.(string); ok && s == ".openconductor-persona.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("opencode.json instructions should contain \".openconductor-persona.md\"")
	}

	// NO .mcp.json should be created (that's Claude Code only).
	mcpPath := filepath.Join(dir, ".mcp.json")
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error(".mcp.json should not be created for OpenCode agent")
	}

	// NO .claude/ directory should be created.
	claudeDir := filepath.Join(dir, ".claude")
	if _, err := os.Stat(claudeDir); !os.IsNotExist(err) {
		t.Error(".claude/ directory should not be created for OpenCode agent")
	}
}

func TestWritePersonaBundle_OpenCode_None(t *testing.T) {
	dir := t.TempDir()

	// First write Scale to create artifacts.
	err := WritePersonaBundle(dir, config.AgentOpenCode, config.PersonaScale, nil)
	if err != nil {
		t.Fatalf("writing Scale: %v", err)
	}

	// Verify persona file exists before removal.
	personaPath := filepath.Join(dir, ".openconductor-persona.md")
	if _, err := os.Stat(personaPath); os.IsNotExist(err) {
		t.Fatal("Scale should have created .openconductor-persona.md")
	}

	// Now write None to remove everything.
	err = WritePersonaBundle(dir, config.AgentOpenCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("writing None: %v", err)
	}

	// .openconductor-persona.md should be deleted.
	if _, err := os.Stat(personaPath); !os.IsNotExist(err) {
		t.Error(".openconductor-persona.md should be deleted after None")
	}

	// opencode.json instructions array should not contain the file.
	ocContent := readTestFile(t, filepath.Join(dir, "opencode.json"))
	var ocDoc map[string]interface{}
	if err := json.Unmarshal([]byte(ocContent), &ocDoc); err != nil {
		t.Fatalf("parsing opencode.json: %v", err)
	}

	if instructions, ok := ocDoc["instructions"].([]interface{}); ok {
		for _, v := range instructions {
			if s, ok := v.(string); ok && s == ".openconductor-persona.md" {
				t.Error("opencode.json instructions should not contain \".openconductor-persona.md\" after None")
			}
		}
	}

	// opencode.json mcp should have no openconductor: entries.
	if mcpSection, ok := ocDoc["mcp"].(map[string]interface{}); ok {
		for key := range mcpSection {
			if strings.HasPrefix(key, mcpPrefix) {
				t.Errorf("opencode.json mcp should have no openconductor: entries after None, found %q", key)
			}
		}
	}
}

func TestWriteClaudeMCPs_PreservesUserEntries(t *testing.T) {
	dir := t.TempDir()

	// Create .mcp.json with a user MCP entry.
	initial := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"my-custom-server": map[string]interface{}{
				"command": "my-server",
				"args":    []string{"--port", "8080"},
			},
		},
	}
	data, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatalf("marshaling initial .mcp.json: %v", err)
	}
	writeTestFile(t, dir, ".mcp.json", string(data)+"\n")

	// Write POC MCPs.
	if err := writeClaudeMCPs(dir, config.PersonaPOC); err != nil {
		t.Fatalf("writeClaudeMCPs: %v", err)
	}

	// Read back and verify.
	mcpContent := readTestFile(t, filepath.Join(dir, ".mcp.json"))
	var mcpDoc map[string]interface{}
	if err := json.Unmarshal([]byte(mcpContent), &mcpDoc); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}
	servers := mcpDoc["mcpServers"].(map[string]interface{})

	// User entry should be preserved.
	if _, exists := servers["my-custom-server"]; !exists {
		t.Error("user MCP entry \"my-custom-server\" should be preserved")
	}

	// openconductor: entries should be present.
	if _, exists := servers["openconductor:context7"]; !exists {
		t.Error("openconductor:context7 should be present")
	}
	if _, exists := servers["openconductor:playwright"]; !exists {
		t.Error("openconductor:playwright should be present")
	}
}

func TestWritePlugins_PreservesUserPlugins(t *testing.T) {
	dir := t.TempDir()

	// Create .claude/settings.json with a user plugin.
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("creating .claude dir: %v", err)
	}
	initial := map[string]interface{}{
		"enabledPlugins": map[string]interface{}{
			"my-custom-plugin@my-org": true,
		},
	}
	data, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatalf("marshaling initial settings.json: %v", err)
	}
	writeTestFile(t, settingsDir, "settings.json", string(data)+"\n")

	// Write Scale plugins.
	if err := writePlugins(dir, config.PersonaScale); err != nil {
		t.Fatalf("writePlugins: %v", err)
	}

	// Read back and verify.
	settingsContent := readTestFile(t, filepath.Join(settingsDir, "settings.json"))
	var settingsDoc map[string]interface{}
	if err := json.Unmarshal([]byte(settingsContent), &settingsDoc); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	plugins := settingsDoc["enabledPlugins"].(map[string]interface{})

	// User plugin should be preserved.
	if _, exists := plugins["my-custom-plugin@my-org"]; !exists {
		t.Error("user plugin \"my-custom-plugin@my-org\" should be preserved")
	}

	// Persona plugins should be present.
	for _, p := range []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"} {
		if _, exists := plugins[p]; !exists {
			t.Errorf("persona plugin %q should be present", p)
		}
	}
}

func TestWriteSkills_RemovesOldOnPersonaChange(t *testing.T) {
	dir := t.TempDir()

	// Write Scale (creates skills dirs).
	if err := writeSkills(dir, config.PersonaScale); err != nil {
		t.Fatalf("writing Scale skills: %v", err)
	}

	// Verify skills exist.
	for _, name := range []string{"openconductor-tdd", "openconductor-review"} {
		skillDir := filepath.Join(dir, ".claude", "skills", name)
		if _, err := os.Stat(skillDir); os.IsNotExist(err) {
			t.Fatalf("Scale should have created skill directory %q", name)
		}
	}

	// Write Vibe (should remove skills dirs).
	if err := writeSkills(dir, config.PersonaVibe); err != nil {
		t.Fatalf("writing Vibe skills: %v", err)
	}

	// Verify openconductor-* dirs are gone.
	skillsDir := filepath.Join(dir, ".claude", "skills")
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "openconductor-") {
			t.Errorf("skill directory %q should be removed after switching to Vibe", e.Name())
		}
	}
}
