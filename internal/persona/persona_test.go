// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
	"testing"

	"github.com/openconductorhq/openconductor/internal/config"
)

func TestInstructionText(t *testing.T) {
	tests := []struct {
		name      string
		persona   config.PersonaType
		wantEmpty bool
	}{
		{"Vibe has text", config.PersonaVibe, false},
		{"POC has text", config.PersonaPOC, false},
		{"Scale has text", config.PersonaScale, false},
		{"None is empty", config.PersonaNone, true},
		{"unknown is empty", config.PersonaType("unknown"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InstructionText(tt.persona)
			if tt.wantEmpty && got != "" {
				t.Errorf("InstructionText(%q) = %q, want empty", tt.persona, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("InstructionText(%q) is empty, want non-empty", tt.persona)
			}
		})
	}
}

func TestTargetFile(t *testing.T) {
	tests := []struct {
		name      string
		agentType config.AgentType
		want      string
	}{
		{"claude-code", config.AgentClaudeCode, "CLAUDE.md"},
		{"opencode", config.AgentOpenCode, "AGENTS.md"},
		{"unknown agent", config.AgentType("unknown"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TargetFile(tt.agentType); got != tt.want {
				t.Errorf("TargetFile(%q) = %q, want %q", tt.agentType, got, tt.want)
			}
		})
	}
}

func TestDefaultApproval(t *testing.T) {
	tests := []struct {
		name    string
		persona config.PersonaType
		want    config.ApprovalLevel
	}{
		{"Vibe is Full", config.PersonaVibe, config.ApprovalFull},
		{"POC is Safe", config.PersonaPOC, config.ApprovalSafe},
		{"Scale is Off", config.PersonaScale, config.ApprovalOff},
		{"None is Off", config.PersonaNone, config.ApprovalOff},
		{"unknown is Off", config.PersonaType("unknown"), config.ApprovalOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultApproval(tt.persona); got != tt.want {
				t.Errorf("DefaultApproval(%q) = %q, want %q", tt.persona, got, tt.want)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	customPersonas := []config.CustomPersona{
		{Name: "security", Label: "Security", Instructions: "focus on security"},
	}

	tests := []struct {
		name    string
		persona config.PersonaType
		custom  []config.CustomPersona
		want    string
	}{
		{"None label", config.PersonaNone, nil, "None"},
		{"Vibe label", config.PersonaVibe, nil, "Vibe"},
		{"POC label", config.PersonaPOC, nil, "POC"},
		{"Scale label", config.PersonaScale, nil, "Scale"},
		{"custom label", config.PersonaType("security"), customPersonas, "Security"},
		{"unknown falls back to name", config.PersonaType("unknown"), nil, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Label(tt.persona, tt.custom); got != tt.want {
				t.Errorf("Label(%q) = %q, want %q", tt.persona, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	customPersonas := []config.CustomPersona{
		{
			Name:         "security",
			Label:        "Security",
			Instructions: "focus on security",
			AutoApprove:  config.ApprovalSafe,
		},
		{
			Name:         "vibe",
			Label:        "Custom Vibe",
			Instructions: "custom vibe text",
			AutoApprove:  config.ApprovalOff,
		},
	}

	tests := []struct {
		name             string
		persona          config.PersonaType
		custom           []config.CustomPersona
		wantFound        bool
		wantLabel        string
		wantInstructions bool // true = non-empty instructions expected
		wantApproval     config.ApprovalLevel
	}{
		{
			name:             "built-in Vibe",
			persona:          config.PersonaVibe,
			custom:           customPersonas,
			wantFound:        true,
			wantLabel:        "Vibe",
			wantInstructions: true,
			wantApproval:     config.ApprovalFull,
		},
		{
			name:             "custom found",
			persona:          config.PersonaType("security"),
			custom:           customPersonas,
			wantFound:        true,
			wantLabel:        "Security",
			wantInstructions: true,
			wantApproval:     config.ApprovalSafe,
		},
		{
			name:             "None found with empty instructions",
			persona:          config.PersonaNone,
			custom:           nil,
			wantFound:        true,
			wantLabel:        "None",
			wantInstructions: false,
			wantApproval:     "",
		},
		{
			name:      "unknown not found",
			persona:   config.PersonaType("nonexistent"),
			custom:    nil,
			wantFound: false,
		},
		{
			name:             "built-in shadows custom with same name",
			persona:          config.PersonaVibe,
			custom:           customPersonas,
			wantFound:        true,
			wantLabel:        "Vibe",
			wantInstructions: true,
			wantApproval:     config.ApprovalFull,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Resolve(tt.persona, tt.custom)
			if result.Found != tt.wantFound {
				t.Fatalf("Resolve(%q).Found = %v, want %v", tt.persona, result.Found, tt.wantFound)
			}
			if !tt.wantFound {
				return
			}
			if result.Label != tt.wantLabel {
				t.Errorf("Resolve(%q).Label = %q, want %q", tt.persona, result.Label, tt.wantLabel)
			}
			if tt.wantInstructions && result.Instructions == "" {
				t.Errorf("Resolve(%q).Instructions is empty, want non-empty", tt.persona)
			}
			if !tt.wantInstructions && result.Instructions != "" {
				t.Errorf("Resolve(%q).Instructions = %q, want empty", tt.persona, result.Instructions)
			}
			if tt.wantApproval != "" && result.Approval != tt.wantApproval {
				t.Errorf("Resolve(%q).Approval = %q, want %q", tt.persona, result.Approval, tt.wantApproval)
			}
		})
	}
}

func TestAllPersonaOptions(t *testing.T) {
	t.Run("built-in only", func(t *testing.T) {
		options := AllPersonaOptions(nil)
		if len(options) != 4 {
			t.Fatalf("AllPersonaOptions(nil) returned %d options, want 4", len(options))
		}
		for _, opt := range options {
			if opt.IsCustom {
				t.Errorf("built-in option %q has IsCustom=true", opt.Name)
			}
		}
	})

	t.Run("with custom personas", func(t *testing.T) {
		customs := []config.CustomPersona{
			{Name: "security", Label: "Security", Instructions: "sec"},
			{Name: "perf", Label: "Performance", Instructions: "perf"},
		}
		options := AllPersonaOptions(customs)
		if len(options) != 6 {
			t.Fatalf("AllPersonaOptions(2 custom) returned %d options, want 6", len(options))
		}

		// First 4 are built-in.
		for i := 0; i < 4; i++ {
			if options[i].IsCustom {
				t.Errorf("options[%d] (%q) has IsCustom=true, want false", i, options[i].Name)
			}
		}

		// Last 2 are custom.
		for i := 4; i < 6; i++ {
			if !options[i].IsCustom {
				t.Errorf("options[%d] (%q) has IsCustom=false, want true", i, options[i].Name)
			}
		}
	})
}
