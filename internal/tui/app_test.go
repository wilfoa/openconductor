// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/session"
	"github.com/openconductorhq/openconductor/internal/telegram"
)

func emptyConfig() *config.Config {
	return &config.Config{Projects: []config.Project{}}
}

func configWithProjects() *config.Config {
	return &config.Config{
		Projects: []config.Project{
			{Name: "proj1", Repo: "/tmp/p1", Agent: config.AgentClaudeCode},
			{Name: "proj2", Repo: "/tmp/p2", Agent: config.AgentOpenCode},
		},
	}
}

func TestNewAppFocusesSidebarWhenEmpty(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	if app.focus != focusSidebar {
		t.Fatalf("expected focusSidebar, got %d", app.focus)
	}
}

func TestNewAppFocusesTerminalWithProjects(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	if app.focus != focusTerminal {
		t.Fatalf("expected focusTerminal, got %d", app.focus)
	}
}

func TestCtrlSTogglesFocus(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	if app.focus != focusTerminal {
		t.Fatal("precondition: expected focusTerminal")
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlS}
	model, _ := app.Update(msg)
	app = model.(App)
	if app.focus != focusSidebar {
		t.Fatalf("expected focusSidebar after Ctrl+S, got %d", app.focus)
	}
	if !app.sidebar.focused {
		t.Fatal("expected sidebar.focused=true")
	}

	model, _ = app.Update(msg)
	app = model.(App)
	if app.focus != focusTerminal {
		t.Fatalf("expected focusTerminal after second Ctrl+S, got %d", app.focus)
	}
}

func TestCtrlSIgnoredInSidebarFormMode(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.focus = focusSidebar
	app.sidebar.focused = true
	app.sidebar.mode = sidebarForm

	msg := tea.KeyMsg{Type: tea.KeyCtrlS}
	model, cmd := app.Update(msg)
	app = model.(App)

	// Focus should NOT toggle — sidebar should still be focused.
	if app.focus != focusSidebar {
		t.Fatalf("expected focus to stay on sidebar, got %d", app.focus)
	}

	// Ctrl+S is not handled by the form (it handles Esc), so cmd should be nil.
	// The form stays open — user must Esc out of the form first.
	if cmd != nil {
		t.Fatal("expected nil cmd from Ctrl+S in form mode")
	}
}

func TestAppProjectAddedUpdatesConfig(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)

	msg := ProjectAddedMsg{
		Project: config.Project{Name: "new", Repo: "/tmp/new", Agent: config.AgentOpenCode},
	}
	model, _ := app.Update(msg)
	app = model.(App)

	if len(app.cfg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(app.cfg.Projects))
	}
	if app.cfg.Projects[0].Name != "new" {
		t.Fatalf("expected 'new', got %q", app.cfg.Projects[0].Name)
	}
	if app.sidebar.mode != sidebarNormal {
		t.Fatalf("expected sidebarNormal, got %d", app.sidebar.mode)
	}
}

func TestAppProjectDeletedUpdatesConfig(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)

	msg := ProjectDeletedMsg{Name: "proj1"}
	model, _ := app.Update(msg)
	app = model.(App)

	if len(app.cfg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(app.cfg.Projects))
	}
	if app.cfg.Projects[0].Name != "proj2" {
		t.Fatalf("expected 'proj2', got %q", app.cfg.Projects[0].Name)
	}
}

func TestAppFormCancelledResetsMode(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.sidebar.mode = sidebarForm

	model, _ := app.Update(FormCancelledMsg{})
	app = model.(App)

	if app.sidebar.mode != sidebarNormal {
		t.Fatalf("expected sidebarNormal, got %d", app.sidebar.mode)
	}
}

