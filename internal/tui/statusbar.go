package tui

import (
	"strconv"
	"strings"

	"github.com/amir/maestro/internal/config"
	"github.com/charmbracelet/lipgloss"
)

type statusBarModel struct {
	projects       []config.Project
	states         map[string]SessionState
	width          int
	activeName     string
	sidebarFocused bool
	ctrlCHint      bool // show "press Ctrl+C again to exit" hint
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
			{"Esc", "terminal"},
			{"j/k", "navigate"},
			{"^j/k", "tab"},
			{"a", "add"},
			{"d", "delete"},
			{"Ctrl+C", "exit"},
		}
	} else {
		hints = []struct{ key, label string }{
			{"Esc", "sidebar"},
			{"^j/k", "tab"},
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

	if m.activeName != "" {
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

	right := strings.Join(rightParts, statusDimStyle.Render("  "))

	leftStr := left.String()
	available := m.width - lipgloss.Width(leftStr) - lipgloss.Width(right) - 2
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
