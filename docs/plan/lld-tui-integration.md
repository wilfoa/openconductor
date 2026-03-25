# LLD: Agent Personas -- TUI Integration

**Status**: Draft
**Scope**: `internal/tui/` only (form, sidebar, app, messages, statusbar, styles)
**Assumes**: A `persona` package and config-layer changes are designed separately.
**Baseline commit**: `f99a232` on `feat/readme-demo-assets`

---

## 1. Prerequisite: Config-Layer Types

The TUI integration depends on these types being added to `internal/config/config.go` by the config-layer LLD. They are listed here as a contract, not as part of this LLD's implementation scope.

```go
// PersonaType identifies a behavioral preset for an agent.
type PersonaType string

const (
    PersonaNone  PersonaType = ""
    PersonaVibe  PersonaType = "vibe"
    PersonaPOC   PersonaType = "poc"
    PersonaScale PersonaType = "scale"
)

// CustomPersona defines a user-created persona stored in config.yaml.
// Built-in personas (None, Vibe, POC, Scale) are hardcoded in the persona
// package and do not need CustomPersona entries.
type CustomPersona struct {
    Name         string        `yaml:"name"`          // slug identifier
    Label        string        `yaml:"label"`         // display name
    Instructions string        `yaml:"instructions"`  // markdown instruction text
    AutoApprove  ApprovalLevel `yaml:"auto_approve,omitempty"`
}

// Updated Project struct (adds Persona field):
type Project struct {
    Name        string        `yaml:"name"`
    Repo        string        `yaml:"repo"`
    Agent       AgentType     `yaml:"agent"`
    Persona     PersonaType   `yaml:"persona,omitempty"`
    AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
}

// Updated Config struct (adds Personas field):
type Config struct {
    Projects      []Project          `yaml:"projects"`
    Personas      []CustomPersona    `yaml:"personas,omitempty"`
    LLM           LLMConfig          `yaml:"llm"`
    Notifications NotificationConfig `yaml:"notifications"`
    Telegram      TelegramConfig     `yaml:"telegram"`
}
```

The TUI also depends on `persona.WritePersonaSection(repoPath string, agentType config.AgentType, persona config.PersonaType, customPersonas []config.CustomPersona) error` that writes the persona instruction block into the appropriate file (CLAUDE.md / AGENTS.md) in the project's repo directory. This is provided by the `internal/persona` package (out of scope).

---

## 2. Messages (`internal/tui/messages.go`)

### 2.1 New Message Types

Add the following after the existing `historyLoadedMsg`:

```go
// PersonaChangeRequestMsg is sent when the user selects a new persona for
// an existing project via the sidebar persona picker. The App handler
// updates config, writes the persona file, and restarts affected sessions.
type PersonaChangeRequestMsg struct {
    ProjectName string
    NewPersona  config.PersonaType
}

// PersonaWrittenMsg signals that the persona file write completed. Err is
// nil on success. When sequenced from ProjectAddedMsg, the App uses this
// to start the session (persona file must exist before agent launch).
type PersonaWrittenMsg struct {
    ProjectName string
    Err         error
}
```

### 2.2 No Changes to Existing Messages

`ProjectAddedMsg` continues to carry `config.Project`. The project's `Persona` field is set by the form before emitting the message. No structural change to the message type itself.

---

## 3. Form Step Addition (`internal/tui/form.go`)

### 3.1 New Step Constant

```go
type formStep int

const (
    stepName        formStep = iota
    stepRepo
    stepAgent
    stepPersona                      // NEW: step 4 of 5
    stepAutoApprove
)
```

The total step count changes from 4 to 5.

### 3.2 Persona Option Type and Built-in List

```go
// personaOption pairs a display label with the config value and an optional
// description. Built-in options are always present; custom options are
// appended from config at form creation time.
type personaOption struct {
    label       string
    description string
    persona     config.PersonaType
    isDivider   bool // true for the separator line between built-in and custom
}

// builtinPersonaOptions is the fixed set of personas always shown.
var builtinPersonaOptions = []personaOption{
    {label: "None", description: "No persona preset", persona: config.PersonaNone},
    {label: "Vibe", description: "Fast iteration, creative, auto-approve safe", persona: config.PersonaVibe},
    {label: "POC", description: "Proof of concept, broad exploration", persona: config.PersonaPOC},
    {label: "Scale", description: "Production quality, thorough review", persona: config.PersonaScale},
}

// defaultApprovalForPersona maps each built-in persona to its recommended
// approval level. Used to pre-select the approvalIndex on stepAutoApprove
// after persona selection.
var defaultApprovalForPersona = map[config.PersonaType]config.ApprovalLevel{
    config.PersonaNone:  config.ApprovalOff,
    config.PersonaVibe:  config.ApprovalFull,
    config.PersonaPOC:   config.ApprovalSafe,
    config.PersonaScale: config.ApprovalOff,
}
```

### 3.3 Building the Persona Options List

The `personaOptions` slice is built dynamically at form creation time because custom personas come from the config. The function is:

```go
// buildPersonaOptions returns the full list of persona options including
// built-in presets and any custom personas from the config. A divider
// separates the two groups when custom personas exist.
func buildPersonaOptions(customPersonas []config.CustomPersona) []personaOption {
    opts := make([]personaOption, len(builtinPersonaOptions))
    copy(opts, builtinPersonaOptions)

    if len(customPersonas) > 0 {
        // Divider line
        opts = append(opts, personaOption{isDivider: true})
        for _, p := range customPersonas {
            desc := p.Description
            if desc == "" {
                desc = "Custom persona"
            }
            opts = append(opts, personaOption{
                label:       string(p.Name),
                description: desc,
                persona:     p.Name,
            })
        }
    }
    return opts
}
```

### 3.4 formModel Changes

```go
type formModel struct {
    step           formStep
    nameInput      textinput.Model
    repoInput      textinput.Model
    agentIndex     int
    personaIndex   int                // NEW: index into personaOptions
    personaOptions []personaOption    // NEW: built at form creation
    approvalIndex  int
    err            string
    existingNames  map[string]bool
    completion     completionModel
}
```

### 3.5 newFormModel Changes

The function signature gains a parameter for custom personas from the config:

```go
func newFormModel(existingNames []string, customPersonas []config.CustomPersona) (formModel, tea.Cmd) {
    // ... existing code for ni, ri, names ...

    return formModel{
        step:           stepName,
        nameInput:      ni,
        repoInput:      ri,
        agentIndex:     0,
        personaIndex:   0,                                      // NEW
        personaOptions: buildPersonaOptions(customPersonas),     // NEW
        existingNames:  names,
    }, cmd
}
```

This change cascades to `sidebarModel.openForm()` which must pass the custom personas:

```go
func (m sidebarModel) openForm() (sidebarModel, tea.Cmd) {
    names := make([]string, len(m.projects))
    for i, p := range m.projects {
        names[i] = p.Name
    }
    form, cmd := newFormModel(names, m.customPersonas)  // NEW parameter
    m.form = form
    m.mode = sidebarForm
    return m, cmd
}
```

The `sidebarModel` gains a `customPersonas []config.CustomPersona` field, set from `cfg.Personas` at construction and updated when `SystemTabExitedMsg` reloads the config. See Section 5.2.

### 3.6 Step Indicator Change

```go
func (m formModel) stepIndicator() string {
    step := int(m.step) + 1
    return formStepStyle.Render(fmt.Sprintf("%d/5", step))  // was %d/4
}
```

### 3.7 Update() Key Handling for stepPersona

Add these cases inside the `switch` block in `formModel.Update()`, after the `stepAgent` cases and before the `stepAutoApprove` cases:

```go
case isRuneKey(msg, 'j') && m.step == stepPersona:
    m.personaIndex = m.nextSelectablePersona(m.personaIndex, +1)
    return m, nil

case isRuneKey(msg, 'k') && m.step == stepPersona:
    m.personaIndex = m.nextSelectablePersona(m.personaIndex, -1)
    return m, nil

case msg.Type == tea.KeyDown && m.step == stepPersona:
    m.personaIndex = m.nextSelectablePersona(m.personaIndex, +1)
    return m, nil

case msg.Type == tea.KeyUp && m.step == stepPersona:
    m.personaIndex = m.nextSelectablePersona(m.personaIndex, -1)
    return m, nil
```

The `nextSelectablePersona` helper skips divider entries:

```go
// nextSelectablePersona returns the next non-divider index in direction dir
// (+1 or -1) from the current index. Returns current if no move is possible.
func (m formModel) nextSelectablePersona(current, dir int) int {
    next := current + dir
    for next >= 0 && next < len(m.personaOptions) {
        if !m.personaOptions[next].isDivider {
            return next
        }
        next += dir
    }
    return current
}
```

### 3.8 advance() Changes

The `stepAgent` case now transitions to `stepPersona` instead of `stepAutoApprove`:

```go
case stepAgent:
    m.step = stepPersona     // was: m.step = stepAutoApprove
    m.repoInput.Blur()
    return m, nil
```

The new `stepPersona` case transitions to `stepAutoApprove` with a pre-selected approval index:

```go
case stepPersona:
    selected := m.personaOptions[m.personaIndex]
    // Pre-select approval level based on persona's default.
    if level, ok := defaultApprovalForPersona[selected.persona]; ok {
        for i, opt := range approvalOptions {
            if opt.level == level {
                m.approvalIndex = i
                break
            }
        }
    }
    // For custom personas with an AutoApprove set, use that.
    // (Handled by looking up the persona def if not in the built-in map.)
    m.step = stepAutoApprove
    return m, nil
```

The `stepAutoApprove` case now includes the persona in the emitted `ProjectAddedMsg`:

```go
case stepAutoApprove:
    project := config.Project{
        Name:        strings.TrimSpace(m.nameInput.Value()),
        Repo:        strings.TrimSpace(m.repoInput.Value()),
        Agent:       agentTypes[m.agentIndex],
        Persona:     m.personaOptions[m.personaIndex].persona,   // NEW
        AutoApprove: approvalOptions[m.approvalIndex].level,
    }
    return m, func() tea.Msg { return ProjectAddedMsg{Project: project} }
```

### 3.9 View() Changes

#### stepAgent done line position

No change to `stepAgent`'s view -- it is identical.

#### stepPersona view (new)

Insert this case between `stepAgent` and `stepAutoApprove`:

```go
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
        if opt.isDivider {
            b.WriteString(formHintStyle.Render("  ──────────"))
            b.WriteString("\n")
            continue
        }
        line := fmt.Sprintf("%-6s  %s", opt.label, opt.description)
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
```

#### stepAutoApprove done-line update

The `stepAutoApprove` view now shows 4 done lines (Name, Repo, Agent, Persona) instead of 3:

```go
case stepAutoApprove:
    b.WriteString(formDoneStyle.Render("Name     " + m.nameInput.Value()))
    b.WriteString("\n")
    b.WriteString(formDoneStyle.Render("Repo     " + m.repoInput.Value()))
    b.WriteString("\n")
    b.WriteString(formDoneStyle.Render("Agent    " + string(agentTypes[m.agentIndex])))
    b.WriteString("\n")
    // NEW: persona done line
    personaLabel := m.personaOptions[m.personaIndex].label
    b.WriteString(formDoneStyle.Render("Persona  " + personaLabel))
    b.WriteString("\n\n")
    b.WriteString(formLabelStyle.Render("Auto-approve permissions"))
    b.WriteString("\n")
    // ... rest is unchanged
```

### 3.10 Mouse Click Constants

The existing constants and a new constant for persona selection:

```go
// formAgentOptionContentStart: unchanged at 6.
// Lines 0-5 for stepAgent are identical.
const formAgentOptionContentStart = 6

// formPersonaOptionContentStart is the screen Y offset of the first persona
// option within the sidebar for stepPersona:
//
//   line 0: "New project  4/5"
//   line 1: (blank)
//   line 2: "Name   ..."
//   line 3: "Repo   ..."
//   line 4: "Agent  ..."
//   line 5: (blank)
//   line 6: "Persona"
//   line 7+i: persona option i
const formPersonaOptionContentStart = 7

// formApprovalOptionContentStart shifts down by 1 because the
// stepAutoApprove now has 4 done lines + 1 blank + label = line 7 for label,
// line 8 for first option:
//
//   line 0: "New project  5/5"
//   line 1: (blank)
//   line 2: "Name     ..."
//   line 3: "Repo     ..."
//   line 4: "Agent    ..."
//   line 5: "Persona  ..."
//   line 6: (blank)
//   line 7: "Auto-approve permissions"
//   line 8+i: approval option i
const formApprovalOptionContentStart = 8   // was 7
```

