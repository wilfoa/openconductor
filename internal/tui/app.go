package tui

import (
	"strings"
	"time"

	"github.com/amir/maestro/internal/config"
	"github.com/amir/maestro/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type focus int

const (
	focusTerminal focus = iota
	focusSidebar
)

// sessionOutputMsg carries raw bytes read from a session's PTY, tagged with
// the originating project name.
type sessionOutputMsg struct {
	ProjectName string
	Data        []byte
}

// sessionStartedMsg signals that a session was started and its read loop
// channel is ready for listening.
type sessionStartedMsg struct {
	ProjectName string
}

// sessionExitedMsg signals that a session's PTY read loop has ended.
type sessionExitedMsg struct {
	ProjectName string
}

// App is the top-level bubbletea model for Maestro.
type App struct {
	cfg          *config.Config
	configPath   string
	sidebar      sidebarModel
	terminal     terminalModel
	statusbar    statusBarModel
	focus        focus
	width        int
	height       int
	ready        bool
	active       int // index of active project
	mgr          *session.Manager
	sidebarWidth int      // content width of sidebar (excludes padding/border)
	dragging     bool     // true during separator drag
	openTabs     []string // project names of opened tabs, in visit order
}

// NewApp creates the application model from a loaded configuration.
func NewApp(cfg *config.Config, configPath string) App {
	initialFocus := focusTerminal
	if len(cfg.Projects) == 0 {
		initialFocus = focusSidebar
	}

	// Pre-open a tab for the first project (it will be auto-started).
	var openTabs []string
	if len(cfg.Projects) > 0 {
		openTabs = []string{cfg.Projects[0].Name}
	}

	return App{
		cfg:          cfg,
		configPath:   configPath,
		sidebar:      newSidebarModel(cfg.Projects, defaultSidebarWidth),
		terminal:     newTerminalModel(),
		statusbar:    newStatusBarModel(cfg.Projects),
		focus:        initialFocus,
		active:       0,
		mgr:          session.NewManager(),
		sidebarWidth: defaultSidebarWidth,
		openTabs:     openTabs,
	}
}

// Init returns the initial command for the bubbletea program.
func (a App) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// Update handles incoming messages and returns the updated model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		// Re-clamp sidebar width for new terminal size.
		a.clampAndSetSidebarWidth(a.sidebarWidth)
		a.layout()

		// Resize all existing sessions.
		termW, termH := a.termDimensions()
		for _, p := range a.cfg.Projects {
			if s := a.mgr.GetSession(p.Name); s != nil {
				s.Resize(termW, termH)
			}
		}

		// Start the first project session if nothing is active yet.
		if len(a.cfg.Projects) == 0 {
			// No projects — keep sidebar focused.
			a.focus = focusSidebar
			a.sidebar.focused = true
			a.terminal.focused = false
		} else if a.mgr.ActiveSession() == nil {
			cmd := a.startSessionCmd(a.cfg.Projects[0])
			cmds = append(cmds, cmd)
		}

		return a, tea.Batch(cmds...)

	case tea.KeyMsg:
		if isKey(msg, keys.Quit) {
			a.mgr.Close()
			a.terminal.Close()
			return a, tea.Quit
		}

		// Escape: toggle focus only when sidebar is in normal mode.
		// When the sidebar has a form or confirm dialog, let it handle Escape.
		if isKey(msg, keys.ToggleFocus) {
			if a.focus == focusSidebar && a.sidebar.mode != sidebarNormal {
				// Pass through to sidebar.
				var cmd tea.Cmd
				a.sidebar, cmd = a.sidebar.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return a, tea.Batch(cmds...)
			}
			a.toggleFocus()
			return a, nil
		}

		if a.focus == focusSidebar {
			var cmd tea.Cmd
			a.sidebar, cmd = a.sidebar.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}

		// Terminal focused — send keystrokes to the active session's PTY.
		if s := a.mgr.ActiveSession(); s != nil {
			data := keyToBytes(msg)
			if data != nil {
				s.Write(data)
			}
		}
		return a, tea.Batch(cmds...)

	case tea.MouseMsg:
		// Drag state machine for sidebar separator.
		if a.dragging {
			switch msg.Action {
			case tea.MouseActionMotion:
				a.clampAndSetSidebarWidth(msg.X)
				a.layout()
				a.resizeAllSessions()
			case tea.MouseActionRelease:
				a.dragging = false
				a.sidebar.dragging = false
			}
			return a, nil
		}

		sbWidth := a.sidebar.Width()

		// Detect click on border (±1 column of sidebar right edge).
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			borderX := sbWidth - 1
			if msg.X >= borderX-1 && msg.X <= borderX+1 {
				a.dragging = true
				a.sidebar.dragging = true
				return a, nil
			}
		}

		if msg.X < sbWidth {
			// Route to sidebar and focus it.
			if a.focus != focusSidebar {
				a.focus = focusSidebar
				a.sidebar.focused = true
				a.terminal.focused = false
			}
			var cmd tea.Cmd
			a.sidebar, cmd = a.sidebar.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			// Click in terminal area — focus terminal.
			if a.focus != focusTerminal {
				a.focus = focusTerminal
				a.sidebar.focused = false
				a.terminal.focused = true
			}
		}
		return a, tea.Batch(cmds...)

	case ProjectSwitchedMsg:
		a.active = msg.Index
		project := msg.Project
		a.statusbar.activeName = project.Name
		a.addTab(project.Name)

		// Focus terminal when switching projects.
		a.focus = focusTerminal
		a.sidebar.focused = false
		a.terminal.focused = true

		// If a session exists, just switch to it. Otherwise start one.
		if s := a.mgr.GetSession(project.Name); s != nil {
			a.mgr.SetActive(project.Name)
			a.syncTerminalFromSession()
		} else {
			cmd := a.startSessionCmd(project)
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)

	case ProjectAddedMsg:
		project := msg.Project
		a.cfg.Projects = append(a.cfg.Projects, project)
		a.sidebar.projects = a.cfg.Projects
		a.sidebar.states[project.Name] = StateIdle
		a.sidebar.selected = len(a.cfg.Projects) - 1
		a.sidebar.mode = sidebarNormal
		a.addTab(project.Name)

		a.statusbar = newStatusBarModel(a.cfg.Projects)
		a.statusbar.activeName = project.Name
		// Carry over existing states.
		for name, state := range a.sidebar.states {
			a.statusbar.states[name] = state
		}

		a.active = len(a.cfg.Projects) - 1

		// Focus terminal.
		a.focus = focusTerminal
		a.sidebar.focused = false
		a.terminal.focused = true

		// Save config and start session.
		cmds = append(cmds, a.saveConfigCmd())
		cmds = append(cmds, a.startSessionCmd(project))
		return a, tea.Batch(cmds...)

	case ProjectDeletedMsg:
		name := msg.Name
		a.removeTab(name)

		// Remove from config.
		var newProjects []config.Project
		for _, p := range a.cfg.Projects {
			if p.Name != name {
				newProjects = append(newProjects, p)
			}
		}
		if newProjects == nil {
			newProjects = []config.Project{}
		}
		a.cfg.Projects = newProjects
		a.sidebar.projects = a.cfg.Projects
		delete(a.sidebar.states, name)
		a.sidebar.mode = sidebarNormal

		// Clamp selection.
		if a.sidebar.selected >= len(a.cfg.Projects) {
			a.sidebar.selected = max(0, len(a.cfg.Projects)-1)
		}

		// Rebuild statusbar.
		a.statusbar = newStatusBarModel(a.cfg.Projects)
		for n, state := range a.sidebar.states {
			a.statusbar.states[n] = state
		}

		// Save config.
		cmds = append(cmds, a.saveConfigCmd())

		// Stop the deleted session and clean up its channel.
		a.mgr.StopSession(name)
		delete(sessionChannels, name)

		// If the deleted project was the active one, switch.
		if a.mgr.ActiveName() == "" && len(a.cfg.Projects) > 0 {
			a.active = a.sidebar.selected
			cmds = append(cmds, a.startSessionCmd(a.cfg.Projects[a.active]))
		} else if len(a.cfg.Projects) == 0 {
			a.terminal.active = false
			a.focus = focusSidebar
			a.sidebar.focused = true
			a.terminal.focused = false
		}

		return a, tea.Batch(cmds...)

	case FormCancelledMsg:
		a.sidebar.mode = sidebarNormal
		return a, nil

	case ConfigSavedMsg:
		// Could show an error in the status bar, but for now just ignore.
		return a, nil

	case sessionStartedMsg:
		a.mgr.SetActive(msg.ProjectName)
		a.statusbar.activeName = msg.ProjectName
		a.addTab(msg.ProjectName)
		a.syncTerminalFromSession()
		// Begin listening for output from this session.
		cmds = append(cmds, a.waitForSessionOutput(msg.ProjectName))
		return a, tea.Batch(cmds...)

	case sessionOutputMsg:
		if msg.ProjectName == a.mgr.ActiveName() {
			a.syncTerminalFromSession()
		}
		// Continue listening.
		cmds = append(cmds, a.waitForSessionOutput(msg.ProjectName))
		return a, tea.Batch(cmds...)

	case sessionExitedMsg:
		if msg.ProjectName == a.mgr.ActiveName() {
			a.terminal.active = false
		}
		a.sidebar.states[msg.ProjectName] = StateDone
		a.statusbar.states[msg.ProjectName] = StateDone
		return a, nil

	case SessionStateChangedMsg:
		a.sidebar.states[msg.ProjectName] = msg.State
		a.statusbar.states[msg.ProjectName] = msg.State
		return a, nil

	case TickMsg:
		// Run attention detection on the active session.
		a.checkAttention()
		cmds = append(cmds, tickCmd())
		return a, tea.Batch(cmds...)
	}

	return a, tea.Batch(cmds...)
}

