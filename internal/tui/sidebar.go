// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openconductorhq/openconductor/internal/config"
)

type sidebarMode int

const (
	sidebarNormal sidebarMode = iota
	sidebarForm
	sidebarConfirmDelete
)

// Layout constants for click hit-testing.
// sidebarTopPaddingding is derived from styles in styles.go init().
// Title "Projects" with MarginBottom(1): 2 content lines.
const (
	sidebarTitleRows = 2 // title + margin
	projectRows      = 3 // name line + agent line + separator
)

type sidebarModel struct {
	projects     []config.Project
	states       map[string]SessionState
	openTabs     map[string]bool // tracks which projects have open tabs
	selected     int
	focused      bool
	height       int
	contentWidth int
	dragging     bool
	mode         sidebarMode
	form         formModel
	animFrame    int // cycles 0..animFrameCount-1 for working badge breathing
}

func newSidebarModel(projects []config.Project, contentWidth int) sidebarModel {
	states := make(map[string]SessionState)
	for _, p := range projects {
		states[p.Name] = StateIdle
	}
	return sidebarModel{
		projects:     projects,
		states:       states,
		openTabs:     make(map[string]bool),
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

		case isRuneKey(msg, 't'):
			return m, func() tea.Msg {
				return SystemTabRequestMsg{
					Name: "Telegram Setup",
					Args: []string{"telegram", "setup"},
				}
			}

		case isRuneKey(msg, 'n'):
			// New instance: always create a new agent session, even if
			// the project already has open tabs.
			if len(m.projects) > 0 && m.selected < len(m.projects) {
				p := m.projects[m.selected]
				return m, func() tea.Msg {
					return NewInstanceMsg{Project: p}
				}
			}
			return m, nil

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
		projStart := sidebarTopPadding + sidebarTitleRows
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
				optionY := sidebarTopPadding + formAgentOptionContentStart + i
				if y == optionY {
					m.form.selectAgent(i)
					return m, nil
				}
			}
		}
		// In form step 4, clicking an approval level option selects it.
		if m.form.step == stepAutoApprove {
			for i := range approvalOptions {
				optionY := sidebarTopPadding + formApprovalOptionContentStart + i
				if y == optionY {
					m.form.selectApproval(i)
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
		return sidebarTopPadding + sidebarTitleRows + 3
	}
	// Each project occupies projectRows (name + agent + separator), but the
	// last project has no separator — the blank line before the button fills
	// that slot.  So total = N * projectRows with no extra +1.
	return sidebarTopPadding + sidebarTitleRows + len(m.projects)*projectRows
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
			b.WriteString(emptyHintStyle.Render("a add  t telegram"))
			b.WriteString("\n")
		} else {
			// Inner content width (container padding subtracted).
			innerWidth := m.contentWidth - sidebarHPad
			if innerWidth < 0 {
				innerWidth = 0
			}
			for i, p := range m.projects {
				name := p.Name

				if i == m.selected {
					// Left accent bar: ▎ in gold with highlight background.
					// Render name + agent as a single block so the accent
					// spans both lines. Width = innerWidth - 1 because the
					// border consumes 1 column.
					//
					// The badge icon is colored inline using a raw ANSI
					// foreground escape (no reset) so the surrounding
					// projectActiveStyle (bold + fg + bg) stays intact
					// for the project name that follows.
					var nameLine string
					var agentLine string
					if m.openTabs[p.Name] {
						state := m.states[p.Name]
						char := badgeChar(state, m.animFrame)
						badgeFG := rawFG(stateBadgeColor(state, m.animFrame))
						restoreFG := rawFG(colorFg)
						nameLine = badgeFG + char + restoreFG + " " + name
						agentLine = "  " + agentDisplayName(p.Agent) + " · " + m.stateLabel(p.Name)
					} else {
						nameLine = "  " + name // space for alignment (no badge)
						agentLine = "  " + agentDisplayName(p.Agent)
					}
					content := nameLine + "\n" + agentLine
					b.WriteString(projectActiveStyle.
						Width(innerWidth - activeProjectBorderW).
						Render(content))
				} else {
					// Aligned: " ● name" — badge at col 1, name at col 3.
					// Leading space occupies the same column as the accent
					// border on selected items.
					badge := m.statusBadge(p.Name)
					label := " " + badge + " " + name
					b.WriteString(projectItemStyle.Render(label))
					b.WriteString("\n")
					var agentLine string
					if m.openTabs[p.Name] {
						agentLine = agentDisplayName(p.Agent) + " · " + m.stateLabel(p.Name)
					} else {
						agentLine = agentDisplayName(p.Agent)
					}
					b.WriteString(projectAgentStyle.Render(agentLine))
				}
				b.WriteString("\n")
				if i < len(m.projects)-1 {
					b.WriteString(projectSeparatorStyle.Render(strings.Repeat("─", innerWidth)))
					b.WriteString("\n")
				}
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
	// Only show badge if the project has an open tab.
	if !m.openTabs[projectName] {
		return " " // space for alignment
	}

	state, ok := m.states[projectName]
	if !ok {
		state = StateIdle
	}

	switch state {
	case StateWorking:
		return breathingBadgeStyles[m.animFrame].String()
	case StateNeedsAttention:
		return badgeAttention.String()
	case StateNeedsPermission:
		return badgePermission.String()
	case StateAsking:
		return badgeAsking.String()
	case StateError:
		return badgeError.String()
	case StateDone:
		return badgeDone.String()
	default:
		// Idle = steady green ● (agent is online).
		return badgeOnline.String()
	}
}

// breathingChars maps animation frame → badge character for the breathing
// cycle: ● → • → · → • → ●  (shrink then grow).
var breathingChars = [animFrameCount]string{"●", "•", "·", "•"}

// badgeChar returns the raw badge character for a session state (no ANSI codes).
// For StateWorking, animFrame selects a frame from the breathing cycle.
// For StateIdle, always returns ● (steady online indicator).
func badgeChar(state SessionState, animFrame int) string {
	switch state {
	case StateWorking:
		return breathingChars[animFrame]
	case StateNeedsAttention:
		return "◆"
	case StateNeedsPermission:
		return "!"
	case StateAsking:
		return "?"
	case StateError:
		return "●"
	case StateDone:
		return "✓"
	default:
		return "●"
	}
}

// stateLabel returns a short state label for inline display next to the agent name.
func (m sidebarModel) stateLabel(projectName string) string {
	state, ok := m.states[projectName]
	if !ok {
		state = StateIdle
	}
	return state.String()
}

// stateBadgeColor returns the foreground color for a state badge icon.
func stateBadgeColor(state SessionState, animFrame int) lipgloss.Color {
	switch state {
	case StateNeedsAttention:
		return colorWarning
	case StateNeedsPermission:
		return colorPermission
	case StateAsking:
		return colorQuestion
	case StateError:
		return colorDanger
	case StateDone:
		return colorInfo
	case StateWorking:
		switch animFrame {
		case 1, 3:
			return colorSuccessMid
		case 2:
			return colorMuted
		}
		return colorSuccess
	default:
		return colorSuccess
	}
}

// rawFG returns a bare ANSI SGR sequence that sets the foreground to the
// given hex color (e.g. "#E5C07B") without a trailing reset. This allows
// coloring a single character inline without breaking a surrounding
// lipgloss-styled block.
func rawFG(c lipgloss.Color) string {
	hex := string(c)
	if len(hex) != 7 || hex[0] != '#' {
		return ""
	}
	r, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b, _ := strconv.ParseUint(hex[5:7], 16, 8)
	return "\x1b[38;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b)) + "m"
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
