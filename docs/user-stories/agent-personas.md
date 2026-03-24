# User Story: Agent Personas (Preset Profiles)

**Story ID:** US-PERSONA-001
**Feature:** Equip a project with a persona preset that shapes how its AI coding agent behaves
**Priority:** High
**Epic:** Agent Customization
**Status:** Draft
**Author:** Product Manager
**Date:** 2026-03-24

---

## 1. Problem Statement

### The Problem

Users managing multiple AI coding agents through OpenConductor currently have no way to differentiate *how* an agent approaches its work. Every agent launches with the same generic instructions regardless of whether the user is hacking on a weekend prototype, building a proof-of-concept demo, or working on production code that will serve thousands of users.

This creates two concrete problems:

1. **Friction at the wrong moments.** A user vibing on a prototype gets interrupted by an agent that insists on writing comprehensive test suites. A user building production code gets an agent that moves fast and breaks things. The agent's behavior does not match the user's intent, forcing manual correction via repeated prompts like "skip the tests" or "actually, write tests for this."

2. **Repeated manual configuration.** Power users who know they can influence agent behavior through CLAUDE.md files or MCP server configs must manually author these files for each new project. There is no mechanism within OpenConductor to template, share, or standardize these configurations. The existing `openconductor bootstrap` CLI command writes a generic CLAUDE.md, but it is never invoked during the TUI add-project flow, and it has no concept of behavioral modes.

### Why Now

- The bootstrap system (`internal/bootstrap/`) already has Go template infrastructure, embedded templates, and language detection -- it just needs persona-aware templates and TUI integration.
- The `AgentAdapter.BootstrapFiles()` interface already returns files to seed into repos.
- The add-project form (`internal/tui/form.go`) is a clean 4-step wizard that can be extended to 5 steps.
- Users are adding more projects and running agents in parallel. The cognitive overhead of managing different behavioral expectations across concurrent agents is growing.

### What Success Looks Like

A user adds a new project, picks "Vibe" as the persona, and the agent immediately starts behaving like a rapid-prototyping partner -- moving fast, making autonomous decisions, and not asking for permission on every file edit. Another user picks "Develop at Scale" for the same repo later, and the agent shifts to TDD, thorough reviews, and cautious changes. The user never has to write a CLAUDE.md by hand.

---

## 2. User Personas Affected

### Primary: The Parallel Builder

- Runs 3-8 agents simultaneously across different projects
- Some projects are exploratory prototypes; others are production services
- Wants each agent tuned to the project's maturity level without manual config
- Currently frustrated by agents that apply the same rigor to a throwaway script and a production API

### Secondary: The Solo Developer

- Runs 1-2 agents, often switching between "exploration" and "implementation" phases on the same project
- Wants to change an agent's behavior mid-project without editing config files
- Values the ability to say "now let's get serious about this code" after prototyping

### Tertiary: The Team Lead

- Manages OpenConductor config for a team
- Wants to define custom personas (e.g., "Security Review", "Documentation Sprint") and share them
- Needs personas that map to organizational standards (linting rules, commit conventions, test coverage requirements)

---

## 3. Persona Definitions

### 3.1 Vibe

**Philosophy:** "Just build it. Move fast. Ship something."

**Behavioral characteristics:**
- Make autonomous decisions without asking for confirmation
- Prefer working code over perfect code
- Skip boilerplate tests unless the user explicitly asks
- Use simple, direct solutions over architecturally elegant ones
- Commit frequently with short messages
- Do not refactor existing code unless it blocks the current task
- Bias toward action: if two approaches exist, pick one and go

**Auto-approval recommendation:** Full (complements the "don't interrupt me" philosophy)

**CLAUDE.md instructions (Claude Code):**
- Move fast, prioritize working code over perfect code
- Make autonomous decisions -- do not ask for confirmation on implementation choices
- Skip tests unless explicitly requested
- Use the simplest approach that works
- Keep commits small and frequent
- Do not refactor code that is not directly related to the current task
- Bias toward action over discussion

**MCP servers:** None by default (minimal tooling overhead)

### 3.2 POC (Proof of Concept)