// View renders the complete TUI.
func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	sidebarView := a.sidebar.View()
	tabHeader := a.tabHeaderView()
	tabBorder := a.tabBorderView()

	style := terminalStyle
	if a.focus == focusTerminal {
		style = terminalFocusedStyle
	}
	terminalView := style.Render(a.terminal.View())

	// Right panel: tab header + border (with active tab gap) + terminal.
	rightPanel := lipgloss.JoinVertical(lipgloss.Left, tabHeader, tabBorder, terminalView)

	a.statusbar.sidebarFocused = (a.focus == focusSidebar)
	statusbarView := a.statusbar.View()

	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, rightPanel)
	return lipgloss.JoinVertical(lipgloss.Left, main, statusbarView)
}

func (a *App) toggleFocus() {
	if a.focus == focusTerminal {
		a.focus = focusSidebar
		a.sidebar.focused = true
		a.terminal.focused = false
	} else {
		a.focus = focusTerminal
		a.sidebar.focused = false
		a.terminal.focused = true
	}
}

// tabHeaderView renders the multi-tab bar above the terminal panel.
// Active tab: bracket-highlight [● name] on default (terminal) bg.
// Inactive tab: dim text on colorBgAlt bg.
// No separator character — the background contrast IS the separator.
func (a App) tabHeaderView() string {
	sbWidth := a.sidebar.Width()
	panelWidth := a.width - sbWidth

	if len(a.openTabs) == 0 {
		return tabHeaderStyle.Width(panelWidth).Render("")
	}

	activeName := ""
	if a.active >= 0 && a.active < len(a.cfg.Projects) {
		activeName = a.cfg.Projects[a.active].Name
	}

	tabCount := len(a.openTabs)
	tabWidth := panelWidth / tabCount
	remainder := panelWidth % tabCount

	var sb strings.Builder
	for i, name := range a.openTabs {
		w := tabWidth
		if i < remainder {
			w++
		}

		state := a.sidebar.states[name]
		char := badgeChar(state)

		if name == activeName {
			label := " [" + char + " " + name + "]"
			sb.WriteString(tabActiveStyle.Width(w).Render(label))
		} else {
			label := " " + char + " " + name
			sb.WriteString(tabInactiveStyle.Width(w).Render(label))
		}
	}

	return sb.String()
}

