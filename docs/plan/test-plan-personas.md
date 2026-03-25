# Test Plan: Agent Personas

**Feature**: Agent Personas (US-PERSONA-001)
**Status**: Draft
**Date**: 2026-03-24
**Depends on**:
- User Story: `docs/user-stories/agent-personas.md`
- HLD: `docs/plan/agent-personas-hld.md`
- LLD Persona Package: `docs/plan/lld-persona-package.md`
- LLD TUI Integration: `docs/plan/lld-tui-integration.md`

**Test Strategy**: Unit tests per package using Go's `testing` package, table-driven where applicable, following the existing codebase patterns (`config_test.go`, `form_test.go`, `sidebar_test.go`). All filesystem tests use `t.TempDir()` for isolation. TUI tests use the existing `sendKey`/`sendRune`/`sidebarSendKey` helpers.

---

## 1. Unit Tests: `internal/config/` Package

**File**: `internal/config/config_test.go` (extend existing file)

All tests follow the existing pattern of individual test functions with descriptive names, matching `TestValidateRejectsUnknownAgent` and similar conventions already in the file.

### 1.1 PersonaType Validation

| # | Test Function | Description | Input | Expected |
|---|---|---|---|---|
| C01 | `TestValidatePersonaFieldAcceptsEmpty` | Zero-value persona passes validation | `Project{Persona: ""}` | No error from `validate()` |
| C02 | `TestValidatePersonaFieldAcceptsVibe` | Built-in "vibe" passes | `Project{Persona: "vibe"}` | No error |
| C03 | `TestValidatePersonaFieldAcceptsPOC` | Built-in "poc" passes | `Project{Persona: "poc"}` | No error |
| C04 | `TestValidatePersonaFieldAcceptsScale` | Built-in "scale" passes | `Project{Persona: "scale"}` | No error |
| C05 | `TestValidatePersonaFieldAcceptsUnknown` | Unknown persona does NOT fail `validate()` by design (orphaned custom reference) | `Project{Persona: "deleted-custom"}` | No error (intentional; `validate()` does not reject project persona refs) |
| C06 | `TestValidatePersonaRefNone` | Explicit ref check accepts empty | `ValidatePersonaRef("")` | No error |
| C07 | `TestValidatePersonaRefBuiltin` | Ref check accepts built-in names | `ValidatePersonaRef("vibe")`, `"poc"`, `"scale"` | No error for each |
| C08 | `TestValidatePersonaRefCustom` | Ref check accepts custom name from config | Config with `CustomPersona{Name: "backend-expert", ...}`, ref `"backend-expert"` | No error |
| C09 | `TestValidatePersonaRefUnknown` | Ref check rejects name not in built-ins or customs | `ValidatePersonaRef("nonexistent")`, no matching custom | Error returned |

### 1.2 CustomPersona Validation

| # | Test Function | Description | Input | Expected |
|---|---|---|---|---|
| C10 | `TestValidateCustomPersonaValid` | Valid custom persona passes | `CustomPersona{Name: "x", Label: "X", Instructions: "do stuff"}` | No error |
| C11 | `TestValidateCustomPersonaMissingName` | Empty name rejected | `CustomPersona{Name: "", Label: "X", Instructions: "text"}` | Error: "missing name" |
| C12 | `TestValidateCustomPersonaMissingLabel` | Empty label rejected | `CustomPersona{Name: "x", Label: "", Instructions: "text"}` | Error: "missing label" |
| C13 | `TestValidateCustomPersonaMissingInstructions` | Empty instructions rejected | `CustomPersona{Name: "x", Label: "X", Instructions: ""}` | Error: "missing instructions" |
| C14 | `TestValidateCustomPersonaDuplicateName` | Two customs with same name rejected | Two `CustomPersona` entries with `Name: "same"` | Error: "duplicate name" |
| C15 | `TestValidateCustomPersonaBadApproval` | Invalid auto_approve level rejected | `CustomPersona{..., AutoApprove: "yolo"}` | Error: "unknown auto_approve level" |
| C16 | `TestValidateCustomPersonaValidApprovalLevels` | All valid approval values accepted | `""`, `"off"`, `"safe"`, `"full"` | No error for each |

### 1.3 Config Round-Trip

| # | Test Function | Description | Setup | Expected |
|---|---|---|---|---|
| C17 | `TestSaveLoadRoundTripWithPersona` | Persona field survives YAML round-trip | `Project{Persona: "vibe"}`, save + load | Loaded project has `Persona == "vibe"` |
| C18 | `TestSaveLoadRoundTripWithCustomPersonas` | Custom personas survive YAML round-trip | Config with 2 custom personas, save + load | Loaded `Personas` slice matches original (Name, Label, Instructions, AutoApprove) |
| C19 | `TestSaveLoadRoundTripPersonaNone` | Empty persona does not appear in YAML (omitempty) | `Project{Persona: ""}`, save + load | Loaded project has `Persona == ""`, YAML does not contain `persona:` key |

### 1.4 Backward Compatibility

| # | Test Function | Description | Setup | Expected |
|---|---|---|---|---|
| C20 | `TestLoadConfigWithoutPersonaField` | Existing config without persona field loads cleanly | YAML string without `persona:` or `personas:` keys | Config loads, `Project.Persona == ""`, `Config.Personas` is nil/empty |
| C21 | `TestLoadConfigWithoutPersonasSection` | Config with projects but no personas section | YAML with projects only | Config loads, `Config.Personas` is nil/empty, no error |

---

## 2. Unit Tests: `internal/persona/` Package

