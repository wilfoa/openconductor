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
)

func emptyConfig() *config.Config {
	return &config.Config{Projects: []config.Project{}}
}

func configWithProjects() *config.Config {
	return &config.Config{
		Projects: []config.Project{
			{Name: "proj1", Repo: "/tmp/p1", Agent: config.AgentClaudeCode},
			{Name: "proj2", Repo: "/tmp/p2", Agent: config.AgentCodex},
		},
	}
}

func TestNewAppFocusesSidebarWhenEmpty(t *testing.T) {
	app := NewApp(emptyConfig(), "")
	if app.focus != focusSidebar {
		t.Fatalf("expected focusSidebar, got %d", app.focus)
	}
}

func TestNewAppFocusesTerminalWithProjects(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	if app.focus != focusTerminal {
		t.Fatalf("expected focusTerminal, got %d", app.focus)
	}
}

func TestCtrlSTogglesFocus(t *testing.T) {
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(emptyConfig(), "")
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
	app := NewApp(emptyConfig(), "")

	msg := ProjectAddedMsg{
		Project: config.Project{Name: "new", Repo: "/tmp/new", Agent: config.AgentGemini},
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
	app := NewApp(configWithProjects(), "")

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
	app := NewApp(emptyConfig(), "")
	app.sidebar.mode = sidebarForm

	model, _ := app.Update(FormCancelledMsg{})
	app = model.(App)

	if app.sidebar.mode != sidebarNormal {
		t.Fatalf("expected sidebarNormal, got %d", app.sidebar.mode)
	}
}

func TestAppMouseRoutesToSidebar(t *testing.T) {
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
	if len(app.openTabs) != 1 {
		t.Fatalf("expected 1 open tab, got %d", len(app.openTabs))
	}
	if app.openTabs[0] != "proj1" {
		t.Fatalf("expected first tab 'proj1', got %q", app.openTabs[0])
	}
}

func TestNewAppEmptyHasNoTabs(t *testing.T) {
	app := NewApp(emptyConfig(), "")
	if len(app.openTabs) != 0 {
		t.Fatalf("expected 0 open tabs, got %d", len(app.openTabs))
	}
}

func TestAppProjectSwitchedAddsTab(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	// Initially only proj1 is open.
	if len(app.openTabs) != 1 {
		t.Fatalf("precondition: expected 1 tab, got %d", len(app.openTabs))
	}

	// Switch to proj2.
	msg := ProjectSwitchedMsg{Index: 1, Project: app.cfg.Projects[1]}
	model, _ := app.Update(msg)
	app = model.(App)

	if len(app.openTabs) != 2 {
		t.Fatalf("expected 2 open tabs, got %d", len(app.openTabs))
	}
	if app.openTabs[0] != "proj1" || app.openTabs[1] != "proj2" {
		t.Fatalf("expected tabs [proj1, proj2], got %v", app.openTabs)
	}
}

func TestAppProjectSwitchedNoDuplicateTab(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	// Switch to proj1 again — should not duplicate.
	msg := ProjectSwitchedMsg{Index: 0, Project: app.cfg.Projects[0]}
	model, _ := app.Update(msg)
	app = model.(App)

	if len(app.openTabs) != 1 {
		t.Fatalf("expected 1 tab (no duplicate), got %d", len(app.openTabs))
	}
}

func TestAppProjectDeletedRemovesTab(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	// Open both tabs.
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

func TestAppProjectAddedOpensTab(t *testing.T) {
	app := NewApp(emptyConfig(), "")

	msg := ProjectAddedMsg{
		Project: config.Project{Name: "new", Repo: "/tmp/new", Agent: config.AgentGemini},
	}
	model, _ := app.Update(msg)
	app = model.(App)

	if len(app.openTabs) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(app.openTabs))
	}
	if app.openTabs[0] != "new" {
		t.Fatalf("expected tab 'new', got %q", app.openTabs[0])
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
			{Name: "beta", Repo: "/tmp/b", Agent: config.AgentCodex},
			{Name: "gamma", Repo: "/tmp/c", Agent: config.AgentGemini},
		},
	}
}

// testTabWidth returns the rendered width of a tab, matching the logic in
// tabBarView and tabHitTest. Active tab has no close button; inactive tabs
// include " ✕" and use state-colored styles.
func testTabWidth(name string, state SessionState, isActive bool) int {
	char := badgeChar(state, 0) // animFrame=0 for steady badge in tests
	label := char + " " + name + " ✕"
	if isActive {
		return lipgloss.Width(tabActiveStyle.Render(label))
	}
	return lipgloss.Width(inactiveTabStyle(state).Render(label))
}

