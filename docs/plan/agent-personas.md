# Implementation Plan: Agent Personas

**Feature**: Equip agents with behavioral presets (Vibe, POC, Scale, Custom) that write persona-specific instructions into project repos.

**Date**: 2026-03-24
**Branch**: `feat/agent-persona-presets`

---

## Document Index

| Document | Purpose |
|----------|---------|
| [User Story](../user-stories/agent-personas.md) | Problem statement, user flows, acceptance criteria |
| [High-Level Design](agent-personas-hld.md) | Component architecture, data flow, technology decisions |
| [LLD: Persona Package](lld-persona-package.md) | Config types, persona logic, file merge engine, CLI wizard |
| [LLD: TUI Integration](lld-tui-integration.md) | Form step, sidebar modes, message handling, app wiring |
| [LLD Review](lld-review.md) | Architect review of both LLDs against HLD |
| [Test Plan](test-plan-personas.md) | 149 tests: unit, integration, edge cases, coverage matrix |

---

## Feature Summary

### Personas

| Name | Philosophy | Default Auto-Approve | Target File |
|------|-----------|---------------------|-------------|
| None | No instructions (default) | - | - |
| Vibe | Move fast, skip tests, autonomous | Full | CLAUDE.md / AGENTS.md |
| POC | Working demos, basic quality | Safe | CLAUDE.md / AGENTS.md |
| Scale | TDD, production quality, thorough | Off | CLAUDE.md / AGENTS.md |
| Custom | User-defined via CLI wizard | User-specified | CLAUDE.md / AGENTS.md |

### Key Interactions

- **Add project**: 5-step form (Name → Repo → Agent → Persona → Auto-approve)
- **Change persona**: `p` in sidebar → pick persona → confirm → file rewrite → session restart prompt
- **Manage custom personas**: `P` in sidebar → opens CLI wizard as system tab
- **CLI bootstrap**: `openconductor bootstrap <path> --persona <name>`

### File Strategy

Persona instructions are written into CLAUDE.md (Claude Code) or AGENTS.md (OpenCode) using HTML comment markers for non-destructive merge:

```markdown
<!-- openconductor:persona:start -->
<!-- This section is managed by OpenConductor. Manual edits will be overwritten. -->
## OpenConductor Persona: Vibe
...
<!-- openconductor:persona:end -->
```

---

## Architecture

### New Package: `internal/persona`

```
internal/persona/
  persona.go       # InstructionText, Resolve, TargetFile, DefaultApproval
  writer.go        # WritePersonaSection, atomicWrite, marker merge engine
  setup.go         # RunSetup() — interactive CLI wizard for custom persona CRUD
  persona_test.go  # 22 tests
  writer_test.go   # 31 tests
```

### Config Changes: `internal/config`

```go
type PersonaType string  // "", "vibe", "poc", "scale", or custom slug

type CustomPersona struct {
    Name         string        `yaml:"name"`
    Label        string        `yaml:"label"`
    Instructions string        `yaml:"instructions"`
    AutoApprove  ApprovalLevel `yaml:"auto_approve,omitempty"`
}

// Project gains: Persona PersonaType `yaml:"persona,omitempty"`
// Config gains:  Personas []CustomPersona `yaml:"personas,omitempty"`
```

### TUI Changes: `internal/tui`

- `form.go` — New `stepPersona` (step 4/5) with built-in + custom options
- `sidebar.go` — Persona label display, `p` change, `P` manage, 2 new modes
- `app.go` — PersonaWrittenMsg/PersonaChangeRequestMsg handlers, sequenced session start
- `messages.go` — 2 new message types

### Dependency Graph

```
cmd/openconductor → persona → config
                  → tui → persona → config
                        → session
                        → agent
```

---

## Implementation Order

### PR 1: Config + Persona Package Core

| Step | File | Work |
|------|------|------|
| 1 | `internal/config/config.go` | Add PersonaType, CustomPersona, Project.Persona, Config.Personas, validation |
| 2 | `internal/config/config_test.go` | 21 tests: type validation, round-trip, backward compat |
| 3 | `internal/persona/persona.go` | InstructionText, Resolve, TargetFile, DefaultApproval, instruction constants |
| 4 | `internal/persona/persona_test.go` | 22 tests: text, resolution, mapping, defaults |
| 5 | `internal/persona/writer.go` | WritePersonaSection, atomicWrite, marker merge engine |
| 6 | `internal/persona/writer_test.go` | 31 tests: all merge scenarios, atomic write, corruption recovery |

