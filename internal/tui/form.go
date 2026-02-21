package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openconductorhq/openconductor/internal/config"
)

type formStep int

const (
	stepName formStep = iota
	stepRepo
	stepAgent
)

var agentTypes = []config.AgentType{
	config.AgentClaudeCode,
	config.AgentCodex,
	config.AgentGemini,
	config.AgentOpenCode,
}

type formModel struct {
	step          formStep
	nameInput     textinput.Model
	repoInput     textinput.Model
	agentIndex    int
	err           string
	existingNames map[string]bool
	completion    completionModel
}

func newFormModel(existingNames []string) (formModel, tea.Cmd) {
	ni := textinput.New()
	ni.Placeholder = "my-project"
	ni.CharLimit = 64

	ri := textinput.New()
	ri.Placeholder = "/path/to/repo"
	ri.CharLimit = 256

	names := make(map[string]bool)
	for _, n := range existingNames {
		names[n] = true
	}

	cmd := ni.Focus()

	return formModel{
		step:          stepName,
		nameInput:     ni,
		repoInput:     ri,
		agentIndex:    0,
		existingNames: names,
	}, cmd
}

func (m formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Completion-specific keys at stepRepo.
		if m.step == stepRepo {
			switch {
			case msg.Type == tea.KeyEscape && m.completion.visible:
				// Dismiss dropdown, don't cancel form.
				m.completion.visible = false
				m.completion.selected = 0
				return m, nil

			case msg.Type == tea.KeyTab && m.completion.visible && len(m.completion.suggestions) > 0:
				// Accept selected suggestion.
				accepted := m.completion.suggestions[m.completion.selected]
				m.repoInput.SetValue(accepted)
				m.repoInput.SetCursor(len(accepted))
				// Re-scan for nested completions.
				m.completion.suggestions = scanDirectories(accepted)
				m.completion.visible = len(m.completion.suggestions) > 0
				m.completion.selected = 0
				m.completion.lastScanned = accepted
				return m, nil

			case msg.Type == tea.KeyDown && m.completion.visible && len(m.completion.suggestions) > 0:
				if m.completion.selected < len(m.completion.suggestions)-1 {
					m.completion.selected++
				}
				return m, nil

			case msg.Type == tea.KeyUp && m.completion.visible && len(m.completion.suggestions) > 0:
				if m.completion.selected > 0 {
					m.completion.selected--
				}
				return m, nil
			}
		}

		switch {
		case msg.Type == tea.KeyEscape:
			return m, func() tea.Msg { return FormCancelledMsg{} }

		case msg.Type == tea.KeyEnter:
			return m.advance()

		case isRuneKey(msg, 'j') && m.step == stepAgent:
			if m.agentIndex < len(agentTypes)-1 {
				m.agentIndex++
			}
			return m, nil

		case isRuneKey(msg, 'k') && m.step == stepAgent:
			if m.agentIndex > 0 {
				m.agentIndex--
			}
			return m, nil

		case msg.Type == tea.KeyDown && m.step == stepAgent:
			if m.agentIndex < len(agentTypes)-1 {
				m.agentIndex++
			}
			return m, nil

		case msg.Type == tea.KeyUp && m.step == stepAgent:
			if m.agentIndex > 0 {
				m.agentIndex--
			}
			return m, nil
		}
	}

	// Forward to active text input.
	var cmd tea.Cmd
	switch m.step {
	case stepName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case stepRepo:
		prevValue := m.repoInput.Value()
		m.repoInput, cmd = m.repoInput.Update(msg)
		// Re-scan when value changed.
		newValue := m.repoInput.Value()
		if newValue != prevValue && newValue != m.completion.lastScanned {
			m.completion.suggestions = scanDirectories(newValue)
			m.completion.visible = len(m.completion.suggestions) > 0
			m.completion.selected = 0
			m.completion.lastScanned = newValue
		}
	}

	return m, cmd
}