**Philosophy:** "Build a working demo with enough structure to present."

**Behavioral characteristics:**
- Write code that is readable and demonstrable
- Add basic error handling (no silent failures)
- Write a few key tests for core logic, skip edge case coverage
- Use reasonable file/package structure but do not over-architect
- Include inline comments explaining non-obvious decisions
- Create a basic README or usage example if the project lacks one
- Ask clarifying questions when requirements are ambiguous

**Auto-approval recommendation:** Safe (balance between speed and safety)

**CLAUDE.md instructions (Claude Code):**
- Build working, demonstrable code with reasonable structure
- Add basic error handling -- no silent failures, no swallowed errors
- Write tests for core logic; skip exhaustive edge case coverage
- Use clear file and package organization but do not over-engineer
- Add inline comments for non-obvious decisions
- If requirements are ambiguous, ask before assuming
- Include a basic usage example or README section if none exists

**MCP servers:** None by default (user can add project-specific tools)

### 3.3 Develop at Scale

**Philosophy:** "This code will be maintained by others and run in production."

**Behavioral characteristics:**
- Follow TDD: write failing tests first, then implement
- Consider edge cases, error paths, and concurrent access
- Write comprehensive tests including unit, integration, and table-driven tests
- Follow existing code style and patterns strictly
- Add meaningful documentation (godoc, JSDoc, docstrings) to public APIs
- Review own changes before committing -- check for security issues, performance problems, and API compatibility
- Keep PRs focused and reviewable -- one logical change per commit
- Never ignore errors or use placeholder implementations

**Auto-approval recommendation:** Off (every change should be reviewed)

**CLAUDE.md instructions (Claude Code):**
- Follow TDD: write a failing test first, then implement the minimum code to pass it
- Consider edge cases, error paths, concurrency, and resource cleanup
- Write comprehensive tests: unit tests, integration tests, table-driven tests where appropriate
- Follow existing code style, naming conventions, and architectural patterns strictly
- Add documentation to all public APIs (godoc/JSDoc/docstrings)
- Review your own changes before committing: check for security issues, performance regressions, API breaking changes
- Keep each commit to one logical change; write descriptive commit messages
- Never ignore errors, never use TODO/FIXME as an excuse to ship incomplete code
- Consider backward compatibility when modifying existing interfaces

**MCP servers:** None by default (production repos typically have their own tooling)

### 3.4 None (No Preset)

**Philosophy:** No persona-specific instructions applied. Agent uses its default behavior. Existing CLAUDE.md / config files in the repo are respected as-is.

This is the default for backward compatibility. Existing projects that were created before the persona feature will behave exactly as they do today.

### 3.5 Custom Personas

Users can define their own personas on top of the built-in ones. Custom personas are stored in `~/.openconductor/config.yaml` under a `personas` key and managed through an interactive CLI wizard (same pattern as Telegram setup).

**Custom persona fields:**
- **Name** (slug): lowercase identifier used in config and CLI (e.g., `security-review`)
- **Label**: display name shown in the form/sidebar (e.g., `Security Review`)
- **Instructions**: the markdown text written between the persona markers
- **Auto-approve suggestion**: Off, Safe, or Full

Custom personas appear alongside built-in ones in the form step and sidebar persona picker. Built-in personas cannot be edited or deleted.

---

## 4. User Flows

### 4.1 Happy Path: New Project with Persona

**Trigger:** User presses `a` in the sidebar to add a new project.

**Flow (5-step form):**

```
Step 1/5: Name
  > my-prototype
  A unique project name
  Esc cancel

Step 2/5: Repo path
  > /Users/dev/projects/my-prototype
  Absolute path to repo
  Esc cancel

Step 3/5: Agent
  > claude-code
    opencode
  j/k to select, Enter to confirm
  Esc cancel

Step 4/5: Persona
    None        No preset instructions
  > Vibe        Move fast, make bold choices, skip tests
    POC         Working demos with reasonable structure
    Scale       TDD, comprehensive tests, production quality
  j/k to select, Enter to confirm
  Esc cancel

Step 5/5: Auto-approve permissions
    Off     Notify me for all permission requests
    Safe    Auto-approve file edits and safe commands
  > Full    Auto-approve everything (use with caution)
  j/k to select, Enter to confirm
  Esc cancel
```

