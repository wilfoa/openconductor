# Low-Level Design: `internal/persona` Package and `internal/config` Changes

**Feature**: Agent Personas -- behavioral presets writing instruction files into project repos
**Status**: Draft
**Date**: 2026-03-24
**Depends on**: HLD (`docs/plan/agent-personas-hld.md`)
**Scope**: `internal/config/config.go` changes, `internal/persona/persona.go`, `internal/persona/writer.go`, `internal/persona/setup.go`, and all associated tests


## 1. Config Changes (`internal/config/config.go`)

### 1.1 New Type: `PersonaType`

Follows the existing `AgentType` and `ApprovalLevel` pattern -- a named `string` type with exported constants.

```go
// PersonaType identifies a behavioral preset that writes agent-specific
// instructions into the project's instruction file. Built-in personas
// use well-known slugs; custom personas reference names from Config.Personas.
type PersonaType string

const (
    // PersonaNone is the zero value -- no persona instructions are written.
    PersonaNone PersonaType = ""
    // PersonaVibe optimises for velocity: skip tests, auto-approve, move fast.
    PersonaVibe PersonaType = "vibe"
    // PersonaPOC builds working prototypes with reasonable quality.
    PersonaPOC PersonaType = "poc"
    // PersonaScale targets production-grade engineering with TDD and thorough review.
    PersonaScale PersonaType = "scale"
)
```

**Placement**: Immediately after the `ApprovalLevel` constants block (after line 34 in the current file), maintaining alphabetical grouping of project-level enums.

**Design note**: The zero value `""` maps to `PersonaNone` by Go convention. This ensures existing YAML configs without a `persona` field deserialise correctly with no migration.

### 1.2 Built-in Persona Names Set

A package-level set for programmatic lookup during validation and in the persona package.

```go
// BuiltinPersonaNames is the set of built-in persona slugs. Exported so
// the persona package can check for name collisions during custom persona
// creation without importing a list of constants.
var BuiltinPersonaNames = map[PersonaType]bool{
    PersonaVibe:  true,
    PersonaPOC:   true,
    PersonaScale: true,
}
```

**Rationale**: A map is cheaper than a switch for external callers that need "is this name built-in?" checks. The persona setup wizard needs this to prevent custom names that shadow built-ins.

### 1.3 New Type: `CustomPersona`

```go
// CustomPersona defines a user-created persona stored in the top-level
// config under the "personas" key. Custom personas are referenced by
// name in Project.Persona, the same way built-in personas are referenced
// by their slug constants.
type CustomPersona struct {
    // Name is a slug identifier used in Project.Persona. Must be lowercase
    // alphanumeric with hyphens, matching the regex ^[a-z][a-z0-9-]*$.
    // Must not collide with built-in persona names.
    Name string `yaml:"name"`

    // Label is a human-readable display name shown in the TUI sidebar
    // and form picker. Example: "Backend Expert".
    Label string `yaml:"label"`

    // Instructions is the markdown text injected into the agent's
    // instruction file between the persona markers.
    Instructions string `yaml:"instructions"`

    // AutoApprove is the suggested default approval level when this
    // persona is selected. The user can override in the form.
    AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
}
```

### 1.4 Modified Structs

**`Project`** -- add `Persona` field after `Agent`, before `AutoApprove`:

```go
type Project struct {
    Name        string        `yaml:"name"`
    Repo        string        `yaml:"repo"`
    Agent       AgentType     `yaml:"agent"`
    Persona     PersonaType   `yaml:"persona,omitempty"`
    AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
}
```

**`Config`** -- add `Personas` slice after `Projects`:

```go
type Config struct {
    Projects      []Project          `yaml:"projects"`
    Personas      []CustomPersona    `yaml:"personas,omitempty"`
    LLM           LLMConfig          `yaml:"llm"`
    Notifications NotificationConfig `yaml:"notifications"`
    Telegram      TelegramConfig     `yaml:"telegram"`
}
```

**Field ordering rationale**: `Personas` is placed immediately after `Projects` because they are semantically related -- personas are referenced by projects. This keeps them adjacent in the YAML file for discoverability.

### 1.5 Validation Changes

The `validate()` method gains persona validation. The core design constraint from the HLD: **do not reject unknown persona names on load**. A user who deletes a custom persona from the `personas:` list should not be locked out of their config because an old project still references that name. Instead, unknown persona names are only flagged during explicit validation (e.g., the form wizard or CLI).

#### 1.5.1 Changes to `validate()` (called on `Load`)

The existing `validate()` method adds validation for the `Personas` slice itself (structural integrity) but intentionally does **not** validate `Project.Persona` references against available personas.

```go
func (c *Config) validate() error {
    // ... existing project validation ...

    // Validate custom persona definitions.
    personaNames := make(map[string]bool)
    for i, cp := range c.Personas {
        if cp.Name == "" {
            return fmt.Errorf("persona %d: missing name", i)
        }
        if personaNames[cp.Name] {
            return fmt.Errorf("persona %q: duplicate name", cp.Name)
        }
        personaNames[cp.Name] = true
        if cp.Label == "" {
            return fmt.Errorf("persona %q: missing label", cp.Name)
        }
        if cp.Instructions == "" {
            return fmt.Errorf("persona %q: missing instructions", cp.Name)
        }
        switch cp.AutoApprove {
        case ApprovalOff, ApprovalSafe, ApprovalFull, "":
            // valid
        default:
            return fmt.Errorf("persona %q: unknown auto_approve level %q",
                cp.Name, cp.AutoApprove)
        }
    }

    return nil
}
```

#### 1.5.2 New Method: `ValidatePersonaRef` (explicit validation)

```go
// ValidatePersonaRef checks whether a persona reference is valid against
// the built-in names and the config's custom personas. Returns an error
// describing the problem, or nil if valid.
//
// This is intentionally NOT called from validate() / Load(). It is used
// by the TUI form and CLI when the user explicitly sets a persona, so
// that a deleted custom persona does not prevent config loading.
func (c *Config) ValidatePersonaRef(persona PersonaType) error {
    if persona == PersonaNone {
        return nil
    }
    if BuiltinPersonaNames[persona] {
        return nil
    }
    for _, cp := range c.Personas {
        if PersonaType(cp.Name) == persona {
            return nil
        }
    }
    return fmt.Errorf("unknown persona %q", persona)
}
```

### 1.6 Complete Diff Summary for `config.go`

| Line Range (current) | Change |
|---|---|
| After line 34 | Insert `PersonaType` type, constants, and `BuiltinPersonaNames` |
| After line 34 (new block) | Insert `CustomPersona` struct |
| Lines 36-41 (`Project`) | Add `Persona PersonaType` field between `Agent` and `AutoApprove` |
| Lines 62-67 (`Config`) | Add `Personas []CustomPersona` field after `Projects` |
| Lines 99-121 (`validate`) | Append custom persona slice validation; do NOT add project persona ref check |
| After `validate()` | Insert `ValidatePersonaRef` method |

### 1.7 YAML Serialisation Examples

**Minimal (no persona)**:
```yaml
projects:
  - name: myproj
    repo: /home/user/myproj
    agent: claude-code
```

