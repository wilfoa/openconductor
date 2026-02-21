// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/session"
)

// Notifier is the interface for sending desktop notifications on attention events.
// This allows the App to use notification.Notifier without a hard import cycle,
// and also makes testing easy with a nil or mock implementation.
type Notifier interface {
	Notify(project string, attnType string, detail string)
}

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

// stateStickDuration is the minimum time an attention state (NeedsAttention,
// Error, Done) is held before it can be downgraded back to Working. This
// prevents visual flicker when attention signals are transient.
const stateStickDuration = 10 * time.Second

// ctrlCWindow is the maximum time between two Ctrl+C presses for them to
// count as a double-tap exit sequence.
const ctrlCWindow = 1 * time.Second

// scrollLinesPerTick is the number of lines scrolled per mouse wheel tick
// in the scrollback buffer. 3 lines matches standard terminal behavior.
const scrollLinesPerTick = 3

// App is the top-level bubbletea model for OpenConductor.
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
	detector     *attention.Detector
	notifier     Notifier
	sidebarWidth int      // content width of sidebar (excludes padding/border)
	dragging     bool     // true during separator drag
	openTabs     []string // project names of opened tabs, in visit order

	// stateStickUntil records the earliest time each project's attention
	// state can be downgraded to Working. Prevents flip-flop when
	// transient signals scroll off screen between ticks.
	stateStickUntil map[string]time.Time

	// animFrame cycles 0..animFrameCount-1 every AnimTickMsg (~600ms) to
	// drive the working badge breathing animation.
	animFrame int

	// lastCtrlC records when Ctrl+C was last pressed. A second press
	// within ctrlCWindow exits OpenConductor; a single press forwards to PTY.
	lastCtrlC time.Time
	// ctrlCHint is true when the "press again to exit" hint should show
	// in the status bar. Cleared on the next non-Ctrl+C key or after
	// the window expires.
	ctrlCHint bool

	// scrollbacks maps project names to their scrollback buffers.
	// Each session gets its own buffer so scrollback is preserved
	// when switching between tabs.
	scrollbacks map[string]*scrollbackBuffer

	// scrollSnapshots stores the last-known screen per project (text for
	// shift detection, glyphs for scrollback capture). The scroll-check
	// tick compares the current VT screen against these to detect lines
	// that scrolled off. This decouples detection from individual PTY
	// writes (which arrive in arbitrary chunks).
	scrollSnapshots      map[string][]string
	scrollGlyphSnapshots map[string][]scrollbackLine

	// scrollDirty tracks which projects received new PTY output since
	// the last scroll check. Only dirty projects are checked on tick.
	scrollDirty map[string]bool
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
		cfg:                  cfg,
		configPath:           configPath,
		sidebar:              newSidebarModel(cfg.Projects, defaultSidebarWidth),
		terminal:             newTerminalModel(),
		statusbar:            newStatusBarModel(cfg.Projects),
		focus:                initialFocus,
		active:               0,
		mgr:                  session.NewManager(),
		detector:             attention.NewDetector(),
		sidebarWidth:         defaultSidebarWidth,
		openTabs:             openTabs,
		stateStickUntil:      make(map[string]time.Time),
		scrollbacks:          make(map[string]*scrollbackBuffer),
		scrollSnapshots:      make(map[string][]string),
		scrollGlyphSnapshots: make(map[string][]scrollbackLine),
		scrollDirty:          make(map[string]bool),
	}
}

// SetClassifier configures the L2 LLM classifier for attention detection.
// Call this before starting the program if the config has LLM settings.
func (a *App) SetClassifier(c *attention.Classifier) {
	a.detector.SetClassifier(c)
}

// SetNotifier configures desktop notifications for attention events.
func (a *App) SetNotifier(n Notifier) {
	a.notifier = n
}

