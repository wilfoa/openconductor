# Agent Personas -- High-Level Design

**Feature**: Allow users to equip a project's agent with a behavioral preset (Vibe, POC, Scale) that writes persona-specific instructions into the project's instruction file.

**Status**: Draft
**Date**: 2026-03-24


## 1. Component Architecture

### 1.1 New Package: `internal/persona`

A new `persona` package owns all persona-related logic: type definitions, instruction text, and the file merge/write engine. This package has zero dependencies on TUI, session, or agent packages -- it depends only on `config` for the `AgentType` and `PersonaType` constants.

**Rationale for a dedicated package over extending `bootstrap/`:**

The `bootstrap` package has a different responsibility profile. It runs once via CLI to scaffold a repo with Go-template-rendered files, skipping files that already exist. It has no concept of partial merge, marker-based replacement, or idempotent updates. The persona writer needs:

- Marker-based insert/replace/remove within existing files
- Atomic writes (temp file + rename)
- Awareness of which file each agent type targets (CLAUDE.md vs AGENTS.md)
- Idempotent re-application when the user changes persona on a running project

Folding this into `bootstrap` would violate single responsibility and require the bootstrap package to handle two fundamentally different file-writing strategies. A separate `persona` package keeps both packages focused and testable in isolation.

The mapping of agent type to target file lives in `persona` rather than on the `AgentAdapter` interface. Adding a method like `PersonaFile() string` to the adapter would be a leaking abstraction -- the adapter knows how to launch and communicate with an agent process, not how to manage instruction files. The persona package can hold a simple map:

```go
var targetFile = map[config.AgentType]string{
    config.AgentClaudeCode: "CLAUDE.md",
    config.AgentOpenCode:   "AGENTS.md",
}
```

If a new agent type is added, a single map entry is all that is needed. This is lower ceremony than implementing an interface method on every adapter, and keeps the persona concern consolidated.

### 1.2 Extended Type: `config.PersonaType`

The `PersonaType` enum lives in `config` alongside `AgentType` and `ApprovalLevel`, following the established pattern. This keeps the `Project` struct self-contained within `config` and avoids an import from `persona` into `config`.

### 1.3 Modified Package: `internal/tui`

- `form.go` gains a new step (`stepPersona`) inserted between `stepAgent` and `stepAutoApprove`.
- `sidebar.go` gains a persona label in the agent line display and a `p` keybinding for persona change.
- `app.go` gains handlers for new messages: `PersonaChangedMsg`, `PersonaWrittenMsg`.
- `messages.go` gains the new message types.
- A new `dialog.go` file encapsulates the confirmation dialog model used by persona change on existing projects.

### 1.4 Modified Package: `internal/bootstrap`

Gains an optional `--persona` flag. The `Bootstrap` function signature expands to accept persona configuration. If a persona is specified, it calls into `persona.WritePersonaSection` after rendering the base template.

### 1.5 Modified Package: `cmd/openconductor`

The `runBootstrap` function parses the new `--persona` flag and passes it to `bootstrap.Bootstrap`.

### 1.6 Dependency Graph

```
cmd/openconductor
    |
    +-- bootstrap  ----> persona ----> config
    |
    +-- tui --------+--> persona ----> config
                    |
                    +--> session
                    +--> agent
                    +--> config
```

The `persona` package is a leaf dependency. It imports only `config` and standard library packages. This keeps the dependency graph acyclic and ensures the persona logic is testable without TUI or agent infrastructure.


## 2. Data Flow

### 2.1 Add Project (TUI Form)

```
User fills form
    |
    v
stepName -> stepRepo -> stepAgent -> stepPersona -> stepAutoApprove
                                        |
                                        | (persona selection suggests
                                        |  default approval level for
                                        |  next step; user can override)
                                        v
                                    stepAutoApprove
                                        |
                                        v
                                    ProjectAddedMsg{Project}
                                        |
                                        v
                                    App.Update():
                                    1. Append project to config
                                    2. Save config
                                    3. tea.Cmd: persona.WritePersonaSection(...)
                                    4. tea.Cmd: startSessionCmd(...)
                                        |
                                        v
                                    PersonaWrittenMsg{ProjectName, Err}
                                        |
                                        v
                                    App.Update(): log result, show error if any
```