**With built-in persona**:
```yaml
projects:
  - name: myproj
    repo: /home/user/myproj
    agent: claude-code
    persona: vibe
    auto_approve: full
```

**With custom persona**:
```yaml
personas:
  - name: backend-expert
    label: Backend Expert
    instructions: |
      ## Backend Expert
      Focus on server-side architecture.
      - Design RESTful APIs
      - Use connection pooling
      - Write integration tests
    auto_approve: safe

projects:
  - name: api-service
    repo: /home/user/api-service
    agent: claude-code
    persona: backend-expert
    auto_approve: safe
```

**Orphaned reference (deleted custom persona -- config still loads)**:
```yaml
# The "backend-expert" persona was deleted from the personas list,
# but api-service still references it. Config loads without error.
# The persona is treated as "unknown" at runtime (no instructions written).
projects:
  - name: api-service
    repo: /home/user/api-service
    agent: claude-code
    persona: backend-expert
```


## 2. Persona Package: `internal/persona/persona.go`

### 2.1 File Header

```go
// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package persona manages behavioral presets that write instruction text
// into agent-specific files (CLAUDE.md, AGENTS.md) in project repos.
package persona
```

### 2.2 Imports

```go
import (
    "github.com/openconductorhq/openconductor/internal/config"
)
```

No standard library imports needed in this file -- only the `config` package for type references.

### 2.3 Target File Mapping

```go
// targetFile maps agent types to the instruction file each agent reads.
// Claude Code reads CLAUDE.md; OpenCode reads AGENTS.md.
var targetFile = map[config.AgentType]string{
    config.AgentClaudeCode: "CLAUDE.md",
    config.AgentOpenCode:   "AGENTS.md",
}

// TargetFile returns the instruction filename for the given agent type.
// Returns an empty string for unknown agent types.
func TargetFile(agentType config.AgentType) string {
    return targetFile[agentType]
}
```

### 2.4 Built-in Instruction Text

```go
// instructionText maps built-in persona types to their markdown instruction
// blocks. These are Go constants, not templates -- they contain no variable
// interpolation. If interpolation is ever needed, the migration to
// text/template is straightforward.
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
```

### 2.5 `InstructionText` Function

```go
// InstructionText returns the markdown instruction text for a built-in
// persona. Returns an empty string for PersonaNone or unknown persona types.
func InstructionText(persona config.PersonaType) string {
    return instructionText[persona]
}
```

### 2.6 Default Approval Mapping

```go
// defaultApproval maps built-in personas to their suggested approval levels.
// These are defaults only -- the user can override in the add-project form.
var defaultApproval = map[config.PersonaType]config.ApprovalLevel{
    config.PersonaNone:  config.ApprovalOff,
    config.PersonaVibe:  config.ApprovalFull,
    config.PersonaPOC:   config.ApprovalSafe,
    config.PersonaScale: config.ApprovalOff,
}

// DefaultApproval returns the suggested ApprovalLevel for a built-in persona.
// Returns ApprovalOff for PersonaNone, unknown, or custom personas.
func DefaultApproval(persona config.PersonaType) config.ApprovalLevel {
    if level, ok := defaultApproval[persona]; ok {
        return level
    }
    return config.ApprovalOff
}
```

### 2.7 Built-in Labels

```go
// builtinLabels maps built-in persona types to their display labels.
var builtinLabels = map[config.PersonaType]string{
    config.PersonaVibe:  "Vibe",
    config.PersonaPOC:   "POC",
    config.PersonaScale: "Scale",
}

// Label returns the display label for a persona. For built-in personas,
// returns the canonical label. For custom personas found in the provided
// slice, returns CustomPersona.Label. Returns the raw persona name string
// if not found in either.
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
    // Unknown persona -- return the raw slug.
    return string(persona)
}
```

### 2.8 `Resolve` Function

```go
// ResolveResult holds the result of resolving a persona by name.
type ResolveResult struct {
    Instructions string
    Approval     config.ApprovalLevel
    Label        string
    Found        bool
}

// Resolve looks up a persona by name. Built-in personas are checked first,
// then custom personas from the config. This ordering means a custom persona
// cannot shadow a built-in name (and the setup wizard prevents creating one).
//
// Returns a ResolveResult. If Found is false, the persona name is unknown
// and no instructions should be written.
func Resolve(persona config.PersonaType, customPersonas []config.CustomPersona) ResolveResult {
    if persona == config.PersonaNone {
        return ResolveResult{
            Label: "None",
            Found: true,
        }
    }

    // Check built-in personas.
    if text, ok := instructionText[persona]; ok {
        return ResolveResult{
            Instructions: text,
            Approval:     defaultApproval[persona],
            Label:        builtinLabels[persona],
            Found:        true,
        }
    }

    // Check custom personas.
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
```

### 2.9 `AllPersonaOptions` Function

Utility for the TUI form and sidebar persona picker to build the full list of selectable personas.

```go
// PersonaOption represents a selectable persona in the TUI.
type PersonaOption struct {
    Name        config.PersonaType
    Label       string
    Description string
    IsCustom    bool
}

// AllPersonaOptions returns the list of persona options for the TUI picker.
// Built-in personas are listed first (None, Vibe, POC, Scale), followed by
// any custom personas from the config. The order is stable.
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
```


## 3. Persona Package: `internal/persona/writer.go`

### 3.1 File Header and Imports

```go
// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/openconductorhq/openconductor/internal/config"
    "github.com/openconductorhq/openconductor/internal/logging"
)
```

### 3.2 Marker Constants

```go
const (
    markerStart = "<!-- openconductor:persona:start -->"
    markerEnd   = "<!-- openconductor:persona:end -->"
    managedComment = "<!-- This section is managed by OpenConductor. Manual edits will be overwritten. -->"
)
```

### 3.3 `WritePersonaSection` -- Public Entry Point

```go
// WritePersonaSection writes, updates, or removes the persona instruction
// block in the agent's instruction file within the given repo directory.
//
// The function uses HTML comment markers to delineate the managed section.
// Content outside the markers is never modified.
//
// Behavior matrix:
//   - No file + persona set    -> create file with markers + text
//   - File without markers     -> append markers + text
//   - File with markers        -> replace content between markers
//   - File with markers + None -> remove markers + content; delete if empty
//   - No file + None           -> no-op
//
// The customPersonas slice is needed to resolve custom persona names to
// their instruction text.
func WritePersonaSection(
    repoPath string,
    agentType config.AgentType,
    persona config.PersonaType,
    customPersonas []config.CustomPersona,
) error {
    filename := TargetFile(agentType)
    if filename == "" {
        return fmt.Errorf("no target file for agent type %q", agentType)
    }
    filePath := filepath.Join(repoPath, filename)

    // Resolve persona to instruction text.
    var personaText string
    if persona != config.PersonaNone {
        result := Resolve(persona, customPersonas)
        if !result.Found {
            logging.Info("persona: unknown persona, skipping write",
                "persona", string(persona),
                "file", filePath,
            )
            return fmt.Errorf("unknown persona %q", persona)
        }
        personaText = result.Instructions
    }

    return writeFile(filePath, personaText)
}
```