### 2.1 Persona Resolution (`internal/persona/persona_test.go`)

| # | Test Function | Description | Input | Expected |
|---|---|---|---|---|
| P01 | `TestInstructionTextBuiltins` | Each built-in returns non-empty text | `PersonaVibe`, `PersonaPOC`, `PersonaScale` | Non-empty string for each; text contains the persona name (e.g., "Vibe") |
| P02 | `TestInstructionTextNone` | None returns empty | `PersonaNone` | `""` |
| P03 | `TestInstructionTextUnknown` | Unknown returns empty | `PersonaType("unknown")` | `""` |
| P04 | `TestTargetFileClaudeCode` | Claude Code maps to CLAUDE.md | `AgentClaudeCode` | `"CLAUDE.md"` |
| P05 | `TestTargetFileOpenCode` | OpenCode maps to AGENTS.md | `AgentOpenCode` | `"AGENTS.md"` |
| P06 | `TestTargetFileUnknown` | Unknown agent returns empty | `AgentType("unknown")` | `""` |
| P07 | `TestDefaultApprovalVibe` | Vibe suggests Full | `PersonaVibe` | `ApprovalFull` |
| P08 | `TestDefaultApprovalPOC` | POC suggests Safe | `PersonaPOC` | `ApprovalSafe` |
| P09 | `TestDefaultApprovalScale` | Scale suggests Off | `PersonaScale` | `ApprovalOff` |
| P10 | `TestDefaultApprovalNone` | None defaults to Off | `PersonaNone` | `ApprovalOff` |
| P11 | `TestDefaultApprovalUnknown` | Unknown defaults to Off | `PersonaType("x")` | `ApprovalOff` |
| P12 | `TestResolveBuiltin` | Built-in resolves with correct fields | `PersonaVibe`, nil customs | `Found=true`, `Instructions` non-empty, `Approval=ApprovalFull`, `Label="Vibe"` |
| P13 | `TestResolveCustom` | Custom persona found in slice | `PersonaType("my-custom")`, custom with matching name | `Found=true`, custom's Instructions, Approval, Label |
| P14 | `TestResolveNone` | None resolves with empty instructions | `PersonaNone` | `Found=true`, `Instructions=""`, `Label="None"` |
| P15 | `TestResolveUnknown` | Unknown returns not found | `PersonaType("gone")`, empty customs | `Found=false` |
| P16 | `TestResolveBuiltinShadowsCustom` | Built-in wins over same-named custom | `PersonaType("vibe")`, custom with `Name="vibe"` | Returns built-in text, not custom Instructions |
| P17 | `TestLabelBuiltin` | Label for built-in | `PersonaVibe`, nil customs | `"Vibe"` |
| P18 | `TestLabelCustom` | Label for custom | `PersonaType("my-thing")`, custom with `Label="My Thing"` | `"My Thing"` |
| P19 | `TestLabelNone` | Label for None | `PersonaNone`, nil customs | `"None"` |
| P20 | `TestLabelUnknown` | Label for unknown returns raw slug | `PersonaType("mystery")`, nil customs | `"mystery"` |
| P21 | `TestAllPersonaOptionsNoCustom` | Only built-ins returned | nil customs | 4 options: None, Vibe, POC, Scale; none have `IsCustom=true` |
| P22 | `TestAllPersonaOptionsWithCustom` | Custom personas appended after built-ins with IsCustom flag | 2 custom personas | 6 options total; last 2 have `IsCustom=true` |

### 2.2 Slug Validation and Name Availability (`internal/persona/setup_test.go`)

**Table-driven: `TestIsValidSlug`**

```go
// Slug validation cases
{"simple", "vibe", true},
{"with-hyphens", "backend-expert", true},
{"with-numbers", "v2-fast", true},
{"single-char", "a", true},
{"max-length-40", strings.Repeat("a", 40), true},
{"too-long-41", strings.Repeat("a", 41), false},
{"starts-with-number", "2fast", false},
{"starts-with-hyphen", "-bad", false},
{"uppercase", "Vibe", false},
{"mixed-case", "backendExpert", false},
{"spaces", "my persona", false},
{"underscores", "my_persona", false},
{"empty", "", false},
{"special-chars", "my@persona", false},
{"trailing-hyphen", "test-", true},
```

**Table-driven: `TestIsNameAvailable`**

```go
{"available-no-existing", "backend-expert", nil, true},
{"builtin-vibe", "vibe", nil, false},
{"builtin-poc", "poc", nil, false},
{"builtin-scale", "scale", nil, false},
{"custom-collision", "existing", []CustomPersona{{Name: "existing"}}, false},
{"no-collision", "new-one", []CustomPersona{{Name: "existing"}}, true},
```

### 2.3 Writer Merge Scenarios (`internal/persona/writer_test.go`)

All writer tests use `t.TempDir()` for filesystem isolation. The numbered scenarios reference the exhaustive list in LLD section 3.13.