The persona write happens as a Bubble Tea command (async, off the main goroutine) to avoid blocking the UI. The session start can proceed in parallel because the agent process takes time to initialize and the file write completes in microseconds. However, to guarantee the instruction file is visible before the agent reads it, the session start command should be sequenced after the persona write completes. This is handled by making `startSessionCmd` a reaction to `PersonaWrittenMsg` rather than batching both commands simultaneously from `ProjectAddedMsg`.

### 2.2 Change Persona on Existing Project (Sidebar `p` Key)

```
User presses 'p' on selected project
    |
    v
sidebarModel enters sidebarPersonaSelect mode
    |
    v
User picks new persona (j/k + Enter)
    |
    v
PersonaChangeRequestMsg{ProjectName, NewPersona}
    |
    v
App.Update():
1. If project has open sessions -> show confirmation dialog:
   "This will update instructions and reset the conversation. Continue? (y/n)"
2. If no open sessions -> proceed directly
    |
    v
(User confirms, or no dialog needed)
    |
    v
App.Update():
1. Update project.Persona in config
2. Save config
3. tea.Cmd: persona.WritePersonaSection(...)
4. If sessions open: stop all sessions, start fresh one
    |
    v
PersonaWrittenMsg -> session restart if needed
```

### 2.3 CLI Bootstrap

```
openconductor bootstrap /path/to/repo --agent claude-code --persona vibe
    |
    v
runBootstrap():
1. Parse --persona flag
2. Call bootstrap.Bootstrap(repoPath, agentType, personaOpt)
    |
    v
bootstrap.Bootstrap():
1. Render base template files (existing behavior)
2. If persona specified:
   persona.WritePersonaSection(repoPath, agentType, personaType)
```

### 2.4 Persona = None (Removal)

```
User changes persona to None
    |
    v
persona.WritePersonaSection(repoPath, agentType, PersonaNone)
    |
    v
persona.writeFile():
1. Read existing file
2. Find markers
3. If markers found: remove markers + content between them
4. If file is now empty (only whitespace): delete file
5. If no file exists: no-op
```


## 3. Interface Changes

### 3.1 New Types in `config`

```go
// PersonaType identifies a behavioral preset for an agent.
type PersonaType string

const (
    PersonaNone  PersonaType = ""      // default: no persona instructions
    PersonaVibe  PersonaType = "vibe"
    PersonaPOC   PersonaType = "poc"
    PersonaScale PersonaType = "scale"
)
```

### 3.2 New Type: `config.CustomPersona`

```go
// CustomPersona defines a user-created persona stored in config.yaml.
type CustomPersona struct {
    Name         string        `yaml:"name"`          // slug identifier
    Label        string        `yaml:"label"`         // display name
    Instructions string        `yaml:"instructions"`  // markdown instruction text
    AutoApprove  ApprovalLevel `yaml:"auto_approve,omitempty"`
}
```

### 3.3 Modified Structs: `config.Project` and `config.Config`

```go
type Project struct {
    Name        string        `yaml:"name"`
    Repo        string        `yaml:"repo"`
    Agent       AgentType     `yaml:"agent"`
    Persona     PersonaType   `yaml:"persona,omitempty"`     // NEW
    AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
}

type Config struct {
    Projects []Project       `yaml:"projects"`
    Personas []CustomPersona `yaml:"personas,omitempty"`  // NEW
    Telegram TelegramConfig  `yaml:"telegram,omitempty"`
}
```

The `omitempty` tags ensure existing configs without persona fields deserialize cleanly. No migration is needed. The `Persona` field on `Project` can reference either a built-in name or a custom persona name.

### 3.4 New Package: `persona`