// tabBorderView renders the border line between tabs and terminal.
// Under inactive tabs: visible ─── line.
// Under active tab: blank (default bg — merges into terminal).
// No bgAlt on the border — just the ─ character vs empty space.
func (a App) tabBorderView() string {
	sbWidth := a.sidebar.Width()
	panelWidth := a.width - sbWidth

	if len(a.openTabs) == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).
			Render(strings.Repeat("─", panelWidth))
	}

	activeName := ""
	if a.active >= 0 && a.active < len(a.cfg.Projects) {
		activeName = a.cfg.Projects[a.active].Name
	}

	tabCount := len(a.openTabs)
	tabWidth := panelWidth / tabCount
	remainder := panelWidth % tabCount

	// Find active tab's column span.
	activeStart, activeEnd := 0, 0
	col := 0
	for i, name := range a.openTabs {
		w := tabWidth
		if i < remainder {
			w++
		}
		if name == activeName {
			activeStart = col
			activeEnd = col + w
		}
		col += w
	}

	// Use colorMuted for the ─ so it's clearly visible.
	borderStyle := lipgloss.NewStyle().Foreground(colorMuted)

	var border strings.Builder
	leftLen := activeStart
	gapLen := activeEnd - activeStart
	rightLen := panelWidth - activeEnd

	if leftLen > 0 {
		border.WriteString(borderStyle.Render(strings.Repeat("─", leftLen)))
	}
	if gapLen > 0 {
		border.WriteString(strings.Repeat(" ", gapLen))
	}
	if rightLen > 0 {
		border.WriteString(borderStyle.Render(strings.Repeat("─", rightLen)))
	}

	return border.String()
}

