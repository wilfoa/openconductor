# LLD Architecture Review: Agent Personas

**Reviewer**: Architecture Review
**Date**: 2026-03-24
**Documents reviewed**:
- HLD: `docs/plan/agent-personas-hld.md`
- LLD 1: `docs/plan/lld-persona-package.md` (persona package + config changes)
- LLD 2: `docs/plan/lld-tui-integration.md` (TUI integration)
- User Story: `docs/user-stories/agent-personas.md`
- Codebase ground truth: `internal/config/config.go`, `internal/tui/messages.go`, `internal/tui/form.go`

---

## 1. LLD 1 Review: Persona Package (`lld-persona-package.md`)

**Status: Approved with Minor Revisions**

### 1.1 HLD Alignment

LLD 1 faithfully implements the HLD's component architecture for `internal/persona` and `internal/config` changes. The dependency graph matches HLD section 1.6 -- `persona` depends only on `config` and `logging`, with no imports of `agent`, `tui`, or `session`. The file structure matches HLD section 4.1 (persona.go, writer.go, setup.go, tests).

Key HLD requirements covered:

- PersonaType enum in config (HLD 3.1) -- covered in LLD 1.1
- CustomPersona struct (HLD 3.2) -- covered in LLD 1.3
- Project and Config struct modifications (HLD 3.3) -- covered in LLD 1.4
- WritePersonaSection signature (HLD 3.4) -- covered in LLD 3.3
- Resolve function (HLD 3.4) -- covered in LLD 2.8
- Marker-based merge (HLD 7.1-7.3) -- covered in LLD 3.2-3.11
- Atomic writes (HLD 5.3) -- covered in LLD 3.11
- CLI wizard (HLD 12.1-12.5) -- covered in LLD 4.1-4.10

### 1.2 Strengths

1. **Thorough edge case enumeration.** Section 3.13 lists 20 edge cases for the writer, exceeding the HLD's specification. Cases like CRLF handling (#15), UTF-8 BOM (#16), and multiple marker pairs (#10) demonstrate careful analysis.

2. **Validation design is well-reasoned.** The decision to NOT reject unknown persona references in `validate()` (section 1.5) is sound. This prevents a deleted custom persona from locking a user out of their config. The `ValidatePersonaRef` method provides explicit validation where needed.

3. **BuiltinPersonaNames exported set** (section 1.2) enables the persona setup wizard to check for name collisions without importing a list of constants. This is clean dependency management.

4. **Test coverage is comprehensive.** 14 config tests, 20+ persona tests, 23+ writer tests, and 10 setup tests provide thorough coverage of all specified behavior.

5. **The `ResolveResult` struct** (section 2.8) is a good design choice over the HLD's multi-return-value `Resolve` function. It groups related outputs cleanly and is easier to extend.

### 1.3 Issues Found

#### ISSUE 1: Vibe Default Approval Mismatch with TUI LLD (Severity: High)

**Location**: LLD 1, section 2.6 (`defaultApproval` map)

The persona package defines Vibe's default approval as `ApprovalFull`:

```go
config.PersonaVibe: config.ApprovalFull,
```

The TUI LLD (section 3.2, `defaultApprovalForPersona` map) defines Vibe's default as `ApprovalSafe`:

```go
config.PersonaVibe: config.ApprovalSafe,
```

The HLD (section 8.1) and user story (section 3.1) both specify Vibe should default to `Full`. The TUI LLD has the wrong value.

**Recommendation**: Fix the TUI LLD to use `ApprovalFull` for Vibe. Additionally, the TUI should not maintain its own default approval mapping -- it should call `persona.DefaultApproval()` instead of duplicating the mapping. This eliminates the divergence risk entirely.

#### ISSUE 2: `AllPersonaOptions` Duplicates Data (Severity: Low)

**Location**: LLD 1, section 2.9