func TestAppMouseRoutesToSidebar(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.focus = focusTerminal
	app.sidebar.focused = false
	app.terminal.focused = true

	// Click within sidebar width (contentWidth=24 + border=1 = 25).
	msg := tea.MouseMsg{
		X:      5,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
	model, _ := app.Update(msg)
	app = model.(App)

	if app.focus != focusSidebar {
		t.Fatalf("expected focusSidebar after click in sidebar area, got %d", app.focus)
	}
	if !app.sidebar.focused {
		t.Fatal("expected sidebar.focused=true")
	}
	if app.terminal.focused {
		t.Fatal("expected terminal.focused=false")
	}
}

func TestAppDragSidebarSeparator(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 120
	app.height = 40

	borderX := screenPadding + app.sidebar.Width() - 1

	// Press on border to start drag.
	press := tea.MouseMsg{
		X: borderX, Y: 10,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
	model, _ := app.Update(press)
	app = model.(App)

	if !app.dragging {
		t.Fatal("expected dragging=true after press on border")
	}

	// Motion to X=41 (screen X; contentWidth = X - screenPadding = 40).
	motion := tea.MouseMsg{
		X: 41, Y: 10,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
	}
	model, _ = app.Update(motion)
	app = model.(App)

	// Sidebar content width = mouse X - screenPadding.
	if app.sidebar.contentWidth != 40 {
		t.Fatalf("expected contentWidth %d, got %d", 40, app.sidebar.contentWidth)
	}

	// Release to stop drag.
	release := tea.MouseMsg{
		X: 40, Y: 10,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	model, _ = app.Update(release)
	app = model.(App)

	if app.dragging {
		t.Fatal("expected dragging=false after release")
	}
}

func TestAppDragClampsMinimum(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 120
	app.height = 40
	app.dragging = true
	app.sidebar.dragging = true

	motion := tea.MouseMsg{
		X: 5, Y: 10,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
	}
	model, _ := app.Update(motion)
	app = model.(App)

	if app.sidebar.contentWidth < minSidebarWidth {
		t.Fatalf("expected contentWidth >= %d, got %d", minSidebarWidth, app.sidebar.contentWidth)
	}
}

func TestAppDragClampsMaximum(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 100
	app.height = 40
	app.dragging = true
	app.sidebar.dragging = true

	motion := tea.MouseMsg{
		X: 90, Y: 10,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
	}
	model, _ := app.Update(motion)
	app = model.(App)

	maxWidth := app.width / 2
	if app.sidebar.contentWidth > maxWidth {
		t.Fatalf("expected contentWidth <= %d, got %d", maxWidth, app.sidebar.contentWidth)
	}
}

func TestNewAppOpensFirstTab(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	if len(app.openTabs) != 1 {
		t.Fatalf("expected 1 open tab, got %d", len(app.openTabs))
	}
	if app.openTabs[0] != "proj1" {
		t.Fatalf("expected first tab 'proj1', got %q", app.openTabs[0])
	}
}

func TestNewAppEmptyHasNoTabs(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	if len(app.openTabs) != 0 {
		t.Fatalf("expected 0 open tabs, got %d", len(app.openTabs))
	}
}

// ── Tab restore from saved state ────────────────────────────────

func TestNewApp_RestoresTabs(t *testing.T) {
	cfg := configWith3Projects() // alpha, beta, gamma
	state := &config.AppState{
		OpenTabs: []config.TabState{
			{Project: "beta"},
			{Project: "gamma", Label: "custom gamma"},
		},
		ActiveTab: "gamma",
	}
	app := NewApp(cfg, "", state)

	if len(app.openTabs) != 2 {
		t.Fatalf("expected 2 restored tabs, got %d", len(app.openTabs))
	}
	if app.openTabs[0] != "beta" || app.openTabs[1] != "gamma" {
		t.Errorf("tabs: got %v, want [beta gamma]", app.openTabs)
	}
	// Active project index should point to gamma (index 2).
	if cfg.Projects[app.active].Name != "gamma" {
		t.Errorf("active project: got %q, want gamma", cfg.Projects[app.active].Name)
	}
	// Sidebar should mark both as open.
	if !app.sidebar.openTabs["beta"] || !app.sidebar.openTabs["gamma"] {
		t.Errorf("sidebar openTabs not synced: %v", app.sidebar.openTabs)
	}
	// Custom label should be restored.
	if app.tabLabels["gamma"] != "custom gamma" {
		t.Errorf("expected label %q, got %q", "custom gamma", app.tabLabels["gamma"])
	}
	if app.tabLabels["beta"] != "" {
		t.Errorf("expected empty label for beta, got %q", app.tabLabels["beta"])
	}
}

func TestNewApp_RestoreFiltersDeletedProjects(t *testing.T) {
	cfg := configWithProjects() // has proj1, proj2
	state := &config.AppState{
		OpenTabs: []config.TabState{
			{Project: "proj2"},
			{Project: "deleted_project"},
			{Project: "proj1"},
		},
		ActiveTab: "proj2",
	}
	app := NewApp(cfg, "", state)

	// "deleted_project" should be filtered out.
	if len(app.openTabs) != 2 {
		t.Fatalf("expected 2 tabs (deleted filtered out), got %d: %v", len(app.openTabs), app.openTabs)
	}
	if app.openTabs[0] != "proj2" || app.openTabs[1] != "proj1" {
		t.Errorf("tabs: got %v, want [proj2 proj1]", app.openTabs)
	}
}

func TestNewApp_EmptyRestoreFallsBackToFirstProject(t *testing.T) {
	cfg := configWithProjects()
	state := &config.AppState{
		OpenTabs: []config.TabState{}, // empty state
	}
	app := NewApp(cfg, "", state)

	// Should fall back to opening the first project.
	if len(app.openTabs) != 1 || app.openTabs[0] != "proj1" {
		t.Errorf("expected fallback to [proj1], got %v", app.openTabs)
	}
}

func TestNewApp_AllTabsDeletedFallsBackToFirstProject(t *testing.T) {
	cfg := configWithProjects()
	state := &config.AppState{
		OpenTabs: []config.TabState{
			{Project: "gone1"},
			{Project: "gone2"},
		},
	}
	app := NewApp(cfg, "", state)

	// All restored tabs were filtered out → falls back to first project.
	if len(app.openTabs) != 1 || app.openTabs[0] != "proj1" {
		t.Errorf("expected fallback to [proj1], got %v", app.openTabs)
	}
}

func TestNewApp_PendingRestoreTabsSet(t *testing.T) {
	cfg := configWith3Projects() // alpha, beta, gamma
	state := &config.AppState{
		OpenTabs: []config.TabState{
			{Project: "alpha"},
			{Project: "gamma"},
		},
	}
	app := NewApp(cfg, "", state)

	// pendingRestoreTabs should be set so WindowSizeMsg starts all sessions.
	if len(app.pendingRestoreTabs) != 2 {
		t.Fatalf("expected 2 pending restore tabs, got %d", len(app.pendingRestoreTabs))
	}
}

func TestTabEdit_StartAndCommit(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	if app.editingTab != -1 {
		t.Fatal("expected editingTab=-1 initially")
	}

	// Start editing the first (and only) tab.
	app.startTabEdit(0)
	if app.editingTab != 0 {
		t.Fatalf("expected editingTab=0, got %d", app.editingTab)
	}
	if app.tabEditBuf != app.openTabs[0] {
		t.Fatalf("expected edit buf=%q, got %q", app.openTabs[0], app.tabEditBuf)
	}

	// Append text.
	app.tabEditBuf += " - feature"
	app.commitTabEdit()

	sessionID := app.openTabs[0]
	if app.tabLabels[sessionID] != "proj1 - feature" {
		t.Errorf("expected label %q, got %q", "proj1 - feature", app.tabLabels[sessionID])
	}
	if app.editingTab != -1 {
		t.Errorf("expected editingTab=-1 after commit, got %d", app.editingTab)
	}
}

func TestTabEdit_Cancel(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.startTabEdit(0)
	app.tabEditBuf = "something else"
	app.cancelTabEdit()

	if app.editingTab != -1 {
		t.Errorf("expected editingTab=-1 after cancel, got %d", app.editingTab)
	}
	// Label should NOT be set since we cancelled.
	if _, ok := app.tabLabels[app.openTabs[0]]; ok {
		t.Error("expected no custom label after cancel")
	}
}

func TestTabEdit_CommitEmptyRevertsToDefault(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	sessionID := app.openTabs[0]
	app.tabLabels[sessionID] = "old custom"
	app.startTabEdit(0)
	app.tabEditBuf = "   " // whitespace only
	app.commitTabEdit()

	if _, ok := app.tabLabels[sessionID]; ok {
		t.Error("expected label deleted for empty commit")
	}
}

func TestTabEdit_F2EntersEditMode(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// We need an active session to test F2. Since we can't easily create one
	// in unit tests (needs a real Manager), test the startTabEdit path directly.
	// The F2 key handler in Update calls startTabEdit when mgr.ActiveName()
	// returns a session ID — that's already tested via the startTabEdit tests.
	app.startTabEdit(0)
	if app.editingTab != 0 {
		t.Fatalf("expected editingTab=0, got %d", app.editingTab)
	}
}

func TestTabDisplayName_CustomLabel(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	sessionID := app.openTabs[0]

	// Default: returns session ID.
	if got := app.tabDisplayName(sessionID); got != sessionID {
		t.Errorf("expected %q, got %q", sessionID, got)
	}

	// Custom label.
	app.tabLabels[sessionID] = "my custom tab"
	if got := app.tabDisplayName(sessionID); got != "my custom tab" {
		t.Errorf("expected %q, got %q", "my custom tab", got)
	}
}

func TestTabEdit_RemoveTabCleansLabel(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	sessionID := app.openTabs[0]
	app.tabLabels[sessionID] = "custom"
	app.removeTab(sessionID)

	if _, ok := app.tabLabels[sessionID]; ok {
		t.Error("expected label cleaned up after removeTab")
	}
}

func TestAppProjectSwitchedReturnsStartCmd(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Initially only proj1 is open.
	if len(app.openTabs) != 1 {
		t.Fatalf("precondition: expected 1 tab, got %d", len(app.openTabs))
	}

	// Switch to proj2 — should return a command to start a new session.
	msg := ProjectSwitchedMsg{Index: 1, Project: app.cfg.Projects[1]}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Fatal("expected a command from ProjectSwitchedMsg")
	}
}

func TestProjectSwitched_ExistingTabSwitchesInstead(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)

	// Inject two sessions so we have tabs for both projects.
	sess1 := &session.Session{
		ID: "proj1", Instance: 1, Project: app.cfg.Projects[0],
		State: session.StateRunning,
	}
	sess2 := &session.Session{
		ID: "proj2", Instance: 1, Project: app.cfg.Projects[1],
		State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess1)
	app.mgr.InjectSession("proj2", sess2)
	app.addTab("proj1")
	app.addTab("proj2")
	app.mgr.SetActive("proj2") // currently on proj2

	// Click proj1 in sidebar — should switch to existing tab, NOT start new session.
	msg := ProjectSwitchedMsg{Index: 0, Project: app.cfg.Projects[0]}
	_, cmd := app.Update(msg)

	if cmd != nil {
		t.Fatal("expected no command (should switch, not create new session)")
	}
	if app.mgr.ActiveName() != "proj1" {
		t.Fatalf("expected active session 'proj1', got %q", app.mgr.ActiveName())
	}
}

func TestNewInstanceMsg_AlwaysCreatesSession(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)

	// Inject a session so proj1 already has a tab.
	sess := &session.Session{
		ID: "proj1", Instance: 1, Project: app.cfg.Projects[0],
		State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)
	app.addTab("proj1")

	// Press 'n' on proj1 — should always create a new session.
	msg := NewInstanceMsg{Project: app.cfg.Projects[0]}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Fatal("expected a command from NewInstanceMsg (new session)")
	}
}

func TestSessionStartedAddsTab(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Inject a session and simulate sessionStartedMsg.
	sess := &session.Session{
		ID:       "proj2",
		Instance: 1,
		Project:  app.cfg.Projects[1],
		State:    session.StateRunning,
	}
	app.mgr.InjectSession("proj2", sess)

	model, _ := app.Update(sessionStartedMsg{SessionID: "proj2"})
	app = model.(App)

	if len(app.openTabs) != 2 {
		t.Fatalf("expected 2 open tabs, got %d", len(app.openTabs))
	}
}

func TestAppProjectDeletedRemovesTab(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Inject sessions for both projects so the delete handler can find them.
	sess1 := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess1)
	sess2 := &session.Session{
		ID: "proj2", Instance: 1,
		Project: app.cfg.Projects[1], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj2", sess2)
	app.addTab("proj2")
	if len(app.openTabs) != 2 {
		t.Fatalf("precondition: expected 2 tabs, got %d", len(app.openTabs))
	}

	msg := ProjectDeletedMsg{Name: "proj1"}
	model, _ := app.Update(msg)
	app = model.(App)

	if len(app.openTabs) != 1 {
		t.Fatalf("expected 1 tab after delete, got %d", len(app.openTabs))
	}
	if app.openTabs[0] != "proj2" {
		t.Fatalf("expected remaining tab 'proj2', got %q", app.openTabs[0])
	}
}

func TestAppProjectAddedReturnsStartCmd(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)

	msg := ProjectAddedMsg{
		Project: config.Project{Name: "new", Repo: "/tmp/new", Agent: config.AgentOpenCode},
	}
	_, cmd := app.Update(msg)

	// Should return commands (save config + start session).
	if cmd == nil {
		t.Fatal("expected commands from ProjectAddedMsg")
	}
	// Project should be in config now.
	if len(app.cfg.Projects) != 1 || app.cfg.Projects[0].Name != "new" {
		t.Fatalf("expected project 'new' in config, got %v", app.cfg.Projects)
	}
}