**On confirmation:**

1. The `ProjectAddedMsg` is extended to include the selected `Persona` field.
2. The persona is stored in `config.yaml` on the `Project` struct.
3. Before launching the agent session, OpenConductor writes persona files into the repo:
   - For Claude Code: writes/merges a `CLAUDE.md` with persona-specific instructions
   - For OpenCode: writes equivalent config (agent-specific adapter responsibility)
4. The agent launches with `--continue` as usual, now picking up the persona instructions.
5. The sidebar shows the persona as a subtle label below the agent type: `claude | vibe`

### 4.2 Happy Path: Change Persona on Existing Project

**Trigger:** User selects a project in the sidebar and presses `p` (new keybinding for "persona").

**Flow (modal or inline):**

```
Change persona for "my-prototype"

  Current: Vibe

    None        No preset instructions
    Vibe        Move fast, make bold choices, skip tests
  > POC         Working demos with reasonable structure
    Scale       TDD, comprehensive tests, production quality

  j/k to select, Enter to confirm, Esc cancel

  Warning: This will update CLAUDE.md in the repo.
  Existing custom content will be preserved.
```

**On confirmation:**

1. A confirmation dialog appears: "This will update CLAUDE.md/AGENTS.md in the repo — continue? (y/n)"
2. If confirmed, config is updated with the new persona.
3. The persona section in CLAUDE.md/AGENTS.md is regenerated (see section 5 for merge strategy).
4. If the project has an active session, the user is prompted: "Start a fresh conversation with the new persona, or keep the current conversation? (fresh/keep)"
   - **Fresh:** The current session is stopped and a new one is started (no `--continue`).
   - **Keep:** The session continues. A notice appears: "Persona updated. Changes take effect on the next agent message."
5. Config is saved to disk.

### 4.3 Alternative Flow: Project Already Has CLAUDE.md

**Trigger:** User adds a project whose repo already contains a CLAUDE.md file.

**Behavior:**

1. The form proceeds normally through all 5 steps.
2. On confirmation, OpenConductor checks for an existing CLAUDE.md.
3. If found, it **appends** a clearly delimited persona section rather than overwriting:

```markdown
<!-- existing CLAUDE.md content is preserved above -->

<!-- openconductor:persona:start -->
## OpenConductor Persona: Vibe

- Move fast, prioritize working code over perfect code
- Make autonomous decisions -- do not ask for confirmation
- Skip tests unless explicitly requested
- ...
<!-- openconductor:persona:end -->
```

4. On persona change, only the content between the markers is replaced.
5. If the user has manually edited content inside the markers, the replacement overwrites it (the markers are the contract).

### 4.4 Alternative Flow: User Selects "None" Persona

**Trigger:** User selects "None" during project creation or persona change.

**Behavior:**

1. No persona-specific content is written to CLAUDE.md.
2. If changing from a previous persona, the marked section (`openconductor:persona:start` to `openconductor:persona:end`) is **removed** from CLAUDE.md.
3. The rest of the file is left untouched.
4. The sidebar shows only the agent type with no persona label.

### 4.5 Alternative Flow: OpenCode Agent

**Trigger:** User selects OpenCode as the agent type and picks a persona.

**Behavior:**

1. The persona selection step appears identically in the form -- personas are agent-agnostic concepts.
2. The *implementation* of file writing is agent-specific:
   - OpenCode reads AGENTS.md (the agent-agnostic convention). The persona bootstrapper writes persona instructions to AGENTS.md using the same marker strategy.
   - The persona instructions content is the same; only the target filename differs per agent type.
3. Claude Code reads CLAUDE.md. OpenCode reads AGENTS.md. Both use the `<!-- openconductor:persona:start/end -->` markers.

### 4.6 Alternative Flow: CLI Bootstrap with Persona

**Trigger:** User runs `openconductor bootstrap <repo-path> --agent claude-code --persona vibe`

**Behavior:**

