package tui

import (
	"testing"

	"github.com/amir/maestro/internal/config"
	tea "github.com/charmbracelet/bubbletea"
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

func TestAppEscapeTogglesFocus(t *testing.T) {
	app := NewApp(configWithProjects(), "")
	if app.focus != focusTerminal {
		t.Fatal("precondition: expected focusTerminal")
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	model, _ := app.Update(msg)
	app = model.(App)
	if app.focus != focusSidebar {
		t.Fatalf("expected focusSidebar after Escape, got %d", app.focus)
	}
	if !app.sidebar.focused {
		t.Fatal("expected sidebar.focused=true")
	}

	model, _ = app.Update(msg)
	app = model.(App)
	if app.focus != focusTerminal {
		t.Fatalf("expected focusTerminal after second Escape, got %d", app.focus)
	}
}

func TestAppEscapePassesToSidebarInFormMode(t *testing.T) {
	app := NewApp(emptyConfig(), "")
	app.focus = focusSidebar
	app.sidebar.focused = true
	app.sidebar.mode = sidebarForm

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	model, cmd := app.Update(msg)
	app = model.(App)

	// Focus should NOT toggle — sidebar should still be focused.
	if app.focus != focusSidebar {
		t.Fatalf("expected focus to stay on sidebar, got %d", app.focus)
	}

	// The cmd should produce a FormCancelledMsg (from the form's Escape handler).
	if cmd == nil {
		t.Fatal("expected a command from form Escape")
	}
	result := cmd()
	if _, ok := result.(FormCancelledMsg); !ok {
		t.Fatalf("expected FormCancelledMsg, got %T", result)
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

	borderX := app.sidebar.Width() - 1

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

	// Motion to X=40.
	motion := tea.MouseMsg{
		X: 40, Y: 10,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
	}
	model, _ = app.Update(motion)
	app = model.(App)

	// Sidebar content width should have updated (mouse X maps directly to contentWidth).
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