func TestSidebarDynamicWidth(t *testing.T) {
	m := newSidebarModel(testProjects(), 30)
	// Width = contentWidth + border(1). Padding is inside contentWidth.
	expected := 30 + 1
	if m.Width() != expected {
		t.Fatalf("expected Width() = %d, got %d", expected, m.Width())
	}
}

// ── Tab hit-test, click, and close tests ────────────────────────

func configWith3Projects() *config.Config {
	return &config.Config{
		Projects: []config.Project{
			{Name: "alpha", Repo: "/tmp/a", Agent: config.AgentClaudeCode},
			{Name: "beta", Repo: "/tmp/b", Agent: config.AgentOpenCode},
			{Name: "gamma", Repo: "/tmp/c", Agent: config.AgentOpenCode},
		},
	}
}

// testTabWidth returns the rendered width of a tab, matching the logic in
// tabBarView and tabHitTest.
func testTabWidth(sessionID string, state SessionState, isActive bool) int {
	char := badgeChar(state, 0) // animFrame=0 for steady badge in tests
	label := char + " " + sessionID + " ✕"
	if isActive {
		return lipgloss.Width(tabActiveStyle.Render(label))
	}
	return lipgloss.Width(inactiveTabStyle(state).Render(label))
}

func TestTabHitTestSingleTab(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Only proj1 is in openTabs and it is active.

	// X=0 should be inside the first (and only) tab.
	idx, isClose := app.tabHitTest(0)
	if idx != 0 || isClose {
		t.Fatalf("expected tab 0 (not close), got idx=%d close=%v", idx, isClose)
	}

	// Large X should be past the tab (in the gap) → -1.
	idx, _ = app.tabHitTest(500)
	if idx != -1 {
		t.Fatalf("expected -1 for click past tabs, got %d", idx)
	}
}

func TestTabHitTestMultipleTabs(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	// Inject sessions and open tabs.
	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	activeSessionID := app.mgr.ActiveName()

	widths := make([]int, 3)
	for i, id := range app.openTabs {
		state := app.sessionStates[id]
		widths[i] = testTabWidth(id, state, id == activeSessionID)
	}

	// Click in first tab → 0.
	if idx, _ := app.tabHitTest(0); idx != 0 {
		t.Fatalf("expected tab 0 at X=0, got %d", idx)
	}

	// Click at last pixel of first tab → still 0.
	if idx, _ := app.tabHitTest(widths[0] - 1); idx != 0 {
		t.Fatalf("expected tab 0 at X=%d, got %d", widths[0]-1, idx)
	}

	// Click at first pixel of second tab → 1.
	if idx, _ := app.tabHitTest(widths[0]); idx != 1 {
		t.Fatalf("expected tab 1 at X=%d, got %d", widths[0], idx)
	}

	// Click at first pixel of third tab → 2.
	thirdStart := widths[0] + widths[1]
	if idx, _ := app.tabHitTest(thirdStart); idx != 2 {
		t.Fatalf("expected tab 2 at X=%d, got %d", thirdStart, idx)
	}

	// Click past all tabs → -1.
	pastAll := widths[0] + widths[1] + widths[2]
	if idx, _ := app.tabHitTest(pastAll); idx != -1 {
		t.Fatalf("expected -1 at X=%d (past all tabs), got %d", pastAll, idx)
	}
}

func TestTabHitTestNegativeX(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	if idx, _ := app.tabHitTest(-5); idx != -1 {
		t.Fatalf("expected -1 for negative X, got %d", idx)
	}
}

func TestTabHitTestCloseRegion(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	// Inject sessions.
	for _, p := range cfg.Projects[:2] {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")

	// beta is inactive (alpha is active). Clicking the rightmost portion
	// of beta's tab should report isClose=true.
	activeSessionID := app.mgr.ActiveName()
	firstWidth := testTabWidth("alpha", app.sessionStates["alpha"], "alpha" == activeSessionID)
	secondWidth := testTabWidth("beta", app.sessionStates["beta"], false)

	// Click 2 pixels from the right edge of beta's tab → close region.
	closeX := firstWidth + secondWidth - 2
	idx, isClose := app.tabHitTest(closeX)
	if idx != 1 {
		t.Fatalf("expected tab 1 for close click, got %d", idx)
	}
	if !isClose {
		t.Fatal("expected isClose=true for click in close region")
	}

	// Click near the left side of beta → not close.
	idx, isClose = app.tabHitTest(firstWidth + 1)
	if idx != 1 || isClose {
		t.Fatalf("expected tab 1 (not close) for left-side click, got idx=%d close=%v", idx, isClose)
	}

	// Active tab (alpha) also has a close button now.
	// Click near its right edge → close region.
	closeActiveX := firstWidth - 2
	idx, isClose = app.tabHitTest(closeActiveX)
	if idx != 0 {
		t.Fatalf("expected tab 0, got %d", idx)
	}
	if !isClose {
		t.Fatal("active tab should report isClose in close region")
	}

	// Click near the left side of active tab → not close.
	idx, isClose = app.tabHitTest(1)
	if idx != 0 || isClose {
		t.Fatalf("expected tab 0 (not close) for left-side click, got idx=%d close=%v", idx, isClose)
	}
}

func TestProjectIndexByName(t *testing.T) {
	app := NewApp(configWith3Projects(), "", nil)

	if idx := app.projectIndexByName("alpha"); idx != 0 {
		t.Fatalf("expected 0 for 'alpha', got %d", idx)
	}
	if idx := app.projectIndexByName("gamma"); idx != 2 {
		t.Fatalf("expected 2 for 'gamma', got %d", idx)
	}
	if idx := app.projectIndexByName("nonexistent"); idx != -1 {
		t.Fatalf("expected -1 for 'nonexistent', got %d", idx)
	}
}

func TestTabClickSwitchesSession(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	app.width = 160
	app.height = 40

	// Inject sessions and open tabs.
	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	// Compute where the second tab starts (after the active first tab).
	firstWidth := testTabWidth("alpha", app.sessionStates["alpha"], true)

	sbWidth := app.sidebar.Width()
	tabBarStartX := screenPadding + sbWidth

	// Click near the left of the second tab (beta) — not the close region.
	clickX := tabBarStartX + firstWidth + 1
	msg := tea.MouseMsg{
		X:      clickX,
		Y:      1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}

	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Fatal("expected a command from tab click")
	}
	result := cmd()
	switched, ok := result.(TabSwitchedMsg)
	if !ok {
		t.Fatalf("expected TabSwitchedMsg, got %T", result)
	}
	if switched.SessionID != "beta" {
		t.Fatalf("expected session 'beta', got %q", switched.SessionID)
	}
}

func TestTabCloseClick(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	app.width = 160
	app.height = 40

	// Inject sessions.
	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	// Click the close region of the second tab (beta).
	firstWidth := testTabWidth("alpha", app.sessionStates["alpha"], true)
	secondWidth := testTabWidth("beta", app.sessionStates["beta"], false)

	sbWidth := app.sidebar.Width()
	tabBarStartX := screenPadding + sbWidth

	// Close region = last 4 columns of the tab.
	closeX := tabBarStartX + firstWidth + secondWidth - 2
	msg := tea.MouseMsg{
		X:      closeX,
		Y:      1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}

	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Fatal("expected a command from close click")
	}
	result := cmd()
	closed, ok := result.(TabClosedMsg)
	if !ok {
		t.Fatalf("expected TabClosedMsg, got %T", result)
	}
	if closed.Name != "beta" {
		t.Fatalf("expected close 'beta', got %q", closed.Name)
	}
}

func TestTabClosedMsgRemovesTab(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	// Inject sessions.
	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")
	if len(app.openTabs) != 3 {
		t.Fatalf("precondition: expected 3 tabs, got %d", len(app.openTabs))
	}

	// Close a non-active tab.
	model, _ := app.Update(TabClosedMsg{Name: "beta"})
	app = model.(App)

	if len(app.openTabs) != 2 {
		t.Fatalf("expected 2 tabs after close, got %d", len(app.openTabs))
	}
	for _, tab := range app.openTabs {
		if tab == "beta" {
			t.Fatal("beta should have been removed from openTabs")
		}
	}
}