### 3.11 selectPersona Helper

```go
// selectPersona sets the persona selection by index (used for mouse clicks).
// Skips divider entries.
func (m *formModel) selectPersona(idx int) {
    if idx >= 0 && idx < len(m.personaOptions) && !m.personaOptions[idx].isDivider {
        m.personaIndex = idx
    }
}
```

### 3.12 Mouse Click Handling in Sidebar (for form)

In `sidebarModel.handleClick()`, add persona option click handling in the `sidebarForm` case:

```go
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
    // NEW: In form step 4, clicking a persona option selects it.
    if m.form.step == stepPersona {
        for i, opt := range m.form.personaOptions {
            if opt.isDivider {
                continue
            }
            optionY := sidebarTopPad + formPersonaOptionContentStart + i
            if y == optionY {
                m.form.selectPersona(i)
                return m, nil
            }
        }
    }
    // In form step 5, clicking an approval level option selects it.
    if m.form.step == stepAutoApprove {
        for i := range approvalOptions {
            optionY := sidebarTopPad + formApprovalOptionContentStart + i
            if y == optionY {
                m.form.selectApproval(i)
                return m, nil
            }
        }
    }
```

---

## 4. Sidebar Changes (`internal/tui/sidebar.go`)

### 4.1 New Sidebar Modes

```go
type sidebarMode int

const (
    sidebarNormal        sidebarMode = iota
    sidebarForm
    sidebarConfirmDelete
    sidebarPersonaSelect               // NEW: inline persona picker
    sidebarConfirmReset                // NEW: confirm session restart
)
```

### 4.2 sidebarModel Struct Changes

```go
type sidebarModel struct {
    projects       []config.Project
    states         map[string]SessionState
    openTabs       map[string]bool
    selected       int
    focused        bool
    height         int
    contentWidth   int
    dragging       bool
    mode           sidebarMode
    form           formModel
    animFrame      int
    customPersonas []config.CustomPersona  // NEW: from cfg.Personas
    personaOptions []personaOption      // NEW: built from builtins + custom
    personaIndex   int                  // NEW: selection index in persona picker
    pendingPersona config.PersonaType   // NEW: chosen persona awaiting restart confirm
}
```

### 4.3 newSidebarModel Changes

```go
func newSidebarModel(projects []config.Project, contentWidth int, customPersonas []config.CustomPersona) sidebarModel {
    states := make(map[string]SessionState)
    for _, p := range projects {
        states[p.Name] = StateIdle
    }
    return sidebarModel{
        projects:       projects,
        states:         states,
        openTabs:       make(map[string]bool),
        selected:       0,
        focused:        false,
        contentWidth:   contentWidth,
        mode:           sidebarNormal,
        customPersonas: customPersonas,
    }
}
```

This change cascades to `NewApp()` in `app.go`, which must pass `cfg.Personas` when constructing the sidebar model.

### 4.4 State Machine Transition Table

| Current Mode | Trigger | Next Mode | Side Effect |
|---|---|---|---|
| `sidebarNormal` | `p` key | `sidebarPersonaSelect` | Build `personaOptions`, set `personaIndex` to current project's persona |
| `sidebarNormal` | `P` key | `sidebarNormal` | Emit `SystemTabRequestMsg{Name: "Persona Manager", Args: ["persona", "manage"]}` |
| `sidebarPersonaSelect` | `Enter` key | `sidebarConfirmReset` if project has open sessions; `sidebarNormal` otherwise | Set `pendingPersona`; emit `PersonaChangeRequestMsg` if no open sessions |
| `sidebarPersonaSelect` | `Esc` key | `sidebarNormal` | Discard selection |
| `sidebarPersonaSelect` | `j`/`k`/Down/Up | (stays) | Move `personaIndex`, skipping dividers |
| `sidebarConfirmReset` | `y` key | `sidebarNormal` | Emit `PersonaChangeRequestMsg{ProjectName, pendingPersona}` |
| `sidebarConfirmReset` | `n`/`Esc` key | `sidebarNormal` | Discard `pendingPersona` |

### 4.5 handleKey Changes

The `default` (sidebarNormal) case gains two new keybindings:

```go
case isRuneKey(msg, 'p'):
    // Open inline persona picker for the selected project.
    if len(m.projects) > 0 && m.selected < len(m.projects) {
        m.personaOptions = buildPersonaOptions(m.customPersonas)
        // Pre-select current persona.
        current := m.projects[m.selected].Persona
        m.personaIndex = 0
        for i, opt := range m.personaOptions {
            if opt.persona == current {
                m.personaIndex = i
                break
            }
        }
        m.mode = sidebarPersonaSelect
    }
    return m, nil

case isRuneKey(msg, 'P'):
    // Open Persona Manager as a system tab.
    return m, func() tea.Msg {
        return SystemTabRequestMsg{
            Name: "Persona Manager",
            Args: []string{"persona", "manage"},
        }
    }
```

Two new mode cases in `handleKey`:

```go
case sidebarPersonaSelect:
    switch {
    case isRuneKey(msg, 'j'), msg.Type == tea.KeyDown:
        m.personaIndex = m.nextSelectablePersonaIndex(m.personaIndex, +1)
        return m, nil
    case isRuneKey(msg, 'k'), msg.Type == tea.KeyUp:
        m.personaIndex = m.nextSelectablePersonaIndex(m.personaIndex, -1)
        return m, nil
    case msg.Type == tea.KeyEnter:
        if m.selected < len(m.projects) {
            selected := m.personaOptions[m.personaIndex]
            proj := m.projects[m.selected]
            if selected.persona == proj.Persona {
                // No change -- just close the picker.
                m.mode = sidebarNormal
                return m, nil
            }
            m.pendingPersona = selected.persona
            // If the project has open sessions, confirm restart.
            if m.openTabs[proj.Name] {
                m.mode = sidebarConfirmReset
                return m, nil
            }
            // No open sessions -- apply immediately.
            m.mode = sidebarNormal
            name := proj.Name
            persona := m.pendingPersona
            return m, func() tea.Msg {
                return PersonaChangeRequestMsg{
                    ProjectName: name,
                    NewPersona:  persona,
                }
            }
        }
        m.mode = sidebarNormal
        return m, nil
    case msg.Type == tea.KeyEscape:
        m.mode = sidebarNormal
        return m, nil
    }
    return m, nil

case sidebarConfirmReset:
    switch {
    case isRuneKey(msg, 'y'):
        m.mode = sidebarNormal
        if m.selected < len(m.projects) {
            name := m.projects[m.selected].Name
            persona := m.pendingPersona
            return m, func() tea.Msg {
                return PersonaChangeRequestMsg{
                    ProjectName: name,
                    NewPersona:  persona,
                }
            }
        }
        return m, nil
    case isRuneKey(msg, 'n'), msg.Type == tea.KeyEscape:
        m.mode = sidebarNormal
        return m, nil
    }
    return m, nil
```