func TestTabHitTestSingleTab(t *testing.T) {
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(cfg, "")
	// Open all three tabs.
	app.addTab("beta")
	app.addTab("gamma")

	activeName := cfg.Projects[app.active].Name

	widths := make([]int, 3)
	for i, name := range app.openTabs {
		state := app.sidebar.states[name]
		widths[i] = testTabWidth(name, state, name == activeName)
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
	app := NewApp(configWithProjects(), "")
	if idx, _ := app.tabHitTest(-5); idx != -1 {
		t.Fatalf("expected -1 for negative X, got %d", idx)
	}
}

func TestTabHitTestCloseRegion(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "")
	app.addTab("beta")

	// beta is inactive (alpha is active). Clicking the rightmost portion
	// of beta's tab should report isClose=true.
	activeName := cfg.Projects[app.active].Name
	firstWidth := testTabWidth("alpha", app.sidebar.states["alpha"], "alpha" == activeName)
	secondWidth := testTabWidth("beta", app.sidebar.states["beta"], false)

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
	app := NewApp(configWith3Projects(), "")

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

func TestTabClickSwitchesProject(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "")
	app.width = 160
	app.height = 40

	// Open all three tabs.
	app.addTab("beta")
	app.addTab("gamma")

	// Compute where the second tab starts (after the active first tab).
	firstWidth := testTabWidth("alpha", app.sidebar.states["alpha"], true)

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
	switched, ok := result.(ProjectSwitchedMsg)
	if !ok {
		t.Fatalf("expected ProjectSwitchedMsg, got %T", result)
	}
	if switched.Project.Name != "beta" {
		t.Fatalf("expected project 'beta', got %q", switched.Project.Name)
	}
	if switched.Index != 1 {
		t.Fatalf("expected index 1, got %d", switched.Index)
	}
}

func TestTabCloseClick(t *testing.T) {
	cfg := configWith3Projects()
	app := NewApp(cfg, "")
	app.width = 160
	app.height = 40

	app.addTab("beta")
	app.addTab("gamma")

	// Click the close region of the second tab (beta).
	firstWidth := testTabWidth("alpha", app.sidebar.states["alpha"], true)
	secondWidth := testTabWidth("beta", app.sidebar.states["beta"], false)

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
	app := NewApp(configWith3Projects(), "")
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
	app := NewApp(configWith3Projects(), "")
	app.addTab("beta")
	// alpha (index 0) is the active tab.

	// Close the active tab.
	model, cmd := app.Update(TabClosedMsg{Name: "alpha"})
	app = model.(App)

	// Should produce a ProjectSwitchedMsg to switch to the remaining tab.
	if cmd == nil {
		t.Fatal("expected a command when closing active tab")
	}
	result := cmd()
	switched, ok := result.(ProjectSwitchedMsg)
	if !ok {
		t.Fatalf("expected ProjectSwitchedMsg, got %T", result)
	}
	if switched.Project.Name != "beta" {
		t.Fatalf("expected switch to 'beta', got %q", switched.Project.Name)
	}
}

func TestTabClosedLastTabClearsTerminal(t *testing.T) {
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
	app.sidebar.states["proj1"] = StateWorking
	app.statusbar.states["proj1"] = StateWorking

	model, _ := app.Update(TabClosedMsg{Name: "proj1"})
	app = model.(App)

	if app.sidebar.states["proj1"] != StateIdle {
		t.Fatalf("expected sidebar state Idle, got %v", app.sidebar.states["proj1"])
	}
	if app.statusbar.states["proj1"] != StateIdle {
		t.Fatalf("expected statusbar state Idle, got %v", app.statusbar.states["proj1"])
	}
}

func TestTabClickInGapNoSwitch(t *testing.T) {
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(cfg, "")
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
	app := NewApp(cfg, "")
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
	app := NewApp(configWithProjects(), "")

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
	app := NewApp(configWithProjects(), "")
	app.stateStickUntil["proj1"] = time.Now().Add(10 * time.Second)

	msg := ProjectSwitchedMsg{Index: 0, Project: app.cfg.Projects[0]}
	model, _ := app.Update(msg)
	app = model.(App)

	if _, ok := app.stateStickUntil["proj1"]; ok {
		t.Fatal("expected sticky timer to be cleared after project switch")
	}
}

// ── Inactive tab style tests ────────────────────────────────────

// ── Too-small terminal tests ────────────────────────────────────

func TestTooSmallBothDimensions(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	app.width = 40
	app.height = 5
	app.ready = true
	if !app.tooSmall() {
		t.Fatal("expected tooSmall() for 40×5")
	}
}

func TestTooSmallWidthOnly(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	app.width = minAppWidth - 1
	app.height = 40
	app.ready = true
	if !app.tooSmall() {
		t.Fatal("expected tooSmall() for narrow terminal")
	}
}

func TestTooSmallHeightOnly(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	app.width = 120
	app.height = minAppHeight - 1
	app.ready = true
	if !app.tooSmall() {
		t.Fatal("expected tooSmall() for short terminal")
	}
}

func TestNotTooSmallAtMinimum(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	app.width = minAppWidth
	app.height = minAppHeight
	app.ready = true
	if app.tooSmall() {
		t.Fatalf("expected not tooSmall() at exact minimum %d×%d", minAppWidth, minAppHeight)
	}
}

func TestTooSmallViewShowsMessage(t *testing.T) {
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	app := NewApp(configWithProjects(), "")
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
	tests := []struct {
		state    SessionState
		expected lipgloss.Style
	}{
		{StateNeedsAttention, tabAttentionStyle},
		{StateNeedsPermission, tabPermissionStyle},
		{StateAsking, tabAskingStyle},
		{StateError, tabErrorStyle},
		{StateDone, tabDoneStyle},
		{StateWorking, tabStyle},
		{StateIdle, tabStyle},
	}
	for _, tt := range tests {
		got := inactiveTabStyle(tt.state)
		// Compare rendered output of a test string to verify style selection.
		if got.Render("X") != tt.expected.Render("X") {
			t.Errorf("inactiveTabStyle(%v): style mismatch", tt.state)
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