| # | Test Function | LLD # | Setup | Action | Assertion |
|---|---|---|---|---|---|
| W01 | `TestWriteNoFilePersonaSet` | 1 | No file exists | `WritePersonaSection(vibe)` | File created; contains `markerStart`, managed comment, vibe text, `markerEnd` |
| W02 | `TestWriteNoFilePersonaNone` | 2 | No file exists | `WritePersonaSection(None)` | No file created, nil error |
| W03 | `TestWriteAppendToExisting` | 3 | File with `"# My Project\n\nCustom stuff"` | `WritePersonaSection(poc)` | File has original content + `\n\n` + markers + POC text |
| W04 | `TestWriteReplaceIdempotent` | 4 | File with markers + vibe text | `WritePersonaSection(vibe)` | File content unchanged (byte-for-byte comparison) |
| W05 | `TestWriteReplaceDifferent` | 5 | File with markers + vibe text | `WritePersonaSection(scale)` | Markers contain scale text; content above/below markers unchanged |
| W06 | `TestWriteRemoveWithUserContent` | 6 | File with user content above + markers + vibe text | `WritePersonaSection(None)` | Markers and persona text removed; user content preserved |
| W07 | `TestWriteRemoveFileEmpty` | 7 | File with only markers + vibe text | `WritePersonaSection(None)` | File deleted entirely |
| W08 | `TestWriteCorruptStartOnly` | 8 | Start marker at line 3, no end marker | `WritePersonaSection(scale)` | Lines 3-EOF replaced with new marked section |
| W09 | `TestWriteCorruptStartOnlyRemoval` | 8 | Start marker present, no end marker | `WritePersonaSection(None)` | Lines from start marker to EOF removed |
| W10 | `TestWriteEndMarkerOnly` | 9 | Only end marker present, no start | `WritePersonaSection(vibe)` | Section appended (no start found = append) |
| W11 | `TestWriteMultipleMarkerPairs` | 10 | Two complete start/end pairs | `WritePersonaSection(scale)` | First pair replaced; second pair untouched |
| W12 | `TestWriteEmptyBetweenMarkers` | 11 | Start and end markers on consecutive lines | `WritePersonaSection(poc)` | Content inserted between markers |
| W13 | `TestWritePreservesUserContent` | -- | User content above and below markers | `WritePersonaSection(vibe)` | Only marker section changes; user content byte-identical |
| W14 | `TestWriteTrailingNewlines` | 14 | N/A | `buildMarkedSection("text\n\n\n")` | Single trailing newline before end marker |
| W15 | `TestWriteUnknownPersona` | 18 | File exists | `WritePersonaSection("nonexistent")` | Error returned, file not modified |
| W16 | `TestWriteUnknownAgentType` | 19 | N/A | `WritePersonaSection` with `agentType="unknown"` | Error returned |
| W17 | `TestWriteCustomPersona` | 17 | No file; custom in slice | `WritePersonaSection("backend-expert")` with matching custom | File contains custom instructions text |
| W18 | `TestWriteRemoveNoMarkers` | -- | File with user content, no markers | `WritePersonaSection(None)` | File unchanged, nil error |
| W19 | `TestWriteCRLFLineEndings` | 15 | File with `\r\n` line endings and markers | `WritePersonaSection(scale)` | Markers matched (via TrimSpace); content replaced correctly |
| W20 | `TestWriteClaudeCodeTargetsCorrectFile` | -- | Repo dir exists | `WritePersonaSection` with `AgentClaudeCode` | Writes `CLAUDE.md`, not `AGENTS.md` |
| W21 | `TestWriteOpenCodeTargetsCorrectFile` | -- | Repo dir exists | `WritePersonaSection` with `AgentOpenCode` | Writes `AGENTS.md`, not `CLAUDE.md` |

### 2.4 Atomic Write (`internal/persona/writer_test.go`)

| # | Test Function | Description | Setup | Assertion |
|---|---|---|---|---|
| W22 | `TestAtomicWriteCreatesFile` | Creates new file | Non-existent path in temp dir | File exists with correct content and 0o644 permissions |
| W23 | `TestAtomicWriteOverwrites` | Overwrites existing file | Existing file with old content | File has new content |
| W24 | `TestAtomicWriteNoTempLeftOnSuccess` | No temp files remain | Any successful write | No `.persona-*` files in directory |
| W25 | `TestAtomicWriteReadOnlyDir` | Error on read-only directory | `chmod 0o555` on temp dir | Error returned, no partial file |

### 2.5 Marker Finding (`internal/persona/writer_test.go`)

| # | Test Function | Description | Input Lines | Expected (startIdx, endIdx) |
|---|---|---|---|---|
| W26 | `TestFindMarkersNone` | No markers present | `["line1", "line2"]` | `(-1, -1)` |
| W27 | `TestFindMarkersNormal` | Both markers present | Start at line 2, end at line 5 | `(2, 5)` |
| W28 | `TestFindMarkersStartOnly` | Only start marker | Start at line 3 | `(3, -1)` |
| W29 | `TestFindMarkersEndOnly` | Only end marker | End at line 2 | `(-1, -1)` -- no start means "not found" |
| W30 | `TestFindMarkersWithLeadingSpace` | Indented markers | `"  <!-- openconductor:persona:start -->"` | Matches via TrimSpace |
| W31 | `TestBuildMarkedSection` | Correct structure | `"hello world"` | `markerStart + \n + managedComment + \n\n + "hello world" + \n + markerEnd + \n` |

### 2.6 Setup Wizard Pure Functions (`internal/persona/setup_test.go`)

| # | Test Function | Description | Input | Expected |
|---|---|---|---|---|
| S01 | `TestParseIndexValid` | Valid 1-based input | `"1"`, count=3 | `(0, nil)` |
| S02 | `TestParseIndexLastItem` | Last valid item | `"3"`, count=3 | `(2, nil)` |
| S03 | `TestParseIndexZero` | Zero is out of range | `"0"`, count=3 | Error |
| S04 | `TestParseIndexOverflow` | Over count | `"4"`, count=3 | Error |
| S05 | `TestParseIndexEmpty` | Empty string | `""`, count=3 | Error |
| S06 | `TestParseIndexNonNumeric` | Letters | `"abc"`, count=3 | Error |
| S07 | `TestReadMultiLine` | Reads until sentinel | Reader with `"line1\nline2\nEND\n"` | `"line1\nline2"` |
| S08 | `TestReadMultiLineEmptyInput` | Only sentinel | Reader with `"END\n"` | `""` |
| S09 | `TestReadMultiLineTrimsTrailingCR` | Windows line endings | Reader with `"line1\r\nEND\r\n"` | `"line1"` |