Helper for sidebar persona navigation (mirroring the form's version):

```go
// nextSelectablePersonaIndex returns the next non-divider index in direction
// dir (+1 or -1). Returns current if no move is possible.
func (m sidebarModel) nextSelectablePersonaIndex(current, dir int) int {
    next := current + dir
    for next >= 0 && next < len(m.personaOptions) {
        if !m.personaOptions[next].isDivider {
            return next
        }
        next += dir
    }
    return current
}
```

### 4.6 Mouse Click Handling for New Modes

In `handleClick`, add cases for the new modes:

```go
case sidebarPersonaSelect:
    // Persona picker starts at sidebarTopPad + sidebarTitleRows + 2
    // (project header + blank line).
    // Layout:
    //   line 0 (topPad=1): padding
    //   line 1: "Change persona for: <name>"
    //   line 2: (blank)
    //   line 3+i: persona option i
    pickerStart := sidebarTopPad + 2
    for i, opt := range m.personaOptions {
        if opt.isDivider {
            continue
        }
        if y == pickerStart+i {
            m.personaIndex = i
            return m, nil
        }
    }
```

### 4.7 Project Display: Persona Label in Agent Line

The agent display line currently shows `claude | working` or `opencode`. The persona label is inserted between agent name and state, using the pipe separator.

In the **selected project** block (lines rendering `agentLine`):

```go
// Current:
agentLine = "  " + agentDisplayName(p.Agent) + " · " + m.stateLabel(p.Name)

// New (with persona):
agentLine = "  " + agentDisplayName(p.Agent)
if p.Persona != "" {
    agentLine += " · " + personaDisplayLabel(p.Persona)
}
agentLine += " · " + m.stateLabel(p.Name)
```

For **unselected projects with open tabs**:

```go
// Current:
agentLine = agentDisplayName(p.Agent) + " · " + m.stateLabel(p.Name)

// New (with persona):
agentLine = agentDisplayName(p.Agent)
if p.Persona != "" {
    agentLine += " · " + personaDisplayLabel(p.Persona)
}
agentLine += " · " + m.stateLabel(p.Name)
```

For **unselected projects without open tabs**:

```go
// Current:
agentLine = agentDisplayName(p.Agent)

// New (with persona):
agentLine = agentDisplayName(p.Agent)
if p.Persona != "" {
    agentLine += " · " + personaDisplayLabel(p.Persona)
}
```

The `personaDisplayLabel` function:

```go
// personaDisplayLabel returns a short label for sidebar rendering.
// Truncates to maxPersonaDisplayLen to prevent overflow in narrow sidebars.
const maxPersonaDisplayLen = 8

func personaDisplayLabel(p config.PersonaType) string {
    s := string(p)
    if len(s) > maxPersonaDisplayLen {
        return s[:maxPersonaDisplayLen-1] + "\u2026" // ellipsis
    }
    return s
}
```

### 4.8 View() Changes for New Modes

The `View()` method needs to handle the persona picker and confirm-reset overlays. These replace the normal project list when active:

```go
func (m sidebarModel) View() string {
    var b strings.Builder

    switch m.mode {
    case sidebarForm:
        b.WriteString(m.form.View())

    case sidebarPersonaSelect:
        // Inline persona picker.
        projName := ""
        if m.selected < len(m.projects) {
            projName = m.projects[m.selected].Name
        }
        b.WriteString(formTitleStyle.Render("Change persona"))
        b.WriteString("\n")
        b.WriteString(formHintStyle.Render(projName))
        b.WriteString("\n\n")
        for i, opt := range m.personaOptions {
            if opt.isDivider {
                b.WriteString(formHintStyle.Render("  ──────────"))
                b.WriteString("\n")
                continue
            }
            // Truncate label+description to fit sidebar width.
            // Inner width = contentWidth - 2 (padding) - 4 (prefix "▸ ").
            maxLen := m.contentWidth - 6
            if maxLen < 10 {
                maxLen = 10
            }
            line := opt.label
            if opt.description != "" && len(line)+2+len(opt.description) <= maxLen {
                line += "  " + opt.description
            }
            if len(line) > maxLen {
                line = line[:maxLen-1] + "\u2026"
            }

            if i == m.personaIndex {
                b.WriteString(formSelectedStyle.Render("▸ " + line))
            } else {
                b.WriteString(formOptionStyle.Render("  " + line))
            }
            b.WriteString("\n")
        }
        b.WriteString("\n")
        b.WriteString(formHintStyle.Render("  j/k select  Enter confirm  Esc cancel"))

    case sidebarConfirmReset:
        projName := ""
        if m.selected < len(m.projects) {
            projName = m.projects[m.selected].Name
        }
        b.WriteString(confirmStyle.Render(
            fmt.Sprintf("Change %s persona to %q?\nSessions will restart. (y/n)",
                projName, string(m.pendingPersona)),
        ))

    default:
        // ... existing sidebarNormal + sidebarConfirmDelete rendering ...
        // (identical to current code, no changes needed)
    }

    // ... existing style/container wrapping (identical) ...
}
```

**Implementation note**: The existing `View()` uses an `if/else` for `sidebarForm` vs everything else. This should be refactored into a `switch m.mode` with the existing `sidebarConfirmDelete` overlay remaining inside the `default` case (it renders after the project list, not instead of it). The new `sidebarPersonaSelect` and `sidebarConfirmReset` modes replace the entire content area, so they get their own top-level cases.

---

## 5. App Integration (`internal/tui/app.go`)

### 5.1 NewApp Changes

Pass custom personas to sidebar constructor:

```go
// In NewApp():
sidebar: newSidebarModel(cfg.Projects, defaultSidebarWidth, cfg.Personas),
```

### 5.2 ProjectAddedMsg Handler Changes

The current handler saves config and starts the session in parallel. The new flow sequences a persona file write between config save and session start:

```go
case ProjectAddedMsg:
    project := msg.Project
    a.cfg.Projects = append(a.cfg.Projects, project)
    a.sidebar.projects = a.cfg.Projects
    a.sidebar.states[project.Name] = StateIdle
    a.sidebar.selected = len(a.cfg.Projects) - 1
    a.sidebar.mode = sidebarNormal

    a.statusbar = newStatusBarModel(a.cfg.Projects)
    for id, state := range a.sessionStates {
        a.statusbar.states[id] = state
    }

    a.active = len(a.cfg.Projects) - 1
    a.focus = focusTerminal
    a.sidebar.focused = false
    a.terminal.focused = true

    // Save config first. If the project has a persona, write the persona
    // file before starting the session (agent needs the file to exist).
    cmds = append(cmds, a.saveConfigCmd())
    if project.Persona != "" && project.Persona != config.PersonaNone {
        cmds = append(cmds, a.writePersonaCmd(project))
        // Session start is deferred to PersonaWrittenMsg handler.
    } else {
        cmds = append(cmds, a.startSessionCmd(project, true))
    }
    return a, tea.Batch(cmds...)
```

### 5.3 PersonaWrittenMsg Handler

```go
case PersonaWrittenMsg:
    if msg.Err != nil {
        logging.Error("persona write failed",
            "project", msg.ProjectName,
            "err", msg.Err,
        )
        // Still start the session -- the agent will work without the persona file.
    } else {
        logging.Info("persona file written", "project", msg.ProjectName)
    }

    // If this was triggered by ProjectAddedMsg, start the session now.
    // Find the project and start it.
    for _, p := range a.cfg.Projects {
        if p.Name == msg.ProjectName {
            cmds = append(cmds, a.startSessionCmd(p, true))
            break
        }
    }
    return a, tea.Batch(cmds...)
```

**Sequencing concern**: When `PersonaWrittenMsg` arrives, how does the handler know whether to start a session (post-add flow) vs. restart sessions (persona change flow)? The distinction is:

- **Post-add**: The project has no open sessions yet. `a.mgr.GetSessionsByProject(msg.ProjectName)` returns empty. Start a new session.
- **Persona change**: The project may have sessions that were already stopped by the `PersonaChangeRequestMsg` handler. The handler restarts them.

Refined handler:

```go
case PersonaWrittenMsg:
    if msg.Err != nil {
        logging.Error("persona write failed",
            "project", msg.ProjectName,
            "err", msg.Err,
        )
    } else {
        logging.Info("persona file written", "project", msg.ProjectName)
    }

    // Find the project.
    for _, p := range a.cfg.Projects {
        if p.Name == msg.ProjectName {
            // Start a session. The --continue flag picks up prior context.
            cmds = append(cmds, a.startSessionCmd(p, true))
            break
        }
    }
    return a, tea.Batch(cmds...)
```

### 5.4 PersonaChangeRequestMsg Handler

This handler updates the config, stops all sessions for the project, saves the config, and writes the persona file. Session restart happens in the `PersonaWrittenMsg` handler.

```go
case PersonaChangeRequestMsg:
    // Update the project's persona in config.
    for i := range a.cfg.Projects {
        if a.cfg.Projects[i].Name == msg.ProjectName {
            a.cfg.Projects[i].Persona = msg.NewPersona
            break
        }
    }
    a.sidebar.projects = a.cfg.Projects

    // Stop all existing sessions for this project.
    for _, s := range a.mgr.GetSessionsByProject(msg.ProjectName) {
        a.removeTab(s.ID)
        a.mgr.StopSession(s.ID)
        delete(sessionChannels, s.ID)
        delete(a.sessionStates, s.ID)
        delete(a.stateStickUntil, s.ID)
        delete(a.autoApproveRuns, s.ID)
        delete(a.statusbar.states, s.ID)
    }
    a.sidebar.openTabs[msg.ProjectName] = false

    // Save config, then write persona file (which triggers session restart).
    cmds = append(cmds, a.saveConfigCmd())

    // Find the project for writePersonaCmd.
    for _, p := range a.cfg.Projects {
        if p.Name == msg.ProjectName {
            if p.Persona != "" && p.Persona != config.PersonaNone {
                cmds = append(cmds, a.writePersonaCmd(p))
            } else {
                // No persona -- start session immediately (no file to write).
                cmds = append(cmds, a.startSessionCmd(p, true))
            }
            break
        }
    }
    return a, tea.Batch(cmds...)
```

### 5.5 SystemTabExitedMsg Extension

When the Persona Manager system tab exits, reload `cfg.Personas`:

```go
case SystemTabExitedMsg:
    configPath := config.DefaultConfigPath()
    freshCfg := config.LoadOrDefault(configPath)
    a.cfg.Telegram = freshCfg.Telegram

    // NEW: Reload custom personas.
    a.cfg.Personas = freshCfg.Personas
    a.sidebar.customPersonas = freshCfg.Personas

    return a, nil
```

### 5.6 writePersonaCmd Helper

```go
// writePersonaCmd returns a tea.Cmd that writes the persona configuration
// file into the project's repo directory and emits a PersonaWrittenMsg.
func (a *App) writePersonaCmd(project config.Project) tea.Cmd {
    p := project // capture for closure
    return func() tea.Msg {
        err := persona.WritePersonaSection(p.Repo, p.Agent, p.Persona, a.cfg.Personas)
        return PersonaWrittenMsg{
            ProjectName: p.Name,
            Err:         err,
        }
    }
}
```

This requires importing `"github.com/openconductorhq/openconductor/internal/persona"` in `app.go`.

---

## 6. Status Bar (`internal/tui/statusbar.go`)

### 6.1 Keybind Hint Changes

Add persona hints to the sidebar-focused hint list:

```go
} else if m.sidebarFocused {
    hints = []struct{ key, label string }{
        {"^S", "terminal"},
        {"j/k", "navigate"},
        {"^j/k", "tab"},
        {"n", "new instance"},
        {"a", "add"},
        {"d", "delete"},
        {"p", "persona"},       // NEW
        {"P", "manage"},        // NEW
        {"Ctrl+C", "exit"},
    }
```

**Width concern**: The status bar renders all hints on a single line. Adding two more hints increases the left-side content by approximately 22 visible characters (`p persona  P manage`). At the default sidebar+terminal width (minimum 60 columns from `minAppWidth`), this fits. However, if the window is narrow, hints will crowd the right-side status. No truncation logic currently exists and none is added here -- the existing design accepts horizontal overflow gracefully since `lipgloss.Width` handles ANSI-aware measurement and the gap calculation already clamps to 0.

---

## 7. Styles (`internal/tui/styles.go`)

No new style definitions are required. The persona picker reuses existing styles:

| Element | Style |
|---|---|
| Persona picker title | `formTitleStyle` |
| Persona picker hint (project name) | `formHintStyle` |
| Selected persona option | `formSelectedStyle` |
| Unselected persona option | `formOptionStyle` |
| Divider line | `formHintStyle` |
| Confirm-reset text | `confirmStyle` |
| Persona label in agent line | No dedicated style -- uses the same foreground as surrounding text (`colorMuted` for agent line, `colorFg` for selected) |

If a distinct visual treatment is desired for the persona label in the agent line (e.g., a different color), a style can be added later. For MVP, the persona text inherits the agent line style.

---

## 8. Message Flow Diagrams

### 8.1 Add Project with Persona

```
User completes form (stepAutoApprove → Enter)
  │
  ├─ formModel.advance() emits ProjectAddedMsg{Project{Persona: "vibe", ...}}
  │
  ▼
App.Update(ProjectAddedMsg)
  │
  ├─ Appends project to cfg.Projects
  ├─ Updates sidebar, statusbar
  ├─ Emits saveConfigCmd()          ─── runs async ──► ConfigSavedMsg (logged)
  ├─ project.Persona != "" ?
  │     YES: emits writePersonaCmd(project) ─── runs async ──┐
  │     NO:  emits startSessionCmd(project, true) ──► sessionStartedMsg
  │                                                           │
  ▼                                                           ▼
App.Update(PersonaWrittenMsg)
  │
  ├─ Logs success/failure
  ├─ Finds project in cfg.Projects
  ├─ Emits startSessionCmd(project, true) ──► sessionStartedMsg
  │
  ▼
App.Update(sessionStartedMsg)
  │
  └─ Normal session startup (addTab, syncTerminal, waitForOutput, loadHistory)
```

### 8.2 Change Persona for Existing Project

```
User presses 'p' in sidebar (sidebarNormal)
  │
  ├─ Sidebar enters sidebarPersonaSelect mode
  │
  ▼
User navigates with j/k, presses Enter
  │
  ├─ Selected persona != current persona?
  │     NO:  sidebar returns to sidebarNormal (no-op)
  │     YES: project has open sessions?
  │            NO:  emit PersonaChangeRequestMsg, return to sidebarNormal
  │            YES: sidebar enters sidebarConfirmReset, stores pendingPersona
  │
  ▼ (if confirm needed)
User presses 'y'
  │
  ├─ Sidebar emits PersonaChangeRequestMsg{ProjectName, pendingPersona}
  ├─ Sidebar returns to sidebarNormal
  │
  ▼
App.Update(PersonaChangeRequestMsg)
  │
  ├─ Updates cfg.Projects[i].Persona
  ├─ Stops all sessions for the project (removeTab + StopSession + cleanup)
  ├─ Marks sidebar.openTabs[name] = false
  ├─ Emits saveConfigCmd()
  ├─ Persona != "" ?
  │     YES: emits writePersonaCmd(project) ──► PersonaWrittenMsg ──► startSessionCmd
  │     NO:  emits startSessionCmd(project, true) directly
  │
  ▼
Sessions restart with the new persona file in place.
```

### 8.3 Persona Manager System Tab

```
User presses 'P' in sidebar
  │
  ├─ Sidebar emits SystemTabRequestMsg{Name: "Persona Manager", Args: ["persona", "manage"]}
  │
  ▼
App.Update(SystemTabRequestMsg)
  │
  ├─ Spawns system tab (existing logic)
  │
  ▼
User exits persona manager
  │
  ├─ Process exits → sessionExitedMsg → SystemTabExitedMsg
  │
  ▼
App.Update(SystemTabExitedMsg)
  │
  ├─ Reloads config from disk
  ├─ Updates a.cfg.Telegram (existing)
  ├─ Updates a.cfg.Personas (NEW)
  ├─ Updates a.sidebar.customPersonas (NEW)
  │
  ▼
Next time user opens persona picker (p key), the updated custom personas appear.
```

---

## 9. Edge Cases

### 9.1 Empty Custom Persona List

When `cfg.Personas` is empty (the common case), `buildPersonaOptions` returns only the 4 built-in options. No divider line is rendered. The form and sidebar picker look identical -- only the built-in list.

### 9.2 Very Long Persona Labels

**In the form**: The `View()` renders `fmt.Sprintf("%-6s  %s", opt.label, opt.description)`. Long custom persona names will break the column alignment. The `%-6s` format pads to 6 characters but does not truncate. For the form, this is acceptable because the sidebar has horizontal scrolling within its container and the form is a one-time interaction.

**In the sidebar agent line**: `personaDisplayLabel` truncates to `maxPersonaDisplayLen` (8 characters) with an ellipsis. For example, `"enterprise-prod"` becomes `"enterpr..."`. This ensures the agent line fits within `innerWidth` even at `minSidebarWidth` (20). At minimum width: `"  claude · enterpr... · working"` = 34 characters, which exceeds `innerWidth` of 18. The `projectActiveStyle.Width(innerWidth - 1)` rendering will clip the line. This is the existing behavior for long project names and is acceptable.

**In the sidebar persona picker**: The picker truncates `label + description` to `maxLen = contentWidth - 6`, adding an ellipsis if exceeded. At `minSidebarWidth = 20`, `maxLen = 14`, which accommodates all built-in labels (`"None"`, `"Vibe"`, `"POC"`, `"Scale"`) but truncates descriptions. This is acceptable -- the picker prioritizes fitting within the sidebar.

### 9.3 Sidebar Width Constraints

The sidebar has a minimum width of 20 (`minSidebarWidth`). The persona-related content is designed for this constraint:

| Element | Width at min (18 inner) |
|---|---|
| Agent line: `"claude · vibe"` | 14 chars -- fits |
| Agent line: `"claude · vibe · working"` | 24 chars -- clipped by `Width()` |
| Picker option: `"▸ Vibe"` | 8 chars -- fits |
| Picker option: `"▸ Vibe  Fast iteration..."` | Truncated to fit |
| Confirm dialog text | Wraps naturally within `confirmStyle` |

For the agent line overflow at minimum width, the existing `projectActiveStyle.Width(innerWidth - 1).Render(content)` handles clipping. The state label (`working`, `idle`, etc.) may be partially hidden. This matches the existing behavior for long project names -- the user can drag-resize the sidebar wider.

### 9.4 Persona Change When No Sessions Exist

If the user selects a project with no open tabs and presses `p` to change persona, the picker applies the change immediately (no restart confirmation needed). The `PersonaChangeRequestMsg` handler detects zero sessions and simply updates the config + writes the persona file. No session start is triggered because the project has no open tabs.

Refined logic in `PersonaChangeRequestMsg` handler:

```go
sessions := a.mgr.GetSessionsByProject(msg.ProjectName)
if len(sessions) > 0 {
    // Stop and clean up sessions (existing code).
    for _, s := range sessions {
        a.removeTab(s.ID)
        a.mgr.StopSession(s.ID)
        // ... cleanup maps ...
    }
    a.sidebar.openTabs[msg.ProjectName] = false
}

cmds = append(cmds, a.saveConfigCmd())

proj := /* find project */
if proj.Persona != "" && proj.Persona != config.PersonaNone {
    cmds = append(cmds, a.writePersonaCmd(proj))
    // PersonaWrittenMsg will start session only if previous sessions existed.
} else if len(sessions) > 0 {
    cmds = append(cmds, a.startSessionCmd(proj, true))
}
```

This means `PersonaWrittenMsg` must also know whether to start a session. The simplest approach: always start a session in `PersonaWrittenMsg` only if there are currently no sessions for the project (the handler already stopped them). Since `PersonaWrittenMsg` runs after `PersonaChangeRequestMsg` has already stopped sessions, `GetSessionsByProject` returns empty, and the handler starts a fresh one. For the "no sessions existed" case, `PersonaWrittenMsg` should not start a session.

To disambiguate, add a `StartSession bool` field to `PersonaWrittenMsg`:

```go
type PersonaWrittenMsg struct {
    ProjectName  string
    Err          error
    StartSession bool // true if a session should be started after writing
}
```

The `writePersonaCmd` helper gains a parameter:

```go
func (a *App) writePersonaCmd(project config.Project, startSession bool) tea.Cmd {
    p := project
    return func() tea.Msg {
        err := persona.WritePersonaSection(p.Repo, p.Agent, p.Persona, a.cfg.Personas)
        return PersonaWrittenMsg{
            ProjectName:  p.Name,
            Err:          err,
            StartSession: startSession,
        }
    }
}
```

Callers:
- `ProjectAddedMsg` handler: `writePersonaCmd(project, true)` -- always start session after add.
- `PersonaChangeRequestMsg` handler: `writePersonaCmd(proj, len(sessions) > 0)` -- only restart if sessions existed.

Updated `PersonaWrittenMsg` handler:

```go
case PersonaWrittenMsg:
    if msg.Err != nil {
        logging.Error("persona write failed", "project", msg.ProjectName, "err", msg.Err)
    } else {
        logging.Info("persona file written", "project", msg.ProjectName)
    }
    if msg.StartSession {
        for _, p := range a.cfg.Projects {
            if p.Name == msg.ProjectName {
                cmds = append(cmds, a.startSessionCmd(p, true))
                break
            }
        }
    }
    return a, tea.Batch(cmds...)
```

### 9.5 Rapid Persona Changes

If the user changes persona multiple times before the first `PersonaWrittenMsg` arrives, each `PersonaChangeRequestMsg` stops existing sessions and starts a new write. The write commands run concurrently but write to the same file. The last write wins, which is the correct final state. The `PersonaWrittenMsg` handlers each try to start a session. The session manager handles duplicate starts gracefully (it assigns unique IDs). However, this could result in multiple tabs opening.

**Mitigation**: The `PersonaChangeRequestMsg` handler already stops all sessions before starting new ones. If a `PersonaWrittenMsg` from a previous change arrives and starts a session, the next `PersonaChangeRequestMsg` will stop it. The worst case is a brief flicker of an extra tab. This is acceptable for an uncommon edge case.

### 9.6 Persona "None" Selection

When `PersonaNone` (`""`) is selected, the TUI still calls `writePersonaCmd` which invokes `persona.WritePersonaSection` with `PersonaNone`. The writer handles removal: it finds and removes the marker block from the instruction file, and deletes the file if it becomes empty. This ensures stale persona markers are cleaned up when the user switches to None.

---

## 10. Files Modified

| File | Nature of Change |
|---|---|
| `internal/tui/messages.go` | Add `PersonaChangeRequestMsg`, `PersonaWrittenMsg` |
| `internal/tui/form.go` | Add `stepPersona`, `personaOption` type, `personaIndex`/`personaOptions` fields, view/update/advance logic, mouse constants |
| `internal/tui/sidebar.go` | Add `sidebarPersonaSelect`/`sidebarConfirmReset` modes, `customPersonas`/`personaOptions`/`personaIndex`/`pendingPersona` fields, key handlers, persona picker view, persona label in agent line, `newSidebarModel` signature change |
| `internal/tui/app.go` | `PersonaChangeRequestMsg` handler, `PersonaWrittenMsg` handler, modified `ProjectAddedMsg` handler, `SystemTabExitedMsg` extension, `writePersonaCmd` helper, `NewApp` passes personas to sidebar |
| `internal/tui/statusbar.go` | Add `p persona` and `P manage` hints |
| `internal/tui/styles.go` | No changes (reuses existing styles) |
| `internal/config/config.go` | (Out of scope) Add `PersonaType`, `CustomPersona`, `Persona` field on `Project`, `Personas` field on `Config` |
| `internal/persona/` | (Out of scope) `WritePersonaSection`, `Resolve`, `InstructionText`, `TargetFile`, `DefaultApproval` functions |

---

## 11. Testing Plan

### 11.1 Form Tests (`internal/tui/form_test.go`)

| Test | Description |
|---|---|
| `TestFormStepCount` | Verify `stepIndicator()` returns `"N/5"` for all 5 steps. |
| `TestFormPersonaNavigation` | Send j/k keys at `stepPersona`, verify `personaIndex` changes. Verify dividers are skipped. |
| `TestFormPersonaAdvance` | Advance through `stepPersona`, verify `approvalIndex` is pre-set based on persona default. |
| `TestFormPersonaAdvanceCustom` | Build form with custom personas, select a custom one, verify it appears in `ProjectAddedMsg`. |
| `TestFormPersonaDividerSkip` | With custom personas (divider present), verify j/k never land on the divider index. |
| `TestFormEmptyCustomPersonas` | Build form with empty custom list, verify only 4 built-in options appear, no divider. |
| `TestFormPersonaMouseClick` | Simulate mouse click at `formPersonaOptionContentStart + i`, verify selection changes. |
| `TestFormApprovalMouseClickShifted` | Verify mouse clicks at the new `formApprovalOptionContentStart` (8, was 7) hit the correct options. |

### 11.2 Sidebar Tests (`internal/tui/sidebar_test.go`)

| Test | Description |
|---|---|
| `TestSidebarPersonaPickerOpen` | Press `p` in normal mode, verify mode becomes `sidebarPersonaSelect` with correct `personaIndex` for current project. |
| `TestSidebarPersonaPickerNavigation` | In `sidebarPersonaSelect`, send j/k, verify index moves and skips dividers. |
| `TestSidebarPersonaPickerSelect_NoSessions` | Select a different persona when project has no open tabs. Verify `PersonaChangeRequestMsg` emitted, mode returns to `sidebarNormal`. |
| `TestSidebarPersonaPickerSelect_WithSessions` | Select a different persona when project has open tabs. Verify mode becomes `sidebarConfirmReset`. |
| `TestSidebarPersonaConfirmRestart_Yes` | In `sidebarConfirmReset`, press `y`. Verify `PersonaChangeRequestMsg` emitted. |
| `TestSidebarPersonaConfirmRestart_No` | In `sidebarConfirmReset`, press `n` or `Esc`. Verify return to `sidebarNormal`, no message emitted. |
| `TestSidebarPersonaPickerEscape` | In `sidebarPersonaSelect`, press `Esc`. Verify return to `sidebarNormal`. |
| `TestSidebarPersonaSameAsCurrentNoop` | Select the same persona that's already set. Verify no message, mode returns to `sidebarNormal`. |
| `TestSidebarPersonaManagerKey` | Press `P` in normal mode. Verify `SystemTabRequestMsg` with `"Persona Manager"` name and `["persona", "manage"]` args. |
| `TestSidebarPersonaLabel` | Render sidebar with project having `Persona: "vibe"`. Verify output contains `"vibe"` in the agent line. |
| `TestSidebarPersonaLabelTruncation` | Set persona to `"my-very-long-persona-name"`. Verify truncation with ellipsis. |
| `TestSidebarPersonaLabelEmpty` | Set persona to `""`. Verify no persona segment in agent line. |

### 11.3 App Tests (integration-level, `internal/tui/app_test.go`)

| Test | Description |
|---|---|
| `TestProjectAddedWithPersona` | Emit `ProjectAddedMsg` with persona set. Verify `writePersonaCmd` is called (not `startSessionCmd`). Then emit `PersonaWrittenMsg` and verify session starts. |
| `TestProjectAddedWithoutPersona` | Emit `ProjectAddedMsg` with `PersonaNone`. Verify `startSessionCmd` called directly (no persona write). |
| `TestPersonaChangeRestartsSessions` | Emit `PersonaChangeRequestMsg`. Verify all sessions stopped, config updated, `writePersonaCmd` issued. |
| `TestPersonaChangeNoSessionsNoRestart` | Emit `PersonaChangeRequestMsg` for project with no open sessions. Verify no session start. |
| `TestSystemTabExitedReloadsPersonas` | Emit `SystemTabExitedMsg`. Verify `cfg.Personas` and `sidebar.customPersonas` updated from fresh config. |

---

## 12. Implementation Order

Recommended sequence for implementation, designed to allow incremental testing:

1. **Config types** (out of scope, but prerequisite): `config.PersonaType`, `config.CustomPersona`, `Project.Persona`, `Config.Personas`.
2. **Messages**: Add `PersonaChangeRequestMsg` and `PersonaWrittenMsg` to `messages.go`. Pure type additions, no behavior change.
3. **Form step**: Add `stepPersona` constant, `personaOption` type, modify `formModel`, `newFormModel`, `advance`, `View`, step indicator. Test with form unit tests.
4. **Sidebar modes**: Add `sidebarPersonaSelect` and `sidebarConfirmReset` modes, state fields, `handleKey` cases, `View` rendering. Test with sidebar unit tests.
5. **Sidebar agent line**: Add persona label rendering (`personaDisplayLabel`). Test with view snapshot tests.
6. **App handlers**: Add `PersonaChangeRequestMsg`, `PersonaWrittenMsg` handlers, modify `ProjectAddedMsg` handler, `SystemTabExitedMsg`, add `writePersonaCmd`. Test with app integration tests.
7. **Status bar**: Add keybind hints. Minimal change, test visually.
8. **Sidebar mouse**: Add click handling for persona picker. Test with mouse event tests.