1. The existing `bootstrap` command gains a `--persona` flag.
2. The template data (`TemplateData`) is extended with a `Persona` field.
3. Templates render persona-specific content based on the selected persona.
4. Default persona (when `--persona` is omitted) is "none" for backward compatibility.

### 4.7 Custom Persona Management (CLI Wizard)

**Trigger:** User presses `P` (shift+p) in the sidebar, or runs `openconductor persona` from the CLI.

**Pattern:** Same as Telegram setup — spawns `openconductor persona` as a system tab in a PTY. The wizard is a pure stdin/stdout CLI program.

**Flow (main menu):**

```
OpenConductor Persona Manager

  Built-in personas:
    vibe     Move fast, skip tests, auto-approve
    poc      Working demos, basic tests
    scale    TDD, production quality, thorough

  Custom personas:
    security-review   Security-focused code review
    docs-sprint       Documentation writing mode

  Actions:
    [c] Create new persona
    [e] Edit custom persona
    [d] Delete custom persona
    [q] Quit
```

**Create flow (`c`):**

```
Create Custom Persona

  Name (slug): security-review
  Label: Security Review
  Auto-approve [off/safe/full]: off

  Instructions (end with a line containing only "END"):
  Focus on security vulnerabilities and best practices.
  - Review code for OWASP Top 10 vulnerabilities
  - Check for injection, XSS, CSRF, and auth issues
  - Validate input sanitization and output encoding
  - Flag hardcoded secrets and insecure configurations
  END

  Persona "security-review" created.
  Press Enter to return to menu.
```

**Edit flow (`e`):**

```
Edit Custom Persona

  Select persona to edit:
  > security-review   Security-focused code review
    docs-sprint       Documentation writing mode

  (Opens current values, user edits fields — same as create but pre-filled)
```

**Delete flow (`d`):**

```
Delete Custom Persona

  Select persona to delete:
  > security-review   Security-focused code review
    docs-sprint       Documentation writing mode

  Delete "security-review"? This will NOT remove persona sections
  from repos that already use it. (y/n): y

  Persona "security-review" deleted.
  Projects using this persona will show it as unknown.
```

**On exit:** The TUI reloads the `Personas` section of the config (same pattern as Telegram config reload on `SystemTabExitedMsg`).

### 4.8 Custom Persona in Project Form

**Trigger:** User adds a project and reaches the persona step.

**Behavior:** Custom personas appear below the built-in ones:

```
Step 4/5: Persona
    None        No persona instructions
    Vibe        Move fast, skip tests, auto-approve
    POC         Working demos, basic tests
    Scale       TDD, production quality, thorough
  ─ Custom ─────────────────────────────────
  > Security Review   Security-focused code review
    Docs Sprint       Documentation writing mode
  j/k to select, Enter to confirm
  Esc cancel
```

---

## 5. Technical Design Decisions

### 5.1 Where Persona Lives in Config

The `config.Project` struct gains a `Persona` field, and a new top-level `Personas` slice stores custom persona definitions:

```go
type PersonaType string

const (
    PersonaNone  PersonaType = ""
    PersonaVibe  PersonaType = "vibe"
    PersonaPOC   PersonaType = "poc"
    PersonaScale PersonaType = "scale"
)

// CustomPersona defines a user-created persona.
type CustomPersona struct {
    Name         string        `yaml:"name"`          // slug: "security-review"
    Label        string        `yaml:"label"`         // display: "Security Review"
    Instructions string        `yaml:"instructions"`  // markdown instruction text
    AutoApprove  ApprovalLevel `yaml:"auto_approve,omitempty"`
}

type Project struct {
    Name        string        `yaml:"name"`
    Repo        string        `yaml:"repo"`
    Agent       AgentType     `yaml:"agent"`
    Persona     PersonaType   `yaml:"persona,omitempty"`
    AutoApprove ApprovalLevel `yaml:"auto_approve,omitempty"`
}

type Config struct {
    Projects []Project       `yaml:"projects"`
    Personas []CustomPersona `yaml:"personas,omitempty"`  // NEW
    Telegram TelegramConfig  `yaml:"telegram,omitempty"`
}
```