func (a *App) layout() {
	sbWidth := a.sidebar.Width()
	termWidth := a.width - sbWidth - 1 // -1 for terminal PaddingLeft(1)
	panelHeight := a.height - 1        // -1 for status bar
	termHeight := panelHeight - 2      // -2 for tab header + border

	a.sidebar.height = panelHeight
	a.terminal.SetSize(termWidth, termHeight)
	a.statusbar.width = a.width
}

// termDimensions returns the terminal panel width and height.
func (a *App) termDimensions() (int, int) {
	sbWidth := a.sidebar.Width()
	termWidth := a.width - sbWidth - 1 // -1 for terminal PaddingLeft(1)
	termHeight := a.height - 3         // -1 status bar, -2 tab header + border
	return termWidth, termHeight
}

// startSessionCmd creates a tea.Cmd that starts a session for the given
// project in the background and emits a sessionStartedMsg on success.
func (a *App) startSessionCmd(project config.Project) tea.Cmd {
	mgr := a.mgr
	termW, termH := a.termDimensions()

	return func() tea.Msg {
		_, err := mgr.StartSession(project, termW, termH)
		if err != nil {
			return SessionStateChangedMsg{
				ProjectName: project.Name,
				State:       StateError,
				Detail:      err.Error(),
			}
		}
		return sessionStartedMsg{ProjectName: project.Name}
	}
}

// saveConfigCmd returns a tea.Cmd that persists the config to disk.
func (a *App) saveConfigCmd() tea.Cmd {
	cfg := a.cfg
	path := a.configPath
	return func() tea.Msg {
		err := cfg.Save(path)
		return ConfigSavedMsg{Err: err}
	}
}