```go
package persona

// WritePersonaSection writes or updates the persona instruction block
// in the agent's instruction file within the given repo directory.
// Uses HTML comment markers for non-destructive merge.
// When persona is PersonaNone, removes the marker block and deletes
// the file if it becomes empty.
// The customPersonas slice is used to resolve custom persona names.
func WritePersonaSection(repoPath string, agentType config.AgentType, persona config.PersonaType, customPersonas []config.CustomPersona) error

// Resolve looks up a persona by name, checking built-in personas first,
// then custom personas. Returns the instruction text, default approval level,
// display label, and whether the persona was found.
func Resolve(persona config.PersonaType, customPersonas []config.CustomPersona) (instructions string, approval config.ApprovalLevel, label string, found bool)

// InstructionText returns the raw instruction text for a built-in persona.
// Returns "" for PersonaNone or unknown names.
func InstructionText(persona config.PersonaType) string

// TargetFile returns the instruction filename for the given agent type.
// Returns "CLAUDE.md" for Claude Code, "AGENTS.md" for OpenCode.
func TargetFile(agentType config.AgentType) string

// DefaultApproval returns the suggested ApprovalLevel for a built-in persona.
// The user can override this in the form; this is only a default.
func DefaultApproval(persona config.PersonaType) config.ApprovalLevel
```

### 3.4 New Form Step

```go
// form.go additions
type formStep int

const (
    stepName formStep = iota
    stepRepo
    stepAgent
    stepPersona      // NEW: inserted here
    stepAutoApprove
)
```

The step count changes from 4 to 5. The step indicator renders "N/5". Mouse click hit-testing constants shift accordingly.

### 3.5 New Persona Selection Data

```go
type personaOption struct {
    label       string
    description string
    persona     config.PersonaType
}

var personaOptions = []personaOption{
    {label: "None",  description: "No persona instructions",              persona: config.PersonaNone},
    {label: "Vibe",  description: "Move fast, skip tests, auto-approve",  persona: config.PersonaVibe},
    {label: "POC",   description: "Working demos, basic tests",           persona: config.PersonaPOC},
    {label: "Scale", description: "TDD, production quality, thorough",    persona: config.PersonaScale},
}
```

### 3.6 New Message Types in `tui/messages.go`

```go
// PersonaChangeRequestMsg is sent when the user selects a new persona
// for an existing project via the 'p' keybinding.
type PersonaChangeRequestMsg struct {
    ProjectName string
    NewPersona  config.PersonaType
}

// PersonaWrittenMsg signals that the persona instruction file was
// written (or an error occurred).
type PersonaWrittenMsg struct {
    ProjectName string
    Err         error
}
```

### 3.7 New Sidebar Mode

```go
type sidebarMode int

const (
    sidebarNormal sidebarMode = iota
    sidebarForm
    sidebarConfirmDelete
    sidebarPersonaSelect  // NEW: persona picker for existing project
    sidebarConfirmReset   // NEW: confirmation before resetting sessions
)
```

### 3.8 Config Validation Extension

The `config.validate()` method gains a new switch for `Persona`:

```go
switch p.Persona {
case PersonaNone, PersonaVibe, PersonaPOC, PersonaScale:
    // valid
default:
    return fmt.Errorf("project %q: unknown persona %q", p.Name, p.Persona)
}
```

### 3.9 Sidebar Display Change

The agent line in the sidebar currently shows:

```
claude | working
```

With a persona, it becomes:

```
claude | vibe | working
```

The `agentLine` construction in `sidebar.View()` inserts the persona label between the agent display name and the state label, separated by ` | `. When persona is None, the segment is omitted and the display is unchanged.


## 4. File Structure

### 4.1 New Files

| File | Purpose |
|------|---------|
| `internal/persona/persona.go` | Type mapping, instruction text, resolution (built-in + custom), default approval levels |
| `internal/persona/writer.go` | File merge engine: read, find markers, insert/replace/remove, atomic write |
| `internal/persona/setup.go` | Interactive CLI wizard for CRUD of custom personas (same pattern as `telegram/setup.go`) |
| `internal/persona/persona_test.go` | Table-driven tests for instruction text, resolution, and defaults |
| `internal/persona/writer_test.go` | Table-driven tests for all merge scenarios |
| `internal/tui/dialog.go` | Reusable confirmation dialog model (y/n prompt within sidebar) |