The `AllPersonaOptions` function hardcodes built-in persona labels and descriptions that duplicate the information in `builtinLabels` and the `instructionText` map. If these ever diverge, the picker and the resolution logic would show different labels.

**Recommendation**: Build `AllPersonaOptions` from the existing maps rather than hardcoding a second copy of the labels. This is a minor maintainability concern, not a blocker.

#### ISSUE 3: HLD Config Validation Divergence (Severity: Low)

**Location**: LLD 1, section 1.5 vs HLD section 3.8

The HLD specifies that `validate()` should reject unknown persona values with a `switch` statement. LLD 1 intentionally diverges by NOT validating project persona references in `validate()`, adding `ValidatePersonaRef` instead. This is a well-reasoned divergence (explained in section 1.5.1) that improves robustness for the custom persona deletion scenario. The divergence should be documented as a deliberate HLD amendment rather than left as an undocumented deviation.

**Recommendation**: Add a note to the HLD acknowledging this design refinement, or add a comment in the LLD explicitly marking the HLD section 3.8 as superseded.

---

## 2. LLD 2 Review: TUI Integration (`lld-tui-integration.md`)

**Status: Revision Required**

### 2.1 HLD Alignment

LLD 2 covers the TUI changes specified in the HLD (sections 1.3, 2.1-2.2, 3.4-3.7, 8.1-8.3, 9.1). The form step insertion, sidebar modes, message types, and app handlers all map to HLD requirements.

However, there are several type naming and API mismatches that must be resolved before implementation.

### 2.2 Strengths

1. **Detailed state machine transition table** (section 4.4) clearly specifies all sidebar mode transitions and side effects. This eliminates ambiguity about the interaction flow.

2. **Edge case analysis** (section 9) is thorough. The "persona change with no sessions" (9.4), "rapid persona changes" (9.5), and sidebar width constraints (9.3) show careful consideration of real-world usage patterns.

3. **The `StartSession` field on `PersonaWrittenMsg`** (section 9.4) is a clean solution to the disambiguation problem that the HLD left implicit. This is a good design refinement.

4. **Message flow diagrams** (section 8) make the async sequencing clear. The three scenarios (add project, change persona, manage personas) each have explicit step-by-step flows.

5. **Mouse click handling** accounts for divider entries correctly (section 3.12), skipping them during hit testing.

### 2.3 Issues Found

#### ISSUE 4: Type Name Mismatch -- `PersonaName` vs `PersonaType` (Severity: Critical)

**Location**: LLD 2, section 1 (`PersonaName`) vs LLD 1, section 1.1 (`PersonaType`) vs HLD section 3.1 (`PersonaType`)

LLD 2 uses `config.PersonaName` throughout:

```go
type PersonaName string
const PersonaNone PersonaName = ""
```

LLD 1 and the HLD both use `config.PersonaType`:

```go
type PersonaType string
const PersonaNone PersonaType = ""
```

This is not a stylistic difference -- it is a compilation error. The TUI code references `config.PersonaName` which does not exist in the config package as designed by LLD 1.

**Impact**: Every reference to `config.PersonaName` in the TUI LLD must be changed to `config.PersonaType`. This affects:
- `PersonaChangeRequestMsg.NewPersona` field type
- `personaOption.persona` field type
- `defaultApprovalForPersona` map key type
- `sidebarModel.pendingPersona` field type
- `personaDisplayLabel` parameter type
- `buildPersonaOptions` parameter type and return value references

**Recommendation**: Replace all occurrences of `PersonaName` with `PersonaType` in LLD 2 to match LLD 1 and the HLD.

#### ISSUE 5: Custom Persona Struct Mismatch -- `PersonaDef` vs `CustomPersona` (Severity: Critical)

**Location**: LLD 2, section 1 (`PersonaDef`) vs LLD 1, section 1.3 (`CustomPersona`)

LLD 2 defines the custom persona struct as:

```go
type PersonaDef struct {
    Name           PersonaName   `yaml:"name"`
    Description    string        `yaml:"description"`
    DefaultApprove ApprovalLevel `yaml:"default_approve,omitempty"`
    SystemPromptFile string      `yaml:"system_prompt_file,omitempty"`
}
```

LLD 1 (matching the HLD) defines it as:

```go
type CustomPersona struct {
    Name         string        `yaml:"name"`
    Label        string        `yaml:"label"`
    Instructions string        `yaml:"instructions"`
    AutoApprove  ApprovalLevel `yaml:"auto_approve,omitempty"`
}
```

Four differences:

| Aspect | LLD 2 (`PersonaDef`) | LLD 1 (`CustomPersona`) |
|--------|---------------------|------------------------|
| Struct name | `PersonaDef` | `CustomPersona` |
| Display field | `Description string` | `Label string` (display name) |
| Content field | `SystemPromptFile string` (file path) | `Instructions string` (inline markdown) |
| Approval field | `DefaultApprove` | `AutoApprove` |

The `SystemPromptFile` approach in LLD 2 contradicts both LLD 1 and the HLD, which store instructions inline in config.yaml. The `Description` field in LLD 2 serves a different purpose than `Label` in LLD 1 (description text vs. display name).

**Impact**: The TUI's `buildPersonaOptions` function references `p.Description` and `p.Name` from `PersonaDef`, but the actual struct has `Label` and `Instructions`. The function signature accepts `[]config.PersonaDef` but should accept `[]config.CustomPersona`. This cascades through `sidebarModel.customPersonas`, `newSidebarModel`, and `openForm`.

**Recommendation**: Replace all references to `PersonaDef` with `CustomPersona` and update field accesses (`Description` to `Label`, remove `SystemPromptFile` references). Update `buildPersonaOptions` signature and body to use the correct struct.

#### ISSUE 6: `WritePersonaFile` vs `WritePersonaSection` API Mismatch (Severity: High)

**Location**: LLD 2, section 5.6 vs LLD 1, section 3.3

LLD 2 calls the persona writer as:

```go
err := persona.WritePersonaFile(p)
```

Where `p` is a `config.Project`. This signature does not exist in LLD 1.

LLD 1 defines the writer as:

```go
func WritePersonaSection(
    repoPath string,
    agentType config.AgentType,
    persona config.PersonaType,
    customPersonas []config.CustomPersona,
) error
```

The LLD 1 signature takes 4 separate parameters, not a `Project` struct. The caller must also pass `customPersonas`, which the TUI LLD's `writePersonaCmd` does not supply.

**Recommendation**: Update the `writePersonaCmd` in LLD 2 to call `WritePersonaSection` with the correct parameters:

```go
func (a *App) writePersonaCmd(project config.Project, startSession bool) tea.Cmd {
    p := project
    customs := a.cfg.Personas
    return func() tea.Msg {
        err := persona.WritePersonaSection(p.Repo, p.Agent, p.Persona, customs)
        return PersonaWrittenMsg{
            ProjectName:  p.Name,
            Err:          err,
            StartSession: startSession,
        }
    }
}
```

Alternatively, if the simplified `WritePersonaFile(project)` signature is preferred, LLD 1 should add a convenience wrapper. Given that the TUI is the primary caller, a wrapper taking `Project` + `[]CustomPersona` would reduce boilerplate.

#### ISSUE 7: `PersonaWrittenMsg` Struct Divergence (Severity: Medium)

**Location**: LLD 2, section 9.4 vs LLD 2, section 2.1

LLD 2 itself is internally inconsistent. Section 2.1 defines `PersonaWrittenMsg` as:

```go
type PersonaWrittenMsg struct {
    ProjectName string
    Err         error
}
```

Section 9.4 later refines it to include a `StartSession` field:

```go
type PersonaWrittenMsg struct {
    ProjectName  string
    Err          error
    StartSession bool
}
```

The earlier handlers in section 5.3 do not use `StartSession` and always start a session unconditionally. The refined version in section 9.4 corrects this.