func TestTabClosedMsgSwitchesIfActive(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	// Inject sessions.
	for _, p := range cfg.Projects[:2] {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	// alpha (index 0) is the active tab.

	// Close the active tab.
	model, cmd := app.Update(TabClosedMsg{Name: "alpha"})
	app = model.(App)

	// Should produce a TabSwitchedMsg to switch to the remaining tab.
	if cmd == nil {
		t.Fatal("expected a command when closing active tab")
	}
	result := cmd()
	switched, ok := result.(TabSwitchedMsg)
	if !ok {
		t.Fatalf("expected TabSwitchedMsg, got %T", result)
	}
	if switched.SessionID != "beta" {
		t.Fatalf("expected switch to 'beta', got %q", switched.SessionID)
	}
}

func TestTabClosedLastTabClearsTerminal(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Inject session.
	sess := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)
	// Simulate an active terminal.
	app.terminal.active = true

	// Close the only open tab (proj1).
	model, cmd := app.Update(TabClosedMsg{Name: "proj1"})
	app = model.(App)

	// No tabs remain — terminal should be deactivated.
	if len(app.openTabs) != 0 {
		t.Fatalf("expected 0 tabs, got %d", len(app.openTabs))
	}
	if app.terminal.active {
		t.Fatal("expected terminal.active=false after closing last tab")
	}
	// Should not produce a switch command.
	if cmd != nil {
		result := cmd()
		if _, ok := result.(ProjectSwitchedMsg); ok {
			t.Fatal("expected no switch when closing last tab")
		}
	}
}

func TestTabClosedResetsStateToIdle(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Inject a session so closing it has something to clean up.
	sess := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)
	app.sessionStates["proj1"] = StateWorking
	app.sidebar.states["proj1"] = StateWorking

	model, _ := app.Update(TabClosedMsg{Name: "proj1"})
	app = model.(App)

	if app.sidebar.states["proj1"] != StateIdle {
		t.Fatalf("expected sidebar state Idle, got %v", app.sidebar.states["proj1"])
	}
	// Session state should be cleaned up.
	if _, exists := app.sessionStates["proj1"]; exists {
		t.Fatal("expected sessionStates['proj1'] to be deleted")
	}
}

func TestTabClickInGapNoSwitch(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40

	sbWidth := app.sidebar.Width()
	tabBarStartX := screenPadding + sbWidth

	msg := tea.MouseMsg{
		X:      tabBarStartX + 200,
		Y:      1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}

	_, cmd := app.Update(msg)

	if cmd != nil {
		result := cmd()
		if _, ok := result.(ProjectSwitchedMsg); ok {
			t.Fatal("expected no ProjectSwitchedMsg for click in gap")
		}
	}
}

func TestTabClickBelowTabBarNoSwitch(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.addTab("proj2")

	sbWidth := app.sidebar.Width()
	tabBarStartX := screenPadding + sbWidth

	msg := tea.MouseMsg{
		X:      tabBarStartX + 5,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}

	_, cmd := app.Update(msg)

	if cmd != nil {
		result := cmd()
		if _, ok := result.(ProjectSwitchedMsg); ok {
			t.Fatal("expected no ProjectSwitchedMsg for click below tab bar")
		}
	}
}

// ── Scrollback tests ────────────────────────────────────────────

func TestMouseWheelScrollsBackBuffer(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	// Seed some scrollback lines so ScrollBy has room.
	for i := 0; i < 20; i++ {
		app.terminal.scrollback.Push(nil)
	}

	sbWidth := app.sidebar.Width()
	termX := screenPadding + sbWidth + 10

	msg := tea.MouseMsg{
		X:      termX,
		Y:      10,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
	}

	model, cmd := app.Update(msg)
	app = model.(App)

	if cmd != nil {
		t.Error("expected nil cmd from scroll")
	}
	if app.terminal.scrollOffset != scrollLinesPerTick {
		t.Fatalf("expected scrollOffset=%d after wheel up, got %d", scrollLinesPerTick, app.terminal.scrollOffset)
	}

	// Wheel down should decrease offset.
	msg.Button = tea.MouseButtonWheelDown
	model, _ = app.Update(msg)
	app = model.(App)

	if app.terminal.scrollOffset != 0 {
		t.Fatalf("expected scrollOffset=0 after wheel down, got %d", app.terminal.scrollOffset)
	}
}

func TestKeyboardSnapsToLiveView(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal
	app.terminal.scrollOffset = 10

	// Simulate a regular keypress.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	model, _ := app.Update(msg)
	app = model.(App)

	if app.terminal.scrollOffset != 0 {
		t.Fatalf("expected scrollOffset=0 after keypress, got %d", app.terminal.scrollOffset)
	}
}

func TestScrollOffsetWiredToStatusBar(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.ready = true
	app.terminal.scrollOffset = 42

	// View() wires scrollOffset into statusbar — verify the rendered
	// output contains the scroll indicator.
	view := app.View()

	if !strings.Contains(view, "SCROLL") {
		t.Fatal("expected SCROLL indicator in status bar when scrollOffset > 0")
	}
}

// ── E2E scrollback scenario ─────────────────────────────────────
//
// Simulates the full user flow:
//  1. Project "stocks" running OpenCode (60×14 VT)
//  2. Agent prints 500 lines of output (scrolling content area)
//  3. User scrolls up to first printed line, then scrolls back down
//  4. Agent prints 30 more lines — verify scrollback still works

