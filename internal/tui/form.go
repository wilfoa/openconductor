// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/persona"
)

// approvalOption pairs a display label with a description and the config value.
type approvalOption struct {
	label       string
	description string
	level       config.ApprovalLevel
}

var approvalOptions = []approvalOption{
	{
		label:       "Off",
		description: "Notify me for all permission requests",
		level:       config.ApprovalOff,
	},
	{
		label:       "Safe",
		description: "Auto-approve file edits and safe commands",
		level:       config.ApprovalSafe,
	},
	{
		label:       "Full",
		description: "Auto-approve everything (use with caution)",
		level:       config.ApprovalFull,
	},
}

type formStep int

const (
	stepName formStep = iota
	stepRepo
	stepAgent
	stepPersona
	stepAutoApprove
)

var agentTypes = []config.AgentType{
	config.AgentOpenCode,
	config.AgentClaudeCode,
}

type formModel struct {
	step           formStep
	nameInput      textinput.Model
	repoInput      textinput.Model
	agentIndex     int
	personaIndex   int // index into personaOptions
	approvalIndex  int // index into approvalOptions
	err            string
	existingNames  map[string]bool
	completion     completionModel
	personaOptions []persona.PersonaOption
	customPersonas []config.CustomPersona
}

func newFormModel(existingNames []string, customPersonas []config.CustomPersona) (formModel, tea.Cmd) {
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
		step:           stepName,
		nameInput:      ni,
		repoInput:      ri,
		agentIndex:     0,
		existingNames:  names,
		personaOptions: persona.AllPersonaOptions(customPersonas),
		customPersonas: customPersonas,
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

		case isRuneKey(msg, 'j') && m.step == stepPersona:
			if m.personaIndex < len(m.personaOptions)-1 {
				m.personaIndex++
			}
			return m, nil

		case isRuneKey(msg, 'k') && m.step == stepPersona:
			if m.personaIndex > 0 {
				m.personaIndex--
			}
			return m, nil

		case msg.Type == tea.KeyDown && m.step == stepPersona:
			if m.personaIndex < len(m.personaOptions)-1 {
				m.personaIndex++
			}
			return m, nil

		case msg.Type == tea.KeyUp && m.step == stepPersona:
			if m.personaIndex > 0 {
				m.personaIndex--
			}
			return m, nil

		case isRuneKey(msg, 'j') && m.step == stepAutoApprove:
			if m.approvalIndex < len(approvalOptions)-1 {
				m.approvalIndex++
			}
			return m, nil

		case isRuneKey(msg, 'k') && m.step == stepAutoApprove:
			if m.approvalIndex > 0 {
				m.approvalIndex--
			}
			return m, nil

		case msg.Type == tea.KeyDown && m.step == stepAutoApprove:
			if m.approvalIndex < len(approvalOptions)-1 {
				m.approvalIndex++
			}
			return m, nil

		case msg.Type == tea.KeyUp && m.step == stepAutoApprove:
			if m.approvalIndex > 0 {
				m.approvalIndex--
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
		m.step = stepPersona
		m.repoInput.Blur()
		return m, nil

	case stepPersona:
		selected := m.personaOptions[m.personaIndex]
		m.step = stepAutoApprove
		result := persona.Resolve(selected.Name, m.customPersonas)
		if result.Found && result.Approval != "" {
			for i, opt := range approvalOptions {
				if opt.level == result.Approval {
					m.approvalIndex = i
					break
				}
			}
		}
		return m, nil

	case stepAutoApprove:
		project := config.Project{
			Name:        strings.TrimSpace(m.nameInput.Value()),
			Repo:        strings.TrimSpace(m.repoInput.Value()),
			Agent:       agentTypes[m.agentIndex],
			Persona:     m.personaOptions[m.personaIndex].Name,
			AutoApprove: approvalOptions[m.approvalIndex].level,
		}
		return m, func() tea.Msg { return ProjectAddedMsg{Project: project} }
	}

	return m, nil
}

