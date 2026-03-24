// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openconductorhq/openconductor/internal/config"
)

func sendKey(t *testing.T, m formModel, k tea.KeyType) (formModel, tea.Cmd) {
	t.Helper()
	return m.Update(tea.KeyMsg{Type: k})
}

func sendRune(t *testing.T, m formModel, r rune) (formModel, tea.Cmd) {
	t.Helper()
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

func newTestForm(existingNames ...string) formModel {
	m, _ := newFormModel(existingNames, nil)
	return m
}

func TestFormStartsAtStepName(t *testing.T) {
	m := newTestForm()
	if m.step != stepName {
		t.Fatalf("expected stepName, got %d", m.step)
	}
}

func TestFormNameValidationEmpty(t *testing.T) {
	m := newTestForm()
	m, _ = sendKey(t, m, tea.KeyEnter)
	if m.err == "" {
		t.Fatal("expected error for empty name")
	}
	if m.step != stepName {
		t.Fatal("should remain on stepName")
	}
}

func TestFormNameValidationDuplicate(t *testing.T) {
	m := newTestForm("existing")
	m.nameInput.SetValue("existing")
	m, _ = sendKey(t, m, tea.KeyEnter)
	if m.err == "" {
		t.Fatal("expected error for duplicate name")
	}
	if m.step != stepName {
		t.Fatal("should remain on stepName")
	}
}

func TestFormAdvanceToRepo(t *testing.T) {
	m := newTestForm()
	m.nameInput.SetValue("myproject")
	m, _ = sendKey(t, m, tea.KeyEnter)
	if m.err != "" {
		t.Fatalf("unexpected error: %s", m.err)
	}
	if m.step != stepRepo {
		t.Fatalf("expected stepRepo, got %d", m.step)
	}
}

func TestFormRepoValidationEmpty(t *testing.T) {
	m := newTestForm()
	m.nameInput.SetValue("myproject")
	m, _ = sendKey(t, m, tea.KeyEnter) // advance to stepRepo
	m, _ = sendKey(t, m, tea.KeyEnter) // submit empty repo
	if m.err == "" {
		t.Fatal("expected error for empty repo")
	}
	if m.step != stepRepo {
		t.Fatal("should remain on stepRepo")
	}
}

func TestFormRepoValidationBadPath(t *testing.T) {
	m := newTestForm()
	m.nameInput.SetValue("myproject")
	m, _ = sendKey(t, m, tea.KeyEnter)
	m.repoInput.SetValue("/nonexistent/path/that/does/not/exist")
	m, _ = sendKey(t, m, tea.KeyEnter)
	if m.err == "" {
		t.Fatal("expected error for non-existent path")
	}
	if m.step != stepRepo {
		t.Fatal("should remain on stepRepo")
	}
}

func TestFormAdvanceToAgent(t *testing.T) {
	m := newTestForm()
	m.nameInput.SetValue("myproject")
	m, _ = sendKey(t, m, tea.KeyEnter)
	m.repoInput.SetValue(t.TempDir()) // valid directory
	m, _ = sendKey(t, m, tea.KeyEnter)
	if m.err != "" {
		t.Fatalf("unexpected error: %s", m.err)
	}
	if m.step != stepAgent {
		t.Fatalf("expected stepAgent, got %d", m.step)
	}
}

func TestFormAgentJKNavigation(t *testing.T) {
	m := newTestForm()
	m.step = stepAgent
	m.agentIndex = 0

	// j moves down (2 agents: opencode=0, claude-code=1)
	m, _ = sendRune(t, m, 'j')
	if m.agentIndex != 1 {
		t.Fatalf("expected agentIndex 1, got %d", m.agentIndex)
	}
	// j at bottom stays
	m, _ = sendRune(t, m, 'j')
	if m.agentIndex != 1 {
		t.Fatalf("expected agentIndex 1 (clamped), got %d", m.agentIndex)
	}

	// k moves up
	m, _ = sendRune(t, m, 'k')
	if m.agentIndex != 0 {
		t.Fatalf("expected agentIndex 0, got %d", m.agentIndex)
	}
	// k at top stays
	m, _ = sendRune(t, m, 'k')
	if m.agentIndex != 0 {
		t.Fatalf("expected agentIndex 0 (clamped), got %d", m.agentIndex)
	}
}

func TestFormEscapeCancels(t *testing.T) {
	m := newTestForm()
	_, cmd := sendKey(t, m, tea.KeyEscape)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	if _, ok := msg.(FormCancelledMsg); !ok {
		t.Fatalf("expected FormCancelledMsg, got %T", msg)
	}
}

func TestFormAdvanceToPersona(t *testing.T) {
	m := newTestForm()
	m.step = stepAgent
	m.agentIndex = 0

	m, cmd := sendKey(t, m, tea.KeyEnter)
	if cmd != nil {
		t.Fatal("expected no command on stepAgent→stepPersona transition")
	}
	if m.step != stepPersona {
		t.Fatalf("expected stepPersona, got %d", m.step)
	}
}

func TestFormAdvanceToAutoApprove(t *testing.T) {
	m := newTestForm()
	m.step = stepPersona
	m.personaIndex = 0

	m, cmd := sendKey(t, m, tea.KeyEnter)
	if cmd != nil {
		t.Fatal("expected no command on stepPersona→stepAutoApprove transition")
	}
	if m.step != stepAutoApprove {
		t.Fatalf("expected stepAutoApprove, got %d", m.step)
	}
}

func TestFormAutoApproveJKNavigation(t *testing.T) {
	m := newTestForm()
	m.step = stepAutoApprove
	m.approvalIndex = 0 // Off

	// j moves down to Safe
	m, _ = sendRune(t, m, 'j')
	if m.approvalIndex != 1 {
		t.Fatalf("expected approvalIndex 1 (Safe), got %d", m.approvalIndex)
	}
	// j moves down to Full
	m, _ = sendRune(t, m, 'j')
	if m.approvalIndex != 2 {
		t.Fatalf("expected approvalIndex 2 (Full), got %d", m.approvalIndex)
	}
	// j at bottom stays
	m, _ = sendRune(t, m, 'j')
	if m.approvalIndex != 2 {
		t.Fatalf("expected approvalIndex 2 (clamped), got %d", m.approvalIndex)
	}
	// k moves up
	m, _ = sendRune(t, m, 'k')
	if m.approvalIndex != 1 {
		t.Fatalf("expected approvalIndex 1, got %d", m.approvalIndex)
	}
	m, _ = sendRune(t, m, 'k')
	if m.approvalIndex != 0 {
		t.Fatalf("expected approvalIndex 0, got %d", m.approvalIndex)
	}
	// k at top stays
	m, _ = sendRune(t, m, 'k')
	if m.approvalIndex != 0 {
		t.Fatalf("expected approvalIndex 0 (clamped), got %d", m.approvalIndex)
	}
}

func TestFormSubmit(t *testing.T) {
	m := newTestForm()
	m.step = stepAutoApprove
	m.nameInput.SetValue("myproject")
	m.repoInput.SetValue("/tmp")
	m.agentIndex = 1    // claude-code (index 1 in the reordered list)
	m.approvalIndex = 1 // safe

	_, cmd := sendKey(t, m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	added, ok := msg.(ProjectAddedMsg)
	if !ok {
		t.Fatalf("expected ProjectAddedMsg, got %T", msg)
	}
	if added.Project.Name != "myproject" {
		t.Fatalf("expected name 'myproject', got %q", added.Project.Name)
	}
	if added.Project.Agent != config.AgentClaudeCode {
		t.Fatalf("expected agent claude-code, got %q", added.Project.Agent)
	}
	if added.Project.AutoApprove != config.ApprovalSafe {
		t.Fatalf("expected auto_approve 'safe', got %q", added.Project.AutoApprove)
	}
}