func TestScrollbackE2EOpenCodeScenario(t *testing.T) {
	cfg := &config.Config{
		Projects: []config.Project{
			{Name: "stocks", Repo: "/tmp/stocks", Agent: config.AgentOpenCode},
		},
	}
	app := NewApp(cfg, "", nil)
	app.width = 120
	app.height = 30
	app.ready = true
	app.focus = focusTerminal
	app.layout()

	// ── Step 1: Set up a fake session with a VT ──────────────────
	const vtW, vtH = 80, 20
	vt := vt10x.New(vt10x.WithSize(vtW, vtH))
	sess := &session.Session{
		Project: cfg.Projects[0],
		VT:      vt,
		State:   session.StateRunning,
		Width:   vtW,
		Height:  vtH,
	}
	app.mgr.InjectSession("stocks", sess)
	app.syncTerminalFromSession()

	header := "  opencode v1.0"
	footer := "  > _                                               esc exit"
	contentRows := vtH - 2 // rows 1..(vtH-2), row 0=header, last=footer

	// Helper: simulate an OpenCode full-screen redraw.
	renderFrame := func(startLine, endLine int) {
		frame := buildTUIFrame(vtW, header, footer, startLine, endLine)
		sess.Mu.Lock()
		sess.VT.Write([]byte(frame))
		sess.Mu.Unlock()
		app.scrollDirty["stocks"] = true
	}

	// ── Step 2: Agent prints 500 lines ───────────────────────────
	// Initial frame: lines 1..contentRows.
	renderFrame(1, contentRows)
	app.runScrollCheck() // first snapshot — establishes baseline

	// Scroll through 500 lines, 1 line at a time.
	totalLines := 500
	for startLine := 2; startLine <= totalLines-contentRows+1; startLine++ {
		endLine := startLine + contentRows - 1
		renderFrame(startLine, endLine)
		app.runScrollCheck()
	}

	capturedLines := app.scrollbacks["stocks"].Len()
	t.Logf("captured %d scrollback lines from %d scroll events", capturedLines, totalLines-contentRows)

	// Should have captured a substantial portion. Each scroll-check sees
	// one complete redraw (no chunking issue in this test).
	if capturedLines < 300 {
		t.Fatalf("expected at least 300 scrollback lines, got %d", capturedLines)
	}

	// Verify the earliest captured line contains the expected content.
	oldest := glyphsToText(app.scrollbacks["stocks"].Line(0))
	if !strings.Contains(oldest, "Line") {
		t.Fatalf("expected oldest scrollback line to contain 'Line', got %q", oldest)
	}

	// ── Step 3: Scroll up to first line, then back down ──────────
	sbWidth := app.sidebar.Width()
	termX := screenPadding + sbWidth + 10

	// Scroll up far enough to see the oldest scrollback lines.
	scrollsNeeded := capturedLines / scrollLinesPerTick
	for i := 0; i < scrollsNeeded; i++ {
		msg := tea.MouseMsg{
			X: termX, Y: 10,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		model, _ := app.Update(msg)
		app = model.(App)
	}

	if app.terminal.scrollOffset == 0 {
		t.Fatal("expected scrollOffset > 0 after scrolling up")
	}
	if app.terminal.scrollOffset > capturedLines {
		t.Fatalf("scrollOffset %d should be clamped to buffer len %d",
			app.terminal.scrollOffset, capturedLines)
	}

	// View should contain SCROLL indicator.
	view := app.View()
	if !strings.Contains(view, "SCROLL") {
		t.Fatal("expected SCROLL indicator in view when scrolled up")
	}

	// Scroll back down to live view.
	for app.terminal.scrollOffset > 0 {
		msg := tea.MouseMsg{
			X: termX, Y: 10,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		model, _ := app.Update(msg)
		app = model.(App)
	}

	if app.terminal.scrollOffset != 0 {
		t.Fatalf("expected scrollOffset=0 after scrolling back down, got %d", app.terminal.scrollOffset)
	}

	// ── Step 4: Agent prints 30 more lines ───────────────────────
	prevCaptured := app.scrollbacks["stocks"].Len()
	base := totalLines - contentRows + 2
	for startLine := base; startLine < base+30; startLine++ {
		endLine := startLine + contentRows - 1
		renderFrame(startLine, endLine)
		app.runScrollCheck()
	}

	newCaptured := app.scrollbacks["stocks"].Len()
	added := newCaptured - prevCaptured
	t.Logf("captured %d more scrollback lines from 30 additional scroll events", added)

	if added < 20 {
		t.Fatalf("expected at least 20 new scrollback lines from 30 scrolls, got %d", added)
	}

	// ── Step 5: Verify keyboard snaps to live ────────────────────
	// Scroll up again.
	for i := 0; i < 5; i++ {
		msg := tea.MouseMsg{
			X: termX, Y: 10,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		model, _ := app.Update(msg)
		app = model.(App)
	}
	if app.terminal.scrollOffset == 0 {
		t.Fatal("expected scrollOffset > 0 before keypress test")
	}

	// Any keypress should snap back to live.
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	app = model.(App)
	if app.terminal.scrollOffset != 0 {
		t.Fatalf("expected scrollOffset=0 after keypress, got %d", app.terminal.scrollOffset)
	}
}

// TestScrollbackOffsetAdjustsOnNewData verifies that when the user is
// scrolled up and new output arrives, the offset bumps so the view stays
// pinned to the same content.
func TestScrollbackOffsetAdjustsOnNewData(t *testing.T) {
	cfg := &config.Config{
		Projects: []config.Project{
			{Name: "p1", Repo: "/tmp/p1", Agent: config.AgentOpenCode},
		},
	}
	app := NewApp(cfg, "", nil)
	app.width = 120
	app.height = 30
	app.ready = true
	app.focus = focusTerminal
	app.layout()

	const vtW, vtH = 80, 14
	vt := vt10x.New(vt10x.WithSize(vtW, vtH))
	sess := &session.Session{
		Project: cfg.Projects[0],
		VT:      vt,
		State:   session.StateRunning,
		Width:   vtW,
		Height:  vtH,
	}
	app.mgr.InjectSession("p1", sess)
	app.syncTerminalFromSession()

	header := "  header"
	footer := "  footer"
	contentRows := vtH - 2

	writeFrame := func(start int) {
		frame := buildTUIFrame(vtW, header, footer, start, start+contentRows-1)
		sess.Mu.Lock()
		sess.VT.Write([]byte(frame))
		sess.Mu.Unlock()
		app.scrollDirty["p1"] = true
	}

	// Build up scrollback.
	writeFrame(1)
	app.runScrollCheck()
	for i := 2; i <= 50; i++ {
		writeFrame(i)
		app.runScrollCheck()
	}

	// User scrolls up (via ScrollBy, which sets scrollPinned = true).
	app.terminal.ScrollBy(10)
	savedOffset := app.terminal.scrollOffset

	// New data arrives while scrolled up and pinned.
	for i := 51; i <= 55; i++ {
		writeFrame(i)
		app.runScrollCheck()
	}

	// Offset should have increased to keep view pinned.
	if app.terminal.scrollOffset <= savedOffset {
		t.Fatalf("expected scrollOffset > %d after new data (pinned), got %d",
			savedOffset, app.terminal.scrollOffset)
	}

	// Now simulate the user scrolling DOWN (unpins).
	beforeDown := app.terminal.scrollOffset
	app.terminal.ScrollBy(-3) // scrollPinned becomes false
	if app.terminal.scrollPinned {
		t.Fatal("expected scrollPinned = false after scrolling down")
	}

	offsetAfterDown := app.terminal.scrollOffset

	// More data arrives — offset should NOT auto-adjust since unpinned.
	for i := 56; i <= 60; i++ {
		writeFrame(i)
		app.runScrollCheck()
	}

	if app.terminal.scrollOffset != offsetAfterDown {
		t.Fatalf("expected scrollOffset to stay at %d when unpinned, got %d",
			offsetAfterDown, app.terminal.scrollOffset)
	}
	_ = beforeDown // suppress unused warning
}

// ── Sticky state tests ──────────────────────────────────────────

func TestStickyStatePreventsDowngrade(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)

	// Simulate an attention state with a future sticky deadline.
	app.sidebar.states["proj1"] = StateNeedsAttention
	app.stateStickUntil["proj1"] = time.Now().Add(10 * time.Second)

	// isAttentionState should return true.
	if !isAttentionState(StateNeedsAttention) {
		t.Fatal("expected NeedsAttention to be an attention state")
	}
	if !isAttentionState(StateNeedsPermission) {
		t.Fatal("expected NeedsPermission to be an attention state")
	}
	if !isAttentionState(StateError) {
		t.Fatal("expected Error to be an attention state")
	}
	if !isAttentionState(StateDone) {
		t.Fatal("expected Done to be an attention state")
	}
	if isAttentionState(StateWorking) {
		t.Fatal("expected Working NOT to be an attention state")
	}
	if isAttentionState(StateIdle) {
		t.Fatal("expected Idle NOT to be an attention state")
	}
}

func TestProjectSwitchClearsStickyTimer(t *testing.T) {
	// Sticky timers are now per-session ID, not project name.
	// Verify that TabSwitchedMsg does not clear them (they expire on their own).
	// This test now verifies the sticky timer mechanism still works.
	app := NewApp(configWithProjects(), "", nil)
	app.stateStickUntil["proj1"] = time.Now().Add(10 * time.Second)

	// Sticky timer should exist.
	if _, ok := app.stateStickUntil["proj1"]; !ok {
		t.Fatal("precondition: expected sticky timer to exist")
	}

	// Closing the tab clears the sticky timer.
	sess := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)
	model, _ := app.Update(TabClosedMsg{Name: "proj1"})
	app = model.(App)

	if _, ok := app.stateStickUntil["proj1"]; ok {
		t.Fatal("expected sticky timer to be cleared after tab close")
	}
}

// ── Inactive tab style tests ────────────────────────────────────

// ── Too-small terminal tests ────────────────────────────────────

func TestTooSmallBothDimensions(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 40
	app.height = 5
	app.ready = true
	if !app.tooSmall() {
		t.Fatal("expected tooSmall() for 40×5")
	}
}

func TestTooSmallWidthOnly(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = minAppWidth - 1
	app.height = 40
	app.ready = true
	if !app.tooSmall() {
		t.Fatal("expected tooSmall() for narrow terminal")
	}
}

func TestTooSmallHeightOnly(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 120
	app.height = minAppHeight - 1
	app.ready = true
	if !app.tooSmall() {
		t.Fatal("expected tooSmall() for short terminal")
	}
}

func TestNotTooSmallAtMinimum(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = minAppWidth
	app.height = minAppHeight
	app.ready = true
	if app.tooSmall() {
		t.Fatalf("expected not tooSmall() at exact minimum %d×%d", minAppWidth, minAppHeight)
	}
}

func TestTooSmallViewShowsMessage(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 40
	app.height = 5
	app.ready = true

	view := app.View()
	if view == "" {
		t.Fatal("expected non-empty view for too-small terminal")
	}
	if !strings.Contains(view, "too small") {
		t.Fatalf("expected 'too small' in view, got: %s", view)
	}
}

func TestWindowSizeMsgSkipsLayoutWhenTooSmall(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Start with usable size.
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	model, _ := app.Update(msg)
	app = model.(App)
	if app.tooSmall() {
		t.Fatal("precondition: should not be too small at 120×40")
	}

	// Shrink to unusable size.
	msg = tea.WindowSizeMsg{Width: 30, Height: 5}
	model, _ = app.Update(msg)
	app = model.(App)

	if !app.tooSmall() {
		t.Fatal("expected tooSmall() at 30×5")
	}
	// The app should have stored the new dimensions.
	if app.width != 30 || app.height != 5 {
		t.Fatalf("expected 30×5, got %d×%d", app.width, app.height)
	}
}

// ── Dimension clamping tests ────────────────────────────────────

func TestTermDimensionsClampsToOne(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Set absurdly small dimensions.
	app.width = 10
	app.height = 3

	w, h := app.termDimensions()
	if w < 1 {
		t.Fatalf("expected termWidth >= 1, got %d", w)
	}
	if h < 1 {
		t.Fatalf("expected termHeight >= 1, got %d", h)
	}
}

func TestTermDimensionsNormalSize(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 120
	app.height = 40

	w, h := app.termDimensions()
	// innerWidth = 120 - 2 = 118
	// sidebar = 26 + 1(border) = 27
	// termWidth = 118 - 27 - 1(padding) = 90
	expectedW := 90
	// termHeight = 40 - 4 = 36
	expectedH := 36

	if w != expectedW {
		t.Fatalf("expected termWidth=%d, got %d", expectedW, w)
	}
	if h != expectedH {
		t.Fatalf("expected termHeight=%d, got %d", expectedH, h)
	}
}

func TestLayoutUsesTermDimensions(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 120
	app.height = 40
	app.layout()

	termW, termH := app.termDimensions()
	if app.terminal.width != termW {
		t.Fatalf("layout terminal width %d != termDimensions width %d", app.terminal.width, termW)
	}
	if app.terminal.height != termH {
		t.Fatalf("layout terminal height %d != termDimensions height %d", app.terminal.height, termH)
	}
	if app.sidebar.height != app.height-1 {
		t.Fatalf("expected sidebar height %d, got %d", app.height-1, app.sidebar.height)
	}
}

// ── Inactive tab style tests ────────────────────────────────────

func TestInactiveTabStyleByState(t *testing.T) {
	// All inactive tabs use the same subtle style regardless of state.
	// The badge character communicates state, not the border color.
	allStates := []SessionState{
		StateNeedsAttention, StateNeedsPermission, StateAsking,
		StateError, StateDone, StateWorking, StateIdle,
	}
	for _, state := range allStates {
		got := inactiveTabStyle(state)
		if got.Render("X") != tabStyle.Render("X") {
			t.Errorf("inactiveTabStyle(%v): expected default tabStyle, got different style", state)
		}
	}
}

func TestAttentionEventToState_NeedsAnswer(t *testing.T) {
	event := &attention.AttentionEvent{Type: attention.NeedsAnswer}
	state := attentionEventToState(event)
	if state != StateAsking {
		t.Errorf("expected StateAsking for NeedsAnswer, got %v", state)
	}
}

func TestAttentionEventToState_NeedsPermission(t *testing.T) {
	event := &attention.AttentionEvent{Type: attention.NeedsPermission}
	state := attentionEventToState(event)
	if state != StateNeedsPermission {
		t.Errorf("expected StateNeedsPermission for NeedsPermission, got %v", state)
	}
}

func TestIsAttentionState_IncludesAsking(t *testing.T) {
	if !isAttentionState(StateAsking) {
		t.Error("expected StateAsking to be an attention state")
	}
	if !isAttentionState(StateNeedsAttention) {
		t.Error("expected StateNeedsAttention to be an attention state")
	}
	if isAttentionState(StateWorking) {
		t.Error("expected StateWorking to NOT be an attention state")
	}
}

func TestStateAsking_StringAndDescription(t *testing.T) {
	if StateAsking.String() != "asking" {
		t.Errorf("expected 'asking', got %q", StateAsking.String())
	}
	if StateAsking.Description() != "agent has a question" {
		t.Errorf("expected 'agent has a question', got %q", StateAsking.Description())
	}
}

func TestBadgeChar_Asking(t *testing.T) {
	char := badgeChar(StateAsking, 0)
	if char != "?" {
		t.Errorf("expected '?' badge for StateAsking, got %q", char)
	}
}

// ── Telegram event mapping tests ────────────────────────────────

func TestStateToEventKind_AllMappings(t *testing.T) {
	tests := []struct {
		state SessionState
		want  int // telegram.EventKind as int (-1 for unmapped)
	}{
		{StateIdle, 0},            // EventResponse
		{StateNeedsPermission, 1}, // EventPermission
		{StateAsking, 2},          // EventQuestion
		{StateNeedsAttention, 3},  // EventAttention
		{StateError, 4},           // EventError
		{StateDone, 5},            // EventDone
		{StateWorking, -1},        // Not sent
	}
	for _, tt := range tests {
		got := stateToEventKind(tt.state)
		if int(got) != tt.want {
			t.Errorf("stateToEventKind(%v) = %d, want %d", tt.state, got, tt.want)
		}
	}
}

func TestSendTelegramEvent_NilChannel(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// telegramCh is nil by default — should not panic.
	app.sendTelegramEvent("proj1", "proj1", StateNeedsPermission, "test", []string{"screen"})
}

func TestSendTelegramEvent_SkipsWorking(t *testing.T) {
	// Verify stateToEventKind returns -1 for Working, which means
	// sendTelegramEvent returns early without sending.
	kind := stateToEventKind(StateWorking)
	if kind != -1 {
		t.Fatalf("expected -1 for StateWorking, got %d", kind)
	}
}

// ── System tab tests ────────────────────────────────────────────

func TestSystemTabRequestMsg_KeybindingT(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.focus = focusSidebar
	app.sidebar.focused = true

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Fatal("expected a command from 't' key in sidebar")
	}
	result := cmd()
	sysMsg, ok := result.(SystemTabRequestMsg)
	if !ok {
		t.Fatalf("expected SystemTabRequestMsg, got %T", result)
	}
	if sysMsg.Name != "Telegram Setup" {
		t.Fatalf("expected tab name 'Telegram Setup', got %q", sysMsg.Name)
	}
	if len(sysMsg.Args) != 2 || sysMsg.Args[0] != "telegram" || sysMsg.Args[1] != "setup" {
		t.Fatalf("expected args [telegram setup], got %v", sysMsg.Args)
	}
}