### 3.4 `writeFile` -- Core Merge Logic

```go
// writeFile implements the read-modify-write merge algorithm.
// When personaText is empty, the managed section is removed.
func writeFile(filePath string, personaText string) error {
    existing, err := os.ReadFile(filePath)
    if err != nil {
        if os.IsNotExist(err) {
            return handleNoFile(filePath, personaText)
        }
        return fmt.Errorf("reading %s: %w", filePath, err)
    }

    lines := strings.Split(string(existing), "\n")
    startIdx, endIdx := findMarkers(lines)

    if personaText == "" {
        return handleRemoval(filePath, lines, startIdx, endIdx)
    }

    if startIdx >= 0 {
        return handleReplace(filePath, lines, startIdx, endIdx, personaText)
    }

    return handleAppend(filePath, string(existing), personaText)
}
```

### 3.5 `handleNoFile` -- File Does Not Exist

```go
// handleNoFile handles the case where the instruction file does not exist.
// If personaText is non-empty, creates the file with markers.
// If personaText is empty (PersonaNone), this is a no-op.
func handleNoFile(filePath string, personaText string) error {
    if personaText == "" {
        // PersonaNone + no file = nothing to do.
        return nil
    }

    content := buildMarkedSection(personaText)

    logging.Debug("persona: creating new file", "path", filePath)
    return atomicWrite(filePath, []byte(content), 0o644)
}
```

### 3.6 `handleRemoval` -- Remove Managed Section

```go
// handleRemoval removes the persona markers and content between them.
// If the file becomes empty (only whitespace), the file is deleted.
func handleRemoval(filePath string, lines []string, startIdx, endIdx int) error {
    if startIdx < 0 {
        // No markers found -- nothing to remove.
        logging.Debug("persona: no markers to remove", "path", filePath)
        return nil
    }

    // Remove lines from startIdx through endIdx (inclusive).
    // If endIdx is -1 (missing end marker), remove from startIdx to EOF.
    removeEnd := endIdx
    if removeEnd < 0 {
        logging.Info("persona: missing end marker, treating start-to-EOF as managed section",
            "path", filePath,
        )
        removeEnd = len(lines) - 1
    }

    remaining := make([]string, 0, len(lines))
    remaining = append(remaining, lines[:startIdx]...)
    if removeEnd+1 < len(lines) {
        remaining = append(remaining, lines[removeEnd+1:]...)
    }

    // Trim leading and trailing blank lines left by the removal.
    content := strings.TrimSpace(strings.Join(remaining, "\n"))

    if content == "" {
        // File is now empty -- delete it.
        logging.Info("persona: file empty after removal, deleting", "path", filePath)
        return os.Remove(filePath)
    }

    // Ensure file ends with a single newline.
    content += "\n"

    logging.Info("persona: removed section", "path", filePath)
    return atomicWrite(filePath, []byte(content), 0o644)
}
```

### 3.7 `handleReplace` -- Replace Between Markers

```go
// handleReplace replaces the content between existing markers with the
// new persona text. The markers themselves are rewritten.
func handleReplace(filePath string, lines []string, startIdx, endIdx int, personaText string) error {
    // If endIdx is -1 (corrupt: start without end), replace from start to EOF.
    replaceEnd := endIdx
    if replaceEnd < 0 {
        logging.Info("persona: missing end marker, replacing start-to-EOF",
            "path", filePath,
        )
        replaceEnd = len(lines) - 1
    }

    markedSection := strings.Split(buildMarkedSection(personaText), "\n")

    result := make([]string, 0, len(lines))
    result = append(result, lines[:startIdx]...)
    result = append(result, markedSection...)
    if replaceEnd+1 < len(lines) {
        result = append(result, lines[replaceEnd+1:]...)
    }

    content := strings.Join(result, "\n")

    logging.Info("persona: replaced section", "path", filePath)
    return atomicWrite(filePath, []byte(content), 0o644)
}
```

### 3.8 `handleAppend` -- Append to Existing File

```go
// handleAppend appends the persona markers and text to an existing file
// that does not yet contain markers.
func handleAppend(filePath string, existing string, personaText string) error {
    var b strings.Builder

    b.WriteString(strings.TrimRight(existing, "\n"))
    b.WriteString("\n\n")
    b.WriteString(buildMarkedSection(personaText))

    logging.Info("persona: appended section", "path", filePath)
    return atomicWrite(filePath, []byte(b.String()), 0o644)
}
```

### 3.9 `findMarkers` -- Locate Marker Positions

```go
// findMarkers scans lines for the start and end markers. Returns the
// line indices (0-based) of each marker, or -1 if not found.
//
// Only the first start marker is recognised. If multiple marker pairs
// exist (e.g., from manual editing), subsequent pairs are ignored.
func findMarkers(lines []string) (startIdx, endIdx int) {
    startIdx = -1
    endIdx = -1

    for i, line := range lines {
        trimmed := strings.TrimSpace(line)
        if startIdx < 0 && trimmed == markerStart {
            startIdx = i
            continue
        }
        if startIdx >= 0 && trimmed == markerEnd {
            endIdx = i
            break
        }
    }

    return startIdx, endIdx
}
```

### 3.10 `buildMarkedSection` -- Build Marker Block

```go
// buildMarkedSection constructs the complete marked section including
// start marker, management comment, persona text, and end marker.
func buildMarkedSection(personaText string) string {
    var b strings.Builder
    b.WriteString(markerStart)
    b.WriteString("\n")
    b.WriteString(managedComment)
    b.WriteString("\n\n")
    b.WriteString(strings.TrimRight(personaText, "\n"))
    b.WriteString("\n")
    b.WriteString(markerEnd)
    b.WriteString("\n")
    return b.String()
}
```

### 3.11 `atomicWrite` -- Safe File Write

```go
// atomicWrite writes content to a temporary file in the same directory
// as path, then renames it to path. This prevents partial writes from
// leaving a corrupted file if the process is interrupted.
//
// os.Rename is atomic on POSIX when source and destination are on the
// same filesystem. Writing the temp file in the same directory guarantees
// same-filesystem.
func atomicWrite(path string, content []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".persona-*")
    if err != nil {
        return fmt.Errorf("creating temp file: %w", err)
    }
    tmpPath := tmp.Name()

    // Deferred cleanup: remove temp file if rename fails or any error
    // occurs after this point.
    defer func() {
        if tmpPath != "" {
            os.Remove(tmpPath)
        }
    }()

    if _, err := tmp.Write(content); err != nil {
        tmp.Close()
        return fmt.Errorf("writing temp file: %w", err)
    }
    if err := tmp.Close(); err != nil {
        return fmt.Errorf("closing temp file: %w", err)
    }
    if err := os.Chmod(tmpPath, perm); err != nil {
        return fmt.Errorf("setting permissions: %w", err)
    }
    if err := os.Rename(tmpPath, path); err != nil {
        return fmt.Errorf("renaming temp file: %w", err)
    }

    tmpPath = "" // prevent deferred cleanup after successful rename
    return nil
}
```

### 3.12 Merge Algorithm -- Detailed Pseudocode

