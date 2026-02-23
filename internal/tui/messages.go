// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import "github.com/openconductorhq/openconductor/internal/config"

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

// ProjectSwitchedMsg is sent when the user selects a different project.
type ProjectSwitchedMsg struct {
	Index   int
	Project config.Project
}

// SessionStateChangedMsg is sent when a session's state changes.
type SessionStateChangedMsg struct {
	ProjectName string
	State       SessionState
	Detail      string
}

// TabClosedMsg is sent when a user clicks the close button on a tab.
type TabClosedMsg struct {
	Name string
}

// TickMsg triggers periodic checks (attention detection, etc).
type TickMsg struct{}

// AnimTickMsg triggers the working badge animation (~300ms).
type AnimTickMsg struct{}

// ProjectAddedMsg is sent when a new project is added via the form.
type ProjectAddedMsg struct {
	Project config.Project
}

// ProjectDeletedMsg is sent when a project is deleted.
type ProjectDeletedMsg struct {
	Name string
}

// FormCancelledMsg is sent when the add-project form is cancelled.
type FormCancelledMsg struct{}

// ConfigSavedMsg signals that the config was persisted to disk.
type ConfigSavedMsg struct {
	Err error
}