This results in config.yaml entries like:

```yaml
personas:
  - name: security-review
    label: Security Review
    instructions: |
      Focus on security vulnerabilities and best practices.
      - Review code for OWASP Top 10 vulnerabilities
      - Check for injection, XSS, CSRF, and auth issues
    auto_approve: off

projects:
  - name: my-prototype
    repo: /Users/dev/projects/my-prototype
    agent: claude-code
    persona: vibe
    auto_approve: full
  - name: my-api
    repo: /Users/dev/projects/my-api
    agent: claude-code
    persona: security-review
    auto_approve: off
```

The `persona` field on a project can reference either a built-in name (`vibe`, `poc`, `scale`) or a custom persona name (`security-review`). Resolution order: built-in first, then custom.

### 5.2 How Persona Maps to Files

The `AgentAdapter` interface is **not** modified. Instead, the persona is handled at the bootstrap/session-start layer:

1. A new `PersonaBootstrapper` in `internal/bootstrap/` knows how to write persona-specific files for each agent type.
2. It is invoked from the `ProjectAddedMsg` handler in `app.go`, after saving config but before `startSessionCmd`.
3. It uses HTML-comment markers to manage its section non-destructively.
4. **Target file per agent type:**
   - Claude Code → `CLAUDE.md`
   - OpenCode → `AGENTS.md`

The `AgentAdapter.BootstrapFiles()` method continues to return the *generic* bootstrap files. Persona files are a separate concern that layer on top.

### 5.3 Instruction File Merge Strategy

The merge uses comment markers to delineate the persona-managed section in the target file (CLAUDE.md or AGENTS.md):

```
<!-- openconductor:persona:start -->
...persona content...
<!-- openconductor:persona:end -->
```

**Rules:**
- If the target file does not exist: create it with *only* the persona section.
- If the target file exists without markers: append the persona section at the end.
- If the target file exists with markers: replace content between markers.
- If persona is "None": remove the markers and their content; if the file becomes empty, delete it.
- Markers use HTML comments because both files are Markdown and HTML comments are invisible to Markdown renderers.

### 5.4 Form Step Ordering Rationale

The persona step is inserted as step 4 (after agent type, before auto-approve) because:

1. Persona options are the same regardless of agent type, so the user does not need to know the agent first. However, placing it after agent selection allows future agent-specific persona filtering.
2. Auto-approve is last because the persona selection can *suggest* a default auto-approve level (Vibe suggests Full, Scale suggests Off), reducing friction.

When the user selects a persona, the auto-approve default changes to match the persona's recommendation. The user can still override it in step 5.

### 5.5 Sidebar Display

The sidebar project entry currently shows:

```
  my-prototype
  claude | idle
```

With personas, it becomes:

```
  my-prototype
  claude | vibe | idle
```

Or when the persona is "None":

```
  my-prototype
  claude | idle
```

This uses the existing `agentDisplayName` + state label pattern with an inserted persona segment.

---

## 6. Edge Cases and Error Scenarios

### 6.1 Repo Path is Read-Only

**Scenario:** The user's repo directory is on a read-only filesystem or lacks write permissions.

**Expected behavior:**
- The persona files cannot be written.
- The project is still added to config (the config file is in `~/.openconductor/`, not the repo).
- The agent launches without persona instructions.
- A warning is displayed in the scrollback: "Warning: could not write persona files to /path/to/repo: permission denied. Agent will run without persona instructions."
- The persona is still stored in config so it can be applied later if permissions change.

### 6.2 CLAUDE.md Has Corrupted Markers

**Scenario:** The user manually edited CLAUDE.md and deleted the end marker but left the start marker.

**Expected behavior:**
- The bootstrapper detects a start marker without a matching end marker.
- It treats everything from the start marker to EOF as the persona section.
- It replaces that entire range with the new persona content (including a proper end marker).
- A log entry is written: "repaired missing persona end marker in CLAUDE.md."

### 6.3 User Manually Edits Persona Section

**Scenario:** The user modifies content between the persona markers and then changes the persona via OpenConductor.