```
WritePersonaSection(repoPath, agentType, persona, customPersonas):
    filename = TargetFile(agentType)          // "CLAUDE.md" or "AGENTS.md"
    filePath = join(repoPath, filename)

    IF persona != None:
        result = Resolve(persona, customPersonas)
        IF !result.Found:
            return error("unknown persona")
        personaText = result.Instructions
    ELSE:
        personaText = ""

    CALL writeFile(filePath, personaText)

writeFile(filePath, personaText):
    existing, err = ReadFile(filePath)

    IF err AND IsNotExist(err):
        IF personaText == "":
            return nil                        // None + no file = no-op
        content = buildMarkedSection(personaText)
        return atomicWrite(filePath, content)

    IF err:
        return err                            // permission denied, etc.

    lines = split(existing, "\n")
    startIdx, endIdx = findMarkers(lines)

    IF personaText == "":                     // REMOVAL
        IF startIdx < 0:
            return nil                        // no markers = nothing to remove
        IF endIdx < 0:
            endIdx = len(lines) - 1           // corrupt: start without end
        remaining = lines[:startIdx] + lines[endIdx+1:]
        content = TrimSpace(join(remaining))
        IF content == "":
            return Remove(filePath)           // file empty after removal
        return atomicWrite(filePath, content + "\n")

    IF startIdx >= 0:                         // REPLACE
        IF endIdx < 0:
            endIdx = len(lines) - 1           // corrupt: start without end
        newSection = split(buildMarkedSection(personaText), "\n")
        result = lines[:startIdx] + newSection + lines[endIdx+1:]
        return atomicWrite(filePath, join(result, "\n"))

    // APPEND (no markers found)
    content = TrimRight(existing, "\n") + "\n\n" + buildMarkedSection(personaText)
    return atomicWrite(filePath, content)
```

### 3.13 Edge Cases -- Exhaustive List

| # | Scenario | Input State | Expected Behavior |
|---|----------|-------------|-------------------|
| 1 | No file, persona set | File does not exist, persona = "vibe" | Create file with `markerStart + managedComment + vibeText + markerEnd` |
| 2 | No file, persona None | File does not exist, persona = "" | No-op, return nil |
| 3 | File without markers, persona set | File exists with user content, no markers | Append `\n\n` + marked section to end of file |
| 4 | File with markers, same persona | File has markers with vibe text, persona = "vibe" | Replace section with identical text (idempotent) |
| 5 | File with markers, different persona | File has markers with vibe text, persona = "scale" | Replace text between markers with scale text |
| 6 | File with markers, persona None | File has markers + user content above | Remove markers + text; keep user content |
| 7 | File with only markers | File contains only the marked section | Remove file entirely |
| 8 | Start marker, no end marker | Corrupt: start marker at line 5, no end | Treat lines 5-EOF as managed section; replace/remove as appropriate |
| 9 | End marker, no start marker | Corrupt: only end marker present | No start marker found -> treated as "no markers" -> append if persona set |
| 10 | Multiple marker pairs | Two start/end pairs (manual editing mistake) | Only the first pair is processed; second pair treated as user content |
| 11 | Markers with empty content | Start and end markers on consecutive lines | Replace with new text, or remove markers on None |
| 12 | File is read-only | Permission denied on read | Return error (propagated to TUI as non-fatal) |
| 13 | Directory does not exist | repoPath is invalid | Return error from ReadFile |
| 14 | Persona text with trailing newlines | Instruction text ends with `\n\n\n` | TrimRight normalises to single trailing newline before end marker |
| 15 | File with CRLF line endings | Windows-style line endings | `strings.Split` on `\n` preserves `\r` in line content; markers still match via `TrimSpace` |
| 16 | File with UTF-8 BOM | BOM at start of file | BOM is preserved as part of content before markers; markers are matched after TrimSpace |
| 17 | Custom persona | persona = "backend-expert", found in customPersonas | Use CustomPersona.Instructions as text |
| 18 | Unknown custom persona | persona = "deleted-persona", not in customPersonas | Return error, no file modification |
| 19 | Unknown agent type | agentType = "unknown" | Return error (TargetFile returns "") |
| 20 | Empty instruction text from custom persona | Should not happen (validation requires non-empty) | If it did, would write markers with empty content |


## 4. Persona Package: `internal/persona/setup.go`

### 4.1 File Header and Imports

```go
// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
    "bufio"
    "fmt"
    "os"
    "regexp"
    "strings"

    "github.com/openconductorhq/openconductor/internal/config"
)
```

### 4.2 Name Validation

```go
// slugRegex validates persona name slugs: lowercase letter followed by
// lowercase letters, digits, or hyphens. Length 1-40 characters.
var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// isValidSlug returns true if the name is a valid persona slug.
func isValidSlug(name string) bool {
    return slugRegex.MatchString(name)
}

// isNameAvailable returns true if the name does not collide with any
// built-in persona name or any existing custom persona name.
func isNameAvailable(name string, existing []config.CustomPersona) bool {
    if config.BuiltinPersonaNames[config.PersonaType(name)] {
        return false
    }
    for _, cp := range existing {
        if cp.Name == name {
            return false
        }
    }
    return true
}
```

### 4.3 `RunSetup` -- Main Entry Point

```go
// RunSetup runs the interactive persona management wizard. It reads from
// stdin and writes to stdout -- designed to run inside a PTY (system tab)
// or as a standalone CLI command.
//
// Follows the same pattern as telegram.RunSetup: bufio.Reader for input,
// fmt.Print for output, direct config save to ~/.openconductor/config.yaml.
func RunSetup() error {
    reader := bufio.NewReader(os.Stdin)
    configPath := config.DefaultConfigPath()
    cfg := config.LoadOrDefault(configPath)

    for {
        printMenu(cfg.Personas)

        action, err := readChar(reader)
        if err != nil {
            return fmt.Errorf("reading input: %w", err)
        }

        switch action {
        case 'c', 'C':
            cp, err := createPersona(reader, cfg.Personas)
            if err != nil {
                fmt.Printf("\n  Error: %v\n\n", err)
                continue
            }
            cfg.Personas = append(cfg.Personas, cp)
            if err := cfg.Save(configPath); err != nil {
                fmt.Printf("\n  Error saving config: %v\n\n", err)
                continue
            }
            fmt.Printf("\n  Created persona %q.\n\n", cp.Name)

        case 'e', 'E':
            if len(cfg.Personas) == 0 {
                fmt.Println("\n  No custom personas to edit.\n")
                continue
            }
            if err := editPersona(reader, cfg); err != nil {
                fmt.Printf("\n  Error: %v\n\n", err)
                continue
            }
            if err := cfg.Save(configPath); err != nil {
                fmt.Printf("\n  Error saving config: %v\n\n", err)
                continue
            }
            fmt.Println("\n  Persona updated.\n")

        case 'd', 'D':
            if len(cfg.Personas) == 0 {
                fmt.Println("\n  No custom personas to delete.\n")
                continue
            }
            if err := deletePersona(reader, cfg); err != nil {
                fmt.Printf("\n  Error: %v\n\n", err)
                continue
            }
            if err := cfg.Save(configPath); err != nil {
                fmt.Printf("\n  Error saving config: %v\n\n", err)
                continue
            }

        case 'q', 'Q':
            fmt.Println("\n  Done.")
            return nil
        }
    }
}
```