### 4.2 Modified Files

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add `PersonaType` constants, `CustomPersona` struct, `Persona` field on `Project`, `Personas` slice on `Config`, extend `validate()` |
| `internal/tui/form.go` | Add `stepPersona`, persona options list (built-in + custom with divider), `personaIndex` field, default approval suggestion |
| `internal/tui/sidebar.go` | Add `sidebarPersonaSelect` and `sidebarConfirmReset` modes, `p` keybinding (change), `P` keybinding (manage), persona label in View |
| `internal/tui/app.go` | Handle `PersonaChangeRequestMsg`, `PersonaWrittenMsg`, `SystemTabExitedMsg` for persona manager; sequence persona write before session start on `ProjectAddedMsg` |
| `internal/tui/messages.go` | Add `PersonaChangeRequestMsg`, `PersonaWrittenMsg` |
| `internal/tui/styles.go` | Add persona-related styles if needed (likely reuses existing `formOptionStyle`, `formSelectedStyle`) |
| `internal/tui/statusbar.go` | Add `p persona  P manage` to sidebar-focused keybind hints |
| `internal/bootstrap/bootstrap.go` | Accept optional persona parameter, call `persona.WritePersonaSection` |
| `cmd/openconductor/main.go` | Add `persona` subcommand, parse `--persona` flag in `runBootstrap` |


## 5. Technology Decisions with Rationale

### 5.1 Marker-Based File Merge vs Template Re-Rendering

**Decision**: HTML comment markers (`<!-- openconductor:persona:start -->` / `<!-- openconductor:persona:end -->`) with a read-modify-write strategy.

**Rationale**: The instruction files (CLAUDE.md, AGENTS.md) are user-editable markdown. Users add their own project-specific instructions. A template-based approach would either overwrite user content or require a complex diff/merge. Markers create a clear contract: OpenConductor owns the content between markers; everything else belongs to the user. This is the same pattern used by tools like Prettier's `// prettier-ignore` and Terraform's managed blocks.

**Rejected alternative**: Separate file (e.g., `.openconductor-persona.md`). This would work but Claude Code and OpenCode read from specific filenames. Adding a separate file would require the user to manually include it, defeating the purpose of automation.

### 5.2 Persona Text as Go Constants vs Embedded Templates

**Decision**: Go string constants in `persona/persona.go`.

**Rationale**: Persona instructions are short (10-20 lines each), fixed text without variable interpolation. Go templates add parsing overhead and indirection for no benefit. Constants are grep-able, type-safe, and trivially testable. If instructions ever need templating (e.g., project name interpolation), the migration from constants to templates is straightforward.

### 5.3 Atomic Writes via Temp File + Rename

**Decision**: Write to a temporary file in the same directory, then `os.Rename` to the target path.

**Rationale**: This prevents partial writes from leaving a corrupted instruction file if the process is interrupted. `os.Rename` is atomic on POSIX systems when source and destination are on the same filesystem. Writing the temp file in the same directory as the target guarantees same-filesystem. This is a standard pattern for reliable file updates.

### 5.4 Persona Type in `config` vs `persona` Package

**Decision**: `PersonaType` and its constants live in `config`.

**Rationale**: The `Project` struct is in `config`. If `PersonaType` lived in `persona`, then `config` would import `persona`, and `persona` would import `config` for `AgentType` -- creating a circular dependency. Keeping all project-level enums in `config` follows the existing pattern (`AgentType`, `ApprovalLevel` are both in `config`).

### 5.5 Persona File Target Mapping in `persona` vs on `AgentAdapter`

**Decision**: Static map in the `persona` package.

**Rationale**: The `AgentAdapter` interface defines agent runtime behavior (launch commands, keystroke sequences, screen parsing). Which file receives persona instructions is a build-time configuration concern, not a runtime behavior. Adding `PersonaFile() string` to the adapter would:

1. Bloat an interface that already has 6 required methods and 8 optional interfaces
2. Force every adapter implementation to carry knowledge about a feature they otherwise have no involvement in
3. Create an import dependency from `persona` to `agent` (or vice versa), complicating the dependency graph

A static map is simpler, more discoverable, and easier to test.

### 5.6 Confirmation Dialog Pattern

**Decision**: New sidebar modes (`sidebarPersonaSelect`, `sidebarConfirmReset`) following the existing `sidebarConfirmDelete` pattern.

**Rationale**: The codebase already has a confirmation dialog for project deletion (`sidebarConfirmDelete`). The persona change flow needs a two-phase interaction: persona selection, then optional confirmation (when sessions are active). Reusing the same mode-based approach keeps the interaction model consistent and familiar. A separate `dialog.go` file extracts the reusable parts without changing the architectural pattern.

