// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package permission

import (
	"context"
	"testing"

	"github.com/openconductorhq/openconductor/internal/config"
)

// ── IsAllowed ────────────────────────────────────────────────────────────────

func TestIsAllowed_Off(t *testing.T) {
	for _, cat := range []Category{FileRead, FileEdit, FileCreate, FileDelete, BashSafe, BashAny, MCPTools, Network} {
		if IsAllowed(config.ApprovalOff, cat) {
			t.Errorf("ApprovalOff should not allow %s", cat)
		}
	}
}

func TestIsAllowed_Safe(t *testing.T) {
	allowed := []Category{FileRead, FileEdit, FileCreate, BashSafe, MCPTools}
	for _, cat := range allowed {
		if !IsAllowed(config.ApprovalSafe, cat) {
			t.Errorf("ApprovalSafe should allow %s", cat)
		}
	}
	blocked := []Category{FileDelete, BashAny, Network}
	for _, cat := range blocked {
		if IsAllowed(config.ApprovalSafe, cat) {
			t.Errorf("ApprovalSafe should NOT allow %s", cat)
		}
	}
}

func TestIsAllowed_Full(t *testing.T) {
	for _, cat := range []Category{FileRead, FileEdit, FileCreate, FileDelete, BashSafe, BashAny, MCPTools, Network} {
		if !IsAllowed(config.ApprovalFull, cat) {
			t.Errorf("ApprovalFull should allow %s", cat)
		}
	}
}

// ── TryMatch — Claude Code ────────────────────────────────────────────────────

func TestTryMatch_ClaudeCode_FileEdit(t *testing.T) {
	lines := []string{
		"Allow editing of src/main.go? [y/n]",
	}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != FileEdit {
		t.Fatalf("expected FileEdit, got %s", p.Category)
	}
	if p.Source != "pattern" {
		t.Fatalf("expected source 'pattern', got %s", p.Source)
	}
	if p.Confidence != 1.0 {
		t.Fatalf("expected confidence 1.0, got %f", p.Confidence)
	}
}

func TestTryMatch_ClaudeCode_FileRead(t *testing.T) {
	lines := []string{"Allow reading file secrets.txt? [y/n]"}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != FileRead {
		t.Fatalf("expected FileRead, got %s", p.Category)
	}
}

func TestTryMatch_ClaudeCode_FileCreate(t *testing.T) {
	lines := []string{"Allow creating file .env? [y/n]"}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != FileCreate {
		t.Fatalf("expected FileCreate, got %s", p.Category)
	}
}

func TestTryMatch_ClaudeCode_FileDelete(t *testing.T) {
	lines := []string{"Allow deleting file tmp/old.log? [y/n]"}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != FileDelete {
		t.Fatalf("expected FileDelete, got %s", p.Category)
	}
}

func TestTryMatch_ClaudeCode_Network(t *testing.T) {
	lines := []string{"Allow fetching https://api.example.com? [y/n]"}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != Network {
		t.Fatalf("expected Network, got %s", p.Category)
	}
}

func TestTryMatch_ClaudeCode_BashSafe(t *testing.T) {
	lines := []string{"Allow running bash command: git status? [y/n]"}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != BashSafe {
		t.Fatalf("expected BashSafe (git is safe), got %s", p.Category)
	}
}

func TestTryMatch_ClaudeCode_BashAny(t *testing.T) {
	lines := []string{"Allow running bash command: rm -rf /tmp/cache? [y/n]"}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected a match")
	}
	if p.Category != BashAny {
		t.Fatalf("expected BashAny (rm is not safe), got %s", p.Category)
	}
}

func TestTryMatch_NoMatch(t *testing.T) {
	lines := []string{
		"✦ Thinking…",
		"Processing your request...",
	}
	p := TryMatch(config.AgentClaudeCode, lines)
	if p != nil {
		t.Fatalf("expected no match, got category %s", p.Category)
	}
}

// ── TryMatch — OpenCode ────────────────────────────────────────────────────
//
// Fixtures use content from a real OpenCode permission dialog terminal capture.