### 4.4 `printMenu`

```go
func printMenu(custom []config.CustomPersona) {
    fmt.Println()
    fmt.Println("  Persona Manager")
    fmt.Println("  ───────────────")
    fmt.Println()
    fmt.Println("  Built-in personas (read-only):")
    fmt.Println("    vibe   — Move fast, skip tests, auto-approve")
    fmt.Println("    poc    — Working demos, basic tests")
    fmt.Println("    scale  — TDD, production quality, thorough")
    fmt.Println()
    if len(custom) > 0 {
        fmt.Println("  Custom personas:")
        for _, cp := range custom {
            fmt.Printf("    %-12s — %s\n", cp.Name, cp.Label)
        }
        fmt.Println()
    }
    fmt.Println("  [c] Create  [e] Edit  [d] Delete  [q] Quit")
    fmt.Print("  > ")
}
```

### 4.5 `readChar`

```go
// readChar reads a single non-whitespace character from the reader.
// Consumes the trailing newline from ReadString.
func readChar(reader *bufio.Reader) (byte, error) {
    line, err := reader.ReadString('\n')
    if err != nil {
        return 0, err
    }
    line = strings.TrimSpace(line)
    if len(line) == 0 {
        return 0, nil
    }
    return line[0], nil
}
```

### 4.6 `createPersona`

```go
func createPersona(reader *bufio.Reader, existing []config.CustomPersona) (config.CustomPersona, error) {
    var cp config.CustomPersona

    // Name (slug).
    fmt.Println()
    fmt.Print("  Name (slug, e.g. 'backend-expert'): ")
    name, err := reader.ReadString('\n')
    if err != nil {
        return cp, fmt.Errorf("reading name: %w", err)
    }
    name = strings.TrimSpace(name)
    if !isValidSlug(name) {
        return cp, fmt.Errorf("invalid name %q: must be lowercase letters, digits, hyphens (1-40 chars, start with letter)", name)
    }
    if !isNameAvailable(name, existing) {
        return cp, fmt.Errorf("name %q already exists (built-in or custom)", name)
    }
    cp.Name = name

    // Label (display name).
    fmt.Print("  Label (display name, e.g. 'Backend Expert'): ")
    label, err := reader.ReadString('\n')
    if err != nil {
        return cp, fmt.Errorf("reading label: %w", err)
    }
    label = strings.TrimSpace(label)
    if label == "" {
        return cp, fmt.Errorf("label cannot be empty")
    }
    cp.Label = label

    // Instructions (multi-line, END sentinel).
    fmt.Println("  Instructions (markdown, type END on a line by itself to finish):")
    instructions, err := readMultiLine(reader, "END")
    if err != nil {
        return cp, fmt.Errorf("reading instructions: %w", err)
    }
    if strings.TrimSpace(instructions) == "" {
        return cp, fmt.Errorf("instructions cannot be empty")
    }
    cp.Instructions = instructions

    // Auto-approve level.
    fmt.Println("  Auto-approve level:")
    fmt.Println("    [1] Off   — Notify for all permission requests")
    fmt.Println("    [2] Safe  — Auto-approve file edits and safe commands")
    fmt.Println("    [3] Full  — Auto-approve everything")
    fmt.Print("  > ")
    choice, err := reader.ReadString('\n')
    if err != nil {
        return cp, fmt.Errorf("reading auto-approve: %w", err)
    }
    switch strings.TrimSpace(choice) {
    case "1", "":
        cp.AutoApprove = config.ApprovalOff
    case "2":
        cp.AutoApprove = config.ApprovalSafe
    case "3":
        cp.AutoApprove = config.ApprovalFull
    default:
        return cp, fmt.Errorf("invalid choice %q", strings.TrimSpace(choice))
    }

    return cp, nil
}
```

### 4.7 `readMultiLine`

```go
// readMultiLine reads lines from the reader until it encounters a line
// that is exactly the sentinel string (after trimming whitespace).
// Returns the collected text without the sentinel line.
func readMultiLine(reader *bufio.Reader, sentinel string) (string, error) {
    var lines []string
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            return strings.Join(lines, "\n"), err
        }
        if strings.TrimSpace(line) == sentinel {
            break
        }
        lines = append(lines, strings.TrimRight(line, "\n\r"))
    }
    return strings.Join(lines, "\n"), nil
}
```

### 4.8 `editPersona`

```go
func editPersona(reader *bufio.Reader, cfg *config.Config) error {
    // Select which persona to edit.
    fmt.Println()
    fmt.Println("  Select persona to edit:")
    for i, cp := range cfg.Personas {
        fmt.Printf("    [%d] %s (%s)\n", i+1, cp.Name, cp.Label)
    }
    fmt.Print("  > ")
    choice, err := reader.ReadString('\n')
    if err != nil {
        return err
    }
    idx, err := parseIndex(strings.TrimSpace(choice), len(cfg.Personas))
    if err != nil {
        return err
    }

    cp := &cfg.Personas[idx]

    // Label.
    fmt.Printf("  Label [%s]: ", cp.Label)
    label, err := reader.ReadString('\n')
    if err != nil {
        return err
    }
    label = strings.TrimSpace(label)
    if label != "" {
        cp.Label = label
    }

    // Instructions.
    fmt.Println("  Current instructions:")
    for _, line := range strings.Split(cp.Instructions, "\n") {
        fmt.Printf("    %s\n", line)
    }
    fmt.Println()
    fmt.Println("  New instructions (type END to finish, or press Enter to keep current):")
    first, err := reader.ReadString('\n')
    if err != nil {
        return err
    }
    if strings.TrimSpace(first) != "" {
        // User started typing new instructions.
        rest, err := readMultiLine(reader, "END")
        if err != nil {
            return err
        }
        newInstructions := strings.TrimRight(first, "\n\r") + "\n" + rest
        newInstructions = strings.TrimSpace(newInstructions)
        if newInstructions != "" {
            cp.Instructions = newInstructions
        }
    }

    // Auto-approve.
    currentApproval := "off"
    switch cp.AutoApprove {
    case config.ApprovalSafe:
        currentApproval = "safe"
    case config.ApprovalFull:
        currentApproval = "full"
    }
    fmt.Printf("  Auto-approve [%s] (1=off, 2=safe, 3=full): ", currentApproval)
    approvalChoice, err := reader.ReadString('\n')
    if err != nil {
        return err
    }
    switch strings.TrimSpace(approvalChoice) {
    case "1":
        cp.AutoApprove = config.ApprovalOff
    case "2":
        cp.AutoApprove = config.ApprovalSafe
    case "3":
        cp.AutoApprove = config.ApprovalFull
    case "":
        // keep current
    default:
        return fmt.Errorf("invalid choice %q", strings.TrimSpace(approvalChoice))
    }

    return nil
}
```

### 4.9 `deletePersona`