### PR 2: Custom Persona CLI + Bootstrap Flag

| Step | File | Work |
|------|------|------|
| 7 | `internal/persona/setup.go` | RunSetup() CLI wizard: menu, create, edit, delete |
| 8 | `cmd/openconductor/main.go` | `persona` subcommand entry point |
| 9 | `cmd/openconductor/main.go` | `--persona` flag on bootstrap command |
| 10 | `internal/bootstrap/bootstrap.go` | Accept persona parameter, call WritePersonaSection |

### PR 3: TUI Form + Project Add Flow

| Step | File | Work |
|------|------|------|
| 11 | `internal/tui/form.go` | stepPersona, personaOptions (built-in + custom), default approval suggestion |
| 12 | `internal/tui/messages.go` | PersonaWrittenMsg type |
| 13 | `internal/tui/app.go` | Modified ProjectAddedMsg handler: persona write → session start sequencing |

### PR 4: Sidebar + Persona Change + System Tab

| Step | File | Work |
|------|------|------|
| 14 | `internal/tui/sidebar.go` | Persona label in project display |
| 15 | `internal/tui/statusbar.go` | `p persona  P manage` hints |
| 16 | `internal/tui/messages.go` | PersonaChangeRequestMsg type |
| 17 | `internal/tui/sidebar.go` | sidebarPersonaSelect mode, sidebarConfirmReset mode, `p` and `P` keybindings |
| 18 | `internal/tui/app.go` | PersonaChangeRequestMsg handler, PersonaWrittenMsg for change flow |
| 19 | `internal/tui/app.go` | SystemTabExitedMsg extension: reload cfg.Personas |

---

## Test Coverage Summary

| Category | Count | Location |
|----------|-------|----------|
| Config unit tests | 21 | `internal/config/config_test.go` |
| Persona core tests | 22 | `internal/persona/persona_test.go` |
| Writer tests | 31 | `internal/persona/writer_test.go` |
| Setup wizard tests | 9 | `internal/persona/setup_test.go` |
| Form tests | 21 | `internal/tui/form_test.go` |
| Sidebar tests | 17 | `internal/tui/sidebar_test.go` |
| App tests | 8 | `internal/tui/app_test.go` |
| Integration tests | 7 | `internal/persona/integration_test.go` |
| Edge case tests | 13 | Across packages |
| **Total** | **149** | |

Full test specifications with table-driven cases: [Test Plan](test-plan-personas.md)

---

## Acceptance Criteria Coverage

All 10 acceptance criteria groups from the user story (sections 8.1-8.10) are mapped to specific test cases in the test plan's coverage matrix (section 7). Key mappings:

- **8.1 Add-Project Form** → F01-F21 (21 form tests)
- **8.2 Config Persistence** → C01-C21 (21 config tests)
- **8.3 File Generation (Claude Code)** → W01-W31 (31 writer tests)
- **8.4 File Generation (OpenCode)** → W01-W31 (same writer, different target file)
- **8.5 Persona Change** → SB01-SB17, A01-A08
- **8.6 Sidebar Display** → SB14-SB17
- **8.7 CLI Bootstrap** → I06-I07
- **8.8 Custom Persona CRUD** → S01-S09, A07-A08
- **8.9 Error Handling** → E01-E14 (14 edge case tests)
- **8.10 Backward Compatibility** → C16-C17, E07

---

## Resolved Design Decisions

1. Persona selection is **optional** (None is default)
2. Changing persona **requires confirmation** dialog
3. **No auto-suggestion** based on repo characteristics
4. Persona change **prompts about conversation reset** (fresh vs keep)
5. Naming: **"Persona"** used consistently
6. Custom personas managed via **CLI wizard in system tab** (same pattern as Telegram setup)
7. Persona instructions as **Go string constants** (not templates)
8. Agent-to-file mapping as **static map in persona package** (not on AgentAdapter)
9. **Marker-based merge** for non-destructive file editing
10. **Atomic writes** via temp file + rename

---

## Open Items

None — all design decisions are resolved. Ready for implementation.