// Init returns the initial command for the bubbletea program.
func (a App) Init() tea.Cmd {
	return tea.Batch(tickCmd(), animTickCmd(), scrollCheckTickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// animFrameCount is the number of frames in the working badge breathing cycle.
// The cycle is: ● bright → • mid → · dim → • mid → repeat.
const animFrameCount = 4

func animTickCmd() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

// scrollCheckInterval is how often we compare screen snapshots to detect
// lines that scrolled off. 100ms is fast enough to catch most scrolling
// while ensuring the VT has a complete screen after PTY chunk reassembly.
const scrollCheckInterval = 100 * time.Millisecond

type scrollCheckTickMsg struct{}

func scrollCheckTickCmd() tea.Cmd {
	return tea.Tick(scrollCheckInterval, func(t time.Time) tea.Msg {
		return scrollCheckTickMsg{}
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

		// When too small, skip layout and session resize — View()
		// will show a placeholder overlay instead.
		if a.tooSmall() {
			return a, tea.Batch(cmds...)
		}

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
		// Ctrl+C double-tap: first press forwards to PTY and shows hint,
		// second press within ctrlCWindow exits OpenConductor.
		if isKey(msg, keys.Quit) {
			now := time.Now()
			if !a.lastCtrlC.IsZero() && now.Sub(a.lastCtrlC) < ctrlCWindow {
				a.mgr.Close()
				a.terminal.Close()
				return a, tea.Quit
			}
			a.lastCtrlC = now
			a.ctrlCHint = true
			a.statusbar.ctrlCHint = true
			// Forward Ctrl+C to the active PTY so the agent receives it.
			if a.focus == focusTerminal {
				if s := a.mgr.ActiveSession(); s != nil {
					s.Write([]byte{0x03}) // ETX
				}
			}
			return a, nil
		}

		// Any other key clears the Ctrl+C hint.
		if a.ctrlCHint {
			a.ctrlCHint = false
			a.statusbar.ctrlCHint = false
		}

		// Ctrl+J / Ctrl+K: switch to next/prev tab.
		// Works regardless of which panel is focused.
		if isKey(msg, tea.KeyCtrlJ) {
			if cmd := a.switchTab(1); cmd != nil {
				return a, cmd
			}
			return a, nil
		}
		if isKey(msg, tea.KeyCtrlK) {
			if cmd := a.switchTab(-1); cmd != nil {
				return a, cmd
			}
			return a, nil
		}

		// Ctrl+S: toggle focus only when sidebar is in normal mode.
		// When the sidebar has a form or confirm dialog, ignore (user must
		// Esc out of the form first).
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

		// Any keyboard input snaps the terminal back to live view.
		if a.terminal.InScrollMode() {
			a.terminal.ScrollToBottom()
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
		// Account for screen padding offset.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			borderX := screenPadding + sbWidth - 1
			if msg.X >= borderX-1 && msg.X <= borderX+1 {
				a.dragging = true
				a.sidebar.dragging = true
				return a, nil
			}
		}

		if msg.X < screenPadding+sbWidth {
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
			// Right panel: check if click is in the tab bar (first 3 rows).
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y < 3 {
				localX := msg.X - screenPadding - sbWidth
				if tabIdx, isClose := a.tabHitTest(localX); tabIdx >= 0 {
					name := a.openTabs[tabIdx]
					if isClose {
						// Close the tab.
						return a, a.closeTabCmd(name)
					}
					if projIdx := a.projectIndexByName(name); projIdx >= 0 {
						// Reuse the ProjectSwitchedMsg path to switch project,
						// update sidebar selection, and focus terminal.
						a.sidebar.selected = projIdx
						return a, func() tea.Msg {
							return ProjectSwitchedMsg{
								Index:   projIdx,
								Project: a.cfg.Projects[projIdx],
							}
						}
					}
				}
			}

			// Mouse wheel in terminal area → navigate scrollback buffer.
			if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
				if msg.Button == tea.MouseButtonWheelUp {
					a.terminal.ScrollBy(scrollLinesPerTick)
				} else {
					a.terminal.ScrollBy(-scrollLinesPerTick)
				}
				return a, nil
			}

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

		// Clear sticky timer — the user is acknowledging this project.
		delete(a.stateStickUntil, project.Name)

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
		// VT is already written by the session's ReadLoop (no DeferVTWrite).
		// Mark this project as dirty so the next scroll-check tick will
		// compare screen snapshots and capture any scrolled-off lines.
		a.scrollDirty[msg.ProjectName] = true
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

	case TabClosedMsg:
		name := msg.Name
		a.removeTab(name)
		delete(a.stateStickUntil, name)

		// Terminate the session behind this tab (sends SIGINT, closes PTY).
		a.mgr.StopSession(name)
		a.sidebar.states[name] = StateIdle
		a.statusbar.states[name] = StateIdle

		// If the closed tab was the active project, switch to the nearest
		// tab or deactivate the terminal if no tabs remain.
		activeName := ""
		if a.active >= 0 && a.active < len(a.cfg.Projects) {
			activeName = a.cfg.Projects[a.active].Name
		}
		if name == activeName {
			if len(a.openTabs) > 0 {
				// Switch to the last tab in the list (most recently visited).
				switchTo := a.openTabs[len(a.openTabs)-1]
				if projIdx := a.projectIndexByName(switchTo); projIdx >= 0 {
					a.sidebar.selected = projIdx
					return a, func() tea.Msg {
						return ProjectSwitchedMsg{
							Index:   projIdx,
							Project: a.cfg.Projects[projIdx],
						}
					}
				}
			} else {
				// No tabs left — clear the terminal so it shows the
				// empty placeholder instead of stale content.
				a.clearTerminal()
			}
		}
		return a, nil

	case AnimTickMsg:
		a.animFrame = (a.animFrame + 1) % animFrameCount
		a.sidebar.animFrame = a.animFrame
		// Expire Ctrl+C hint after the window passes.
		if a.ctrlCHint && time.Since(a.lastCtrlC) >= ctrlCWindow {
			a.ctrlCHint = false
			a.statusbar.ctrlCHint = false
		}
		cmds = append(cmds, animTickCmd())
		return a, tea.Batch(cmds...)

	case scrollCheckTickMsg:
		a.runScrollCheck()
		cmds = append(cmds, scrollCheckTickCmd())
		return a, tea.Batch(cmds...)

	case TickMsg:
		// Run attention detection on the active session.
		a.checkAttention()
		cmds = append(cmds, tickCmd())
		return a, tea.Batch(cmds...)
	}

	return a, tea.Batch(cmds...)
}

// tooSmall reports whether the host terminal is below the minimum usable
// dimensions. When true, View() renders a placeholder instead of the UI.
func (a App) tooSmall() bool {
	return a.width < minAppWidth || a.height < minAppHeight
}

// innerWidth returns the usable content width after screen padding.
func (a App) innerWidth() int {
	return a.width - 2*screenPadding
}

// View renders the complete TUI.
func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	if a.tooSmall() {
		msg := fmt.Sprintf("Terminal too small (%d×%d)", a.width, a.height)
		hint := fmt.Sprintf("Need at least %d×%d", minAppWidth, minAppHeight)
		content := emptyHintStyle.Render(msg) + "\n" + emptyHintStyle.Render(hint)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	sidebarView := a.sidebar.View()
	tabBar := a.tabBarView()

	style := terminalStyle
	if a.focus == focusTerminal {
		style = terminalFocusedStyle
	}
	terminalView := style.Render(a.terminal.View())

	// Right panel: tab bar (3-line bordered tabs) + terminal.
	rightPanel := lipgloss.JoinVertical(lipgloss.Left, tabBar, terminalView)

	a.statusbar.sidebarFocused = (a.focus == focusSidebar)
	a.statusbar.scrollOffset = a.terminal.scrollOffset
	if a.sidebar.selected >= 0 && a.sidebar.selected < len(a.cfg.Projects) {
		a.statusbar.selectedName = a.cfg.Projects[a.sidebar.selected].Name
	}
	statusbarView := a.statusbar.View()

	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, rightPanel)
	content := lipgloss.JoinVertical(lipgloss.Left, main, statusbarView)

	// Wrap with screen-level horizontal padding.
	return lipgloss.NewStyle().
		PaddingLeft(screenPadding).
		PaddingRight(screenPadding).
		Render(content)
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

// inactiveTabStyle returns the appropriate inactive tab style based on the
// project's current session state. Attention/error/done states get colored
// borders and text; idle/working uses the default subtle style.
func inactiveTabStyle(state SessionState) lipgloss.Style {
	switch state {
	case StateNeedsAttention:
		return tabAttentionStyle
	case StateError:
		return tabErrorStyle
	case StateDone:
		return tabDoneStyle
	default:
		return tabStyle
	}
}

// tabBarView renders the tab bar using the lipgloss border technique.
// Each tab is a bordered box (3 lines: top border, content, bottom border).
// Active tab: open bottom (space) merges into terminal, gold border.
// Inactive tab: closed bottom (─) with subtle/state-colored border.
// A gap element fills remaining width with ─ to complete the line.
// See .opencode/skills/lipgloss-guide/SKILL.md for the technique reference.
func (a App) tabBarView() string {
	sbWidth := a.sidebar.Width()
	panelWidth := a.innerWidth() - sbWidth

	activeName := ""
	if a.active >= 0 && a.active < len(a.cfg.Projects) {
		activeName = a.cfg.Projects[a.active].Name
	}

	// Render each tab as a bordered box. All tabs show a close button ✕.
	var renderedTabs []string
	for _, name := range a.openTabs {
		state := a.sidebar.states[name]
		char := badgeChar(state, a.animFrame)
		label := char + " " + name + " ✕"

		if name == activeName {
			renderedTabs = append(renderedTabs, tabActiveStyle.Render(label))
		} else {
			renderedTabs = append(renderedTabs, inactiveTabStyle(state).Render(label))
		}
	}

	// Join tabs side by side (aligned at top).
	var row string
	if len(renderedTabs) > 0 {
		row = lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
	}

	// Fill remaining width with ─ border (the gap).
	rowWidth := lipgloss.Width(row)
	gapWidth := panelWidth - rowWidth
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := tabGapStyle.Render(strings.Repeat(" ", gapWidth))

	// Join row + gap with bottom alignment so ─ lines connect.
	if row != "" {
		return lipgloss.JoinHorizontal(lipgloss.Bottom, row, gap)
	}
	// No tabs: pad gap to 3 lines for consistent layout.
	blank := strings.Repeat(" ", panelWidth)
	return blank + "\n" + gap
}

func (a *App) layout() {
	termW, termH := a.termDimensions()
	panelHeight := a.height - 1 // -1 for status bar

	a.sidebar.height = panelHeight
	a.terminal.SetSize(termW, termH)
	a.statusbar.width = a.innerWidth()
}

// termDimensions returns the terminal panel width and height, clamped to
// at least 1 in each dimension to prevent negative/zero sizes when the
// host terminal is very small.
func (a *App) termDimensions() (int, int) {
	inner := a.innerWidth()
	sbWidth := a.sidebar.Width()
	termWidth := inner - sbWidth - 1 // -1 for terminal PaddingLeft(1)
	termHeight := a.height - 4       // -1 status bar, -3 tab bar

	if termWidth < 1 {
		termWidth = 1
	}
	if termHeight < 1 {
		termHeight = 1
	}
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

	// Point the terminal at this project's scrollback buffer.
	name := s.Project.Name
	sb := a.scrollbacks[name]
	if sb == nil {
		sb = newScrollbackBuffer(defaultScrollbackCapacity)
		a.scrollbacks[name] = sb
	}
	a.terminal.scrollback = sb
}

// runScrollCheck iterates over dirty projects and compares the current VT
// screen against the last-known snapshot to detect lines that scrolled off.
// This runs on a 100ms tick — by then the VT has a stable screen regardless
// of how PTY data was chunked across reads.
func (a *App) runScrollCheck() {
	for name, dirty := range a.scrollDirty {
		if !dirty {
			continue
		}
		a.scrollDirty[name] = false

		s := a.mgr.GetSession(name)
		if s == nil {
			continue
		}

		pushed := a.checkScrollback(s, name)

		// If the user is scrolled up AND pinned (scrolled up into history,
		// not scrolling back down), bump the offset so the view stays on
		// the same content. When not pinned (user is scrolling down toward
		// live), skip adjustment so they can reach offset 0.
		if name == a.mgr.ActiveName() && a.terminal.scrollOffset > 0 && pushed > 0 && a.terminal.scrollPinned {
			a.terminal.scrollOffset += pushed
		}
	}
}

// checkScrollback compares the current VT screen against the stored snapshot
// for this project, detects scroll shifts, and pushes scrolled-off rows (as
// glyph data) to the project's scrollback buffer. Returns the number of
// lines pushed.
func (a *App) checkScrollback(s *session.Session, projectName string) int {
	sb := a.scrollbacks[projectName]
	if sb == nil {
		sb = newScrollbackBuffer(defaultScrollbackCapacity)
		a.scrollbacks[projectName] = sb
	}

	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if s.VT == nil {
		return 0
	}

	w, h := s.Width, s.Height

	// Build current screen snapshot (text + glyphs).
	curTexts := make([]string, h)
	curGlyphs := make([]scrollbackLine, h)
	for row := 0; row < h; row++ {
		glyphs := make(scrollbackLine, w)
		for col := 0; col < w; col++ {
			glyphs[col] = s.VT.Cell(col, row)
		}
		curGlyphs[row] = glyphs
		curTexts[row] = glyphsToText(glyphs)
	}

	oldTexts := a.scrollSnapshots[projectName]
	oldGlyphs := a.scrollGlyphSnapshots[projectName]

	// First snapshot — just store it, nothing to compare.
	if oldTexts == nil {
		a.scrollSnapshots[projectName] = curTexts
		a.scrollGlyphSnapshots[projectName] = curGlyphs
		return 0
	}

	// Detect scroll shift between last snapshot and current screen.
	shift := detectScrollShift(oldTexts, curTexts)
	pushed := 0
	if shift > 0 && oldGlyphs != nil {
		// Find the first row that changed — skip fixed headers in TUI apps.
		firstDiff := 0
		for firstDiff < len(oldTexts) && firstDiff < len(curTexts) && oldTexts[firstDiff] == curTexts[firstDiff] {
			firstDiff++
		}

		end := firstDiff + shift
		if end > len(oldGlyphs) {
			end = len(oldGlyphs)
		}

		// Push scrolled-off rows using the PREVIOUS snapshot's glyph data.
		// These rows have been destroyed in the current VT by scrollUp(),
		// but we preserved them in the last glyph snapshot.
		for i := firstDiff; i < end; i++ {
			sb.Push(oldGlyphs[i])
			pushed++
		}
	}

	// Store current snapshot for next comparison.
	a.scrollSnapshots[projectName] = curTexts
	a.scrollGlyphSnapshots[projectName] = curGlyphs
	return pushed
}

// clearTerminal deactivates the terminal widget so it shows the empty
// placeholder. Called when all tabs are closed and there is nothing to display.
func (a *App) clearTerminal() {
	a.terminal.mu.Lock()
	a.terminal.vt = nil
	a.terminal.active = false
	a.terminal.mu.Unlock()
}

// clampAndSetSidebarWidth sets the sidebar content width from a mouse X
// position, clamping between minSidebarWidth and half the terminal width.
// The border is at column contentWidth (0-indexed), so mouse X maps directly.
func (a *App) clampAndSetSidebarWidth(x int) {
	contentWidth := x - screenPadding // adjust for left screen padding
	if contentWidth < minSidebarWidth {
		contentWidth = minSidebarWidth
	}
	maxWidth := a.innerWidth() / 2
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

// tabHitTest returns the index into openTabs for a mouse X position relative
// to the tab bar start (i.e. after screenPadding + sidebar). Returns
// (tabIndex, isClose). tabIndex is -1 if the click falls outside any tab.
// isClose is true if the click landed on the close button region (✕) of any tab.
func (a App) tabHitTest(localX int) (int, bool) {
	activeName := ""
	if a.active >= 0 && a.active < len(a.cfg.Projects) {
		activeName = a.cfg.Projects[a.active].Name
	}

	offset := 0
	for i, name := range a.openTabs {
		state := a.sidebar.states[name]
		char := badgeChar(state, a.animFrame)
		label := char + " " + name + " ✕"

		var w int
		if name == activeName {
			w = lipgloss.Width(tabActiveStyle.Render(label))
		} else {
			w = lipgloss.Width(inactiveTabStyle(state).Render(label))
		}

		if localX >= offset && localX < offset+w {
			// Check if the click is in the close region
			// (last 4 columns: space + ✕ + padding).
			closeRegionStart := offset + w - 4
			if localX >= closeRegionStart {
				return i, true
			}
			return i, false
		}
		offset += w
	}
	return -1, false
}

// switchTab returns a command to switch to the tab at offset delta from the
// current active tab in the openTabs list. delta=+1 for next, -1 for prev.
// Wraps around. Returns nil if there are fewer than 2 open tabs.
func (a App) switchTab(delta int) tea.Cmd {
	if len(a.openTabs) < 2 {
		return nil
	}

	// Find current tab index in openTabs.
	activeName := ""
	if a.active >= 0 && a.active < len(a.cfg.Projects) {
		activeName = a.cfg.Projects[a.active].Name
	}
	cur := -1
	for i, name := range a.openTabs {
		if name == activeName {
			cur = i
			break
		}
	}
	if cur < 0 {
		return nil
	}

	next := (cur + delta + len(a.openTabs)) % len(a.openTabs)
	targetName := a.openTabs[next]
	projIdx := a.projectIndexByName(targetName)
	if projIdx < 0 {
		return nil
	}
	return func() tea.Msg {
		return ProjectSwitchedMsg{
			Index:   projIdx,
			Project: a.cfg.Projects[projIdx],
		}
	}
}

// projectIndexByName returns the index of the project with the given name, or -1.
func (a App) projectIndexByName(name string) int {
	for i, p := range a.cfg.Projects {
		if p.Name == name {
			return i
		}
	}
	return -1
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

// closeTabCmd returns a tea.Cmd that produces a TabClosedMsg. The actual tab
// removal and potential project switch happen in the Update handler.
func (a App) closeTabCmd(name string) tea.Cmd {
	return func() tea.Msg {
		return TabClosedMsg{Name: name}
	}
}

// isAttentionState returns true for states that should be sticky (not
// immediately downgraded to Working).
func isAttentionState(s SessionState) bool {
	return s == StateNeedsAttention || s == StateError || s == StateDone
}

// checkAttention runs attention detection on ALL sessions (not just the
// active one) using the L1 heuristic detector with optional L2 LLM
// escalation. Updates sidebar/statusbar state accordingly.
//
// Sticky logic: when a project enters an attention state, it is held for at
// least stateStickDuration before being downgraded back to Working. This
// prevents visual flicker when transient signals scroll off screen between
// ticks.
func (a *App) checkAttention() {
	ctx := context.Background()
	now := time.Now()

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

		// Get the process PID for liveness checking.
		pid := 0
		if s.Cmd != nil {
			pid = s.Cmd.Pid
		}

		prevState := a.sidebar.states[name]

		event, isWorking := a.detector.Check(ctx, name, lines, pid, string(p.Agent))
		if event != nil {
			state := attentionEventToState(event)
			if state != prevState {
				// State transition — apply sticky timer for attention states.
				if isAttentionState(state) {
					a.stateStickUntil[name] = now.Add(stateStickDuration)

					// Send desktop notification on entering attention/error.
					if a.notifier != nil && (state == StateNeedsAttention || state == StateError) {
						a.notifier.Notify(name, event.Type.String(), event.Detail)
					}
				}
			}
			a.sidebar.states[name] = state
			a.statusbar.states[name] = state
		} else if isWorking {
			// Positive working signal (agent-specific: spinner, progress
			// bar, etc). Only upgrade to Working; respect sticky attention
			// states.
			if s.State == session.StateRunning {
				if isAttentionState(prevState) {
					if deadline, ok := a.stateStickUntil[name]; ok && now.Before(deadline) {
						continue
					}
				}
				a.sidebar.states[name] = StateWorking
				a.statusbar.states[name] = StateWorking
				delete(a.stateStickUntil, name)
			}
		} else {
			// No signal — keep current state. Only expire sticky attention
			// states back to Idle (not Working) once the timer lapses.
			if s.State == session.StateRunning && isAttentionState(prevState) {
				if deadline, ok := a.stateStickUntil[name]; ok && now.Before(deadline) {
					continue
				}
				// Sticky expired — return to Idle, not Working.
				a.sidebar.states[name] = StateIdle
				a.statusbar.states[name] = StateIdle
				delete(a.stateStickUntil, name)
			}
		}
	}
}

// attentionEventToState maps an attention.AttentionEvent to a TUI SessionState.
func attentionEventToState(event *attention.AttentionEvent) SessionState {
	switch event.Type {
	case attention.NeedsInput, attention.NeedsPermission:
		return StateNeedsAttention
	case attention.HitError, attention.Stuck:
		return StateError
	case attention.NeedsReview:
		return StateDone
	default:
		return StateWorking
	}
}