```go
func deletePersona(reader *bufio.Reader, cfg *config.Config) error {
    fmt.Println()
    fmt.Println("  Select persona to delete:")
    for i, cp := range cfg.Personas {
        fmt.Printf("    [%d] %s (%s)\n", i+1, cp.Name, cp.Label)
    }
    fmt.Print("  > ")
    choice, err := reader.ReadString('\n')
    if err != nil {
        return err
    }
    idx, err := parseIndex(strings.TrimSpace(choice), len(cfg.Personas))
    if err != nil {
        return err
    }

    name := cfg.Personas[idx].Name

    // Check if any projects reference this persona.
    var referencing []string
    for _, p := range cfg.Projects {
        if string(p.Persona) == name {
            referencing = append(referencing, p.Name)
        }
    }
    if len(referencing) > 0 {
        fmt.Printf("  Warning: %d project(s) use this persona: %s\n",
            len(referencing), strings.Join(referencing, ", "))
        fmt.Print("  Those projects will have no persona after deletion. Continue? (y/n): ")
        confirm, err := reader.ReadString('\n')
        if err != nil {
            return err
        }
        if strings.TrimSpace(strings.ToLower(confirm)) != "y" {
            fmt.Println("  Cancelled.")
            return nil
        }
    }

    // Remove from slice.
    cfg.Personas = append(cfg.Personas[:idx], cfg.Personas[idx+1:]...)
    fmt.Printf("\n  Deleted persona %q.\n\n", name)
    return nil
}
```

### 4.10 `parseIndex`

```go
// parseIndex parses a 1-based index string and returns a 0-based index.
// Returns an error if the string is not a valid number or out of range.
func parseIndex(s string, count int) (int, error) {
    if s == "" {
        return 0, fmt.Errorf("no selection")
    }
    n := 0
    for _, ch := range s {
        if ch < '0' || ch > '9' {
            return 0, fmt.Errorf("invalid number %q", s)
        }
        n = n*10 + int(ch-'0')
    }
    if n < 1 || n > count {
        return 0, fmt.Errorf("selection %d out of range (1-%d)", n, count)
    }
    return n - 1, nil
}
```

### 4.11 CLI Wizard Interaction Example

```
$ openconductor persona

  Persona Manager
  ───────────────

  Built-in personas (read-only):
    vibe   — Move fast, skip tests, auto-approve
    poc    — Working demos, basic tests
    scale  — TDD, production quality, thorough

  [c] Create  [e] Edit  [d] Delete  [q] Quit
  > c

  Name (slug, e.g. 'backend-expert'): backend-expert
  Label (display name, e.g. 'Backend Expert'): Backend Expert
  Instructions (markdown, type END on a line by itself to finish):
  ## Backend Expert
  Focus on server-side architecture and API design.
  - Design RESTful APIs with proper HTTP semantics
  - Use connection pooling and caching strategies
  - Write integration tests for all endpoints
  - Follow OWASP security guidelines
  END
  Auto-approve level:
    [1] Off   — Notify for all permission requests
    [2] Safe  — Auto-approve file edits and safe commands
    [3] Full  — Auto-approve everything
  > 2

  Created persona "backend-expert".

  Persona Manager
  ───────────────

  Built-in personas (read-only):
    vibe   — Move fast, skip tests, auto-approve
    poc    — Working demos, basic tests
    scale  — TDD, production quality, thorough

  Custom personas:
    backend-expert — Backend Expert

  [c] Create  [e] Edit  [d] Delete  [q] Quit
  > d

  Select persona to delete:
    [1] backend-expert (Backend Expert)
  > 1
  Warning: 1 project(s) use this persona: api-service
  Those projects will have no persona after deletion. Continue? (y/n): y

  Deleted persona "backend-expert".

  Persona Manager
  ───────────────

  Built-in personas (read-only):
    vibe   — Move fast, skip tests, auto-approve
    poc    — Working demos, basic tests
    scale  — TDD, production quality, thorough

  [c] Create  [e] Edit  [d] Delete  [q] Quit
  > q

  Done.
```


## 5. Test Specifications

### 5.1 `internal/config/config_test.go` -- New Tests

All tests follow the existing table-driven pattern in the file.

| Test Function | Description | Inputs | Expected |
|---|---|---|---|
| `TestValidatePersonaFieldAcceptsEmpty` | Empty persona (zero value) passes validation | `Project{Persona: ""}` | No error |
| `TestValidatePersonaFieldAcceptsBuiltin` | Built-in persona passes validation | `Project{Persona: "vibe"}` | No error |
| `TestValidatePersonaFieldAcceptsUnknown` | Unknown persona does NOT fail `validate()` | `Project{Persona: "deleted-custom"}` | No error (by design) |
| `TestValidatePersonaRefBuiltin` | `ValidatePersonaRef` accepts built-in | `persona: "vibe"` | No error |
| `TestValidatePersonaRefCustom` | `ValidatePersonaRef` accepts custom from config | `persona: "backend-expert"`, config has matching custom | No error |
| `TestValidatePersonaRefNone` | `ValidatePersonaRef` accepts empty | `persona: ""` | No error |
| `TestValidatePersonaRefUnknown` | `ValidatePersonaRef` rejects unknown name | `persona: "nonexistent"`, no matching custom | Error |
| `TestValidateCustomPersonaDuplicateName` | Duplicate names in `Personas` slice | Two `CustomPersona` with same name | Error from `validate()` |
| `TestValidateCustomPersonaMissingName` | Empty name in `CustomPersona` | `CustomPersona{Name: ""}` | Error from `validate()` |
| `TestValidateCustomPersonaMissingLabel` | Empty label | `CustomPersona{Name: "x", Label: ""}` | Error from `validate()` |
| `TestValidateCustomPersonaMissingInstructions` | Empty instructions | `CustomPersona{Name: "x", Label: "X", Instructions: ""}` | Error from `validate()` |
| `TestValidateCustomPersonaBadApproval` | Invalid approval level on custom persona | `CustomPersona{AutoApprove: "yolo"}` | Error from `validate()` |
| `TestSaveLoadRoundTripWithPersona` | Persona field survives YAML round-trip | `Project{Persona: "vibe"}` | Loaded persona == "vibe" |
| `TestSaveLoadRoundTripWithCustomPersonas` | Custom personas survive YAML round-trip | Config with custom personas | Loaded custom personas match |

### 5.2 `internal/persona/persona_test.go`

```go
// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona
```