### 5.7 Session Reset on Persona Change

**Decision**: Stop all sessions for the project and start a fresh one when persona changes on a project with active sessions.

**Rationale**: The persona instructions are written to a file that the agent reads at startup. Changing the file while an agent is running has no effect -- the agent already loaded its instructions. The user needs a conversation reset to pick up the new persona. This is explicit and predictable, avoiding subtle "why isn't my persona working?" confusion. The confirmation dialog makes this action deliberate.

### 5.8 Default Approval Suggestion (Not Enforcement)

**Decision**: Selecting a persona in the form pre-selects the auto-approve level for the next step, but the user can change it.

**Rationale**: Personas represent opinionated defaults. Vibe = move fast = Full auto-approve. Scale = cautious = Off. But users may have their own preferences. Making it a suggestion (pre-selected index) rather than a locked value preserves user agency while reducing configuration friction.


## 6. Cross-Cutting Concerns

### 6.1 Backward Compatibility

- Existing configs without a `persona` field deserialize to `PersonaNone` via Go's zero-value semantics and `omitempty` YAML tag. No migration script or version bumping needed.
- Existing instruction files without markers are handled by the "append" strategy: persona content is appended with markers. User content is never modified.
- The form step count changes from 4 to 5. This is a UI-only change with no data compatibility impact.

### 6.2 Error Handling

- `persona.WritePersonaSection` returns an error. The TUI handles it in `PersonaWrittenMsg` by logging via `slog` and displaying a transient error in the status bar. The session still starts -- a missing persona instruction is non-fatal.
- File permission errors (read-only repo, disk full) are surfaced to the user but do not block project creation. The project is usable without persona instructions.
- The temp file is cleaned up in a deferred call if the rename fails.

### 6.3 Logging

All persona operations log via the existing `logging` package:

- `logging.Info("persona: wrote section", "project", name, "persona", persona, "file", path)`
- `logging.Info("persona: removed section", "project", name, "file", path)`
- `logging.Debug("persona: file not found, creating", "path", path)`
- `logging.Error("persona: write failed", "err", err, "project", name)`

### 6.4 Testing Strategy

- `persona/writer_test.go`: Table-driven tests covering all 5 merge scenarios (no file, file without markers, file with markers, persona=None with markers, persona=None without file). Uses `os.MkdirTemp` for isolated filesystem tests.
- `persona/persona_test.go`: Tests for `InstructionText` (non-empty for each persona, empty for None), `TargetFile` mapping, `DefaultApproval` mapping.
- `tui/form_test.go`: Tests for step progression with the new persona step, default approval suggestion propagation.
- `config/config_test.go`: Validation tests for valid and invalid persona values.

### 6.5 Concurrency

The persona file write runs as a `tea.Cmd` (goroutine managed by Bubble Tea). There is no shared mutable state: the function takes value parameters (repo path, agent type, persona type) and operates on the filesystem. Concurrent writes to different projects target different directories and are safe. Concurrent writes to the same project are serialized by the TUI's message loop -- only one `PersonaWrittenMsg` can be in flight per project at a time because the second write is triggered by user interaction that cannot happen while the first write's command is pending.

### 6.6 Idempotency

`WritePersonaSection` is idempotent: calling it twice with the same persona produces the same file content. Calling it with a different persona replaces the previous content. This makes the "change persona" flow simple -- just write the new persona, no need to check what was there before.


## 7. Detailed Design: Persona Writer (File Merge Engine)

This is the most architecturally significant new component and benefits from detailed specification here.

### 7.1 Marker Format

```markdown
<!-- openconductor:persona:start -->
[persona instruction text]
<!-- openconductor:persona:end -->
```

Markers are full-line HTML comments. They are valid markdown (rendered as invisible comments) and valid in CLAUDE.md/AGENTS.md syntax. The `openconductor:` prefix namespaces them to avoid collisions with other tools.

### 7.2 Merge Algorithm

