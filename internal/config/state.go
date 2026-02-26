// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TabState stores the persistent state of a single tab.
type TabState struct {
	// Project is the project name this tab is associated with.
	Project string `json:"project"`
	// Label is the user-assigned display name. Empty means use the
	// default session ID (project name or "project (N)").
	Label string `json:"label,omitempty"`
}

// AppState stores ephemeral runtime state that persists across restarts,
// such as which tabs were open and which was active. This is intentionally
// separate from the user-editable config.yaml.
type AppState struct {
	// OpenTabs is the list of tabs that were open, in order.
	OpenTabs []TabState `json:"open_tabs"`
	// ActiveTab is the project name of the tab that was focused when the
	// user exited. Empty string means "use the first open tab".
	ActiveTab string `json:"active_tab"`
}

// DefaultStatePath returns the path to the state file.
func DefaultStatePath() string {
	return filepath.Join(DefaultConfigDir(), "state.json")
}

// LoadState reads the app state from disk. Returns a zero-value AppState
// (no open tabs) if the file doesn't exist or can't be parsed.
func LoadState(path string) AppState {
	data, err := os.ReadFile(path)
	if err != nil {
		return AppState{}
	}
	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return AppState{}
	}
	return state
}

// SaveState writes the app state to disk, creating directories as needed.
func SaveState(path string, state AppState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// FilterValidTabs returns only the tabs whose project still exists in the
// current config. Projects that were deleted since the last session are
// silently removed.
func FilterValidTabs(tabs []TabState, projects []Project) []TabState {
	valid := make(map[string]bool, len(projects))
	for _, p := range projects {
		valid[p.Name] = true
	}
	var result []TabState
	for _, ts := range tabs {
		if valid[ts.Project] {
			result = append(result, ts)
		}
	}
	return result
}

// TabProjectNames extracts just the project names from a slice of TabState,
// preserving order.
func TabProjectNames(tabs []TabState) []string {
	names := make([]string, len(tabs))
	for i, ts := range tabs {
		names[i] = ts.Project
	}
	return names
}