| Test Function | Description | Inputs | Expected |
|---|---|---|---|
| `TestInstructionTextBuiltins` | Each built-in persona returns non-empty text | PersonaVibe, PersonaPOC, PersonaScale | Non-empty string for each |
| `TestInstructionTextNone` | PersonaNone returns empty string | PersonaNone | `""` |
| `TestInstructionTextUnknown` | Unknown persona returns empty string | `PersonaType("unknown")` | `""` |
| `TestTargetFileClaudeCode` | Claude Code maps to CLAUDE.md | AgentClaudeCode | `"CLAUDE.md"` |
| `TestTargetFileOpenCode` | OpenCode maps to AGENTS.md | AgentOpenCode | `"AGENTS.md"` |
| `TestTargetFileUnknown` | Unknown agent returns empty | `AgentType("unknown")` | `""` |
| `TestDefaultApprovalVibe` | Vibe suggests Full | PersonaVibe | ApprovalFull |
| `TestDefaultApprovalPOC` | POC suggests Safe | PersonaPOC | ApprovalSafe |
| `TestDefaultApprovalScale` | Scale suggests Off | PersonaScale | ApprovalOff |
| `TestDefaultApprovalNone` | None defaults to Off | PersonaNone | ApprovalOff |
| `TestDefaultApprovalUnknown` | Unknown defaults to Off | `PersonaType("x")` | ApprovalOff |
| `TestResolveBuiltin` | Built-in resolution returns correct fields | PersonaVibe, nil customs | Found=true, Instructions=vibe text, Approval=Full, Label="Vibe" |
| `TestResolveCustom` | Custom persona found in slice | `PersonaType("my-custom")`, custom slice with match | Found=true, custom fields |
| `TestResolveNone` | PersonaNone resolves with empty instructions | PersonaNone | Found=true, Instructions="", Label="None" |
| `TestResolveUnknown` | Unknown name not found | `PersonaType("gone")`, empty customs | Found=false |
| `TestResolveBuiltinShadowsCustom` | Built-in wins over custom with same name | `PersonaType("vibe")`, custom with Name="vibe" | Returns built-in text, not custom |
| `TestAllPersonaOptionsNoCustom` | Returns 4 built-in options | nil customs | 4 options: None, Vibe, POC, Scale |
| `TestAllPersonaOptionsWithCustom` | Custom personas appended after built-ins | 2 custom personas | 6 options, last 2 have IsCustom=true |
| `TestLabelBuiltin` | Label for built-in persona | PersonaVibe | "Vibe" |
| `TestLabelCustom` | Label for custom persona | custom with Label="My Thing" | "My Thing" |
| `TestLabelNone` | Label for None | PersonaNone | "None" |
| `TestLabelUnknown` | Label for unknown returns raw slug | `PersonaType("mystery")` | "mystery" |
| `TestIsValidSlug` | Table-driven slug validation | See table below | Boolean results |
| `TestIsNameAvailable` | Name availability checking | See table below | Boolean results |

**Slug validation test table**:

```go
var slugTests = []struct {
    name  string
    input string
    valid bool
}{
    {"simple", "vibe", true},
    {"with-hyphens", "backend-expert", true},
    {"with-numbers", "v2-fast", true},
    {"single-char", "a", true},
    {"max-length", strings.Repeat("a", 40), true},
    {"too-long", strings.Repeat("a", 41), false},
    {"starts-with-number", "2fast", false},
    {"starts-with-hyphen", "-bad", false},
    {"uppercase", "Vibe", false},
    {"mixed-case", "backendExpert", false},
    {"spaces", "my persona", false},
    {"underscores", "my_persona", false},
    {"empty", "", false},
    {"special-chars", "my@persona", false},
    {"trailing-hyphen", "test-", true}, // allowed by regex
}
```

**Name availability test table**:

```go
var availabilityTests = []struct {
    name     string
    slug     string
    existing []config.CustomPersona
    want     bool
}{
    {"available", "backend-expert", nil, true},
    {"builtin-vibe", "vibe", nil, false},
    {"builtin-poc", "poc", nil, false},
    {"builtin-scale", "scale", nil, false},
    {"custom-collision", "existing",
        []config.CustomPersona{{Name: "existing"}}, false},
    {"no-collision", "new-one",
        []config.CustomPersona{{Name: "existing"}}, true},
}
```

### 5.3 `internal/persona/writer_test.go`

All writer tests use `t.TempDir()` for filesystem isolation.

```go
// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona
```

| Test Function | Scenario | Setup | Action | Assertion |
|---|---|---|---|---|
| `TestWriteNoFilePersonaSet` | #1: Create new | No file | WritePersonaSection(vibe) | File created, contains markers + vibe text |
| `TestWriteNoFilePersonaNone` | #2: No-op | No file | WritePersonaSection(None) | No file created, no error |
| `TestWriteAppendToExisting` | #3: Append | File with `# My Project\n\nCustom stuff` | WritePersonaSection(poc) | File has original + `\n\n` + markers + poc text |
| `TestWriteReplaceExisting` | #5: Replace | File with markers + vibe text | WritePersonaSection(scale) | Markers contain scale text; rest unchanged |
| `TestWriteReplaceIdempotent` | #4: Idempotent | File with markers + vibe text | WritePersonaSection(vibe) | File unchanged (byte-for-byte) |
| `TestWriteRemoveWithContent` | #6: Remove, keep user content | File with user content + markers + vibe | WritePersonaSection(None) | Markers removed, user content preserved |
| `TestWriteRemoveFileEmpty` | #7: Remove, delete file | File with only markers + text | WritePersonaSection(None) | File deleted |
| `TestWriteRemoveNoMarkers` | Variant of #6 | File with user content, no markers | WritePersonaSection(None) | No change, no error |
| `TestWriteCorruptStartOnly` | #8: Start, no end | File with start marker at line 3, no end | WritePersonaSection(scale) | Lines 3-EOF replaced with new section |
| `TestWriteCorruptStartOnlyRemoval` | #8 removal | File with start marker, no end | WritePersonaSection(None) | Lines from start marker to EOF removed |
| `TestWriteEndMarkerOnly` | #9: Only end marker | File with end marker, no start | WritePersonaSection(vibe) | Section appended (no start found = append) |
| `TestWriteMultipleMarkerPairs` | #10: Two pairs | File with two start/end pairs | WritePersonaSection(scale) | First pair replaced, second untouched |
| `TestWriteEmptyBetweenMarkers` | #11: Empty content | Start and end on adjacent lines | WritePersonaSection(poc) | Content inserted between markers |
| `TestWritePreservesUserContent` | Content preservation | User content above and below markers | WritePersonaSection(vibe) | Only marker section changes |
| `TestWriteTrailingNewlines` | #14: Normalisation | Instruction text with trailing `\n\n\n` | buildMarkedSection | Single trailing newline before end marker |
| `TestWriteUnknownPersona` | #18: Unknown | persona = "nonexistent" | WritePersonaSection | Error returned, file not modified |
| `TestWriteUnknownAgentType` | #19: Bad agent | agentType = "unknown" | WritePersonaSection | Error returned |
| `TestWriteCustomPersona` | #17: Custom | persona = "my-custom", custom in slice | WritePersonaSection | File contains custom instructions |
| `TestAtomicWriteCreatesFile` | Atomic write | Non-existent path | atomicWrite | File created with correct content and perms |
| `TestAtomicWriteOverwrites` | Atomic overwrite | Existing file with old content | atomicWrite | File has new content |
| `TestAtomicWriteNoTempLeftOnSuccess` | Cleanup | Any | atomicWrite | No `.persona-*` temp files in directory |
| `TestFindMarkersNone` | No markers | Lines without markers | findMarkers | (-1, -1) |
| `TestFindMarkersNormal` | Both markers | Lines with start at 2, end at 5 | findMarkers | (2, 5) |
| `TestFindMarkersStartOnly` | Start only | Start at line 3, no end | findMarkers | (3, -1) |
| `TestFindMarkersEndOnly` | End only | End at line 2, no start | findMarkers | (-1, -1) |
| `TestFindMarkersWithLeadingSpace` | Indented markers | `  <!-- openconductor:persona:start -->` | findMarkers | Matches (TrimSpace) |
| `TestBuildMarkedSection` | Structure | "hello world" | buildMarkedSection | markerStart + `\n` + managedComment + `\n\n` + "hello world" + `\n` + markerEnd + `\n` |