**Expected behavior:**
- The manual edits are overwritten. The markers define the contract: content between them is managed by OpenConductor.
- This is documented in the marker comments themselves:

```markdown
<!-- openconductor:persona:start -->
<!-- This section is managed by OpenConductor. Manual edits will be overwritten. -->
## OpenConductor Persona: Vibe
...
<!-- openconductor:persona:end -->
```

### 6.4 Multiple OpenConductor Instances Editing the Same Repo

**Scenario:** Two OpenConductor instances (or an OpenConductor instance and the CLI `bootstrap` command) both try to write persona files to the same repo.

**Expected behavior:**
- File writes are atomic (write to temp file, then rename).
- Last writer wins -- no locking is implemented.
- This is acceptable because persona changes are infrequent human-initiated actions, not automated concurrent operations.

### 6.5 Agent Type Change After Persona Was Set

**Scenario:** The user edits config.yaml to change a project's agent from `claude-code` to `opencode` but keeps the persona.

**Expected behavior:**
- On next launch, the persona bootstrapper writes OpenCode-specific files instead of CLAUDE.md.
- The old CLAUDE.md persona section remains in the repo (it is not automatically cleaned up since the user might switch back).
- The OpenCode adapter reads its own config format.

### 6.6 Persona on a Repo That Already Has Substantial CLAUDE.md

**Scenario:** A production repo has a 200-line CLAUDE.md with detailed project-specific instructions.

**Expected behavior:**
- The persona section is appended at the end, after all existing content.
- The existing instructions remain authoritative for project-specific concerns.
- The persona instructions add behavioral tone (how to approach work) without conflicting with project-specific rules (what conventions to follow).
- If there is a conflict (e.g., existing CLAUDE.md says "always write tests" but persona is Vibe which says "skip tests"), the existing project instructions should take precedence because they appear first in the file and are more specific.

### 6.7 Backward Compatibility for Existing Projects

**Scenario:** User upgrades OpenConductor. Their existing config.yaml has projects without the `persona` field.

**Expected behavior:**
- `persona: ""` (omitempty) is equivalent to `PersonaNone`.
- No persona files are written.
- The sidebar shows the project exactly as before.
- No migration step is needed.

### 6.8 Persona Change While Agent Is Actively Working

**Scenario:** User presses `p` to change persona while the agent is in `StateWorking`.

**Expected behavior:**
- The change is allowed. Config is updated, persona files are rewritten.
- A notice appears: "Persona updated to POC. The agent will pick up the new instructions on its next turn."
- The current agent turn completes under the old persona's behavioral mode. Claude Code re-reads CLAUDE.md on each new turn, so the next response will reflect the new persona.

---

## 7. Success KPIs

### Adoption Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Persona selection rate on new projects | >50% of new projects pick a non-None persona within 30 days of feature launch | Count of `ProjectAddedMsg` events with `Persona != ""` / total `ProjectAddedMsg` events |
| Persona distribution | No single persona exceeds 70% share (indicates all three are useful) | Distribution of persona values across all projects in anonymized telemetry |
| Persona change rate | 10-20% of projects change persona at least once | Count of persona change events / total projects with personas |

### User Satisfaction Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Reduced "correction prompts" | 30% fewer user messages that override agent behavior (e.g., "skip tests", "add tests") in persona-equipped projects vs. no-persona projects | Analyzed from scrollback content (opt-in analytics) or user survey |
| Feature satisfaction score | >4.0 / 5.0 in post-launch survey | In-app survey or GitHub Discussions poll |
| NPS impact | Persona feature mentioned in >15% of positive NPS verbatims | Qualitative analysis of NPS responses |

### Engagement Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Form completion rate | >90% of users who reach the persona step complete it | Form step funnel analysis |
| Persona step abandonment | <5% of users who reach step 4 press Esc specifically at that step | Form step-level tracking |
| Time-to-first-persona | Median <5 minutes after feature availability | Time from first launch with new version to first persona selection |

### Business Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Retention impact | 5% improvement in 7-day retention for users who set a persona | Cohort comparison: persona users vs. no-persona users |
| Multi-project growth | Users with personas manage 20% more concurrent projects | Average projects per user, persona vs. no-persona cohorts |