---

## 3. Unit Tests: `internal/tui/` Package

### 3.1 Form Tests (`internal/tui/form_test.go`)

The existing `newTestForm` helper needs updating to accept custom personas:

```go
// Updated signature to match newFormModel changes
func newTestForm(existingNames ...string) formModel {
    m, _ := newFormModel(existingNames, nil) // nil custom personas
    return m
}

func newTestFormWithPersonas(customs []config.CustomPersona, existingNames ...string) formModel {
    m, _ := newFormModel(existingNames, customs)
    return m
}
```

| # | Test Function | Description | Action | Expected |
|---|---|---|---|---|
| F01 | `TestFormStepCount` | Step indicator shows N/5 | Check `stepIndicator()` at each step | Returns `"1/5"` through `"5/5"` |
| F02 | `TestFormAdvanceAgentToPersona` | Agent step advances to persona (not auto-approve) | At `stepAgent`, press Enter | `m.step == stepPersona` |
| F03 | `TestFormAdvancePersonaToAutoApprove` | Persona step advances to auto-approve | At `stepPersona`, press Enter | `m.step == stepAutoApprove` |
| F04 | `TestFormPersonaJKNavigation` | j/k navigates persona options | At `stepPersona`, send j/k | `personaIndex` increments/decrements, clamps at bounds |
| F05 | `TestFormPersonaArrowNavigation` | Up/Down arrows navigate persona options | At `stepPersona`, send KeyUp/KeyDown | Same behavior as j/k |
| F06 | `TestFormPersonaDividerSkip` | j/k skips divider entries | Build with custom personas (divider present), navigate through | `personaIndex` never lands on a divider entry |
| F07 | `TestFormPersonaDefaultApproval_Vibe` | Selecting Vibe pre-sets approval to Full | Select Vibe (index 1), advance | `approvalIndex` maps to `ApprovalFull` (index 2) |
| F08 | `TestFormPersonaDefaultApproval_POC` | Selecting POC pre-sets approval to Safe | Select POC (index 2), advance | `approvalIndex` maps to `ApprovalSafe` (index 1) |
| F09 | `TestFormPersonaDefaultApproval_Scale` | Selecting Scale pre-sets approval to Off | Select Scale (index 3), advance | `approvalIndex` maps to `ApprovalOff` (index 0) |
| F10 | `TestFormPersonaDefaultApproval_None` | Selecting None pre-sets approval to Off | Select None (index 0), advance | `approvalIndex` maps to `ApprovalOff` (index 0) |
| F11 | `TestFormPersonaInProjectAddedMsg` | Submitted project includes persona | Complete form with Vibe selected | `ProjectAddedMsg.Project.Persona == "vibe"` |
| F12 | `TestFormPersonaNoneInProjectAddedMsg` | None persona serializes as empty | Complete form with None selected | `ProjectAddedMsg.Project.Persona == ""` |
| F13 | `TestFormCustomPersonaInProjectAddedMsg` | Custom persona name in submitted project | Build with customs, select custom | `ProjectAddedMsg.Project.Persona == customName` |
| F14 | `TestFormEmptyCustomPersonas` | No divider when no custom personas | Build with nil customs | `len(personaOptions) == 4`, no divider |
| F15 | `TestFormWithCustomPersonas` | Customs appear after divider | Build with 2 customs | `len(personaOptions) == 7` (4 built-in + divider + 2 custom) |
| F16 | `TestFormPersonaEscapeCancels` | Esc on persona step cancels form | At `stepPersona`, press Esc | `FormCancelledMsg` emitted |
| F17 | `TestFormPersonaMouseClick` | Mouse click selects persona option | Click at Y = `formPersonaOptionContentStart + i` | `personaIndex == i` |
| F18 | `TestFormPersonaMouseClickDivider` | Mouse click on divider is ignored | Click at divider Y offset | `personaIndex` unchanged |
| F19 | `TestFormApprovalMouseClickShifted` | Approval click Y shifted by 1 | Click at Y = `formApprovalOptionContentStart + 0` (now 8, was 7) | Correct approval option selected |
| F20 | `TestFormPersonaStepView` | Persona step renders correctly | At `stepPersona` | Output contains "Persona", "None", "Vibe", "POC", "Scale" |
| F21 | `TestFormAutoApproveStepShowsPersonaDoneLine` | Auto-approve step shows persona in done lines | At `stepAutoApprove` with Vibe selected | Output contains `"Persona"` and `"Vibe"` |

### 3.2 Sidebar Tests (`internal/tui/sidebar_test.go`)

Note: `newSidebarModel` signature changes to accept `customPersonas`. Existing tests must be updated:

```go
// Before: newSidebarModel(testProjects(), defaultSidebarWidth)
// After:  newSidebarModel(testProjects(), defaultSidebarWidth, nil)
```

