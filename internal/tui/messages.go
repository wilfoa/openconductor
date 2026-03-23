// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/openconductorhq/openconductor/internal/config"
)

// SessionState represents the state of an agent session.
type SessionState int

const (
	StateIdle SessionState = iota
	StateWorking
	StateNeedsAttention
	// StateNeedsPermission indicates the agent is requesting a permission
	// decision (e.g. file read/write, command execution).
	StateNeedsPermission
	// StateAsking indicates the agent is asking the user a structured question
	// (e.g. OpenCode's multi-option question dialog).
	StateAsking
	StateError
	StateDone
)

func (s SessionState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateWorking:
		return "working"
	case StateNeedsAttention:
		return "attention"
	case StateNeedsPermission:
		return "permission"
	case StateAsking:
		return "asking"
	case StateError:
		return "error"
	case StateDone:
		return "done"
	default:
		return "unknown"
	}
}

// Description returns a human-readable explanation of the state,
// suitable for the status bar when the sidebar item is focused.
func (s SessionState) Description() string {
	switch s {
	case StateIdle:
		return "waiting for prompt"
	case StateWorking:
		return "agent is working"
	case StateNeedsAttention:
		return "needs your input"
	case StateNeedsPermission:
		return "needs permission"
	case StateAsking:
		return "agent has a question"
	case StateError:
		return "agent error"
	case StateDone:
		return "task completed"
	default:
		return ""
	}
}

// ProjectSwitchedMsg is sent when the user selects a project in the sidebar
// (click or Enter). If the project already has an open tab, the app switches
// to it. Otherwise a new session is created.
type ProjectSwitchedMsg struct {
	Index   int
	Project config.Project
}

// NewInstanceMsg is sent when the user explicitly requests a new agent
// instance for the selected project (n key in sidebar). Always creates a
// new session, even if the project already has open tabs.
type NewInstanceMsg struct {
	Project config.Project
}

// TabSwitchedMsg is sent when the user clicks a tab or uses Ctrl+J/K to
// switch between existing sessions. Does NOT create a new session.
type TabSwitchedMsg struct {
	SessionID string
}

// SessionStateChangedMsg is sent when a session's state changes.
type SessionStateChangedMsg struct {
	SessionID string
	State     SessionState
	Detail    string
}

// TabClosedMsg is sent when a user clicks the close button on a tab.
type TabClosedMsg struct {
	Name string // session ID
}

// TickMsg triggers periodic checks (attention detection, etc).
type TickMsg struct{}

// AnimTickMsg triggers the working badge animation (~300ms).
type AnimTickMsg struct{}

// ProjectAddedMsg is sent when a new project is added via the form.
type ProjectAddedMsg struct {
	Project config.Project
}

// AgentSwitchedMsg is sent when the user switches a project's agent type
// via the sidebar (s key). All existing sessions are stopped and a new
// session is started with the new agent.
type AgentSwitchedMsg struct {
	ProjectName string
	NewAgent    config.AgentType
}

// ProjectDeletedMsg is sent when a project is deleted.
type ProjectDeletedMsg struct {
	Name string
}

// FocusTerminalMsg is sent when the sidebar wants to return focus to the
// terminal. When ForwardEsc is true, an Esc keypress is also forwarded to
// the active session's PTY (e.g. to dismiss a dialog in OpenCode).
type FocusTerminalMsg struct {
	ForwardEsc bool
}

// FormCancelledMsg is sent when the add-project form is cancelled.
type FormCancelledMsg struct{}

// ConfigSavedMsg signals that the config was persisted to disk.
type ConfigSavedMsg struct {
	Err error
}

// SystemTabRequestMsg asks the App to open a system tab running a command.
// The tab is not backed by an agent — it runs an arbitrary process in a PTY.
type SystemTabRequestMsg struct {
	Name string   // tab name (e.g. "Telegram Setup")
	Args []string // args to pass to the openconductor binary (e.g. ["telegram", "setup"])
}

// SystemTabExitedMsg signals that a system tab process has exited.
// The App uses this to trigger post-exit actions (e.g. config reload).
type SystemTabExitedMsg struct {
	Name string
}

// historyLoadedMsg delivers pre-loaded conversation history to be pushed
// into the session's scrollback buffer.
type historyLoadedMsg struct {
	SessionID string
	Lines     []string
}

// clipboardResultMsg signals that a clipboard copy operation completed.
type clipboardResultMsg struct {
	Err error
}

// telegramSessionRequestMsg is sent by the Telegram handler (via
// Program.Send) when a message arrives for a project with no active
// session. The TUI creates a tab + session and signals back on the
// done channel.
type telegramSessionRequestMsg struct {
	ProjectName string
	Done        chan<- bool
}

// TelegramSessionRequest creates a telegramSessionRequestMsg for use
// with Program.Send from main.go.
func TelegramSessionRequest(projectName string, done chan<- bool) tea.Msg {
	return telegramSessionRequestMsg{
		ProjectName: projectName,
		Done:        done,
	}
}