func (m formModel) stepIndicator() string {
	step := int(m.step) + 1
	return formStepStyle.Render(fmt.Sprintf("%d/5", step))
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
		b.WriteString("\n")
		b.WriteString(formHintStyle.Render("  Esc cancel"))

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
		b.WriteString("\n")
		b.WriteString(formHintStyle.Render("  Esc cancel"))

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
		b.WriteString("\n")
		b.WriteString(formHintStyle.Render("  Esc cancel"))

	case stepPersona:
		b.WriteString(formDoneStyle.Render("Name   " + m.nameInput.Value()))
		b.WriteString("\n")
		b.WriteString(formDoneStyle.Render("Repo   " + m.repoInput.Value()))
		b.WriteString("\n")
		b.WriteString(formDoneStyle.Render("Agent  " + string(agentTypes[m.agentIndex])))
		b.WriteString("\n\n")
		b.WriteString(formLabelStyle.Render("Persona"))
		b.WriteString("\n")
		for i, opt := range m.personaOptions {
			line := fmt.Sprintf("%-8s  %s", opt.Label, opt.Description)
			if i == m.personaIndex {
				b.WriteString(formSelectedStyle.Render("▸ " + line))
			} else {
				b.WriteString(formOptionStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}
		b.WriteString(formHintStyle.Render("  j/k to select, Enter to confirm"))
		b.WriteString("\n")
		b.WriteString(formHintStyle.Render("  Esc cancel"))

	case stepAutoApprove:
		b.WriteString(formDoneStyle.Render("Name   " + m.nameInput.Value()))
		b.WriteString("\n")
		b.WriteString(formDoneStyle.Render("Repo   " + m.repoInput.Value()))
		b.WriteString("\n")
		b.WriteString(formDoneStyle.Render("Agent  " + string(agentTypes[m.agentIndex])))
		b.WriteString("\n")
		personaLabel := persona.Label(m.personaOptions[m.personaIndex].Name, m.customPersonas)
		b.WriteString(formDoneStyle.Render("Persona  " + personaLabel))
		b.WriteString("\n\n")
		b.WriteString(formLabelStyle.Render("Auto-approve permissions"))
		b.WriteString("\n")
		for i, opt := range approvalOptions {
			line := fmt.Sprintf("%-6s  %s", opt.label, opt.description)
			if i == m.approvalIndex {
				b.WriteString(formSelectedStyle.Render("▸ " + line))
			} else {
				b.WriteString(formOptionStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}
		b.WriteString(formHintStyle.Render("  j/k to select, Enter to confirm"))
		b.WriteString("\n")
		b.WriteString(formHintStyle.Render("  Esc cancel"))
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

// selectApproval sets the approval level selection by index (used for mouse clicks).
func (m *formModel) selectApproval(idx int) {
	if idx >= 0 && idx < len(approvalOptions) {
		m.approvalIndex = idx
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

// formPersonaOptionContentStart is the screen Y offset of the first persona
// option within the sidebar for stepPersona:
//
//	line 0: "Add Project"
//	line 1: (blank)
//	line 2: "Name   ..."
//	line 3: "Repo   ..."
//	line 4: "Agent  ..."
//	line 5: (blank)
//	line 6: "Persona"
//	line 7+i: persona option i
const formPersonaOptionContentStart = 7

// formApprovalOptionContentStart is the screen Y offset of the first approval
// option within the sidebar for stepAutoApprove:
//
//	line 0: "Add Project"
//	line 1: (blank)
//	line 2: "Name   ..."
//	line 3: "Repo   ..."
//	line 4: "Agent  ..."
//	line 5: "Persona  ..."
//	line 6: (blank)
//	line 7: "Auto-approve permissions"
//	line 8+i: approval option i
const formApprovalOptionContentStart = 8

// selectPersona sets the persona selection by index (used for mouse clicks).
func (m *formModel) selectPersona(idx int) {
	if idx >= 0 && idx < len(m.personaOptions) {
		m.personaIndex = idx
	}
}