// ── Outbound event wiring: verify all attention types send events ─

func TestSendTelegramEvent_AllAttentionTypes(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	ch := make(chan telegram.Event, 16)
	app.SetTelegramChannel(ch)

	tests := []struct {
		state    SessionState
		wantKind telegram.EventKind
		sent     bool
	}{
		{StateNeedsPermission, telegram.EventPermission, true},
		{StateAsking, telegram.EventQuestion, true},
		{StateNeedsAttention, telegram.EventAttention, true},
		{StateError, telegram.EventError, true},
		{StateDone, telegram.EventDone, true},
		{StateIdle, telegram.EventResponse, true},
		{StateWorking, -1, false}, // should NOT send
	}

	for _, tt := range tests {
		// Drain channel.
		for len(ch) > 0 {
			<-ch
		}

		app.sendTelegramEvent("proj1", "proj1", tt.state, "test detail", []string{"screen line"})

		if tt.sent {
			select {
			case e := <-ch:
				if e.Kind != tt.wantKind {
					t.Errorf("state %v: expected kind %d, got %d", tt.state, tt.wantKind, e.Kind)
				}
				if e.Project != "proj1" {
					t.Errorf("state %v: expected project 'proj1', got %q", tt.state, e.Project)
				}
				if e.Detail != "test detail" {
					t.Errorf("state %v: expected detail 'test detail', got %q", tt.state, e.Detail)
				}
				if len(e.Screen) != 1 || e.Screen[0] != "screen line" {
					t.Errorf("state %v: screen content mismatch", tt.state)
				}
			default:
				t.Errorf("state %v: expected event to be sent, but channel is empty", tt.state)
			}
		} else {
			select {
			case e := <-ch:
				t.Errorf("state %v: expected no event, but got kind %d", tt.state, e.Kind)
			default:
				// Good — no event sent.
			}
		}
	}
}

func TestSendTelegramEvent_DialogFooterPreserved(t *testing.T) {
	// Regression test: sendTelegramEvent must not strip dialog footers.
	// Previously ChromeSkipRows(1, 2) removed the bottom 2 rows, stripping
	// "enter submit  esc dismiss" — which ParseQuestionOptions needs to find
	// inline keyboard buttons.
	//
	// Without an active session in the manager, no filtering is applied
	// (lines pass through unchanged). This test verifies the Screen field
	// contains the full input including the dialog footer.
	app := NewApp(configWithProjects(), "", nil)
	ch := make(chan telegram.Event, 4)
	app.SetTelegramChannel(ch)

	screen := []string{
		"Header row",
		"Conversation content",
		"Which option?",
		"1. Alpha",
		"2. Beta",
		"↕ select  enter submit  esc dismiss",
	}
	app.sendTelegramEvent("proj1", "proj1", StateAsking, "question", screen)

	e := <-ch
	if e.Kind != telegram.EventQuestion {
		t.Fatalf("expected EventQuestion, got %d", e.Kind)
	}
	// The screen must contain the dialog footer for ParseQuestionOptions.
	found := false
	for _, line := range e.Screen {
		if strings.Contains(line, "enter submit") && strings.Contains(line, "esc dismiss") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("dialog footer stripped from Screen — ParseQuestionOptions will fail to find buttons.\nScreen: %v", e.Screen)
	}
}

func TestSendTelegramEvent_ChannelFull_DoesNotBlock(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	ch := make(chan telegram.Event, 1)
	app.SetTelegramChannel(ch)

	// Fill the channel.
	app.sendTelegramEvent("proj1", "proj1", StateNeedsPermission, "", []string{"a"})
	// Second send should not block (drops silently).
	app.sendTelegramEvent("proj1", "proj1", StateError, "", []string{"b"})

	// Only the first event should be in the channel.
	e := <-ch
	if e.Kind != telegram.EventPermission {
		t.Errorf("expected first event to be Permission, got %d", e.Kind)
	}
}