```
Input: filePath, personaText (may be empty for removal)

1. Read filePath
   - If error and persona is None: return nil (nothing to do)
   - If error (not exists) and persona is not None: create file with markers + text
   - If error (permission denied): return error

2. Find marker positions
   - Scan lines for start marker -> startLine
   - Scan lines for end marker -> endLine
   - If start found but no end: treat as corrupt, append end marker, log warning

3. If persona is None (removal):
   - If no markers found: return nil (nothing to remove)
   - Remove lines[startLine..endLine] (inclusive)
   - Trim leading/trailing blank lines left by removal
   - If remaining content is only whitespace: delete file, return nil
   - Write remaining content via atomic write

4. If markers found (replacement):
   - Replace lines[startLine..endLine] with markers + new text
   - Write via atomic write

5. If no markers found (append):
   - Append blank line + markers + text to existing content
   - Write via atomic write
```

### 7.3 Atomic Write Implementation

```go
func atomicWrite(path string, content []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".persona-*")
    if err != nil {
        return err
    }
    tmpPath := tmp.Name()
    defer func() {
        // Clean up temp file on any error.
        if tmpPath != "" {
            os.Remove(tmpPath)
        }
    }()

    if _, err := tmp.Write(content); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    if err := os.Chmod(tmpPath, perm); err != nil {
        return err
    }
    if err := os.Rename(tmpPath, path); err != nil {
        return err
    }
    tmpPath = "" // prevent deferred cleanup
    return nil
}
```


## 8. Detailed Design: Form Step Interaction

### 8.1 Persona Step Behavior

When the user advances from `stepAgent` to `stepPersona`:

1. The persona picker renders with j/k navigation (same as agent and approval pickers).
2. Default selection is index 0 ("None") -- no persona by default.
3. On Enter, the form advances to `stepAutoApprove` with the approval index pre-set based on persona:
   - None -> approval index 0 (Off)
   - Vibe -> approval index 2 (Full)
   - POC -> approval index 1 (Safe)
   - Scale -> approval index 0 (Off)
4. The user can still change the approval level. The persona only sets the default.

### 8.2 View Layout

```
New project  4/5

Name   my-project
Repo   /path/to/repo
Agent  claude-code

Persona
  None    No persona instructions
> Vibe    Move fast, skip tests, auto-approve
  POC     Working demos, basic tests
  Scale   TDD, production quality, thorough
  j/k to select, Enter to confirm
  Esc cancel
```

### 8.3 Mouse Click Constants

The content offset for persona options shifts because of the new step. A new constant is needed:

```go
// line 0: "New project"
// line 1: (blank)
// line 2: "Name   ..."
// line 3: "Repo   ..."
// line 4: "Agent  ..."
// line 5: (blank)
// line 6: "Persona"
// line 7+i: persona option i
const formPersonaOptionContentStart = 7
```

The existing `formApprovalOptionContentStart` shifts to account for the additional "Persona ..." done line:

```go
// line 0: "New project"
// line 1: (blank)
// line 2: "Name   ..."
// line 3: "Repo   ..."
// line 4: "Agent  ..."
// line 5: "Persona ..."
// line 6: (blank)
// line 7: "Auto-approve permissions"
// line 8+i: approval option i
const formApprovalOptionContentStart = 8
```


## 9. Detailed Design: Sidebar Persona Change

### 9.1 Interaction Flow

1. User focuses sidebar (`Ctrl+S`) and navigates to a project.
2. User presses `p`.
3. Sidebar enters `sidebarPersonaSelect` mode, showing the persona picker inline (same rendering as the form persona step, but within the sidebar).
4. User selects persona with j/k and confirms with Enter.
5. If the selected persona is the same as current: return to normal mode (no-op).
6. If the project has no open sessions: emit `PersonaChangeRequestMsg`, return to normal mode.
7. If the project has open sessions: transition to `sidebarConfirmReset` mode showing "Change persona to [X]? Sessions will restart. (y/n)".
8. On `y`: emit `PersonaChangeRequestMsg`, return to normal mode.
9. On `n` or Esc: return to normal mode.

### 9.2 App.Update Handling of PersonaChangeRequestMsg

