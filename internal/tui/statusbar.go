// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openconductorhq/openconductor/internal/config"
)

type statusBarModel struct {
	projects       []config.Project
	states         map[string]SessionState
	width          int
	activeName     string
	selectedName   string // sidebar-highlighted project (may differ from active tab)
	sidebarFocused bool
	ctrlCHint      bool // show "press Ctrl+C again to exit" hint
	scrollOffset   int  // >0 when terminal is in scrollback mode
}

func newStatusBarModel(projects []config.Project) statusBarModel {
	states := make(map[string]SessionState)
	for _, p := range projects {
		states[p.Name] = StateIdle
	}
	return statusBarModel{
		projects: projects,
		states:   states,
	}
}

func (m statusBarModel) View() string {
	// Left: context-sensitive keybind hints.
	var hints []struct{ key, label string }
	if m.ctrlCHint {
		hints = []struct{ key, label string }{
			{"Ctrl+C", "again to exit"},
		}
	} else if m.sidebarFocused {
		hints = []struct{ key, label string }{
			{"^S", "terminal"},
			{"j/k", "navigate"},
			{"^j/k", "tab"},
			{"n", "new instance"},
			{"a", "add"},
			{"d", "delete"},
			{"Ctrl+C", "exit"},
		}
	} else {
		hints = []struct{ key, label string }{
			{"^S", "sidebar"},
			{"^j/k", "tab"},
			{"F2", "rename"},
			{"Ctrl+C", "exit"},
		}
	}

	var left strings.Builder
	for i, h := range hints {
		if i > 0 {
			left.WriteString(statusDimStyle.Render("  "))
		}
		if m.ctrlCHint && h.key == "Ctrl+C" {
			// Highlight the exit hint in danger color.
			left.WriteString(statusExitHintStyle.Render(h.key + " " + h.label))
		} else {
			left.WriteString(statusKeyStyle.Render(h.key))
			left.WriteString(statusDimStyle.Render(" " + h.label))
		}
	}

	// Right: active project + state + aggregate health.
	var rightParts []string

	// When sidebar is focused, show the selected project's state description.
	// Otherwise show the active tab's state.
	if m.sidebarFocused && m.selectedName != "" {
		state := m.states[m.selectedName]
		rightParts = append(rightParts,
			statusAccentStyle.Render(m.selectedName)+" "+stateStyle(state).Render(state.Description()))
	} else if m.activeName != "" {
		state := m.states[m.activeName]
		rightParts = append(rightParts,
			statusAccentStyle.Render(m.activeName)+" "+stateStyle(state).Render(state.String()))
	}

	// Count agents that need attention.
	attentionCount := 0
	for _, state := range m.states {
		if state == StateNeedsAttention || state == StateError {
			attentionCount++
		}
	}
	if attentionCount > 0 {
		badge := lipgloss.NewStyle().Foreground(colorWarning)
		rightParts = append(rightParts, badge.Render("◆ "+strconv.Itoa(attentionCount)+" need attention"))
	}

	if m.scrollOffset > 0 {
		scrollStyle := lipgloss.NewStyle().Foreground(colorInfo)
		rightParts = append(rightParts, scrollStyle.Render("SCROLL ↑"+strconv.Itoa(m.scrollOffset)))
	}

	right := strings.Join(rightParts, statusDimStyle.Render("  "))

	leftStr := left.String()
	available := m.width - lipgloss.Width(leftStr) - lipgloss.Width(right) - statusBarHPad
	if available < 0 {
		available = 0
	}
	gap := strings.Repeat(" ", available)

	content := leftStr + gap + right
	return statusBarStyle.Width(m.width).Render(content)
}

// stateStyle returns a lipgloss style with the appropriate color for a session state.
func stateStyle(s SessionState) lipgloss.Style {
	switch s {
	case StateWorking:
		return lipgloss.NewStyle().Foreground(colorSuccess)
	case StateNeedsAttention:
		return lipgloss.NewStyle().Foreground(colorWarning)
	case StateError:
		return lipgloss.NewStyle().Foreground(colorDanger)
	case StateDone:
		return lipgloss.NewStyle().Foreground(colorInfo)
	default:
		return statusDimStyle
	}
}