// simulateOpenCodeExternalDirLines returns the permission-relevant lines
// visible during the "Access external directory" dialog (from real screenshot).
func simulateOpenCodeExternalDirLines() []string {
	return []string{
		"Found 5 .env files. Let me copy each one to the corresponding location in your current project:",
		"$ cp /Users/amir/Downloads/.../rebuilding-bots/.env.sample /Users/amir/Development/.../rebuilding-bots/.env.sample",
		"",
		"■ Build · claude-opus-4-6",
		"",
		"⚠ Permission required",
		"← Access external directory ~/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots",
		"",
		"Patterns",
		"- /Users/amir/Downloads/drive/users/amir/Development/parlibot/rebuilding-bots/*",
		"",
		"Allow once  Allow always  Reject    ctrl+f fullscreen  ⌘ select  enter confirm",
	}
}

func TestTryMatch_OpenCode_ExternalDirectory(t *testing.T) {
	lines := simulateOpenCodeExternalDirLines()
	p := TryMatch(config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected a match for 'Access external directory'")
	}
	if p.Category != FileRead {
		t.Fatalf("expected FileRead for external directory access, got %s", p.Category)
	}
	if p.Source != "pattern" {
		t.Fatalf("expected source 'pattern', got %s", p.Source)
	}
}

func TestTryMatch_OpenCode_EditFile(t *testing.T) {
	lines := []string{
		"⚠ Permission required",
		"← Edit file src/handler.go",
		"",
		"Allow once  Allow always  Reject",
	}
	p := TryMatch(config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected a match for 'Edit file'")
	}
	if p.Category != FileEdit {
		t.Fatalf("expected FileEdit, got %s", p.Category)
	}
}

func TestTryMatch_OpenCode_WriteFile(t *testing.T) {
	lines := []string{
		"⚠ Permission required",
		"← Write file .env",
		"",
		"Allow once  Allow always  Reject",
	}
	p := TryMatch(config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected a match for 'Write file'")
	}
	if p.Category != FileCreate {
		t.Fatalf("expected FileCreate, got %s", p.Category)
	}
}

func TestTryMatch_OpenCode_ExecuteCommand(t *testing.T) {
	lines := []string{
		"⚠ Permission required",
		"← Execute command npm install",
		"",
		"Allow once  Allow always  Reject",
	}
	p := TryMatch(config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected a match for 'Execute command'")
	}
	// npm is a safe command
	if p.Category != BashSafe {
		t.Fatalf("expected BashSafe for npm, got %s", p.Category)
	}
}

func TestTryMatch_OpenCode_ExecuteUnsafeCommand(t *testing.T) {
	lines := []string{
		"⚠ Permission required",
		"← Execute command rm -rf /tmp/cache",
		"",
		"Allow once  Allow always  Reject",
	}
	p := TryMatch(config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected a match for 'Execute command rm'")
	}
	if p.Category != BashAny {
		t.Fatalf("expected BashAny for rm, got %s", p.Category)
	}
}

func TestTryMatch_OpenCode_FetchURL(t *testing.T) {
	lines := []string{
		"⚠ Permission required",
		"← Fetch URL https://api.example.com/data",
		"",
		"Allow once  Allow always  Reject",
	}
	p := TryMatch(config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected a match for 'Fetch URL'")
	}
	if p.Category != Network {
		t.Fatalf("expected Network, got %s", p.Category)
	}
}

func TestTryMatch_OpenCode_NoMatch_WorkingState(t *testing.T) {
	// Working state output — should not match any permission pattern.
	lines := []string{
		"   ┃  Analyzing the codebase for potential issues...",
		"   ┃",
		"   ┃  Looking at main.go, config.go, and server.go",
		"",
		"   · · · · ■ ■  esc interrupt",
	}
	p := TryMatch(config.AgentOpenCode, lines)
	if p != nil {
		t.Fatalf("working state should not match any permission pattern, got %s", p.Category)
	}
}

// ── DetectionModeFor ─────────────────────────────────────────────────────────

func TestDetectionModeFor_AllAgentsUseL1First(t *testing.T) {
	agents := []config.AgentType{
		config.AgentClaudeCode,
		config.AgentOpenCode,
		config.AgentCodex,
		config.AgentGemini,
	}
	for _, at := range agents {
		if DetectionModeFor(at) != ModeL1First {
			t.Fatalf("expected ModeL1First for %s", at)
		}
	}
}

// ── Detector ─────────────────────────────────────────────────────────────────