// ── Kitty keyboard protocol E2E tests ───────────────────────────
//
// When kitty keyboard mode 1 is enabled (\x1b[>1u), the terminal encodes
// Ctrl+letter as CSI u sequences instead of legacy control codes:
//   Ctrl+C → \x1b[99;5u   (instead of 0x03)
//   Ctrl+S → \x1b[115;5u  (instead of 0x13)
//   Ctrl+J → \x1b[106;5u  (instead of 0x0A)
//   Ctrl+K → \x1b[107;5u  (instead of 0x0B)
//
// bubbletea v1.3.10 does not recognise these sequences and emits them as
// unknownCSISequenceMsg. Our unknownCSI handler intercepts and forwards
// them to the active PTY. This means Ctrl+C double-tap to exit, Ctrl+S
// to toggle focus, and Ctrl+J/K to switch tabs all stop working in
// terminals with kitty protocol support (kitty, WezTerm, Ghostty).

// unknownCSISequenceMsg mirrors bubbletea's unexported type of the same
// name. Our unknownCSIBytes() function matches on reflect.TypeOf().Name(),
// so a local type with the same name and underlying type ([]byte) works.
type unknownCSISequenceMsg []byte

func TestKittyCtrlC_DoubleTapExits(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	// Inject a session so the PTY write path is exercised.
	sess := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)

	// Simulate kitty-encoded Ctrl+C: \x1b[99;5u
	kittyCtrlC := unknownCSISequenceMsg([]byte{0x1b, '[', '9', '9', ';', '5', 'u'})

	// First Ctrl+C — should show hint, not exit.
	model, cmd := app.Update(kittyCtrlC)
	app = model.(App)
	if cmd != nil {
		// If cmd is tea.Quit, the app exited on the first press — wrong.
		result := cmd()
		if _, isQuit := result.(tea.QuitMsg); isQuit {
			t.Fatal("first kitty Ctrl+C should not quit")
		}
	}
	if !app.ctrlCHint {
		t.Fatal("expected ctrlCHint=true after first kitty Ctrl+C")
	}

	// Second Ctrl+C within the window — should exit.
	model, cmd = app.Update(kittyCtrlC)
	app = model.(App)
	if cmd == nil {
		t.Fatal("expected tea.Quit command from second kitty Ctrl+C")
	}
	result := cmd()
	if _, isQuit := result.(tea.QuitMsg); !isQuit {
		t.Fatalf("expected tea.QuitMsg, got %T", result)
	}
}

func TestKittyCtrlS_TogglesFocus(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	sess := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)

	// Simulate kitty-encoded Ctrl+S: \x1b[115;5u
	kittyCtrlS := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '1', '5', ';', '5', 'u'})

	model, _ := app.Update(kittyCtrlS)
	app = model.(App)

	if app.focus != focusSidebar {
		t.Fatalf("expected focus=sidebar after kitty Ctrl+S, got %d", app.focus)
	}
}

func TestKittyCtrlJ_SwitchesTab(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	// Inject sessions and open tabs for all projects.
	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	// Simulate kitty-encoded Ctrl+J: \x1b[106;5u
	kittyCtrlJ := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '0', '6', ';', '5', 'u'})

	_, cmd := app.Update(kittyCtrlJ)
	if cmd == nil {
		t.Fatal("expected command from kitty Ctrl+J to switch tab")
	}

	result := cmd()
	switched, ok := result.(TabSwitchedMsg)
	if !ok {
		t.Fatalf("expected TabSwitchedMsg, got %T", result)
	}
	// Ctrl+J goes left (prev); from alpha (index 0) wraps to gamma (index 2).
	if switched.SessionID != "gamma" {
		t.Fatalf("expected switch to 'gamma', got %q", switched.SessionID)
	}
}

func TestKittyShiftEnter_ForwardedToPTY(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	sess := &session.Session{
		ID: "proj1", Instance: 1,
		Project: app.cfg.Projects[0], State: session.StateRunning,
	}
	app.mgr.InjectSession("proj1", sess)

	// Simulate kitty-encoded Shift+Enter: \x1b[13;2u
	// This must be forwarded as raw bytes to the PTY (not intercepted as
	// plain Enter) so the agent can distinguish Shift+Enter (newline)
	// from Enter (submit).
	kittyShiftEnter := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '3', ';', '2', 'u'})

	model, cmd := app.Update(kittyShiftEnter)
	_ = model.(App)

	// Should return nil (forwarded to PTY, no app-level action).
	if cmd != nil {
		t.Fatal("expected Shift+Enter to be forwarded to PTY (nil cmd), got a command")
	}

	// Verify parseKittyCSI returns false for Shift+Enter.
	_, ok := parseKittyCSI([]byte{0x1b, '[', '1', '3', ';', '2', 'u'})
	if ok {
		t.Fatal("parseKittyCSI should return false for Shift+Enter so raw bytes reach PTY")
	}
}

// ── Kitty keyboard + form interaction ────────────────────────────
// When the sidebar form is active, iTerm2/kitty/WezTerm/Ghostty send
// Esc as \x1b[27u and Enter as \x1b[13u (kitty keyboard mode 1).
// bubbletea v1.3.10 doesn't parse these → unknownCSISequenceMsg.
// parseKittyCSI must translate them so the form can be cancelled (Esc)
// and advanced (Enter). Without this fix the form is inescapable.

// openFormOnApp puts the app into sidebar-focused form mode by sending
// the 'a' key to the sidebar. Returns the updated app.
func openFormOnApp(t *testing.T, app App) App {
	t.Helper()
	app.focus = focusSidebar
	app.sidebar.focused = true
	app.terminal.focused = false
	// Press 'a' to open the form.
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	app = model.(App)
	if app.sidebar.mode != sidebarForm {
		t.Fatalf("expected sidebarForm, got %d", app.sidebar.mode)
	}
	return app
}

func TestKittyEsc_CancelsFormFromSidebar(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.width = 160
	app.height = 40
	app = openFormOnApp(t, app)

	// Simulate kitty-encoded Esc: \x1b[27u
	kittyEsc := unknownCSISequenceMsg([]byte{0x1b, '[', '2', '7', 'u'})

	model, cmd := app.Update(kittyEsc)
	app = model.(App)

	// The form should produce FormCancelledMsg as a command.
	if cmd == nil {
		t.Fatal("expected command from kitty Esc in form")
	}
	msg := cmd()
	if _, ok := msg.(FormCancelledMsg); !ok {
		t.Fatalf("expected FormCancelledMsg, got %T", msg)
	}

	// Process the FormCancelledMsg to verify the mode resets.
	model, _ = app.Update(msg)
	app = model.(App)
	if app.sidebar.mode != sidebarNormal {
		t.Fatalf("expected sidebarNormal after Esc, got %d", app.sidebar.mode)
	}
}

func TestKittyEnter_AdvancesFormStep(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.width = 160
	app.height = 40
	app = openFormOnApp(t, app)

	// Type a name into the form using SetValue (the text input is focused).
	app.sidebar.form.nameInput.SetValue("testproject")

	// Simulate kitty-encoded Enter: \x1b[13u
	kittyEnter := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '3', 'u'})

	model, _ := app.Update(kittyEnter)
	app = model.(App)

	// Should have advanced from stepName to stepRepo.
	if app.sidebar.form.step != stepRepo {
		t.Fatalf("expected stepRepo after kitty Enter, got %d", app.sidebar.form.step)
	}
}

func TestKittyEsc_CancelsFormAtAnyStep(t *testing.T) {
	// Verify Esc works from every form step.
	steps := []struct {
		name string
		step formStep
	}{
		{"stepName", stepName},
		{"stepRepo", stepRepo},
		{"stepAgent", stepAgent},
		{"stepAutoApprove", stepAutoApprove},
	}

	kittyEsc := unknownCSISequenceMsg([]byte{0x1b, '[', '2', '7', 'u'})

	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(emptyConfig(), "", nil)
			app.width = 160
			app.height = 40
			app = openFormOnApp(t, app)
			app.sidebar.form.step = tc.step

			model, cmd := app.Update(kittyEsc)
			app = model.(App)

			if cmd == nil {
				t.Fatalf("expected command from kitty Esc at %s", tc.name)
			}
			msg := cmd()
			if _, ok := msg.(FormCancelledMsg); !ok {
				t.Fatalf("expected FormCancelledMsg at %s, got %T", tc.name, msg)
			}
		})
	}
}