| # | Test Function | Description | Setup | Action | Expected |
|---|---|---|---|---|---|
| SB01 | `TestSidebarPersonaPickerOpen` | `p` opens persona picker | Focused sidebar, `sidebarNormal` | Press `p` | `mode == sidebarPersonaSelect`, `personaIndex` set to current project persona |
| SB02 | `TestSidebarPersonaPickerNavigation` | j/k navigates in picker | `sidebarPersonaSelect` mode | Press j, k | `personaIndex` changes, skips dividers |
| SB03 | `TestSidebarPersonaPickerSelect_NoSessions` | Select emits message when no open tabs | Project has no open tabs | Select different persona, press Enter | `PersonaChangeRequestMsg` emitted, `mode == sidebarNormal` |
| SB04 | `TestSidebarPersonaPickerSelect_WithSessions` | Select shows confirm when sessions open | `openTabs[projName] = true` | Select different persona, press Enter | `mode == sidebarConfirmReset`, no message yet |
| SB05 | `TestSidebarPersonaConfirmRestart_Yes` | Confirm emits message | `sidebarConfirmReset` mode | Press `y` | `PersonaChangeRequestMsg` emitted, `mode == sidebarNormal` |
| SB06 | `TestSidebarPersonaConfirmRestart_No` | Decline cancels | `sidebarConfirmReset` mode | Press `n` | `mode == sidebarNormal`, no message emitted |
| SB07 | `TestSidebarPersonaConfirmRestart_Esc` | Escape cancels confirm | `sidebarConfirmReset` mode | Press Esc | `mode == sidebarNormal`, no message emitted |
| SB08 | `TestSidebarPersonaPickerEscape` | Escape closes picker | `sidebarPersonaSelect` mode | Press Esc | `mode == sidebarNormal` |
| SB09 | `TestSidebarPersonaSameAsCurrentNoop` | No-op when same persona selected | Current persona "vibe", select "vibe" | Press Enter | `mode == sidebarNormal`, no message emitted |
| SB10 | `TestSidebarPersonaManagerKey` | `P` opens Persona Manager system tab | Focused sidebar, `sidebarNormal` | Press `P` | `SystemTabRequestMsg` with Name `"Persona Manager"` |
| SB11 | `TestSidebarPersonaLabel_WithPersona` | Persona label in agent line | Project with `Persona: "vibe"` | Render `View()` | Output contains `"vibe"` |
| SB12 | `TestSidebarPersonaLabel_NoPersona` | No persona segment when None | Project with `Persona: ""` | Render `View()` | Agent line has no extra persona separator |
| SB13 | `TestSidebarPersonaLabelTruncation` | Long persona name truncated | `Persona: "my-very-long-persona-name"` | `personaDisplayLabel()` | Truncated with ellipsis |
| SB14 | `TestSidebarPersonaPickerPreselects` | Picker pre-selects current persona | Project with `Persona: "scale"` | Press `p` | `personaIndex` points to Scale |
| SB15 | `TestSidebarPersonaPickerWithCustom` | Custom personas in picker | `customPersonas` with 2 entries | Press `p` | Options include built-ins + divider + customs |
| SB16 | `TestSidebarPKeyIgnoredInFormMode` | `p` ignored in form mode | `sidebarForm` mode | Press `p` | Mode unchanged |
| SB17 | `TestSidebarPKeyIgnoredWhenNoProjects` | `p` ignored with no projects | Empty projects | Press `p` | Mode unchanged |

### 3.3 App Tests (`internal/tui/app_test.go`)

These are higher-level integration tests within the TUI package.

| # | Test Function | Description | Setup | Action | Expected |
|---|---|---|---|---|---|
| A01 | `TestProjectAddedWithPersona_TriggersPersonaWrite` | Persona write precedes session start | Emit `ProjectAddedMsg{Project{Persona: "vibe"}}` | Check returned cmds | `writePersonaCmd` in batch; `startSessionCmd` is NOT |
| A02 | `TestProjectAddedWithoutPersona_StartsDirectly` | No persona write when None | Emit `ProjectAddedMsg{Project{Persona: ""}}` | Check returned cmds | `startSessionCmd` in batch directly |
| A03 | `TestPersonaWrittenMsg_StartsSession` | Session starts after successful write | Emit `PersonaWrittenMsg{Err: nil, StartSession: true}` | Check returned cmds | `startSessionCmd` in batch |
| A04 | `TestPersonaWrittenMsg_StartsSessionOnError` | Session starts even on write failure | Emit `PersonaWrittenMsg{Err: someErr, StartSession: true}` | Check returned cmds | `startSessionCmd` still in batch |
| A05 | `TestPersonaWrittenMsg_NoStartWhenFlagFalse` | No session start when flag false | Emit `PersonaWrittenMsg{StartSession: false}` | Check returned cmds | No `startSessionCmd` |
| A06 | `TestPersonaChangeRequestMsg_UpdatesConfig` | Config updated on change | Emit `PersonaChangeRequestMsg{"proj1", "scale"}` | Check config | `cfg.Projects[0].Persona == "scale"` |
| A07 | `TestPersonaChangeRequestMsg_UpdatesSidebar` | Sidebar synced after change | Emit `PersonaChangeRequestMsg` | Check sidebar | `sidebar.projects` matches `cfg.Projects` |
| A08 | `TestSystemTabExitedMsg_ReloadsPersonas` | Custom personas reloaded on exit | Write config to temp, emit `SystemTabExitedMsg` | Check state | `cfg.Personas` and `sidebar.customPersonas` updated |

---

## 4. Integration Tests

### 4.1 Add Project with Persona End-to-End

**File**: `internal/persona/writer_test.go` (filesystem-level integration)