### 5.4 `internal/persona/setup_test.go`

Setup wizard tests are more limited because the wizard reads from stdin. Key testable units are the pure functions.

| Test Function | Description | Inputs | Expected |
|---|---|---|---|
| `TestIsValidSlugTable` | Table-driven, see section 5.2 | slug strings | boolean results |
| `TestIsNameAvailableTable` | Table-driven, see section 5.2 | slug + existing personas | boolean results |
| `TestParseIndexValid` | Valid 1-based input | "1", count=3 | 0, nil |
| `TestParseIndexLastItem` | Last valid | "3", count=3 | 2, nil |
| `TestParseIndexZero` | Zero is invalid | "0", count=3 | error |
| `TestParseIndexOverflow` | Over count | "4", count=3 | error |
| `TestParseIndexEmpty` | Empty string | "", count=3 | error |
| `TestParseIndexNonNumeric` | Letters | "abc", count=3 | error |
| `TestReadMultiLine` | Reads until sentinel | Reader with "line1\nline2\nEND\n" | "line1\nline2" |
| `TestReadMultiLineEmptyInput` | Only sentinel | Reader with "END\n" | "" |


## 6. Error Handling Strategy

### 6.1 Per-Function Error Summary

| Function | Error Conditions | Handling |
|---|---|---|
| `WritePersonaSection` | Unknown agent type, unknown persona, file read error, atomic write error | Returns `error`. Caller (TUI) logs via `logging.Error`, shows transient status bar message. Non-fatal: session still starts. |
| `atomicWrite` | Temp file creation, write, close, chmod, rename | Returns wrapped `error`. Temp file cleaned up in deferred call on any failure. |
| `writeFile` | File read (non-ENOENT), all sub-handler errors | Propagated from `atomicWrite` and `os.Remove`. |
| `handleRemoval` (file delete) | `os.Remove` failure | Returns `error`. Edge case: file became read-only between read and delete. |
| `Resolve` | Never errors | Returns `ResolveResult{Found: false}` for unknown personas. Caller decides how to handle. |
| `RunSetup` | Reader EOF, invalid input, config save | Prints error to stdout, continues loop (does not exit). Only returns error on reader EOF. |
| `Config.validate` | Structural errors in custom persona definitions | Returns `error`, preventing config load. Strict: names must be unique, labels and instructions must be non-empty. |
| `Config.ValidatePersonaRef` | Unknown persona name | Returns `error`. Called explicitly, not on load. |

### 6.2 Error Messages Convention

All error messages from the persona package use the `"persona: "` prefix for grep-ability in logs, consistent with the `"telegram: "` prefix pattern in the existing codebase.

Format: `fmt.Errorf("operation description: %w", err)` for wrapping, `fmt.Errorf("descriptive message")` for terminal errors.


## 7. File Structure Summary

### 7.1 New Files

| File | Lines (est.) | Purpose |
|---|---|---|
| `internal/persona/persona.go` | ~140 | Type mapping, instruction text constants, Resolve, DefaultApproval, TargetFile, Label, AllPersonaOptions |
| `internal/persona/writer.go` | ~200 | writeFile, findMarkers, buildMarkedSection, handleNoFile, handleAppend, handleReplace, handleRemoval, atomicWrite |
| `internal/persona/setup.go` | ~250 | RunSetup, printMenu, createPersona, editPersona, deletePersona, readMultiLine, readChar, parseIndex, isValidSlug, isNameAvailable |
| `internal/persona/persona_test.go` | ~200 | Tests for persona.go functions |
| `internal/persona/writer_test.go` | ~350 | Tests for writer.go functions |
| `internal/persona/setup_test.go` | ~100 | Tests for setup.go pure functions |

### 7.2 Modified Files

| File | Change Description |
|---|---|
| `internal/config/config.go` | Add PersonaType, BuiltinPersonaNames, CustomPersona, Persona field on Project, Personas on Config, extend validate(), add ValidatePersonaRef() |
| `internal/config/config_test.go` | Add ~14 new test functions for persona-related validation and round-trip |
| `cmd/openconductor/main.go` | Add `"persona"` case in subcommand switch, calling `persona.RunSetup()`. Add `--persona` flag in `runBootstrap`. Update `printUsage`. |


## 8. Implementation Notes

### 8.1 Import Path

```
github.com/openconductorhq/openconductor/internal/persona
```

### 8.2 Dependency Graph (This LLD's Scope Only)

```
cmd/openconductor/main.go
    |
    +-- internal/persona  (RunSetup, WritePersonaSection)
    |       |
    |       +-- internal/config  (PersonaType, CustomPersona, AgentType, etc.)
    |       +-- internal/logging
    |
    +-- internal/config    (existing: Load, Save, etc.)
```

The `persona` package depends only on `config` and `logging` from internal packages. It has no dependency on `agent`, `tui`, `session`, or any other internal package. This is intentional and must be preserved.

### 8.3 Git Ignore

Add `.persona-*` to the project's `.gitignore` template (not the repo's own .gitignore, but the bootstrap template) to prevent temp files from appearing in git status if a write is interrupted mid-operation. In practice, the temp file lifetime is microseconds, making this a defense-in-depth measure.

### 8.4 Concurrency Safety

All functions in the persona package are stateless -- they operate on value parameters and the filesystem. No package-level mutable state exists (the maps `instructionText`, `targetFile`, `defaultApproval`, `builtinLabels` are read-only after init). Concurrent calls to `WritePersonaSection` targeting different files are safe. Concurrent calls targeting the same file rely on the TUI message loop serialisation described in HLD section 6.5.

### 8.5 Line Ending Handling

The writer operates on `\n` (LF) line endings. Files with `\r\n` (CRLF) are split on `\n`, leaving `\r` at the end of line strings. The marker matching uses `strings.TrimSpace`, which strips `\r`, so markers are recognised regardless of line ending style. However, the output always uses `\n` endings. This means a CRLF file will be normalised to LF after a persona write -- this is acceptable because Go source repos are predominantly LF, and both CLAUDE.md and AGENTS.md are markdown files where line ending style has no semantic impact.

### 8.6 Relation to Bootstrap Package

The `bootstrap` package currently creates a placeholder `CLAUDE.md` via `BootstrapFiles()` on the Claude adapter (see `internal/agent/claude.go`, line 155). When a persona is set, `WritePersonaSection` is called after bootstrap. If the bootstrap-created `CLAUDE.md` already exists, the persona writer appends the managed section (scenario #3). If the user deletes the persona later, only the managed section is removed, leaving the bootstrap content intact.

The `bootstrap.Bootstrap()` function in `internal/bootstrap/bootstrap.go` will gain an optional `--persona` flag that calls `persona.WritePersonaSection` after template rendering. This is a thin integration -- bootstrap does not need to know about persona internals.