---

## 8. Acceptance Criteria

### 8.1 Add-Project Form

- [ ] The add-project wizard has 5 steps: Name, Repo path, Agent, Persona, Auto-approve
- [ ] Step indicator shows "N/5" for each step
- [ ] Persona step displays 4 options: None, Vibe, POC, Scale
- [ ] Each persona option shows a label and a one-line description
- [ ] j/k and arrow keys navigate persona options
- [ ] Mouse click selects a persona option
- [ ] Selecting a persona updates the default auto-approve level for the next step (Vibe->Full, POC->Safe, Scale->Off, None->no change)
- [ ] The user can override the suggested auto-approve in step 5
- [ ] Enter on the persona step advances to auto-approve
- [ ] Esc on the persona step cancels the entire form
- [ ] Step indicator and "done" summary line show the selected persona when on steps 5

### 8.2 Config Persistence

- [ ] The `persona` field is stored in `config.yaml` under each project
- [ ] Existing config files without `persona` load without error (defaults to empty/None)
- [ ] Config validation accepts `""`, `"vibe"`, `"poc"`, `"scale"` and rejects unknown values
- [ ] Persona value survives config round-trip (load, save, load)

### 8.3 File Generation (Claude Code)

- [ ] When persona is non-None and agent is `claude-code`, a persona section is written to CLAUDE.md in the repo
- [ ] The persona section is wrapped in `<!-- openconductor:persona:start -->` / `<!-- openconductor:persona:end -->` markers
- [ ] If CLAUDE.md does not exist, it is created with only the persona section
- [ ] If CLAUDE.md exists without markers, the persona section is appended at the end
- [ ] If CLAUDE.md exists with markers, content between markers is replaced
- [ ] If persona is None, any existing marked section is removed from CLAUDE.md
- [ ] If CLAUDE.md becomes empty after marker removal, the file is deleted
- [ ] Persona section includes a comment warning that manual edits will be overwritten
- [ ] File writes are atomic (temp file + rename)

### 8.4 File Generation (OpenCode)

- [ ] When persona is non-None and agent is `opencode`, a persona section is written to AGENTS.md in the repo
- [ ] Same marker strategy (`<!-- openconductor:persona:start/end -->`) as CLAUDE.md
- [ ] Same merge/create/remove rules apply to AGENTS.md
- [ ] The adapter does not write a CLAUDE.md (that is Claude Code-specific)

### 8.5 Persona Change

- [ ] Pressing `p` on a selected project in the sidebar opens a persona change dialog
- [ ] The dialog shows the current persona highlighted
- [ ] After selection, a confirmation dialog asks: "This will update CLAUDE.md/AGENTS.md in the repo — continue?"
- [ ] If confirmed and session is active, user is prompted: "Start a fresh conversation or keep current?"
- [ ] Selecting a new persona updates config, rewrites persona files, and saves config
- [ ] The sidebar updates to show the new persona label

### 8.6 Sidebar Display

- [ ] Projects with a persona show it between agent type and state: `claude | vibe | working`
- [ ] Projects without a persona (None) show only agent and state: `claude | working`
- [ ] The persona label uses the same dim style as the agent type label

### 8.7 CLI Bootstrap

- [ ] `openconductor bootstrap <path> --persona vibe` writes persona-specific templates
- [ ] `--persona` flag accepts built-in names and custom persona names
- [ ] Default `--persona` value is "none" for backward compatibility
- [ ] `--persona` and `--agent` flags work together correctly

### 8.8 Custom Persona CRUD