| # | Test Function | Description | Flow | Assertions |
|---|---|---|---|---|
| I01 | `TestEndToEnd_AddProjectWithVibe_ClaudeCode` | Full write for Claude Code | 1. Create temp repo; 2. `WritePersonaSection(repo, AgentClaudeCode, "vibe", nil)` | `CLAUDE.md` exists with markers + vibe text; `AGENTS.md` does NOT exist |
| I02 | `TestEndToEnd_AddProjectWithScale_OpenCode` | Full write for OpenCode | 1. Create temp repo; 2. `WritePersonaSection(repo, AgentOpenCode, "scale", nil)` | `AGENTS.md` exists with markers + scale text; `CLAUDE.md` does NOT exist |
| I03 | `TestEndToEnd_ChangePersona` | Persona change updates file | 1. Write vibe; 2. Write scale to same file | File has scale text, not vibe; markers present |
| I04 | `TestEndToEnd_ChangeToNone_RemovesSection` | None removes section | 1. Write vibe; 2. Write None | Section removed; file deleted if only content |
| I05 | `TestEndToEnd_CustomPersonaWrite` | Custom persona written | 1. Define custom; 2. Write with custom name | File has custom instructions between markers |

### 4.2 Config + Persona Integration

| # | Test Function | Description | Flow | Assertions |
|---|---|---|---|---|
| I06 | `TestConfigRoundTrip_CustomPersona_ThenWrite` | Config -> load -> write persona | 1. Build config with custom; 2. Save; 3. Load; 4. Write using loaded customs | File has correct custom instructions |
| I07 | `TestConfigRoundTrip_ProjectWithPersona` | Config preserves persona | 1. Create project with persona; 2. Save; 3. Load | Persona intact; `Resolve()` succeeds |

---

## 5. Edge Case Tests

Cross-referenced against User Story section 6 (edge cases 6.1-6.8) and LLD scenario table.

### 5.1 Read-Only Repo (US 6.1)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E01 | `TestWriteReadOnlyRepo` | `writer_test.go` | Write fails gracefully | `chmod 0o555` on temp dir | Error returned; no crash |
| E02 | `TestWriteReadOnlyExistingFile` | `writer_test.go` | Cannot read 0o000 file | File with `0o000` perms | Error from ReadFile |

### 5.2 Corrupted Markers (US 6.2)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E03 | `TestWriteCorruptStartOnly` | `writer_test.go` | Start without end | Start at line 5, no end | Replaces from start to EOF; end marker added |
| E04 | `TestWriteCorruptStartOnlyRemoval` | `writer_test.go` | Removal with corrupt markers | Start, no end | Removes from start to EOF |
| E05 | `TestWriteEndMarkerOnly` | `writer_test.go` | End without start | Only end marker | Treated as no markers; appends |

### 5.3 Manual Edits to Persona Section (US 6.3)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E06 | `TestWriteOverwritesManualEdits` | `writer_test.go` | Manual edits overwritten | Markers with manually edited text | New persona text replaces edits |

### 5.4 Multiple Instances (US 6.4)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E07 | `TestAtomicWriteConcurrency` | `writer_test.go` | Concurrent writes valid | Two goroutines, same path | Final file valid; no corruption |

### 5.5 Agent Type Change (US 6.5)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E08 | `TestWriteAgentSwitch_ClaudeToOpenCode` | `writer_test.go` | OpenCode write does not clean CLAUDE.md | 1. Write CLAUDE.md (claude-code); 2. Write AGENTS.md (opencode) | Both files exist |

### 5.6 Large Existing CLAUDE.md (US 6.6)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E09 | `TestWriteAppendToLargeFile` | `writer_test.go` | Append to 200-line file | 200 lines of content | All 200 lines preserved; persona appended |
| E10 | `TestWriteReplaceLargeFile` | `writer_test.go` | Replace in 200-line file | 200 lines with markers in middle | Content before/after markers preserved |

### 5.7 Backward Compatibility (US 6.7)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E11 | `TestLoadConfigWithoutPersonaField` | `config_test.go` | Config loads without persona | YAML without `persona:` | `Persona == ""`, no error |
| E12 | `TestNoPersonaWriteForNone` | `writer_test.go` | None creates no files | No file, `WritePersonaSection(None)` | No file created |

### 5.8 Persona Change While Agent Working (US 6.8)

| # | Test Function | File | Description | Setup | Expected |
|---|---|---|---|---|---|
| E13 | `TestSidebarAllowsPersonaChangeWhileWorking` | `sidebar_test.go` | `p` works during StateWorking | Session in `StateWorking` | `sidebarPersonaSelect` entered |
| E14 | `TestPersonaChangeRequestMsg_StopsSessions` | `app_test.go` | Sessions stopped before write | Active sessions, emit change msg | Sessions stopped, write issued |

---

## 6. Test Data Requirements

### 6.1 Fixture Data

No external fixture files are needed. All test data is constructed inline, following the existing codebase pattern (see `config_test.go` which builds YAML strings inline). Specifically:

**YAML strings for config tests**:

```go
// Config without persona fields (backward compat)
configNoPersona := `projects:
  - name: myproj
    repo: /tmp
    agent: claude-code
`

// Config with built-in persona
configWithPersona := `projects:
  - name: myproj
    repo: /tmp
    agent: claude-code
    persona: vibe
    auto_approve: full
`

// Config with custom personas
configWithCustom := `personas:
  - name: backend-expert
    label: Backend Expert
    instructions: |
      Focus on APIs and backends.
    auto_approve: safe
projects:
  - name: api-svc
    repo: /tmp
    agent: claude-code
    persona: backend-expert
`

// Config with orphaned persona ref (deleted custom)
configOrphaned := `projects:
  - name: api-svc
    repo: /tmp
    agent: claude-code
    persona: deleted-one
