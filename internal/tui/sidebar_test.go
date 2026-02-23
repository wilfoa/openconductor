// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openconductorhq/openconductor/internal/config"
)

func sidebarSendKey(t *testing.T, m sidebarModel, k tea.KeyType) (sidebarModel, tea.Cmd) {
	t.Helper()
	return m.Update(tea.KeyMsg{Type: k})
}

func sidebarSendRune(t *testing.T, m sidebarModel, r rune) (sidebarModel, tea.Cmd) {
	t.Helper()
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

func sidebarSendMouse(t *testing.T, m sidebarModel, action tea.MouseAction, button tea.MouseButton, x, y int) (sidebarModel, tea.Cmd) {
	t.Helper()
	return m.Update(tea.MouseMsg{X: x, Y: y, Action: action, Button: button})
}

func testProjects() []config.Project {
	return []config.Project{
		{Name: "alpha", Repo: "/tmp/alpha", Agent: config.AgentClaudeCode},
		{Name: "beta", Repo: "/tmp/beta", Agent: config.AgentCodex},
		{Name: "gamma", Repo: "/tmp/gamma", Agent: config.AgentGemini},
	}
}

func TestSidebarIgnoresWhenUnfocused(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = false
	m.selected = 0

	m, cmd := sidebarSendRune(t, m, 'j')
	if cmd != nil {
		t.Fatal("expected nil cmd when unfocused")
	}
	if m.selected != 0 {
		t.Fatalf("expected selected 0, got %d", m.selected)
	}
}

func TestSidebarJKNavigation(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true

	m, _ = sidebarSendRune(t, m, 'j')
	if m.selected != 1 {
		t.Fatalf("expected selected 1, got %d", m.selected)
	}
	m, _ = sidebarSendRune(t, m, 'j')
	if m.selected != 2 {
		t.Fatalf("expected selected 2, got %d", m.selected)
	}
	// Clamp at bottom
	m, _ = sidebarSendRune(t, m, 'j')
	if m.selected != 2 {
		t.Fatalf("expected selected 2 (clamped), got %d", m.selected)
	}

	m, _ = sidebarSendRune(t, m, 'k')
	if m.selected != 1 {
		t.Fatalf("expected selected 1, got %d", m.selected)
	}
	m, _ = sidebarSendRune(t, m, 'k')
	if m.selected != 0 {
		t.Fatalf("expected selected 0, got %d", m.selected)
	}
	// Clamp at top
	m, _ = sidebarSendRune(t, m, 'k')
	if m.selected != 0 {
		t.Fatalf("expected selected 0 (clamped), got %d", m.selected)
	}
}

func TestSidebarEnterEmitsProjectSwitched(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true
	m.selected = 1

	_, cmd := sidebarSendKey(t, m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	switched, ok := msg.(ProjectSwitchedMsg)
	if !ok {
		t.Fatalf("expected ProjectSwitchedMsg, got %T", msg)
	}
	if switched.Index != 1 {
		t.Fatalf("expected index 1, got %d", switched.Index)
	}
	if switched.Project.Name != "beta" {
		t.Fatalf("expected 'beta', got %q", switched.Project.Name)
	}
}

func TestSidebarAOpensForm(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true

	m, _ = sidebarSendRune(t, m, 'a')
	if m.mode != sidebarForm {
		t.Fatalf("expected sidebarForm mode, got %d", m.mode)
	}
}

func TestSidebarDStartsConfirmDelete(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true
	m.selected = 0

	m, _ = sidebarSendRune(t, m, 'd')
	if m.mode != sidebarConfirmDelete {
		t.Fatalf("expected sidebarConfirmDelete mode, got %d", m.mode)
	}
}

func TestSidebarConfirmDeleteY(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true
	m.selected = 1
	m.mode = sidebarConfirmDelete

	_, cmd := sidebarSendRune(t, m, 'y')
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	deleted, ok := msg.(ProjectDeletedMsg)
	if !ok {
		t.Fatalf("expected ProjectDeletedMsg, got %T", msg)
	}
	if deleted.Name != "beta" {
		t.Fatalf("expected 'beta', got %q", deleted.Name)
	}
}

func TestSidebarConfirmDeleteEscape(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true
	m.mode = sidebarConfirmDelete

	m, _ = sidebarSendKey(t, m, tea.KeyEscape)
	if m.mode != sidebarNormal {
		t.Fatalf("expected sidebarNormal mode, got %d", m.mode)
	}
}

func TestSidebarMouseClickProject(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true

	// Project 1 starts at Y = sidebarTopPad + sidebarTitleRows + 1*projectRows = 1 + 2 + 2 = 5
	clickY := sidebarTopPad + sidebarTitleRows + 1*projectRows
	_, cmd := sidebarSendMouse(t, m, tea.MouseActionPress, tea.MouseButtonLeft, 5, clickY)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	switched, ok := msg.(ProjectSwitchedMsg)
	if !ok {
		t.Fatalf("expected ProjectSwitchedMsg, got %T", msg)
	}
	if switched.Index != 1 {
		t.Fatalf("expected index 1, got %d", switched.Index)
	}
}

func TestSidebarMouseClickAddButton(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true

	addY := m.addButtonY()
	m, _ = sidebarSendMouse(t, m, tea.MouseActionPress, tea.MouseButtonLeft, 5, addY)
	if m.mode != sidebarForm {
		t.Fatalf("expected sidebarForm mode, got %d", m.mode)
	}
}

func TestSidebarMouseWheelScroll(t *testing.T) {
	m := newSidebarModel(testProjects(), defaultSidebarWidth)
	m.focused = true
	m.selected = 1

	m, _ = sidebarSendMouse(t, m, tea.MouseActionPress, tea.MouseButtonWheelDown, 0, 0)
	if m.selected != 2 {
		t.Fatalf("expected selected 2 after wheel down, got %d", m.selected)
	}
	m, _ = sidebarSendMouse(t, m, tea.MouseActionPress, tea.MouseButtonWheelUp, 0, 0)
	if m.selected != 1 {
		t.Fatalf("expected selected 1 after wheel up, got %d", m.selected)
	}
}

func TestSidebarEmptyStateView(t *testing.T) {
	m := newSidebarModel(nil, defaultSidebarWidth)
	m.focused = true
	m.height = 20

	view := m.View()
	if !strings.Contains(view, "No projects") {
		t.Fatalf("expected empty state hint, got:\n%s", view)
	}
	if !strings.Contains(view, "new project") {
		t.Fatalf("expected add button hint, got:\n%s", view)
	}
}

// TestSidebarStateLabelOnlyWithOpenTab verifies that the state label (idle,
// working, attention, etc.) is shown ONLY for projects with an open tab.
// Projects without an open tab show just the agent name.
func TestSidebarStateLabelOnlyWithOpenTab(t *testing.T) {
	projects := testProjects()
	m := newSidebarModel(projects, defaultSidebarWidth)
	m.focused = true
	m.height = 30
	m.selected = 0 // alpha is selected

	// Set different states for each project
	m.states["alpha"] = StateWorking
	m.states["beta"] = StateNeedsAttention
	m.states["gamma"] = StateIdle

	// Only alpha has an open tab
	m.openTabs["alpha"] = true

	view := m.View()

	// Alpha has open tab - state label should be visible
	if !strings.Contains(view, "working") {
		t.Errorf("expected 'working' state label for alpha (has open tab), got:\n%s", view)
	}

	// Beta has no open tab - "attention" state label should NOT be visible
	if strings.Contains(view, "attention") {
		t.Errorf("expected NO 'attention' state label for beta (no open tab), got:\n%s", view)
	}
}