**Recommendation**: Consolidate to the section 9.4 version with `StartSession` and update all handler code to use it consistently. Remove the earlier definition to avoid confusion.

#### ISSUE 8: Persona Manager CLI Args Inconsistency (Severity: Low)

**Location**: LLD 2, section 4.5 vs HLD section 12.1-12.2

LLD 2 specifies the system tab args as:

```go
Args: []string{"persona", "manage"}
```

The HLD specifies:

```go
Args: ["persona"]
```

And the HLD's CLI entry point is:

```go
case "persona":
    return persona.RunSetup()
```

The existing Telegram pattern uses `["telegram", "setup"]` with a two-level dispatch. The LLD 2 uses `["persona", "manage"]`, which would require the `"persona"` case in `main.go` to sub-dispatch on `"manage"`. This is workable but inconsistent with the HLD.

**Recommendation**: Align on one pattern. If the persona CLI is just `openconductor persona` (no subcommands), use `Args: []string{"persona"}`. If subcommands are planned (e.g., `persona manage`, `persona apply`), document the dispatch table. The simpler approach (matching the HLD) is preferred unless there is a concrete need for subcommands.

#### ISSUE 9: `PersonaChangeRequestMsg.NewPersona` Type (Severity: Medium)

**Location**: LLD 2, section 2.1 vs HLD section 3.6

LLD 2 uses `config.PersonaName` for the `NewPersona` field. As noted in Issue 4, this must be `config.PersonaType`. Additionally, the HLD section 3.6 matches LLD 1's type.

This is a consequence of Issue 4 but worth calling out separately because message types are the primary API contract between LLDs.

**Recommendation**: Covered by the fix for Issue 4.

#### ISSUE 10: `newFormModel` Signature Change Not Propagated to Tests (Severity: Low)

**Location**: LLD 2, section 3.5

The `newFormModel` signature changes from:

```go
func newFormModel(existingNames []string) (formModel, tea.Cmd)
```

to:

```go
func newFormModel(existingNames []string, customPersonas []config.PersonaDef) (formModel, tea.Cmd)
```

The existing test helper in `form_test.go` (line 24) calls `newFormModel(existingNames)` with one argument. The LLD does not mention updating this call site.

**Recommendation**: Add a note in the testing section that `newFormModel` callers in existing tests must be updated to pass an empty or populated custom personas slice. This is minor but would cause a compilation failure if missed.

#### ISSUE 11: Persona Change on Active Session -- HLD Flow Divergence (Severity: Medium)

**Location**: LLD 2, sections 4.4-4.5 vs HLD section 2.2 vs User Story section 4.2

The HLD specifies a two-step confirmation:
1. Confirmation dialog: "This will update instructions and reset the conversation. Continue? (y/n)"
2. Session restart if confirmed.

The user story specifies an additional choice:
- "Start a fresh conversation with the new persona, or keep the current conversation? (fresh/keep)"

LLD 2 implements a simplified version:
1. `sidebarPersonaSelect` for persona picking
2. `sidebarConfirmReset` for "Sessions will restart. (y/n)"
3. Always restarts sessions on confirmation.

The user story's "fresh/keep" option is not implemented. This simplification is reasonable for MVP but should be explicitly called out as a deliberate scope reduction rather than an oversight. The HLD's confirmation wording ("reset the conversation") aligns with the LLD 2 approach, so the divergence is primarily against the user story.

**Recommendation**: Add a note in LLD 2 acknowledging that the user story's "fresh/keep" choice is deferred. The always-restart behavior matches the HLD's section 5.7 rationale (agents read instructions at startup; a running agent will not pick up changed instructions).

#### ISSUE 12: Missing Persona Removal Handling in TUI (Severity: Medium)

**Location**: LLD 2, section 9.6

When the user changes persona to "None", section 9.6 states:

> "The existing persona file in the repo (if any) is not deleted -- the persona package's WritePersonaFile is responsible for handling the 'none' case."

But the `PersonaChangeRequestMsg` handler in section 5.4 has this logic:

```go
if p.Persona != "" && p.Persona != config.PersonaNone {
    cmds = append(cmds, a.writePersonaCmd(p))
} else {
    cmds = append(cmds, a.startSessionCmd(p, true))
}
```

When persona is `PersonaNone`, the handler skips the write command entirely and starts the session directly. This means the marker block in CLAUDE.md/AGENTS.md is never removed. The HLD section 2.4 explicitly specifies that setting persona to None should remove the marker block.

LLD 1's `WritePersonaSection` handles this correctly when `persona == PersonaNone` -- it removes the markers and deletes the file if empty. But the TUI never calls it for the None case.

**Recommendation**: When persona changes to `PersonaNone`, the TUI should still call `writePersonaCmd` (which calls `WritePersonaSection` with `PersonaNone`), allowing the persona package to remove the marker block as designed. The conditional should be removed or changed to always call `writePersonaCmd` regardless of persona value:

```go
cmds = append(cmds, a.writePersonaCmd(proj, len(sessions) > 0))
```

The same issue exists in the `ProjectAddedMsg` handler (section 5.2) but is less impactful there since a new project with PersonaNone has no markers to remove.

---

## 3. Cross-LLD Consistency Check

### 3.1 Type System Contract

| Type | LLD 1 (persona package) | LLD 2 (TUI) | Match? |
|------|------------------------|-------------|--------|
| Persona identifier type | `config.PersonaType` | `config.PersonaName` | **MISMATCH** (Issue 4) |
| Custom persona struct | `config.CustomPersona` | `config.PersonaDef` | **MISMATCH** (Issue 5) |
| Display field | `CustomPersona.Label` | `PersonaDef.Description` | **MISMATCH** (Issue 5) |
| Content storage | `CustomPersona.Instructions` (inline) | `PersonaDef.SystemPromptFile` (file path) | **MISMATCH** (Issue 5) |
| Approval field | `CustomPersona.AutoApprove` | `PersonaDef.DefaultApprove` | **MISMATCH** (Issue 5) |

### 3.2 Function API Contract

| Function | LLD 1 Signature | LLD 2 Call Site | Match? |
|----------|----------------|-----------------|--------|
| Write persona | `WritePersonaSection(repoPath, agentType, persona, customPersonas)` | `WritePersonaFile(project)` | **MISMATCH** (Issue 6) |
| Default approval | `DefaultApproval(persona PersonaType) ApprovalLevel` | `defaultApprovalForPersona` local map | **DUPLICATE** (Issue 1) |
| All options | `AllPersonaOptions(customPersonas) []PersonaOption` | `buildPersonaOptions(customPersonas) []personaOption` | **PARALLEL** -- see note |

**Note on AllPersonaOptions vs buildPersonaOptions**: LLD 1 provides `AllPersonaOptions` in the persona package for the TUI to consume. LLD 2 defines its own `buildPersonaOptions` in the TUI with a local `personaOption` type. These serve the same purpose but use different types. The `personaOption` in LLD 2 has an `isDivider` field that `PersonaOption` in LLD 1 lacks. The TUI needs the divider concept for rendering but the persona package does not.

**Recommendation**: The TUI should consume `persona.AllPersonaOptions` and map the results to its local `personaOption` type (adding divider entries between built-in and custom groups). This avoids duplicating persona metadata while keeping the rendering-specific `isDivider` field in the TUI layer. Alternatively, if the persona package is not the right place for UI-adjacent concerns, `AllPersonaOptions` can be removed from LLD 1 and the TUI's `buildPersonaOptions` becomes the single source. Either approach works; the current state of having both is redundant.

### 3.3 Message Type Contract