func (m formModel) advance() (formModel, tea.Cmd) {
	m.err = ""

	switch m.step {
	case stepName:
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.err = "Name cannot be empty"
			return m, nil
		}
		if m.existingNames[name] {
			m.err = "Name already exists"
			return m, nil
		}
		m.step = stepRepo
		m.nameInput.Blur()
		cmd := m.repoInput.Focus()
		return m, cmd

	case stepRepo:
		repo := strings.TrimSpace(m.repoInput.Value())
		if repo == "" {
			m.err = "Repo path cannot be empty"
			return m, nil
		}
		// Expand ~ to home dir.
		if strings.HasPrefix(repo, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				repo = filepath.Join(home, repo[2:])
			}
		}
		info, err := os.Stat(repo)
		if err != nil || !info.IsDir() {
			m.err = "Path does not exist or is not a directory"
			return m, nil
		}
		m.repoInput.SetValue(repo)
		m.step = stepAgent
		m.repoInput.Blur()
		return m, nil

	case stepAgent:
		project := config.Project{
			Name:  strings.TrimSpace(m.nameInput.Value()),
			Repo:  strings.TrimSpace(m.repoInput.Value()),
			Agent: agentTypes[m.agentIndex],
		}
		return m, func() tea.Msg { return ProjectAddedMsg{Project: project} }
	}

	return m, nil
}

func (m formModel) stepIndicator() string {
	step := int(m.step) + 1
	return formStepStyle.Render(fmt.Sprintf("%d/3", step))
}

func (m formModel) View() string {
	var b strings.Builder

	b.WriteString(formTitleStyle.Render("New project"))
	b.WriteString(" ")
	b.WriteString(m.stepIndicator())
	b.WriteString("\n\n")

	switch m.step {
	case stepName:
		b.WriteString(formLabelStyle.Render("Name"))
		b.WriteString("\n")
		b.WriteString(formInputStyle.Render(m.nameInput.View()))
		b.WriteString("\n")
		b.WriteString(formHintStyle.Render("  A unique project name"))

	case stepRepo:
		b.WriteString(formDoneStyle.Render("Name  " + m.nameInput.Value()))
		b.WriteString("\n\n")
		b.WriteString(formLabelStyle.Render("Repo path"))
		b.WriteString("\n")
		b.WriteString(formInputStyle.Render(m.repoInput.View()))
		b.WriteString("\n")
		if m.completion.visible && len(m.completion.suggestions) > 0 {
			for i, s := range m.completion.suggestions {
				name := completionDisplayName(s)
				if i == m.completion.selected {
					b.WriteString(completionSelectedStyle.Render("> " + name))
				} else {
					b.WriteString(completionItemStyle.Render("  " + name))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString(formHintStyle.Render("  Absolute path to repo"))

	case stepAgent:
		b.WriteString(formDoneStyle.Render("Name  " + m.nameInput.Value()))
		b.WriteString("\n")
		b.WriteString(formDoneStyle.Render("Repo  " + m.repoInput.Value()))
		b.WriteString("\n\n")
		b.WriteString(formLabelStyle.Render("Agent"))
		b.WriteString("\n")
		for i, a := range agentTypes {
			if i == m.agentIndex {
				b.WriteString(formSelectedStyle.Render("▸ " + string(a)))
			} else {
				b.WriteString(formOptionStyle.Render("  " + string(a)))
			}
			b.WriteString("\n")
		}
		b.WriteString(formHintStyle.Render("  j/k to select, Enter to confirm"))
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(formErrorStyle.Render(m.err))
	}

	return b.String()
}

// selectAgent sets the agent selection by index (used for mouse clicks).
func (m *formModel) selectAgent(idx int) {
	if idx >= 0 && idx < len(agentTypes) {
		m.agentIndex = idx
	}
}

// agentOptionY returns the screen Y of agent option i within the sidebar.
// Sidebar top padding = 1, form content for stepAgent:
//
//	line 0: "Add Project"
//	line 1: (blank)
//	line 2: "Name: ..."
//	line 3: "Repo: ..."
//	line 4: (blank)
//	line 5: "Agent:"
//	line 6+i: agent option i
const formAgentOptionContentStart = 6