```go
case PersonaChangeRequestMsg:
    // Update config.
    for i := range a.cfg.Projects {
        if a.cfg.Projects[i].Name == msg.ProjectName {
            a.cfg.Projects[i].Persona = msg.NewPersona
            break
        }
    }
    a.sidebar.projects = a.cfg.Projects

    // Save config + write persona file.
    project := a.projectByNameMust(msg.ProjectName)
    cmds = append(cmds, a.saveConfigCmd())
    cmds = append(cmds, a.writePersonaCmd(project))

    // If sessions are open, stop them. A fresh session will be
    // started when PersonaWrittenMsg arrives.
    if a.mgr.HasSessionsForProject(msg.ProjectName) {
        for _, s := range a.mgr.GetSessionsByProject(msg.ProjectName) {
            a.removeTab(s.ID)
            a.mgr.StopSession(s.ID)
            // ... clean up state maps
        }
    }
    return a, tea.Batch(cmds...)
```


## 10. Persona Instruction Content

Each persona provides clear, actionable instructions. The text is short enough to not overwhelm the agent's context but specific enough to shape behavior.

### 10.1 Vibe

```markdown
## OpenConductor Persona: Vibe

Move fast and ship. Optimize for velocity over perfection.

- Make autonomous decisions without asking for confirmation
- Skip writing tests unless explicitly requested
- Prefer quick implementations over architecturally pure ones
- Use simple, direct solutions -- avoid over-engineering
- When in doubt, pick the simpler option and move on
- Commit frequently with short messages
- It is okay to cut corners for speed
```

### 10.2 POC

```markdown
## OpenConductor Persona: POC

Build working demonstrations with reasonable quality.

- Create functional prototypes that demonstrate the concept
- Include basic error handling for common failure modes
- Write tests for critical paths and core logic
- Use pragmatic architecture -- good enough for demos, not overbuilt
- Ask before making major architectural decisions
- Handle the happy path thoroughly; edge cases can be basic
- Include brief inline comments for non-obvious logic
```

### 10.3 Scale

```markdown
## OpenConductor Persona: Scale

Production-grade engineering with comprehensive quality.

- Practice TDD: write tests before implementation
- Comprehensive test coverage including edge cases and error paths
- Follow SOLID principles and established design patterns
- Thorough error handling with meaningful error messages
- Always ask before making decisions with broad impact
- Write clear documentation for public APIs
- Consider performance, security, and maintainability
- Use meaningful commit messages that explain the why
- Refactor proactively when you see code smells
```


## 11. Domains Requiring Low-Level Design

The following areas have sufficient complexity or ambiguity to warrant dedicated LLD documents before implementation:

### 11.1 Form Step Insertion and Auto-Approve Default Propagation

**Why LLD needed**: The form currently has a clean 4-step linear flow. Inserting step 4 (persona) and shifting step 5 (auto-approve) requires careful attention to:

- Mouse click hit-testing Y-coordinate constants for all steps
- Step indicator arithmetic ("N/5" instead of "N/4")
- Keyboard navigation state transitions between steps
- The one-directional data flow from persona selection to approval default (set index on advance, but do not lock it)
- View rendering for the new step and the shifted done-line positions

### 11.2 Sidebar Persona Selection and Confirmation Dialog

**Why LLD needed**: The sidebar has a mode-based state machine (`sidebarNormal`, `sidebarForm`, `sidebarConfirmDelete`). Adding two new modes creates more state transitions to reason about:

- Transition matrix: which modes can transition to which
- Keyboard handling per mode (Esc behavior, which keys are active)
- View rendering for the inline persona picker within the sidebar's constrained width
- Interaction between persona select, confirm reset, and the existing confirm delete modes
- Edge case: what happens if the user presses `p` while in `sidebarConfirmDelete` mode

### 11.3 Persona Writer Edge Cases

**Why LLD needed**: The file merge engine has subtle edge cases that need explicit test case specification:

- File with only a persona section (removing it should delete the file)
- File with markers but empty content between them
- File with start marker but no end marker (corruption recovery)
- File with multiple marker pairs (should only the first pair be processed?)
- File encoding: UTF-8 BOM handling
- Line ending normalization (CRLF vs LF)
- Interaction with git: should `.persona-*` temp files be gitignored?

### 11.4 Session Restart Sequencing

**Why LLD needed**: The persona change on a running project involves stopping sessions and starting a new one after the persona file is written. The sequencing must handle:

- Multiple open sessions for the same project (which one becomes the new active tab?)
- Tab label preservation across restart
- Scrollback buffer handling (clear or preserve?)
- Race condition: what if the user switches away from the project while the restart is in progress?
- Integration with the `stateStickUntil` debounce mechanism