func TestDetector_L1Hit_NoClassifier(t *testing.T) {
	d := NewDetector(nil) // no L2 classifier
	lines := []string{"Allow editing of main.go? [y/n]"}
	p := d.Detect(context.Background(), "proj", config.AgentClaudeCode, lines)
	if p == nil {
		t.Fatal("expected result from L1 pattern match")
	}
	if p.Category != FileEdit {
		t.Fatalf("expected FileEdit, got %s", p.Category)
	}
}

func TestDetector_L1Miss_NoClassifier_ReturnsNil(t *testing.T) {
	d := NewDetector(nil)
	lines := []string{"✦ Thinking…"}
	p := d.Detect(context.Background(), "proj", config.AgentClaudeCode, lines)
	if p != nil {
		t.Fatalf("expected nil (no match, no classifier), got %+v", p)
	}
}

func TestDetector_OpenCode_PermissionDialog_L1Hit(t *testing.T) {
	// OpenCode now uses ModeL1First. A real permission dialog fixture should
	// be classified by the L1 pattern matcher without needing the LLM.
	d := NewDetector(nil) // no L2 classifier
	lines := simulateOpenCodeExternalDirLines()
	p := d.Detect(context.Background(), "proj", config.AgentOpenCode, lines)
	if p == nil {
		t.Fatal("expected L1 match for OpenCode permission dialog")
	}
	if p.Category != FileRead {
		t.Fatalf("expected FileRead, got %s", p.Category)
	}
	if p.Source != "pattern" {
		t.Fatalf("expected source 'pattern', got %s", p.Source)
	}
}

func TestDetector_OpenCode_WorkingState_NoMatch(t *testing.T) {
	// Working-state output should not match any permission pattern.
	d := NewDetector(nil)
	lines := []string{
		"   ┃  Analyzing the codebase...",
		"   · · · · ■ ■  esc interrupt",
	}
	p := d.Detect(context.Background(), "proj", config.AgentOpenCode, lines)
	if p != nil {
		t.Fatalf("working state should produce no match, got %+v", p)
	}
}

// ── parseClassifierResponse ───────────────────────────────────────────────────

func TestParseClassifierResponse_Valid(t *testing.T) {
	raw := `{"category":"file_edit","description":"Edit src/main.go","confidence":0.95}`
	p := parseClassifierResponse(raw)
	if p.Category != FileEdit {
		t.Fatalf("expected FileEdit, got %s", p.Category)
	}
	if p.Description != "Edit src/main.go" {
		t.Fatalf("expected description, got %q", p.Description)
	}
	if p.Confidence != 0.95 {
		t.Fatalf("expected 0.95, got %f", p.Confidence)
	}
	if p.Source != "llm" {
		t.Fatalf("expected source 'llm', got %s", p.Source)
	}
}

func TestParseClassifierResponse_MarkdownFences(t *testing.T) {
	raw := "```json\n{\"category\":\"bash_safe\",\"description\":\"git status\",\"confidence\":1.0}\n```"
	p := parseClassifierResponse(raw)
	if p.Category != BashSafe {
		t.Fatalf("expected BashSafe, got %s", p.Category)
	}
}

func TestParseClassifierResponse_InvalidJSON(t *testing.T) {
	p := parseClassifierResponse("not json at all")
	if p.Category != Unknown {
		t.Fatalf("expected Unknown on bad JSON, got %s", p.Category)
	}
	if p.Confidence != 0 {
		t.Fatalf("expected zero confidence on error, got %f", p.Confidence)
	}
}

func TestParseClassifierResponse_UnknownCategory(t *testing.T) {
	raw := `{"category":"something_made_up","description":"?","confidence":0.9}`
	p := parseClassifierResponse(raw)
	if p.Category != Unknown {
		t.Fatalf("expected Unknown for unrecognized category, got %s", p.Category)
	}
}

// ── refineBashCategory ────────────────────────────────────────────────────────

func TestRefineBashCategory_Safe(t *testing.T) {
	cat := refineBashCategory("Allow running bash command: git status")
	if cat != BashSafe {
		t.Fatalf("expected BashSafe for git, got %s", cat)
	}
}

func TestRefineBashCategory_Any(t *testing.T) {
	cat := refineBashCategory("Allow running bash command: rm -rf /")
	if cat != BashAny {
		t.Fatalf("expected BashAny for rm, got %s", cat)
	}
}

func TestRefineBashCategory_NoCommand(t *testing.T) {
	cat := refineBashCategory("Allow running something unrecognized")
	if cat != BashAny {
		t.Fatalf("expected BashAny when command not parseable, got %s", cat)
	}
}