- [ ] `Shift+P` in sidebar opens "Persona Manager" as a system tab (same pattern as Telegram setup)
- [ ] `openconductor persona` launches the persona manager CLI directly
- [ ] Main menu lists built-in personas (read-only) and custom personas (editable)
- [ ] **Create**: prompts for name (slug), label, instructions (multi-line), auto-approve level
- [ ] **Create**: validates name is unique, non-empty, slug-format (lowercase, hyphens)
- [ ] **Create**: validates name does not collide with built-in persona names
- [ ] **Edit**: shows list of custom personas, opens selected with pre-filled values
- [ ] **Edit**: allows changing label, instructions, and auto-approve (name is immutable)
- [ ] **Delete**: shows list of custom personas, confirms before deleting
- [ ] **Delete**: warns that existing projects using this persona will show it as unknown
- [ ] All CRUD operations save to `~/.openconductor/config.yaml` under `personas` key
- [ ] On system tab exit, TUI reloads the `Personas` section of config
- [ ] Custom personas appear below built-in ones in the form persona step, separated by a divider
- [ ] Custom personas appear in the sidebar persona change picker

### 8.9 Error Handling

- [ ] If the repo path is not writable, the project is still added to config with a warning
- [ ] If marker parsing finds a start without an end, the section from start to EOF is treated as the persona section
- [ ] If marker parsing finds an end without a start, the end marker is ignored (content preserved)
- [ ] All file I/O errors are logged and do not crash the application

### 8.9 Backward Compatibility

- [ ] Existing projects without the persona field continue to work identically
- [ ] No migration step is required
- [ ] The add-project form's step count change does not break existing sidebar click hit-testing (constants in `form.go` are updated)

### 8.10 Tests

- [ ] Config validation tests cover all persona values including empty and invalid
- [ ] Config round-trip test verifies persona survives save/load
- [ ] Instruction file merge tests cover: no file, file without markers, file with markers, marker repair, removal on None (for both CLAUDE.md and AGENTS.md)
- [ ] Form tests verify 5-step progression and persona selection
- [ ] Form tests verify auto-approve default suggestion based on persona

---

## 9. Out of Scope (Future Iterations)

These are explicitly **not** part of this story but are noted for future planning:

1. **Per-persona MCP server bundles.** Associating specific MCP servers (e.g., a linting MCP, a deployment MCP) with personas.

2. **Persona marketplace or sharing.** Importing/exporting persona definitions for team distribution.

3. **Persona-aware attention detection.** Adjusting the attention detector's behavior based on persona (e.g., Vibe mode might suppress "needs input" notifications since the agent should be autonomous).

4. **Persona in Telegram bridge.** Showing the active persona in Telegram forum topic messages or allowing persona changes via Telegram commands.

5. **Persona analytics dashboard.** In-app visualization of how personas affect agent behavior and task completion rates.

---

## 10. Implementation Sequence

Suggested order for engineering:

1. **Config layer** -- Add `PersonaType`, `CustomPersona` types, `Persona` field on `Project`, `Personas` slice on `Config`, validation, tests
2. **Persona package** -- Instruction text, target file mapping, persona resolution (built-in + custom), default approval
3. **Persona writer** -- CLAUDE.md/AGENTS.md merge logic with markers, atomic writes, tests
4. **Form extension** -- Add step 4 (persona selection) with built-in + custom personas, update step counter
5. **Auto-approve suggestion** -- Wire persona selection to default auto-approve in step 5
6. **Session integration** -- Invoke persona writer from `ProjectAddedMsg` handler before session start
7. **Sidebar display** -- Show persona label in project entries
8. **Persona change** -- `p` keybinding, change dialog, file rewrite, confirmation, session restart prompt
9. **Custom persona CLI wizard** -- `openconductor persona` subcommand with create/edit/delete flows
10. **System tab integration** -- `Shift+P` keybinding, SystemTabRequestMsg, config reload on exit
11. **CLI extension** -- Add `--persona` flag to bootstrap command (accepts built-in + custom names)
12. **OpenCode adapter** -- Persona file writing to AGENTS.md

---

## 11. Resolved Design Decisions

1. **Persona selection is optional.** None is the default. No nudging or "recommended" badge.
2. **Changing persona requires confirmation.** A confirmation dialog ("This will update CLAUDE.md/AGENTS.md in the repo — continue?") is shown before writing files.
3. **No auto-suggestion.** Persona is always an explicit user choice.
4. **Persona change prompts about conversation reset.** When changing persona on an existing project, the user is asked: "Start a fresh conversation with the new persona, or keep the current conversation?"
5. **Naming: "Persona."** Used consistently in code, config, and UI.