### 11.5 Custom Persona CLI Wizard

**Why LLD needed**: The CRUD wizard follows the Telegram setup pattern but has more interaction complexity:

- Multi-line input for instructions (END sentinel vs editor launch)
- Edit flow pre-filling existing values
- Name uniqueness validation against both built-in and existing custom names
- Config file concurrent access (if multiple OpenConductor instances run)
- Persona deletion when projects reference the deleted persona


## 12. Custom Persona CLI Wizard Architecture

### 12.1 Pattern: Subprocess-in-PTY

Follows the established Telegram setup pattern:

```
Sidebar (Shift+P) → SystemTabRequestMsg{Name: "Persona Manager", Args: ["persona"]}
                                    ↓
                    App spawns: exec.Command(os.Executable(), "persona")
                                    ↓
                    StartSystemSession("Persona Manager", cmd, w, h)
                                    ↓
                    Wizard runs as stdin/stdout CLI in PTY tab
                                    ↓
                    Wizard saves to ~/.openconductor/config.yaml
                                    ↓
                    Process exits → sessionExitedMsg → SystemTabExitedMsg
                                    ↓
                    App reloads: a.cfg.Personas = freshCfg.Personas
```

### 12.2 CLI Entry Point

```go
// cmd/openconductor/main.go
case "persona":
    return persona.RunSetup()
```

### 12.3 Wizard Structure (`persona/setup.go`)

```go
func RunSetup() error {
    // Load config
    cfg := config.LoadOrDefault(config.DefaultConfigPath())

    for {
        // Display menu
        displayMenu(cfg.Personas)

        // Read action
        switch readAction() {
        case 'c':
            p, err := createPersona(cfg.Personas)
            if err == nil {
                cfg.Personas = append(cfg.Personas, p)
                saveConfig(cfg)
            }
        case 'e':
            editPersona(cfg)
            saveConfig(cfg)
        case 'd':
            deletePersona(cfg)
            saveConfig(cfg)
        case 'q':
            return nil
        }
    }
}
```

### 12.4 Persona Resolution

When a project references a persona by name, resolution follows this order:

1. Check built-in personas (`vibe`, `poc`, `scale`)
2. Check custom personas from `cfg.Personas`
3. If not found: treat as "unknown" — log a warning, no instructions written, sidebar shows `???`

This means a custom persona with the same name as a built-in is unreachable (built-in wins). Validation during create prevents this.

### 12.5 Config Reload on Wizard Exit

Extends the existing `SystemTabExitedMsg` handler:

```go
case SystemTabExitedMsg:
    configPath := config.DefaultConfigPath()
    freshCfg := config.LoadOrDefault(configPath)
    a.cfg.Telegram = freshCfg.Telegram
    a.cfg.Personas = freshCfg.Personas  // NEW: reload custom personas
    return a, nil
```


## 13. Implementation Order

The recommended implementation sequence, based on dependency analysis:

1. **`config` types** -- Add `PersonaType`, `CustomPersona`, `Persona` field on `Project`, `Personas` slice on `Config`. Zero external deps. Unblocks everything.
2. **`persona` package core** -- Instruction text, target file mapping, `Resolve()`, `DefaultApproval()`. Depends only on config. Fully testable in isolation.
3. **`persona` writer** -- File merge engine. Depends on persona core. Testable with temp dirs.
4. **Custom persona CLI wizard** -- `persona/setup.go` + CLI entry point. Can be tested standalone.
5. **CLI `--persona` flag** -- Wire into bootstrap. Small change, proves persona package works end-to-end.
6. **`tui/form.go` -- Persona step** -- Add the form step with built-in + custom persona listing.
7. **`tui/app.go` -- ProjectAddedMsg handler** -- Wire persona write into the project add flow.
8. **`tui/sidebar.go` -- Display + keybindings** -- Persona label, `p` change, `P` manage (SystemTabRequestMsg).
9. **`tui/app.go` -- SystemTabExitedMsg** -- Reload custom personas on wizard exit.
10. **`tui/sidebar.go` -- Confirmation + session restart** -- Confirmation dialog and session restart on persona change.

Steps 1-3 in PR 1. Steps 4-5 in PR 2. Steps 6-7 in PR 3. Steps 8-10 in PR 4.
