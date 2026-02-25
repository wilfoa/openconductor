// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AppState stores ephemeral runtime state that persists across restarts,
// such as which tabs were open and which was active. This is intentionally
// separate from the user-editable config.yaml.
type AppState struct {
	// OpenTabs is the list of project names that had open tabs, in order.
	OpenTabs []string `json:"open_tabs"`
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

// FilterValidProjects returns only the project names from tabs that still
// exist in the current config. Projects that were deleted since the last
// session are silently removed.
func FilterValidProjects(tabs []string, projects []Project) []string {
	valid := make(map[string]bool, len(projects))
	for _, p := range projects {
		valid[p.Name] = true
	}
	var result []string
	for _, name := range tabs {
		if valid[name] {
			result = append(result, name)
		}
	}
	return result
}