| Message | LLD 1 | LLD 2 | HLD | Match? |
|---------|-------|-------|-----|--------|
| `PersonaChangeRequestMsg` | Not in scope | `{ProjectName string, NewPersona config.PersonaName}` | `{ProjectName string, NewPersona config.PersonaType}` | Type mismatch (Issue 4) |
| `PersonaWrittenMsg` | Not in scope | `{ProjectName string, Err error}` initially, later `{ProjectName string, Err error, StartSession bool}` | `{ProjectName string, Err error}` | LLD 2 internal inconsistency (Issue 7), refinement is good |

### 3.4 Vibe Default Approval

| Source | Vibe Default |
|--------|-------------|
| HLD section 8.1 | Full |
| User story section 3.1 | Full |
| LLD 1 `defaultApproval` map | `ApprovalFull` |
| LLD 2 `defaultApprovalForPersona` map | `ApprovalSafe` |

**Verdict**: LLD 2 is wrong. Must be `ApprovalFull`.

---

## 4. User Story Coverage

### 4.1 Acceptance Criteria Traceability

| AC | Description | LLD 1 | LLD 2 |
|----|-------------|-------|-------|
| 8.1 | 5-step form | -- | Covered (sec 3.1-3.9) |
| 8.2 | Config persistence | Covered (sec 1.4, 1.7) | -- |
| 8.3 | File generation (Claude Code) | Covered (sec 3.3-3.11) | -- |
| 8.4 | File generation (OpenCode) | Covered (sec 2.3) | -- |
| 8.5 | Persona change | -- | Covered (sec 4.4-4.8, 5.4) |
| 8.6 | Sidebar display | -- | Covered (sec 4.7) |
| 8.7 | CLI bootstrap | Partially (sec 8.6 mentions it) | -- |
| 8.8 | Custom persona CRUD | Covered (sec 4.3-4.10) | Covered (sec 4.5, 5.5) |
| 8.9 | Error handling | Covered (sec 6.1) | Covered (sec 5.3) |
| 8.9 (dup) | Backward compatibility | Covered (sec 1.1, 1.5) | Covered (sec 1) |
| 8.10 | Tests | Covered (sec 5.1-5.4) | Covered (sec 11.1-11.3) |

### 4.2 Gaps

1. **AC 8.5 partial gap**: The user story specifies "a confirmation dialog asks: 'This will update CLAUDE.md/AGENTS.md in the repo -- continue?'" as a separate step before checking if sessions are active. LLD 2 combines persona selection and session restart confirmation into one flow, skipping the file-update confirmation for projects without open sessions. This is a reasonable simplification but diverges from the user story.

2. **AC 8.7 CLI bootstrap**: Neither LLD provides detailed design for the `--persona` flag on the bootstrap command. LLD 1 section 8.6 mentions it briefly. The HLD sections 1.4-1.5 and 2.3 describe the flow. This may warrant a small section in LLD 1 or a separate mini-LLD, though it is simple enough to implement from the HLD alone.

3. **AC 8.8 custom persona display**: The user story specifies "Custom personas appear below built-in ones in the form persona step, separated by a divider." LLD 2 implements this with the `isDivider` field. LLD 1's `AllPersonaOptions` does not include divider support. The TUI's local `buildPersonaOptions` handles it correctly. This is covered but the two LLDs should agree on which layer provides this.

---

## 5. Summary of Issues

