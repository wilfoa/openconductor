// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package attention

import (
	"context"
	"testing"

	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/permission"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeAutoApprover() *AutoApprover {
	// L1-only detector (no LLM classifier) — sufficient for Claude Code patterns.
	d := permission.NewDetector(nil)
	return NewAutoApprover(d)
}

func claudeProject(level config.ApprovalLevel) config.Project {
	return config.Project{
		Name:        "test-project",
		Repo:        "/tmp/test",
		Agent:       config.AgentClaudeCode,
		AutoApprove: level,
	}
}

// ── CheckAndApprove ───────────────────────────────────────────────────────────

func TestAutoApprove_Off_NeverApproves(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	lines := []string{"Allow editing of main.go? [y/n]"}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalOff), lines, adapter)
	if result.ShouldApprove {
		t.Fatal("ApprovalOff should never approve")
	}
}

func TestAutoApprove_Safe_ApprovesFileEdit(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	lines := []string{"Allow editing of src/main.go? [y/n]"}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalSafe), lines, adapter)
	if !result.ShouldApprove {
		t.Fatal("ApprovalSafe should approve file_edit")
	}
	// Claude Code keystroke should be "y\n"
	if string(result.Keystroke) != "y\n" {
		t.Fatalf("expected 'y\\n' keystroke, got %q", string(result.Keystroke))
	}
	if result.Parsed == nil {
		t.Fatal("Parsed should be non-nil")
	}
	if result.Parsed.Category != permission.FileEdit {
		t.Fatalf("expected FileEdit, got %s", result.Parsed.Category)
	}
}

func TestAutoApprove_Safe_BlocksFileDelete(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	lines := []string{"Allow deleting file tmp/old.log? [y/n]"}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalSafe), lines, adapter)
	if result.ShouldApprove {
		t.Fatal("ApprovalSafe should NOT approve file_delete")
	}
}

func TestAutoApprove_Full_ApprovesFileDelete(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	lines := []string{"Allow deleting file tmp/old.log? [y/n]"}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalFull), lines, adapter)
	if !result.ShouldApprove {
		t.Fatal("ApprovalFull should approve file_delete")
	}
}

func TestAutoApprove_Safe_ApprovesBashSafe(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	lines := []string{"Allow running bash command: git status? [y/n]"}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalSafe), lines, adapter)
	if !result.ShouldApprove {
		t.Fatal("ApprovalSafe should approve bash_safe (git)")
	}
}

func TestAutoApprove_Safe_BlocksBashAny(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	lines := []string{"Allow running bash command: rm -rf /tmp? [y/n]"}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalSafe), lines, adapter)
	if result.ShouldApprove {
		t.Fatal("ApprovalSafe should NOT approve bash_any (rm)")
	}
}

func TestAutoApprove_NoMatch_ReturnsNotApproved(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentClaudeCode)
	// Output that doesn't match any permission pattern.
	lines := []string{"✦ Thinking…", "Analyzing your codebase..."}

	result := aa.CheckAndApprove(context.Background(), claudeProject(config.ApprovalFull), lines, adapter)
	if result.ShouldApprove {
		t.Fatal("no-match should not approve")
	}
	if result.Parsed != nil {
		t.Fatal("no-match should have nil Parsed")
	}
}

func TestAutoApprove_OpenCode_ExternalDirectory_Safe(t *testing.T) {
	aa := makeAutoApprover()
	adapter, _ := agent.Get(config.AgentOpenCode)
	project := config.Project{
		Name:        "opencode-proj",
		Repo:        "/tmp/oc",
		Agent:       config.AgentOpenCode,
		AutoApprove: config.ApprovalSafe,
	}
	// Simulate the real OpenCode permission dialog from terminal capture.
	lines := simulateOpenCodeExternalDirPermission()
	result := aa.CheckAndApprove(context.Background(), project, lines, adapter)
	if !result.ShouldApprove {
		t.Fatal("ApprovalSafe should approve 'Access external directory' (FileRead)")
	}
	// OpenCode session keystroke is "A" (Allow always).
	if string(result.Keystroke) != "A" {
		t.Fatalf("expected OpenCode session keystroke 'A', got %q", string(result.Keystroke))
	}
}

func TestAutoApprove_OpenCode_Keystrokes(t *testing.T) {
	adapter, _ := agent.Get(config.AgentOpenCode)
	if string(adapter.ApproveKeystroke()) != "a" {
		t.Fatalf("expected 'a', got %q", adapter.ApproveKeystroke())
	}
	if string(adapter.ApproveSessionKeystroke()) != "A" {
		t.Fatalf("expected 'A', got %q", adapter.ApproveSessionKeystroke())
	}
	if string(adapter.DenyKeystroke()) != "d" {
		t.Fatalf("expected 'd', got %q", adapter.DenyKeystroke())
	}
}