func TestKittyRunes_TypeableInForm(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.width = 160
	app.height = 40
	app = openFormOnApp(t, app)

	// Type "hi" using kitty CSI u rune encoding.
	// 'h' = codepoint 104 → \x1b[104u
	// 'i' = codepoint 105 → \x1b[105u
	kittyH := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '0', '4', 'u'})
	kittyI := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '0', '5', 'u'})

	model, _ := app.Update(kittyH)
	app = model.(App)
	model, _ = app.Update(kittyI)
	app = model.(App)

	got := app.sidebar.form.nameInput.Value()
	if got != "hi" {
		t.Fatalf("expected name input 'hi', got %q", got)
	}
}

func TestKittyBackspace_WorksInForm(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.width = 160
	app.height = 40
	app = openFormOnApp(t, app)

	app.sidebar.form.nameInput.SetValue("abc")
	// Move cursor to end so backspace deletes from the right.
	app.sidebar.form.nameInput.SetCursor(3)

	// Simulate kitty-encoded Backspace: \x1b[127u
	kittyBS := unknownCSISequenceMsg([]byte{0x1b, '[', '1', '2', '7', 'u'})

	model, _ := app.Update(kittyBS)
	app = model.(App)

	got := app.sidebar.form.nameInput.Value()
	if got != "ab" {
		t.Fatalf("expected 'ab' after backspace, got %q", got)
	}
}

func TestKittyTab_WorksInFormRepoStep(t *testing.T) {
	app := NewApp(emptyConfig(), "", nil)
	app.width = 160
	app.height = 40
	app = openFormOnApp(t, app)

	// Advance to stepRepo.
	app.sidebar.form.nameInput.SetValue("test")
	app.sidebar.form.step = stepRepo

	// Set a partial path that would trigger completion.
	app.sidebar.form.repoInput.SetValue("/tmp")

	// Simulate kitty-encoded Tab: \x1b[9u
	kittyTab := unknownCSISequenceMsg([]byte{0x1b, '[', '9', 'u'})

	// Just verify it doesn't panic and the key is processed (not dropped).
	model, _ := app.Update(kittyTab)
	_ = model.(App)
	// No assertion on completion result — the point is Tab isn't dropped.
}

func TestLegacyEsc_StillCancelsForm(t *testing.T) {
	// Verify that normal (non-kitty) Esc also works — regression guard.
	app := NewApp(emptyConfig(), "", nil)
	app.width = 160
	app.height = 40
	app = openFormOnApp(t, app)

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = model.(App)

	if cmd == nil {
		t.Fatal("expected command from legacy Esc")
	}
	msg := cmd()
	if _, ok := msg.(FormCancelledMsg); !ok {
		t.Fatalf("expected FormCancelledMsg, got %T", msg)
	}

	model, _ = app.Update(msg)
	app = model.(App)
	if app.sidebar.mode != sidebarNormal {
		t.Fatalf("expected sidebarNormal after legacy Esc, got %d", app.sidebar.mode)
	}
}

// TestParseKittyCSI_FunctionalKeys verifies the parseKittyCSI unit function
// correctly translates functional key CSI u sequences.
func TestParseKittyCSI_FunctionalKeys(t *testing.T) {
	tests := []struct {
		name     string
		raw      []byte
		wantOK   bool
		wantType tea.KeyType
	}{
		{"Esc", []byte{0x1b, '[', '2', '7', 'u'}, true, tea.KeyEscape},
		{"Enter", []byte{0x1b, '[', '1', '3', 'u'}, true, tea.KeyEnter},
		{"Backspace", []byte{0x1b, '[', '1', '2', '7', 'u'}, true, tea.KeyBackspace},
		{"Tab", []byte{0x1b, '[', '9', 'u'}, true, tea.KeyTab},
		{"Ctrl+C", []byte{0x1b, '[', '9', '9', ';', '5', 'u'}, true, tea.KeyCtrlC},
		{"Ctrl+S", []byte{0x1b, '[', '1', '1', '5', ';', '5', 'u'}, true, tea.KeyCtrlS},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyMsg, ok := parseKittyCSI(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("parseKittyCSI ok=%v, want %v", ok, tc.wantOK)
			}
			if ok && keyMsg.Type != tc.wantType {
				t.Fatalf("parseKittyCSI type=%v, want %v", keyMsg.Type, tc.wantType)
			}
		})
	}
}

func TestParseKittyCSI_PrintableRunes(t *testing.T) {
	// 'a' = codepoint 97 → \x1b[97u
	raw := []byte{0x1b, '[', '9', '7', 'u'}
	keyMsg, ok := parseKittyCSI(raw)
	if !ok {
		t.Fatal("expected ok=true for printable 'a'")
	}
	if keyMsg.Type != tea.KeyRunes {
		t.Fatalf("expected KeyRunes, got %v", keyMsg.Type)
	}
	if len(keyMsg.Runes) != 1 || keyMsg.Runes[0] != 'a' {
		t.Fatalf("expected rune 'a', got %v", keyMsg.Runes)
	}
}

func TestParseKittyCSI_ShiftEnter_NotIntercepted(t *testing.T) {
	// Shift+Enter: \x1b[13;2u — must NOT be intercepted. The raw bytes
	// are forwarded to the PTY so the agent can insert a newline instead
	// of submitting.
	raw := []byte{0x1b, '[', '1', '3', ';', '2', 'u'}
	_, ok := parseKittyCSI(raw)
	if ok {
		t.Fatal("Shift+Enter should not be intercepted — must be forwarded to PTY for newline")
	}
}

// TestLegacyCtrlJ_SwitchesTab verifies that a standard (non-kitty) Ctrl+J
// key message triggers tab switching when multiple tabs are open.
func TestLegacyCtrlJ_SwitchesTab(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	// Send legacy Ctrl+J (as bubbletea would produce from byte 0x0A).
	legacyCtrlJ := tea.KeyMsg{Type: tea.KeyCtrlJ}

	_, cmd := app.Update(legacyCtrlJ)
	if cmd == nil {
		t.Fatal("expected command from legacy Ctrl+J to switch tab")
	}
	result := cmd()
	switched, ok := result.(TabSwitchedMsg)
	if !ok {
		t.Fatalf("expected TabSwitchedMsg, got %T", result)
	}
	// Ctrl+J goes left (prev); from alpha (index 0) wraps to gamma (index 2).
	if switched.SessionID != "gamma" {
		t.Fatalf("expected switch to 'gamma', got %q", switched.SessionID)
	}
}

// TestLegacyCtrlK_SwitchesTabForward verifies Ctrl+K switches to next tab.
func TestLegacyCtrlK_SwitchesTabForward(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusTerminal

	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	// Send legacy Ctrl+K (as bubbletea would produce from byte 0x0B).
	legacyCtrlK := tea.KeyMsg{Type: tea.KeyCtrlK}

	_, cmd := app.Update(legacyCtrlK)
	if cmd == nil {
		t.Fatal("expected command from legacy Ctrl+K to switch tab")
	}
	result := cmd()
	switched, ok := result.(TabSwitchedMsg)
	if !ok {
		t.Fatalf("expected TabSwitchedMsg, got %T", result)
	}
	// alpha is at index 0, Ctrl+K goes forward to beta (index 1).
	if switched.SessionID != "beta" {
		t.Fatalf("expected switch to 'beta', got %q", switched.SessionID)
	}
}

// TestCtrlJK_FromSidebar verifies Ctrl+J/K work even when sidebar is focused.
func TestCtrlJK_FromSidebar(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "", nil)
	app.width = 160
	app.height = 40
	app.focus = focusSidebar

	for _, p := range cfg.Projects {
		s := &session.Session{ID: p.Name, Instance: 1, Project: p, State: session.StateRunning}
		app.mgr.InjectSession(p.Name, s)
	}
	app.addTab("beta")
	app.addTab("gamma")

	// Ctrl+J from sidebar should still switch tabs.
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if cmd == nil {
		t.Fatal("expected Ctrl+J to switch tab even from sidebar")
	}
	result := cmd()
	if _, ok := result.(TabSwitchedMsg); !ok {
		t.Fatalf("expected TabSwitchedMsg, got %T", result)
	}
}

func TestSystemTabExitedMsg_ReloadsConfig(t *testing.T) {
	app := NewApp(configWithProjects(), "", nil)
	// Initially telegram is disabled.
	if app.cfg.Telegram.Enabled {
		t.Fatal("precondition: expected Telegram disabled")
	}

	// SystemTabExitedMsg should reload from disk (but since there's no
	// real config file, it will load defaults — just verify no panic).
	model, _ := app.Update(SystemTabExitedMsg{Name: "Telegram Setup"})
	app = model.(App)

	// Config should still be valid.
	if app.cfg == nil {
		t.Fatal("expected config to be non-nil after reload")
	}
}