| # | Severity | LLD | Description | Section |
|---|----------|-----|-------------|---------|
| 1 | High | Both | Vibe default approval: LLD 1 says Full, LLD 2 says Safe | LLD 1 sec 2.6, LLD 2 sec 3.2 |
| 2 | Low | LLD 1 | `AllPersonaOptions` duplicates data from other maps | LLD 1 sec 2.9 |
| 3 | Low | LLD 1 | HLD validation divergence undocumented | LLD 1 sec 1.5 |
| 4 | Critical | LLD 2 | `PersonaName` type does not exist; should be `PersonaType` | LLD 2 throughout |
| 5 | Critical | LLD 2 | `PersonaDef` struct does not exist; should be `CustomPersona` with correct fields | LLD 2 throughout |
| 6 | High | LLD 2 | `WritePersonaFile(project)` does not exist; should be `WritePersonaSection(repoPath, agentType, persona, customPersonas)` | LLD 2 sec 5.6 |
| 7 | Medium | LLD 2 | `PersonaWrittenMsg` defined twice with different fields | LLD 2 sec 2.1 vs 9.4 |
| 8 | Low | LLD 2 | CLI args `["persona", "manage"]` vs HLD `["persona"]` | LLD 2 sec 4.5 |
| 9 | Medium | LLD 2 | `PersonaChangeRequestMsg.NewPersona` uses wrong type | LLD 2 sec 2.1 (consequence of Issue 4) |
| 10 | Low | LLD 2 | Test helpers not updated for `newFormModel` signature change | LLD 2 sec 3.5 |
| 11 | Medium | LLD 2 | "fresh/keep" choice from user story not addressed | LLD 2 sec 4.4 |
| 12 | Medium | LLD 2 | PersonaNone skips writer call, leaving stale markers in repo | LLD 2 sec 5.4, 9.6 |

---

## 6. Recommended Fixes

### Critical (must fix before implementation)

1. **Issue 4 + 5 + 9**: Perform a global find-and-replace in LLD 2:
   - `PersonaName` -> `PersonaType`
   - `PersonaDef` -> `CustomPersona`
   - `p.Description` -> `p.Label` (in `buildPersonaOptions`)
   - `p.DefaultApprove` -> `p.AutoApprove`
   - Remove `SystemPromptFile` reference
   - Update `Config.Personas` type from `[]PersonaDef` to `[]CustomPersona`

2. **Issue 6**: Rewrite `writePersonaCmd` to call `persona.WritePersonaSection(p.Repo, p.Agent, p.Persona, customPersonas)` with the 4-parameter signature from LLD 1.

### High (must fix before implementation)

3. **Issue 1**: Change `defaultApprovalForPersona` in LLD 2 to use `ApprovalFull` for Vibe. Better yet, remove the local map entirely and call `persona.DefaultApproval()` from LLD 1.

### Medium (should fix, can be addressed during implementation)

4. **Issue 7**: Consolidate `PersonaWrittenMsg` to the `StartSession` version from section 9.4. Update all handlers to use it.

5. **Issue 12**: Remove the conditional that skips `writePersonaCmd` when persona is None. Always call the writer so that `WritePersonaSection` can remove stale markers.

6. **Issue 11**: Add a note explaining that the "fresh/keep" choice is deferred to a future iteration.

### Low (can be tracked as follow-ups)

7. **Issue 2**: Refactor `AllPersonaOptions` to derive from existing maps.
8. **Issue 3**: Document the validation divergence.
9. **Issue 8**: Align CLI args with HLD.
10. **Issue 10**: Note test helper updates.

---

## 7. Overall Verdict

**Can we proceed to test planning?** Not yet.

Issues 4, 5, and 6 are compilation-level blockers -- the TUI LLD references types and functions that do not exist in the persona package LLD. These must be corrected first. The fixes are mechanical (type renames and function signature alignment) and should not require architectural rethinking.

**After the critical and high issues are resolved**: Yes, proceed to test planning. The architectural foundation is sound. Both LLDs demonstrate thorough analysis, proper separation of concerns, and appropriate handling of edge cases. The core design -- marker-based file merge, async persona writes via Bubble Tea commands, sidebar mode-based state machine -- is well-aligned with the existing codebase patterns and the HLD's architectural vision.

The LLD 1 (persona package) is essentially ready. The LLD 2 (TUI integration) needs a revision pass to align its type contracts with LLD 1, fix the Vibe approval default, and handle the PersonaNone removal case. Estimated revision effort: 1-2 hours of document editing, no design rethinking required.
