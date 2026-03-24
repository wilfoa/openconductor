// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package persona manages behavioral presets that write instruction text
// into agent-specific files (CLAUDE.md, AGENTS.md) in project repos.
package persona

import "github.com/openconductorhq/openconductor/internal/config"

// targetFile maps each agent type to the filename where persona instructions
// are written inside the project repository.
var targetFile = map[config.AgentType]string{
	config.AgentClaudeCode: "CLAUDE.md",
	config.AgentOpenCode:   "AGENTS.md",
}

// TargetFile returns the instruction filename for the given agent type.
func TargetFile(agentType config.AgentType) string {
	return targetFile[agentType]
}

// instructionText maps built-in persona types to their markdown instruction
// blocks.
var instructionText = map[config.PersonaType]string{
	config.PersonaVibe: `## OpenConductor Persona: Vibe

Move fast and ship. Optimize for velocity over perfection.

- Make autonomous decisions without asking for confirmation
- Skip writing tests unless explicitly requested
- Prefer quick implementations over architecturally pure ones
- Use simple, direct solutions -- avoid over-engineering
- When in doubt, pick the simpler option and move on
- Commit frequently with short messages
- It is okay to cut corners for speed`,

	config.PersonaPOC: `## OpenConductor Persona: POC

Build working demonstrations with reasonable quality.

- Create functional prototypes that demonstrate the concept
- Include basic error handling for common failure modes
- Write tests for critical paths and core logic
- Use pragmatic architecture -- good enough for demos, not overbuilt
- Ask before making major architectural decisions
- Handle the happy path thoroughly; edge cases can be basic
- Include brief inline comments for non-obvious logic`,

	config.PersonaScale: `## OpenConductor Persona: Scale

Production-grade engineering with comprehensive quality.

- Practice TDD: write tests before implementation
- Comprehensive test coverage including edge cases and error paths
- Follow SOLID principles and established design patterns
- Thorough error handling with meaningful error messages
- Always ask before making decisions with broad impact
- Write clear documentation for public APIs
- Consider performance, security, and maintainability
- Use meaningful commit messages that explain the why
- Refactor proactively when you see code smells`,
}

// InstructionText returns the markdown instruction block for a built-in
// persona. Returns an empty string for unknown or None personas.
func InstructionText(persona config.PersonaType) string {
	return instructionText[persona]
}

// defaultApproval maps each persona type to its suggested auto-approval level.
var defaultApproval = map[config.PersonaType]config.ApprovalLevel{
	config.PersonaNone:  config.ApprovalOff,
	config.PersonaVibe:  config.ApprovalFull,
	config.PersonaPOC:   config.ApprovalSafe,
	config.PersonaScale: config.ApprovalOff,
}

// DefaultApproval returns the suggested auto-approval level for the given
// persona. Returns ApprovalOff for unknown personas.
func DefaultApproval(persona config.PersonaType) config.ApprovalLevel {
	if level, ok := defaultApproval[persona]; ok {
		return level
	}
	return config.ApprovalOff
}

// builtinLabels maps built-in persona types to their human-readable labels.
var builtinLabels = map[config.PersonaType]string{
	config.PersonaVibe:  "Vibe",
	config.PersonaPOC:   "POC",
	config.PersonaScale: "Scale",
}

// Label returns the human-readable label for a persona. It checks built-in
// labels first, then falls back to custom persona definitions, and finally
// returns the raw persona name as a string.
func Label(persona config.PersonaType, customPersonas []config.CustomPersona) string {
	if persona == config.PersonaNone {
		return "None"
	}
	if label, ok := builtinLabels[persona]; ok {
		return label
	}
	for _, cp := range customPersonas {
		if config.PersonaType(cp.Name) == persona {
			return cp.Label
		}
	}
	return string(persona)
}

// ResolveResult holds the resolved persona information including instruction
// text, approval level, label, and whether the persona was found.
type ResolveResult struct {
	Instructions string
	Approval     config.ApprovalLevel
	Label        string
	Found        bool
}

// Resolve looks up a persona by type, checking built-in personas first then
// custom persona definitions. Returns a ResolveResult with Found=false if the
// persona is not recognized.
func Resolve(persona config.PersonaType, customPersonas []config.CustomPersona) ResolveResult {
	if persona == config.PersonaNone {
		return ResolveResult{Label: "None", Found: true}
	}
	if text, ok := instructionText[persona]; ok {
		return ResolveResult{
			Instructions: text,
			Approval:     defaultApproval[persona],
			Label:        builtinLabels[persona],
			Found:        true,
		}
	}
	for _, cp := range customPersonas {
		if config.PersonaType(cp.Name) == persona {
			return ResolveResult{
				Instructions: cp.Instructions,
				Approval:     cp.AutoApprove,
				Label:        cp.Label,
				Found:        true,
			}
		}
	}
	return ResolveResult{Found: false}
}

// PersonaOption represents a selectable persona entry for display in the TUI.
type PersonaOption struct {
	Name        config.PersonaType
	Label       string
	Description string
	IsCustom    bool
}

// AllPersonaOptions returns the full list of selectable personas: built-in
// options followed by any custom personas from the config.
func AllPersonaOptions(customPersonas []config.CustomPersona) []PersonaOption {
	options := []PersonaOption{
		{Name: config.PersonaNone, Label: "None", Description: "No persona instructions"},
		{Name: config.PersonaVibe, Label: "Vibe", Description: "Move fast, skip tests, auto-approve"},
		{Name: config.PersonaPOC, Label: "POC", Description: "Working demos, basic tests"},
		{Name: config.PersonaScale, Label: "Scale", Description: "TDD, production quality, thorough"},
	}
	for _, cp := range customPersonas {
		options = append(options, PersonaOption{
			Name:        config.PersonaType(cp.Name),
			Label:       cp.Label,
			Description: "(custom)",
			IsCustom:    true,
		})
	}
	return options
}