`
```

**File content strings for writer tests**:

```go
// Existing CLAUDE.md without markers
existingClaude := "# My Project\n\nThis is my project readme.\n"

// Existing CLAUDE.md with markers
existingWithMarkers := "# My Project\n\nThis is my project readme.\n\n" +
    "<!-- openconductor:persona:start -->\n" +
    "<!-- This section is managed by OpenConductor. Manual edits will be overwritten. -->\n\n" +
    "## OpenConductor Persona: Vibe\n\n" +
    "- Move fast and ship.\n" +
    "<!-- openconductor:persona:end -->\n"

// Large CLAUDE.md (200 lines) -- generated programmatically in test:
// var lines []string
// for i := 0; i < 200; i++ {
//     lines = append(lines, fmt.Sprintf("Line %d of project documentation.", i+1))
// }
// largeClaude := strings.Join(lines, "\n")

// CRLF file
crlfFile := "# Project\r\n\r\n" +
    "<!-- openconductor:persona:start -->\r\n" +
    "old content\r\n" +
    "<!-- openconductor:persona:end -->\r\n"

// File with manually edited persona section
manualEdits := "<!-- openconductor:persona:start -->\n" +
    "## My custom changes\n" +
    "I edited this by hand.\n" +
    "<!-- openconductor:persona:end -->\n"
```

**Custom persona definitions for persona package tests**:

```go
testCustomPersonas := []config.CustomPersona{
    {
        Name:         "backend-expert",
        Label:        "Backend Expert",
        Instructions: "Focus on APIs.\n- Design REST endpoints\n- Write integration tests",
        AutoApprove:  config.ApprovalSafe,
    },
    {
        Name:         "docs-sprint",
        Label:        "Docs Sprint",
        Instructions: "Focus on documentation.\n- Write clear README files",
        AutoApprove:  config.ApprovalOff,
    },
}
```

### 6.2 Test Helpers

New helpers needed across test files:

```go
// internal/persona/writer_test.go
func setupRepo(t *testing.T) string {
    t.Helper()
    return t.TempDir()
}

func setupRepoWithFile(t *testing.T, filename, content string) string {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, filename)
    if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
        t.Fatal(err)
    }
    return dir
}

func readFile(t *testing.T, path string) string {
    t.Helper()
    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatal(err)
    }
    return string(data)
}

func fileExists(t *testing.T, path string) bool {
    t.Helper()
    _, err := os.Stat(path)
    return err == nil
}
```

```go
// internal/tui/form_test.go
func newTestFormWithPersonas(customs []config.CustomPersona, existingNames ...string) formModel {
    m, _ := newFormModel(existingNames, customs)
    return m
}

func advanceToPersonaStep(t *testing.T, m formModel) formModel {
    t.Helper()
    m.nameInput.SetValue("testproj")
    m, _ = sendKey(t, m, tea.KeyEnter)   // name -> repo
    m.repoInput.SetValue(t.TempDir())
    m, _ = sendKey(t, m, tea.KeyEnter)   // repo -> agent
    m, _ = sendKey(t, m, tea.KeyEnter)   // agent -> persona
    if m.step != stepPersona {
        t.Fatalf("expected stepPersona, got %d", m.step)
    }
    return m
}
```

---

## 7. Coverage Matrix

Maps each acceptance criterion from User Story section 8 to specific test case IDs.

### 7.1 Add-Project Form (AC 8.1)

| Acceptance Criterion | Test IDs |
|---|---|
| Wizard has 5 steps: Name, Repo, Agent, Persona, Auto-approve | F01, F02, F03 |
| Step indicator shows "N/5" | F01 |
| Persona step displays 4 options: None, Vibe, POC, Scale | F14, F20 |
| Each option shows label and one-line description | F20 |
| j/k and arrow keys navigate persona options | F04, F05 |
| Mouse click selects a persona option | F17 |
| Selecting persona updates default auto-approve | F07, F08, F09, F10 |
| User can override suggested auto-approve in step 5 | F07 (advance then change approvalIndex) |
| Enter advances from persona to auto-approve | F03 |
| Esc cancels the entire form | F16 |
| Step indicator shows persona on step 5 | F21 |

### 7.2 Config Persistence (AC 8.2)

| Acceptance Criterion | Test IDs |
|---|---|
| `persona` field stored in config.yaml | C17 |
| Existing configs without persona load without error | C20, C21, E11 |
| Validation accepts "", "vibe", "poc", "scale" | C01-C05 |
| Validation rejects unknown values (explicit ref check) | C09 |
| Persona survives config round-trip | C17, C18, C19 |

### 7.3 File Generation -- Claude Code (AC 8.3)

| Acceptance Criterion | Test IDs |
|---|---|
| Non-None persona writes to CLAUDE.md | W01, W20, I01 |
| Section wrapped in markers | W01, W31 |
| Non-existent CLAUDE.md is created | W01 |
| Existing CLAUDE.md without markers: section appended | W03 |
| Existing CLAUDE.md with markers: content replaced | W04, W05 |
| None persona removes marked section | W06, W07, W18 |
| Empty file after removal is deleted | W07 |
| Section includes managed-by comment | W31 |
| File writes are atomic (temp + rename) | W22, W23, W24 |

### 7.4 File Generation -- OpenCode (AC 8.4)

| Acceptance Criterion | Test IDs |
|---|---|
| Non-None persona writes to AGENTS.md | W21, I02 |
| Same marker strategy | W21 (same writeFile logic) |
| Same merge rules | All W* tests apply (shared writeFile) |
| Does not write CLAUDE.md | I02 |

