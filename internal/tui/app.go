// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
	"github.com/openconductorhq/openconductor/internal/session"
	"github.com/openconductorhq/openconductor/internal/telegram"
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
// the originating session ID.
type sessionOutputMsg struct {
	SessionID string
	Data      []byte
}

// sessionStartedMsg signals that a session was started and its read loop
// channel is ready for listening.
type sessionStartedMsg struct {
	SessionID string
}

// sessionExitedMsg signals that a session's PTY read loop has ended.
type sessionExitedMsg struct {
	SessionID string
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
	cfg           *config.Config
	configPath    string
	sidebar       sidebarModel
	terminal      terminalModel
	statusbar     statusBarModel
	focus         focus
	width         int
	height        int
	ready         bool
	active        int // index of active project
	mgr           *session.Manager
	detector      *attention.Detector
	autoApprover  *attention.AutoApprover
	notifier      Notifier
	sidebarWidth  int                     // content width of sidebar (excludes padding/border)
	dragging      bool                    // true during separator drag
	openTabs      []string                // session IDs of opened tabs, in visit order
	sessionStates map[string]SessionState // per-session state, keyed by session ID

	// telegramCh, when non-nil, receives events for the Telegram bridge.
	// Set via SetTelegramChannel before starting the program.
	telegramCh chan<- telegram.Event

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

	sidebar := newSidebarModel(cfg.Projects, defaultSidebarWidth)
	// Sync sidebar's openTabs map with the initial open tabs.
	for _, name := range openTabs {
		sidebar.openTabs[name] = true
	}

	return App{
		cfg:                  cfg,
		configPath:           configPath,
		sidebar:              sidebar,
		terminal:             newTerminalModel(),
		statusbar:            newStatusBarModel(cfg.Projects),
		focus:                initialFocus,
		active:               0,
		mgr:                  session.NewManager(),
		detector:             attention.NewDetector(),
		sidebarWidth:         defaultSidebarWidth,
		openTabs:             openTabs,
		sessionStates:        make(map[string]SessionState),
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

// SetAutoApprover configures automatic permission approval. When set,
// permission events are classified and, if within the project's ApprovalLevel,
// auto-approved by sending the appropriate keystroke to the PTY.
func (a *App) SetAutoApprover(aa *attention.AutoApprover) {
	a.autoApprover = aa
}

// SetNotifier configures desktop notifications for attention events.
func (a *App) SetNotifier(n Notifier) {
	a.notifier = n
}

// SetTelegramChannel configures the outbound channel for the Telegram bot
// bridge. Events are sent non-blocking; the bridge handles dedup and rate
// limiting on the receiving side.
func (a *App) SetTelegramChannel(ch chan<- telegram.Event) {
	a.telegramCh = ch
}

// SessionManager returns the underlying session manager. Used by the Telegram
// bot to read/write agent PTYs for inbound message routing.
func (a *App) SessionManager() *session.Manager {
	return a.mgr
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

	// Handle kitty keyboard protocol CSI u sequences. bubbletea v1.3.10 does
	// not parse these and emits them as unknownCSISequenceMsg. We intercept
	// sequences that map to app shortcuts (Ctrl+C/S/J/K) and convert them to
	// tea.KeyMsg so the normal key handling below processes them. Everything
	// else (e.g. Shift+Enter) is forwarded to the active PTY.
	if raw := unknownCSIBytes(msg); len(raw) > 0 {
		if keyMsg, ok := parseKittyCSI(raw); ok {
			// Recognised app shortcut — replace msg and fall through
			// to the switch below so key handlers see it.
			msg = keyMsg
		} else if a.focus == focusTerminal {
			if s := a.mgr.ActiveSession(); s != nil {
				s.Write(raw)
			}
			return a, nil
		} else {
			return a, nil
		}
	}

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
		for _, s := range a.mgr.AllSessions() {
			s.Resize(termW, termH)
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

		// Ctrl+J / Ctrl+K: switch to prev/next tab.
		// Works regardless of which panel is focused.
		if isKey(msg, tea.KeyCtrlJ) {
			if cmd := a.switchTab(-1); cmd != nil {
				return a, cmd
			}
			return a, nil
		}
		if isKey(msg, tea.KeyCtrlK) {
			if cmd := a.switchTab(1); cmd != nil {
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
					sessionID := a.openTabs[tabIdx]
					if isClose {
						// Close the tab.
						return a, a.closeTabCmd(sessionID)
					}
					// Switch to the clicked session tab.
					return a, func() tea.Msg {
						return TabSwitchedMsg{SessionID: sessionID}
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

			// Forward non-wheel mouse events to the child PTY when the
			// child process has requested mouse tracking. This enables
			// text selection, cursor positioning, and click interactions
			// inside the embedded agent (e.g., OpenCode's input field).
			if s := a.mgr.ActiveSession(); s != nil {
				s.Mu.RLock()
				var vtMode vt10x.ModeFlag
				if s.VT != nil {
					vtMode = s.VT.Mode()
				}
				s.Mu.RUnlock()

				if vtMode&vt10x.ModeMouseMask != 0 {
					localX := msg.X - screenPadding - sbWidth - 1 // -1 for terminal PaddingLeft
					localY := msg.Y - 3                           // -3 for tab bar height
					termW, termH := a.termDimensions()
					if localX >= 0 && localX < termW && localY >= 0 && localY < termH {
						sgrMode := vtMode&vt10x.ModeMouseSgr != 0
						motionMode := vtMode&(vt10x.ModeMouseMotion|vt10x.ModeMouseMany) != 0
						if seq := mouseToBytes(msg, localX, localY, sgrMode, motionMode); seq != nil {
							s.Write(seq)
						}
					}
				}
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

		// Focus terminal when creating a new session.
		a.focus = focusTerminal
		a.sidebar.focused = false
		a.terminal.focused = true

		// Always create a new session — each Enter in sidebar is a new
		// agent invocation (new opencode process).
		cmd := a.startSessionCmd(project)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)

	case TabSwitchedMsg:
		// Switch to an existing session (tab click or Ctrl+J/K).
		s := a.mgr.GetSession(msg.SessionID)
		if s == nil {
			return a, nil
		}
		a.mgr.SetActive(msg.SessionID)
		a.statusbar.activeName = msg.SessionID
		if projIdx := a.projectIndexByName(s.Project.Name); projIdx >= 0 {
			a.active = projIdx
			a.sidebar.selected = projIdx
		}
		a.syncTerminalFromSession()

		// Focus terminal when switching tabs.
		a.focus = focusTerminal
		a.sidebar.focused = false
		a.terminal.focused = true
		return a, nil

	case ProjectAddedMsg:
		project := msg.Project
		a.cfg.Projects = append(a.cfg.Projects, project)
		a.sidebar.projects = a.cfg.Projects
		a.sidebar.states[project.Name] = StateIdle
		a.sidebar.selected = len(a.cfg.Projects) - 1
		a.sidebar.mode = sidebarNormal

		a.statusbar = newStatusBarModel(a.cfg.Projects)
		// Carry over existing session states into statusbar.
		for id, state := range a.sessionStates {
			a.statusbar.states[id] = state
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

		// Close all sessions for this project and remove their tabs.
		for _, s := range a.mgr.GetSessionsByProject(name) {
			a.removeTab(s.ID)
			a.mgr.StopSession(s.ID)
			delete(sessionChannels, s.ID)
			delete(a.sessionStates, s.ID)
			delete(a.stateStickUntil, s.ID)
			delete(a.statusbar.states, s.ID)
		}

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
		delete(a.sidebar.openTabs, name)
		a.sidebar.mode = sidebarNormal

		// Clamp selection.
		if a.sidebar.selected >= len(a.cfg.Projects) {
			a.sidebar.selected = max(0, len(a.cfg.Projects)-1)
		}

		// Rebuild statusbar.
		a.statusbar = newStatusBarModel(a.cfg.Projects)
		for id, state := range a.sessionStates {
			a.statusbar.states[id] = state
		}

		// Save config.
		cmds = append(cmds, a.saveConfigCmd())

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
		a.mgr.SetActive(msg.SessionID)
		a.statusbar.activeName = msg.SessionID
		a.addTab(msg.SessionID)
		a.syncTerminalFromSession()
		// Update sidebar to show this project has an open tab.
		if s := a.mgr.GetSession(msg.SessionID); s != nil {
			a.sidebar.openTabs[s.Project.Name] = true
			// Load conversation history in the background so the user can
			// scroll up to see prior context immediately.
			cmds = append(cmds, a.loadHistoryCmd(msg.SessionID, s.Project))
		}
		// Begin listening for output from this session.
		cmds = append(cmds, a.waitForSessionOutput(msg.SessionID))
		return a, tea.Batch(cmds...)

	case historyLoadedMsg:
		if len(msg.Lines) > 0 {
			sb := a.scrollbacks[msg.SessionID]
			if sb == nil {
				sb = newScrollbackBuffer(defaultScrollbackCapacity)
				a.scrollbacks[msg.SessionID] = sb
			}
			for _, line := range msg.Lines {
				glyphs := textToGlyphs(line)
				sb.Push(glyphs)
			}
			logging.Info("scrollback: pre-populated history",
				"session", msg.SessionID,
				"lines", len(msg.Lines),
			)
		}
		return a, nil

	case sessionOutputMsg:
		// VT is already written by the session's ReadLoop (no DeferVTWrite).
		// Mark this session as dirty so the next scroll-check tick will
		// compare screen snapshots and capture any scrolled-off lines.
		a.scrollDirty[msg.SessionID] = true
		if msg.SessionID == a.mgr.ActiveName() {
			a.syncTerminalFromSession()
		}
		// Continue listening.
		cmds = append(cmds, a.waitForSessionOutput(msg.SessionID))
		return a, tea.Batch(cmds...)

	case sessionExitedMsg:
		if msg.SessionID == a.mgr.ActiveName() {
			a.terminal.active = false
		}
		// Check if this is a system tab (look up by session ID).
		s := a.mgr.GetSession(msg.SessionID)
		isSystemTab := s != nil && a.projectIndexByName(s.Project.Name) < 0
		if isSystemTab {
			// Emit SystemTabExitedMsg so the app can reload config.
			return a, func() tea.Msg {
				return SystemTabExitedMsg{Name: msg.SessionID}
			}
		}
		a.sessionStates[msg.SessionID] = StateDone
		a.statusbar.states[msg.SessionID] = StateDone
		if s != nil {
			a.sidebar.states[s.Project.Name] = a.aggregateProjectState(s.Project.Name)
		}
		// Notify Telegram that the session completed.
		if s != nil {
			a.sendTelegramEvent(s.Project.Name, msg.SessionID, StateDone, "", s.GetScreenLines())
		}
		return a, nil

	case SessionStateChangedMsg:
		a.sessionStates[msg.SessionID] = msg.State
		a.statusbar.states[msg.SessionID] = msg.State
		if s := a.mgr.GetSession(msg.SessionID); s != nil {
			a.sidebar.states[s.Project.Name] = a.aggregateProjectState(s.Project.Name)
		}
		return a, nil

	case TabClosedMsg:
		sessionID := msg.Name
		a.removeTab(sessionID)
		delete(a.stateStickUntil, sessionID)
		delete(a.sessionStates, sessionID)
		delete(a.statusbar.states, sessionID)

		// Get project name before stopping the session.
		var projectName string
		if s := a.mgr.GetSession(sessionID); s != nil {
			projectName = s.Project.Name
		}

		// Terminate the session behind this tab (sends SIGINT, closes PTY).
		a.mgr.StopSession(sessionID)
		delete(sessionChannels, sessionID)

		// Update sidebar state for the project.
		if projectName != "" {
			if a.mgr.HasSessionsForProject(projectName) {
				a.sidebar.states[projectName] = a.aggregateProjectState(projectName)
			} else {
				a.sidebar.states[projectName] = StateIdle
				delete(a.sidebar.openTabs, projectName)
			}
		}

		// If the closed tab was the active session, switch to the nearest
		// tab or deactivate the terminal if no tabs remain.
		wasActive := sessionID == a.mgr.ActiveName() || a.mgr.ActiveName() == ""
		if wasActive {
			if len(a.openTabs) > 0 {
				// Switch to the last tab in the list (most recently visited).
				switchTo := a.openTabs[len(a.openTabs)-1]
				return a, func() tea.Msg {
					return TabSwitchedMsg{SessionID: switchTo}
				}
			} else {
				// No tabs left — clear the terminal.
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

	case SystemTabRequestMsg:
		// Spawn the current binary with the given args in a system tab.
		exe, err := os.Executable()
		if err != nil {
			return a, nil
		}
		cmd := exec.Command(exe, msg.Args...)
		termW, termH := a.termDimensions()
		_, err = a.mgr.StartSystemSession(msg.Name, cmd, termW, termH)
		if err != nil {
			return a, nil
		}
		a.mgr.SetActive(msg.Name)
		a.addTab(msg.Name)
		a.syncTerminalFromSession()
		a.focus = focusTerminal
		a.sidebar.focused = false
		a.terminal.focused = true
		cmds = append(cmds, a.waitForSessionOutput(msg.Name))
		return a, tea.Batch(cmds...)

	case SystemTabExitedMsg:
		// System tab process finished — reload config for post-setup changes.
		configPath := config.DefaultConfigPath()
		freshCfg := config.LoadOrDefault(configPath)
		a.cfg.Telegram = freshCfg.Telegram
		return a, nil

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

// inactiveTabStyle returns the style for an inactive tab. All inactive tabs
// use the same subtle charcoal border for visual consistency — the badge
// character (◆ ● ! ? ✓) already communicates session state.
func inactiveTabStyle(_ SessionState) lipgloss.Style {
	return tabStyle
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

	activeSessionID := a.mgr.ActiveName()

	// Render each tab as a bordered box. All tabs show a close button ✕.
	var renderedTabs []string
	for _, sessionID := range a.openTabs {
		state := a.sessionStates[sessionID]
		char := badgeChar(state, a.animFrame)
		label := char + " " + sessionID + " ✕"

		if sessionID == activeSessionID {
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

// startSessionCmd creates a tea.Cmd that starts a new session for the given
// project in the background and emits a sessionStartedMsg on success.
// Each call creates a fresh agent process.
func (a *App) startSessionCmd(project config.Project) tea.Cmd {
	mgr := a.mgr
	termW, termH := a.termDimensions()

	return func() tea.Msg {
		s, err := mgr.StartSession(project, termW, termH)
		if err != nil {
			return SessionStateChangedMsg{
				SessionID: project.Name,
				State:     StateError,
				Detail:    err.Error(),
			}
		}
		return sessionStartedMsg{SessionID: s.ID}
	}
}

// loadHistoryCmd returns a tea.Cmd that loads conversation history from the
// agent's data store (e.g. OpenCode's SQLite DB) in the background.
func (a *App) loadHistoryCmd(sessionID string, project config.Project) tea.Cmd {
	agentType := project.Agent
	repoPath := project.Repo
	return func() tea.Msg {
		lines, err := agent.LoadHistory(agentType, repoPath)
		if err != nil {
			logging.Debug("scrollback: history load failed",
				"session", sessionID,
				"err", err,
			)
			return historyLoadedMsg{SessionID: sessionID}
		}
		return historyLoadedMsg{SessionID: sessionID, Lines: lines}
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
func (a *App) waitForSessionOutput(sessionID string) tea.Cmd {
	s := a.mgr.GetSession(sessionID)
	if s == nil {
		return nil
	}

	return a.readPTYOnce(sessionID, s)
}

// sessionChannels caches per-session read-loop channels so we only call
// ReadLoop once per session.
var sessionChannels = make(map[string]<-chan []byte)

// readPTYOnce returns a tea.Cmd that reads from the session's PTY channel.
// On first call for a given session it initialises the channel via ReadLoop.
func (a *App) readPTYOnce(sessionID string, s *session.Session) tea.Cmd {
	ch, ok := sessionChannels[sessionID]
	if !ok {
		ch = s.ReadLoop()
		sessionChannels[sessionID] = ch
	}

	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			delete(sessionChannels, sessionID)
			return sessionExitedMsg{SessionID: sessionID}
		}
		return sessionOutputMsg{
			SessionID: sessionID,
			Data:      data,
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

	// Point the terminal at this session's scrollback buffer.
	id := s.ID
	sb := a.scrollbacks[id]
	if sb == nil {
		sb = newScrollbackBuffer(defaultScrollbackCapacity)
		a.scrollbacks[id] = sb
	}
	a.terminal.scrollback = sb
}

// runScrollCheck iterates over dirty projects and compares the current VT
// screen against the last-known snapshot to detect lines that scrolled off.
// This runs on a 100ms tick — by then the VT has a stable screen regardless
// of how PTY data was chunked across reads.
func (a *App) runScrollCheck() {
	for sessionID, dirty := range a.scrollDirty {
		if !dirty {
			continue
		}
		a.scrollDirty[sessionID] = false

		s := a.mgr.GetSession(sessionID)
		if s == nil {
			continue
		}

		pushed := a.checkScrollback(s, sessionID)

		// If the user is scrolled up AND pinned (scrolled up into history,
		// not scrolling back down), bump the offset so the view stays on
		// the same content. When not pinned (user is scrolling down toward
		// live), skip adjustment so they can reach offset 0.
		if sessionID == a.mgr.ActiveName() && a.terminal.scrollOffset > 0 && pushed > 0 && a.terminal.scrollPinned {
			a.terminal.scrollOffset += pushed
		}
	}
}

// checkScrollback compares the current VT screen against the stored snapshot
// for this project, detects scroll shifts, and pushes scrolled-off rows (as
// glyph data) to the project's scrollback buffer. Returns the number of
// lines pushed.
//
// For sessions running in alternate screen mode (TUI apps like OpenCode),
// traditional scroll-shift detection doesn't work because the app redraws
// the entire screen on every update. In that case, we fall back to pushing
// all old non-blank rows that disappeared from the new screen, giving the
// user access to previous screen content when scrolling back.
func (a *App) checkScrollback(s *session.Session, sessionID string) int {
	sb := a.scrollbacks[sessionID]
	if sb == nil {
		sb = newScrollbackBuffer(defaultScrollbackCapacity)
		a.scrollbacks[sessionID] = sb
	}

	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if s.VT == nil {
		return 0
	}

	w, h := s.Width, s.Height
	altScreen := s.VT.Mode()&vt10x.ModeAltScreen != 0

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

	oldTexts := a.scrollSnapshots[sessionID]
	oldGlyphs := a.scrollGlyphSnapshots[sessionID]

	// First snapshot — just store it, nothing to compare.
	if oldTexts == nil {
		a.scrollSnapshots[sessionID] = curTexts
		a.scrollGlyphSnapshots[sessionID] = curGlyphs
		return 0
	}

	// Detect scroll shift between last snapshot and current screen.
	shift := detectScrollShift(oldTexts, curTexts)
	pushed := 0

	if shift > 0 && oldGlyphs != nil {
		// Traditional scroll detected — push scrolled-off rows.
		// Find the first row that changed — skip fixed headers in TUI apps.
		firstDiff := 0
		for firstDiff < len(oldTexts) && firstDiff < len(curTexts) && oldTexts[firstDiff] == curTexts[firstDiff] {
			firstDiff++
		}

		end := firstDiff + shift
		if end > len(oldGlyphs) {
			end = len(oldGlyphs)
		}

		for i := firstDiff; i < end; i++ {
			sb.Push(oldGlyphs[i])
			pushed++
		}
	} else if shift == 0 && altScreen && oldGlyphs != nil {
		// Alt-screen TUI app: no traditional scroll, but the screen may have
		// been fully repainted. Push old non-blank rows that disappeared from
		// the new screen, so the user can scroll back to see previous content.
		//
		// Skip TUI chrome rows configured by the agent adapter (e.g. OpenCode
		// skips row 0 header and last 2 rows for status bar + footer). These
		// change frequently and would pollute the scrollback buffer with noise.
		chromeTop, chromeBottom := agent.ChromeSkipRows(s.Project.Agent)
		pushed = pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, chromeTop, chromeBottom)
	}

	// Store current snapshot for next comparison.
	a.scrollSnapshots[sessionID] = curTexts
	a.scrollGlyphSnapshots[sessionID] = curGlyphs
	return pushed
}

// pushAltScreenDiff pushes old screen rows that are non-blank and no longer
// present in the new screen. This captures content that disappeared during a
// TUI app full-screen repaint. Returns the number of lines pushed.
//
// chromeSkipFirst and chromeSkipLast specify the number of rows to exclude from
// the top and bottom of the screen respectively. TUI apps typically have fixed
// chrome (header, status bar, footer) that changes frequently (timer ticks,
// token counters) and should not be pushed to scrollback.
//
// To avoid flooding scrollback with identical TUI frames, a row is only pushed
// if it:
//   - is non-blank in the old screen
//   - is outside the chrome skip zones
//   - does NOT appear at the same position in the new screen
//   - does NOT appear ANYWHERE in the new screen (dedup against full content)
//
// At least minAltDiffRows rows must qualify — small diffs (1-2 rows) are
// typically just cursor blinks or status updates, not meaningful content loss.
func pushAltScreenDiff(sb *scrollbackBuffer, oldTexts []string, oldGlyphs []scrollbackLine, curTexts []string, chromeSkipFirst, chromeSkipLast int) int {
	const minAltDiffRows = 3

	// Build a set of all current screen text for dedup.
	curSet := make(map[string]struct{}, len(curTexts))
	for _, t := range curTexts {
		if t != "" {
			curSet[t] = struct{}{}
		}
	}

	// Determine the content row range (excluding TUI chrome).
	startRow := chromeSkipFirst
	endRow := len(oldTexts) - chromeSkipLast
	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(oldTexts) {
		endRow = len(oldTexts)
	}
	if startRow >= endRow {
		return 0
	}

	// Collect old rows that disappeared.
	type candidate struct {
		row    int
		glyphs scrollbackLine
	}
	var candidates []candidate
	for i := startRow; i < endRow; i++ {
		oldText := oldTexts[i]
		if oldText == "" {
			continue
		}
		// Skip if the row is unchanged at the same position.
		if i < len(curTexts) && oldText == curTexts[i] {
			continue
		}
		// Skip if the row still appears anywhere on the new screen.
		if _, exists := curSet[oldText]; exists {
			continue
		}
		candidates = append(candidates, candidate{row: i, glyphs: oldGlyphs[i]})
	}

	if len(candidates) < minAltDiffRows {
		return 0
	}

	for _, c := range candidates {
		sb.Push(c.glyphs)
	}
	return len(candidates)
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
	for _, s := range a.mgr.AllSessions() {
		s.Resize(termW, termH)
	}
}

// tabHitTest returns the index into openTabs for a mouse X position relative
// to the tab bar start (i.e. after screenPadding + sidebar). Returns
// (tabIndex, isClose). tabIndex is -1 if the click falls outside any tab.
// isClose is true if the click landed on the close button region (✕) of any tab.
func (a App) tabHitTest(localX int) (int, bool) {
	activeSessionID := a.mgr.ActiveName()

	offset := 0
	for i, sessionID := range a.openTabs {
		state := a.sessionStates[sessionID]
		char := badgeChar(state, a.animFrame)
		label := char + " " + sessionID + " ✕"

		var w int
		if sessionID == activeSessionID {
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
	activeSessionID := a.mgr.ActiveName()
	cur := -1
	for i, id := range a.openTabs {
		if id == activeSessionID {
			cur = i
			break
		}
	}
	if cur < 0 {
		return nil
	}

	next := (cur + delta + len(a.openTabs)) % len(a.openTabs)
	targetID := a.openTabs[next]
	return func() tea.Msg {
		return TabSwitchedMsg{SessionID: targetID}
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

// addTab appends a session ID to the open tabs list. Each session ID is
// unique, so no dedup check is needed.
func (a *App) addTab(sessionID string) {
	for _, t := range a.openTabs {
		if t == sessionID {
			return
		}
	}
	a.openTabs = append(a.openTabs, sessionID)
	// Mark the project as having an open tab.
	if s := a.mgr.GetSession(sessionID); s != nil {
		a.sidebar.openTabs[s.Project.Name] = true
	}
}

// removeTab removes a session ID from the open tabs list.
// If this was the last session for a project, clears the project's openTab flag.
func (a *App) removeTab(sessionID string) {
	for i, t := range a.openTabs {
		if t == sessionID {
			a.openTabs = append(a.openTabs[:i], a.openTabs[i+1:]...)
			break
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
	return s == StateNeedsAttention || s == StateNeedsPermission || s == StateAsking || s == StateError || s == StateDone
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

	for _, s := range a.mgr.AllSessions() {
		lines := s.GetScreenLines()
		if lines == nil {
			continue
		}

		sessionID := s.ID
		projectName := s.Project.Name

		// Get the process PID for liveness checking.
		pid := 0
		if s.Cmd != nil {
			pid = s.Cmd.Pid
		}

		prevState := a.sessionStates[sessionID]

		// Look up the agent adapter and cast to AttentionChecker if supported.
		var checker attention.AttentionChecker
		if adapter, err := agent.Get(s.Project.Agent); err == nil {
			if c, ok := adapter.(attention.AttentionChecker); ok {
				checker = c
			}
		}

		event, isWorking := a.detector.Check(ctx, sessionID, lines, pid, checker)
		logging.Debug("attention check",
			"session", sessionID,
			"project", projectName,
			"agent", string(s.Project.Agent),
			"prevState", prevState.String(),
			"hasEvent", event != nil,
			"isWorking", isWorking,
		)
		if event != nil {
			// Auto-approve permission requests when the project is configured
			// to do so and the classifier identifies the category as allowed.
			if event.Type == attention.NeedsPermission && a.autoApprover != nil {
				adapter, adapterErr := agent.Get(s.Project.Agent)
				if adapterErr == nil {
					keystrokes := attention.ApprovalKeystrokes{
						Approve:        adapter.ApproveKeystroke(),
						ApproveSession: adapter.ApproveSessionKeystroke(),
					}
					result := a.autoApprover.CheckAndApprove(ctx, s.Project, lines, keystrokes)
					if result.ShouldApprove {
						// Send the approval keystroke to the PTY and treat
						// the session as Working — no notification needed.
						s.Write(result.Keystroke)
						a.sessionStates[sessionID] = StateWorking
						a.statusbar.states[sessionID] = StateWorking
						a.sidebar.states[projectName] = a.aggregateProjectState(projectName)
						delete(a.stateStickUntil, sessionID)
						continue
					}
				}
			}

			state := attentionEventToState(event)
			logging.Debug("attention event",
				"session", sessionID,
				"eventType", event.Type.String(),
				"detail", event.Detail,
				"source", event.Source,
				"newState", state.String(),
				"prevState", prevState.String(),
			)
			if state != prevState {
				logging.Debug("attention state transition",
					"session", sessionID,
					"from", prevState.String(),
					"to", state.String(),
				)
				// State transition — apply sticky timer for attention states.
				if isAttentionState(state) {
					a.stateStickUntil[sessionID] = now.Add(stateStickDuration)

					// Send desktop notification on entering attention/permission/asking/error.
					if a.notifier != nil && (state == StateNeedsAttention || state == StateNeedsPermission || state == StateAsking || state == StateError) {
						a.notifier.Notify(projectName, event.Type.String(), event.Detail)
					}

					// Send Telegram event on attention state transitions.
					a.sendTelegramEvent(projectName, sessionID, state, event.Detail, lines)
				}
			}
			a.sessionStates[sessionID] = state
			a.statusbar.states[sessionID] = state
		} else if isWorking {
			// Positive working signal (agent-specific: spinner, progress
			// bar, etc). Only upgrade to Working; respect sticky attention
			// states.
			if s.State == session.StateRunning {
				if isAttentionState(prevState) {
					if deadline, ok := a.stateStickUntil[sessionID]; ok && now.Before(deadline) {
						continue
					}
				}
				a.sessionStates[sessionID] = StateWorking
				a.statusbar.states[sessionID] = StateWorking
				delete(a.stateStickUntil, sessionID)
			}
		} else {
			// No signal — keep current state. Only expire sticky attention
			// states back to Idle (not Working) once the timer lapses.
			if s.State == session.StateRunning && isAttentionState(prevState) {
				if deadline, ok := a.stateStickUntil[sessionID]; ok && now.Before(deadline) {
					continue
				}
				// Sticky expired — return to Idle, not Working.
				a.sessionStates[sessionID] = StateIdle
				a.statusbar.states[sessionID] = StateIdle
				delete(a.stateStickUntil, sessionID)
			} else if s.State == session.StateRunning && prevState == StateWorking {
				// Working → Idle (agent finished responding).
				a.sendTelegramEvent(projectName, sessionID, StateIdle, "", lines)
			}
		}

		// Update sidebar aggregate state for this project.
		a.sidebar.states[projectName] = a.aggregateProjectState(projectName)
	}
}

// sendTelegramEvent sends an event to the Telegram bridge channel if configured.
// Non-blocking; the bridge handles dedup and rate limiting.
// Screen lines are filtered through the agent adapter's ScreenFilter (if any)
// to extract only the conversation area before sending.
func (a *App) sendTelegramEvent(project, sessionID string, state SessionState, detail string, lines []string) {
	if a.telegramCh == nil {
		return
	}

	kind := stateToEventKind(state)
	if kind < 0 {
		return
	}

	// Filter screen lines through the agent adapter to remove sidebar noise.
	if s := a.mgr.GetSession(sessionID); s != nil {
		lines = agent.FilterScreen(s.Project.Agent, lines)
	}

	select {
	case a.telegramCh <- telegram.Event{
		Project:   project,
		SessionID: sessionID,
		Kind:      kind,
		Detail:    detail,
		Screen:    lines,
	}:
	default:
		// Channel full — drop the event (bridge dedup will cover next tick).
	}
}

// aggregateProjectState returns the highest-priority state across all
// sessions for a project. Priority: NeedsPermission > Asking >
// NeedsAttention > Error > Working > Done > Idle.
func (a *App) aggregateProjectState(projectName string) SessionState {
	best := StateIdle
	for _, s := range a.mgr.GetSessionsByProject(projectName) {
		state := a.sessionStates[s.ID]
		if statePriority(state) > statePriority(best) {
			best = state
		}
	}
	return best
}

// statePriority returns a numeric priority for session states, used to
// compute the aggregate "most urgent" state for sidebar display.
func statePriority(s SessionState) int {
	switch s {
	case StateNeedsPermission:
		return 7
	case StateAsking:
		return 6
	case StateNeedsAttention:
		return 5
	case StateError:
		return 4
	case StateWorking:
		return 3
	case StateDone:
		return 2
	case StateIdle:
		return 1
	default:
		return 0
	}
}

// stateToEventKind maps a SessionState to a telegram.EventKind.
// Returns -1 for states that should not be sent.
func stateToEventKind(state SessionState) telegram.EventKind {
	switch state {
	case StateIdle:
		return telegram.EventResponse
	case StateNeedsPermission:
		return telegram.EventPermission
	case StateAsking:
		return telegram.EventQuestion
	case StateNeedsAttention:
		return telegram.EventAttention
	case StateError:
		return telegram.EventError
	case StateDone:
		return telegram.EventDone
	default:
		return -1
	}
}

// attentionEventToState maps an attention.AttentionEvent to a TUI SessionState.
func attentionEventToState(event *attention.AttentionEvent) SessionState {
	switch event.Type {
	case attention.NeedsInput:
		return StateNeedsAttention
	case attention.NeedsPermission:
		return StateNeedsPermission
	case attention.NeedsAnswer:
		return StateAsking
	case attention.HitError, attention.Stuck:
		return StateError
	case attention.NeedsReview:
		return StateDone
	default:
		return StateWorking
	}
}