// waitForSessionOutput returns a tea.Cmd that blocks until data is available
// from the session's read loop, then emits a sessionOutputMsg.
func (a *App) waitForSessionOutput(projectName string) tea.Cmd {
	s := a.mgr.GetSession(projectName)
	if s == nil {
		return nil
	}

	return a.readPTYOnce(projectName, s)
}

// sessionChannels caches per-session read-loop channels so we only call
// ReadLoop once per session.
var sessionChannels = make(map[string]<-chan []byte)

// readPTYOnce returns a tea.Cmd that reads from the session's PTY channel.
// On first call for a given project it initialises the channel via ReadLoop.
func (a *App) readPTYOnce(projectName string, s *session.Session) tea.Cmd {
	ch, ok := sessionChannels[projectName]
	if !ok {
		ch = s.ReadLoop()
		sessionChannels[projectName] = ch
	}

	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			delete(sessionChannels, projectName)
			return sessionExitedMsg{ProjectName: projectName}
		}
		return sessionOutputMsg{
			ProjectName: projectName,
			Data:        data,
		}
	}
}

// syncTerminalFromSession copies the active session's vt10x terminal into
// the terminal model so that View() renders the right content.
func (a *App) syncTerminalFromSession() {
	s := a.mgr.ActiveSession()
	if s == nil {
		return
	}

	vt, w, h := s.GetVT()

	a.terminal.mu.Lock()
	a.terminal.vt = vt
	a.terminal.width = w
	a.terminal.height = h
	a.terminal.active = true
	a.terminal.mu.Unlock()
}

// clampAndSetSidebarWidth sets the sidebar content width from a mouse X
// position, clamping between minSidebarWidth and half the terminal width.
// The border is at column contentWidth (0-indexed), so mouse X maps directly.
func (a *App) clampAndSetSidebarWidth(x int) {
	contentWidth := x
	if contentWidth < minSidebarWidth {
		contentWidth = minSidebarWidth
	}
	maxWidth := a.width / 2
	if contentWidth > maxWidth {
		contentWidth = maxWidth
	}
	a.sidebarWidth = contentWidth
	a.sidebar.contentWidth = contentWidth
}

// resizeAllSessions resizes all PTY sessions to the new terminal dimensions.
func (a *App) resizeAllSessions() {
	termW, termH := a.termDimensions()
	for _, p := range a.cfg.Projects {
		if s := a.mgr.GetSession(p.Name); s != nil {
			s.Resize(termW, termH)
		}
	}
}

// addTab appends a project name to the open tabs list if not already present.
func (a *App) addTab(name string) {
	for _, t := range a.openTabs {
		if t == name {
			return
		}
	}
	a.openTabs = append(a.openTabs, name)
}

// removeTab removes a project name from the open tabs list.
func (a *App) removeTab(name string) {
	for i, t := range a.openTabs {
		if t == name {
			a.openTabs = append(a.openTabs[:i], a.openTabs[i+1:]...)
			return
		}
	}
}

// checkAttention runs attention detection on ALL sessions (not just the
// active one) and updates sidebar/statusbar state accordingly.
func (a *App) checkAttention() {
	for _, p := range a.cfg.Projects {
		s := a.mgr.GetSession(p.Name)
		if s == nil {
			continue
		}

		lines := s.GetScreenLines()
		if lines == nil {
			continue
		}

		name := p.Name
		hint := s.Agent.AttentionHints(lines)
		if hint != nil {
			switch hint.Type {
			case "needs_input", "needs_permission":
				a.sidebar.states[name] = StateNeedsAttention
				a.statusbar.states[name] = StateNeedsAttention
			case "error":
				a.sidebar.states[name] = StateError
				a.statusbar.states[name] = StateError
			case "done":
				a.sidebar.states[name] = StateDone
				a.statusbar.states[name] = StateDone
			}
		} else {
			// If running and no hint, show as working.
			if s.State == session.StateRunning {
				a.sidebar.states[name] = StateWorking
				a.statusbar.states[name] = StateWorking
			}
		}
	}
}