### 7.5 Persona Change (AC 8.5)

| Acceptance Criterion | Test IDs |
|---|---|
| `p` on selected project opens persona dialog | SB01 |
| Dialog shows current persona highlighted | SB14 |
| Confirmation when session active | SB04, SB05, SB06, SB07 |
| Config updated, files rewritten, config saved | A06, A07, I03 |
| Sidebar updates with new persona label | SB11 |

### 7.6 Sidebar Display (AC 8.6)

| Acceptance Criterion | Test IDs |
|---|---|
| Persona shown between agent and state | SB11 |
| No persona segment for None | SB12 |
| Persona label uses dim style | SB11 (view rendering) |

### 7.7 CLI Bootstrap (AC 8.7)

| Acceptance Criterion | Test IDs |
|---|---|
| `--persona vibe` writes persona templates | I01 |
| Accepts built-in and custom names | W17, P12, P13 |
| Default `--persona` is "none" | W02 |
| `--persona` + `--agent` work together | W20, W21 |

### 7.8 Custom Persona CRUD (AC 8.8)

| Acceptance Criterion | Test IDs |
|---|---|
| Shift+P opens Persona Manager as system tab | SB10 |
| Create validates slug format | `TestIsValidSlug` table |
| Create validates no collision with built-in names | `TestIsNameAvailable` table |
| CRUD saves to config.yaml under `personas` key | C18 |
| On exit, TUI reloads Personas | A08 |
| Custom personas appear below built-ins with divider | F15, SB15 |

### 7.9 Error Handling (AC 8.9)

| Acceptance Criterion | Test IDs |
|---|---|
| Read-only repo: project still added with warning | E01, E02 |
| Start without end: section from start to EOF | E03, E04, W08, W09 |
| End without start: end marker ignored | E05, W10, W29 |
| All I/O errors logged, no crash | E01, E02, A04 |

### 7.10 Backward Compatibility (AC 8.9b)

| Acceptance Criterion | Test IDs |
|---|---|
| Existing projects without persona work identically | C20, C21, E11, E12 |
| No migration step required | C20 |
| Form step count change does not break constants | F19 |

### 7.11 Tests (AC 8.10)

| Acceptance Criterion | Test IDs |
|---|---|
| Config validation tests cover all persona values | C01-C09 |
| Config round-trip test | C17, C18, C19 |
| Instruction file merge tests (all scenarios) | W01-W21 |
| Form tests verify 5-step progression | F01-F03 |
| Form tests verify auto-approve default suggestion | F07-F10 |

---

## 8. Test Execution Order

Tests have no cross-package dependencies and can run in any order. The recommended execution during development follows the implementation sequence:

1. **`go test ./internal/config/...`** -- Run first after adding PersonaType, CustomPersona, and validation changes. Fast (no filesystem I/O beyond temp files).

2. **`go test ./internal/persona/...`** -- Run after implementing the persona package. Tests persona.go (pure functions) and writer.go (filesystem operations in temp dirs).

3. **`go test ./internal/tui/...`** -- Run after implementing form, sidebar, and app changes. Tests TUI model updates (no actual terminal rendering).

4. **`make check`** -- Full suite with race detector, lint, vet. Run before pushing.

---

## 9. Quality Gates

### 9.1 Coverage Targets

| Package | Target | Rationale |
|---|---|---|
| `internal/config/` (persona-related) | 100% | Small, critical validation logic |
| `internal/persona/persona.go` | 100% | All public functions are pure and easily testable |
| `internal/persona/writer.go` | >95% | All merge scenarios covered; deferred cleanup paths may be hard to trigger |
| `internal/persona/setup.go` | >70% | Pure functions at 100%; interactive stdin/stdout flows harder to unit test |
| `internal/tui/form.go` (persona additions) | >90% | All step transitions and data propagation tested |
| `internal/tui/sidebar.go` (persona additions) | >90% | All mode transitions and message emissions tested |
| `internal/tui/app.go` (persona handlers) | >85% | Message handlers tested; async command verification tested |

### 9.2 Race Detection

All tests run with `-race` via `make test`. The persona package has no shared mutable state (all maps are read-only after init), so race conditions are unlikely. The `atomicWrite` concurrent test (E07) explicitly validates filesystem-level safety.

### 9.3 Exit Criteria

- All tests in sections 1-5 pass with `-race`
- No new `golangci-lint` warnings
- Coverage meets targets in section 9.1
- All 20 writer merge scenarios from LLD section 3.13 have corresponding tests
- All acceptance criteria from User Story section 8 are mapped to at least one test (section 7 coverage matrix is complete)

---

## 10. Test Count Summary

| Package | Unit Tests | Integration Tests | Edge Case Tests | Total |
|---|---|---|---|---|
| `internal/config/` | 21 (C01-C21) | -- | 1 (E11) | 22 |
| `internal/persona/persona.go` | 22 (P01-P22) | -- | -- | 22 |
| `internal/persona/writer.go` | 31 (W01-W31) | 5 (I01-I05) | 10 (E01-E10) | 46 |
| `internal/persona/setup.go` | 9 (S01-S09) | -- | -- | 9 |
| `internal/tui/form.go` | 21 (F01-F21) | -- | -- | 21 |
| `internal/tui/sidebar.go` | 17 (SB01-SB17) | -- | 1 (E13) | 18 |
| `internal/tui/app.go` | 8 (A01-A08) | 2 (I06-I07) | 1 (E14) | 11 |
| **Total** | **129** | **7** | **13** | **149** |
