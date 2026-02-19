package tui

import (
	"fmt"
	"strings"

	"github.com/amir/maestro/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sidebarMode int

const (
	sidebarNormal sidebarMode = iota
	sidebarForm
	sidebarConfirmDelete
)

// Layout constants for click hit-testing.
// Sidebar uses Padding(1,1): 1 line top padding.
// Title "Projects" with MarginBottom(1): 2 content lines.
const (
	sidebarTopPad    = 1
	sidebarTitleRows = 2 // title + margin
	projectRows      = 2 // name line + agent line
)

type sidebarModel struct {
	projects     []config.Project
	states       map[string]SessionState
	selected     int
	focused      bool
	height       int
	contentWidth int
	dragging     bool
	mode         sidebarMode
	form         formModel
}

func newSidebarModel(projects []config.Project, contentWidth int) sidebarModel {
	states := make(map[string]SessionState)
	for _, p := range projects {
		states[p.Name] = StateIdle
	}
	return sidebarModel{
		projects:     projects,
		states:       states,
		selected:     0,
		focused:      false,
		contentWidth: contentWidth,
		mode:         sidebarNormal,
	}
}

func (m sidebarModel) Update(msg tea.Msg) (sidebarModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	// Forward to form if active.
	if m.mode == sidebarForm {
		var cmd tea.Cmd
		m.form, cmd = m.form.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m sidebarModel) handleKey(msg tea.KeyMsg) (sidebarModel, tea.Cmd) {
	switch m.mode {
	case sidebarForm:
		var cmd tea.Cmd
		m.form, cmd = m.form.Update(msg)
		return m, cmd

	case sidebarConfirmDelete:
		switch {
		case isRuneKey(msg, 'y'):
			if m.selected < len(m.projects) {
				name := m.projects[m.selected].Name
				m.mode = sidebarNormal
				return m, func() tea.Msg { return ProjectDeletedMsg{Name: name} }
			}
			m.mode = sidebarNormal
			return m, nil
		case isRuneKey(msg, 'n'), msg.Type == tea.KeyEscape:
			m.mode = sidebarNormal
			return m, nil
		}
		return m, nil

	default: // sidebarNormal
		switch {
		case isRuneKey(msg, 'a'):
			return m.openForm()

		case isRuneKey(msg, 'd'), isRuneKey(msg, 'x'):
			if len(m.projects) > 0 && m.selected < len(m.projects) {
				m.mode = sidebarConfirmDelete
			}
			return m, nil

		case isKey(msg, keys.Up) || isRuneKey(msg, 'k'):
			if m.selected > 0 {
				m.selected--
			}

		case isKey(msg, keys.Down) || isRuneKey(msg, 'j'):
			if m.selected < len(m.projects)-1 {
				m.selected++
			}

		case isKey(msg, keys.Select):
			if len(m.projects) > 0 && m.selected < len(m.projects) {
				return m, func() tea.Msg {
					return ProjectSwitchedMsg{
						Index:   m.selected,
						Project: m.projects[m.selected],
					}
				}
			}
		}
	}

	return m, nil
}

func (m sidebarModel) handleMouse(msg tea.MouseMsg) (sidebarModel, tea.Cmd) {
	switch {
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		return m.handleClick(msg.X, msg.Y)

	case msg.Button == tea.MouseButtonWheelUp:
		if m.mode == sidebarNormal && m.selected > 0 {
			m.selected--
		}
		return m, nil

	case msg.Button == tea.MouseButtonWheelDown:
		if m.mode == sidebarNormal && m.selected < len(m.projects)-1 {
			m.selected++
		}
		return m, nil
	}

	return m, nil
}

func (m sidebarModel) handleClick(x, y int) (sidebarModel, tea.Cmd) {
	switch m.mode {
	case sidebarNormal:
		// Check if a project was clicked.
		projStart := sidebarTopPad + sidebarTitleRows
		for i := range m.projects {
			projY := projStart + i*projectRows
			if y == projY || y == projY+1 {
				m.selected = i
				return m, func() tea.Msg {
					return ProjectSwitchedMsg{
						Index:   i,
						Project: m.projects[i],
					}
				}
			}
		}

		// Check if [+] button was clicked.
		addY := m.addButtonY()
		if y == addY {
			return m.openForm()
		}

	case sidebarForm:
		// In form step 3, clicking an agent option selects it.
		if m.form.step == stepAgent {
			for i := range agentTypes {
				optionY := sidebarTopPad + formAgentOptionContentStart + i
				if y == optionY {
					m.form.selectAgent(i)
					return m, nil
				}
			}
		}
	}

	return m, nil
}

func (m sidebarModel) openForm() (sidebarModel, tea.Cmd) {
	names := make([]string, len(m.projects))
	for i, p := range m.projects {
		names[i] = p.Name
	}
	form, cmd := newFormModel(names)
	m.form = form
	m.mode = sidebarForm
	return m, cmd
}

// addButtonY returns the screen Y of the [+] button.
func (m sidebarModel) addButtonY() int {
	if len(m.projects) == 0 {
		// Empty state: title(2) + 2 hint lines + 1 blank
		return sidebarTopPad + sidebarTitleRows + 3
	}
	// After projects + 1 blank line for spacing.
	return sidebarTopPad + sidebarTitleRows + len(m.projects)*projectRows + 1
}

func (m sidebarModel) View() string {
	var b strings.Builder

	if m.mode == sidebarForm {
		b.WriteString(m.form.View())
	} else {
		if m.focused {
			b.WriteString(sidebarTitleFocusedStyle.Render("▸ Projects"))
		} else {
			b.WriteString(sidebarTitleStyle.Render("  Projects"))
		}
		b.WriteString("\n")

		if len(m.projects) == 0 {
			b.WriteString(emptyHintStyle.Render("No projects"))
			b.WriteString("\n")
			b.WriteString(emptyHintStyle.Render("Press a to add"))
			b.WriteString("\n")
		} else {
			// Inner content width (container has Padding 1 on each side).
			innerWidth := m.contentWidth - 2
			if innerWidth < 0 {
				innerWidth = 0
			}
			for i, p := range m.projects {
				state := m.states[p.Name]
				name := p.Name

				if i == m.selected {
					// Bracket-highlighted: [● name] — single style, no nested ANSI.
					char := badgeChar(state)
					label := "[" + char + " " + name + "]"
					b.WriteString(projectActiveStyle.Width(innerWidth).Render(label))
					b.WriteString("\n")
					agentStyle := lipgloss.NewStyle().
						Foreground(colorMuted).
						Background(colorHighlight).
						PaddingLeft(3).
						Width(innerWidth)
					b.WriteString(agentStyle.Render(agentDisplayName(p.Agent)))
				} else {
					// Aligned: " ● name" — badge at col 1, name at col 3.
					badge := m.statusBadge(p.Name)
					label := " " + badge + " " + name
					b.WriteString(projectItemStyle.Render(label))
					b.WriteString("\n")
					b.WriteString(projectAgentStyle.Render(agentDisplayName(p.Agent)))
				}
				b.WriteString("\n")
			}
		}

		if m.mode == sidebarConfirmDelete && m.selected < len(m.projects) {
			b.WriteString("\n")
			name := m.projects[m.selected].Name
			b.WriteString(confirmStyle.Render(fmt.Sprintf("Delete %s? (y/n)", name)))
		} else {
			b.WriteString("\n")
			b.WriteString(addButtonStyle.Render("+ new project"))
		}
	}

	style := sidebarStyle
	if m.dragging {
		style = sidebarDraggingStyle
	} else if m.focused {
		style = sidebarFocusedStyle
	}

	content := b.String()
	return style.Width(m.contentWidth).Height(m.height).Render(content)
}

func (m sidebarModel) statusBadge(projectName string) string {
	state, ok := m.states[projectName]
	if !ok {
		state = StateIdle
	}

	switch state {
	case StateWorking:
		return badgeWorking.String()
	case StateNeedsAttention:
		return badgeAttention.String()
	case StateError:
		return badgeError.String()
	case StateDone:
		return badgeDone.String()
	default:
		return badgeIdle.String()
	}
}

// badgeChar returns the raw badge character for a session state (no ANSI codes).
func badgeChar(state SessionState) string {
	switch state {
	case StateWorking:
		return "●"
	case StateNeedsAttention:
		return "◆"
	case StateError:
		return "●"
	case StateDone:
		return "✓"
	default:
		return "○"
	}
}

// agentDisplayName returns a compact name for sidebar rendering.
func agentDisplayName(agent config.AgentType) string {
	if agent == config.AgentClaudeCode {
		return "claude"
	}
	return string(agent)
}

func (m sidebarModel) Width() int {
	border := lipgloss.NormalBorder()
	// lipgloss .Width(n) includes padding but excludes borders.
	// Rendered = contentWidth (includes padding) + border right.
	return m.contentWidth + lipgloss.Width(border.Right)
}
