# Architecture Simplification Plan

## Goal

Simplify OpenConductor's architecture to reduce the `tui/app.go` god object, make adding new agents trivial, and unify notification dispatch. All changes are additive and incremental — each phase compiles and passes tests independently.

## Design Principle

Attention heuristics are per-agent domain knowledge and stay WITH the agent adapters. The `agent → attention` import is acceptable since `attention/` defines the contract types (`HeuristicResult`, `AttentionEvent`, `AttentionChecker`) that agents implement.

---

## Phase 1: Quick Wins (bugs + dead code + stdlib)

**Effort:** Low | **Risk:** None | **Files:** 5

| # | Task | File:Line | Detail |
|---|------|-----------|--------|
| 1 | Fix `TestBackoff_CapsAtMax` timeout | `telegram/bot_test.go:83-92` | Cancel context before calling `b.backoff(backoffMax)`, or use a tiny duration. Currently sleeps 60s, exceeding test timeout. |
| 2 | Delete `intToStr` dead code | `telegram/bot.go:771-773` | Zero callers (confirmed via grep). |
| 3 | Delete `isAltRune` dead code | `tui/keys.go:33-35` | Zero callers (confirmed via grep). |
| 4 | Delete `historyPart` dead code | `agent/opencode.go:833-836` | Struct defined but never used; `formatHistory` uses an inline anonymous struct. |
| 5 | Replace `containsByte` with stdlib | `agent/claude.go:538-545` | Replace with `bytes.IndexByte(data, b) >= 0`. The stdlib version uses assembly-optimized search. |
| 6 | Replace `time.Sleep` with `tea.Cmd` | `tui/app.go:1935` | `time.Sleep(d)` blocks the bubbletea Update loop. Instead, return a `tea.Cmd` that sleeps and then writes. Same fix at lines 1884, 1955. |

### Verification
```
go test ./internal/telegram/...
go test ./internal/agent/...
go build ./...
```

---

## Phase 2: Agent Capabilities Struct + BaseAgent

**Effort:** Medium | **Risk:** Low (additive, then swap) | **Reduces:** ~50 lines net, 8 interfaces eliminated

### 2a. New `AgentCapabilities` struct (in `agent/agent.go`)

Replace the 8 optional interfaces (`ScreenFilter`, `ChromeLayout`, `ChromeLineFilter`, `SubmitDelay`, `ImageInputFormatter`, `OutputFilter`, `QuestionResponder`, `HistoryProvider`) with a single struct:

```go
type AgentCapabilities struct {
    SubmitDelay      time.Duration
    ChromeSkipTop    int
    ChromeSkipBottom int
    FilterScreen     func(lines []string) []string          // nil = no-op
    IsChromeLine     func(line string) bool                  // nil = no-op
    FormatImageInput func(imagePath, caption string) string  // nil = default
    NewOutputFilter  func() func([]byte) []byte              // nil = none
    QuestionKeystroke func(optionNum int) []byte              // nil = not supported
    LoadHistory      func(repoPath string) ([]string, error) // nil = no-op
}
```

Add `Capabilities() AgentCapabilities` to the `AgentAdapter` interface.

**NOT changed:** `attention.AttentionChecker` stays as a separate interface. `CheckAttention()` stays on each adapter.

### 2b. Shared `DownArrowKeystroke` helper (in `agent/agent.go`)

Deduplicates identical `QuestionKeystroke` from `claude.go:401-410` and `opencode.go:66-75`:

```go
func DownArrowKeystroke(optionNum int) []byte {
    if optionNum <= 1 { return nil }
    ks := make([]byte, 0, (optionNum-1)*3)
    for i := 1; i < optionNum; i++ {
        ks = append(ks, '\x1b', '[', 'B')
    }
    return ks
}
```

### 2c. `BaseAgent` struct (in `agent/agent.go`)

Provides defaults so new agents need minimal boilerplate:

```go
type BaseAgent struct {
    AgentType_ config.AgentType
    Caps_      AgentCapabilities
}

func (b *BaseAgent) Type() config.AgentType            { return b.AgentType_ }
func (b *BaseAgent) BootstrapFiles() []BootstrapFile    { return nil }
func (b *BaseAgent) ApproveKeystroke() []byte           { return []byte("y\n") }
func (b *BaseAgent) ApproveSessionKeystroke() []byte    { return nil }
func (b *BaseAgent) DenyKeystroke() []byte              { return []byte("n\n") }
func (b *BaseAgent) Capabilities() AgentCapabilities    { return b.Caps_ }
```

**Does NOT implement `Command()` — compiler enforces every adapter provides its own.**

### 2d. Adapter changes

Each adapter adds a `Capabilities()` method and deletes methods now covered by the struct:

- **claude.go:** Add `Capabilities()`. Delete `QuestionKeystroke()` (use `DownArrowKeystroke`).
- **opencode.go:** Add `Capabilities()`. Delete `QuestionKeystroke()`, `SubmitDelay()`, `ChromeSkipRows()` (values inline in capabilities).

### 2e. Wrapper functions simplified

Package-level wrappers (`FilterScreen`, `ChromeSkipRows`, `GetSubmitDelay`, etc.) stay but internals simplify from type-assertion to `GetCapabilities(agentType).FieldName`.

### 2f. Caller changes (2 files)

- `session/session.go:81-83` — Use `agent.NewOutputFilter(agentType)` instead of `agent.GetOutputFilter().NewOutputFilter()`.
- `telegram/handler.go:140-151` — Use `agent.QuestionKeystroke(agentType, num)` wrapper instead of `QuestionResponder` type assertion.

### Transformation order
1. Add new types (compiles clean alongside old interfaces)
2. Add `Capabilities()` to both adapters (compiler-forced)
3. Add new wrapper functions alongside old ones
4. Migrate the 2 callers that need changes
5. Delete 8 old interfaces + old wrappers
6. Delete redundant adapter methods
7. Update tests

### Verification
```
go build ./...
go test ./...
```

---

## Phase 3: Decompose `tui/app.go` (2,162 lines → ~1,370 lines)

**Effort:** Medium-High | **Risk:** Medium (pure refactoring, no behavior change) | **4 new files**

### App struct: 35 fields → 20 fields

After all extractions, `App` becomes:

```go
type App struct {
    cfg, configPath    // config
    sidebar, terminal, statusbar  // UI sub-models
    focus, width, height, ready, active  // layout state
    mgr *session.Manager
    sidebarWidth int
    dragging bool

    // Sub-components (new)
    scroll    scrollState       // 6 maps consolidated
    tabs      tabManager        // 6 fields consolidated
    attention attentionHandler  // 7 fields consolidated

    // Remaining (small)
    animFrame int
    lastCtrlC time.Time
    stateSaved, ctrlCHint bool
    pendingRestoreTabs []string
    statePath string
}
```

### 3a. Extract `scrollState` → `tui/scroll_state.go` (~180 lines)

**Consolidates:** `scrollbacks`, `scrollSnapshots`, `scrollGlyphSnapshots`, `scrollDirty`, `scrollOffsets`, `scrollPinned` (6 maps → 1 struct).

**Methods that move:**
- `runScrollCheck()` → `scrollState.RunCheck(mgr, activeSessionID, terminal)`
- `checkScrollback()` → `scrollState.checkScrollback(s, sessionID)`
- `pushAltScreenDiff()` → stays as free function in same file

**New helper methods:** `MarkDirty`, `PushHistory`, `GetBuffer`, `SaveOffset`, `RestoreOffset`, `CleanupSession`.

### 3b. Extract `tabManager` → `tui/tab_manager.go` (~180 lines)

**Consolidates:** `openTabs`, `tabProjects`, `tabLabels`, `editingTab`, `tabEditBuf`, `tabEditOrig` (6 fields → 1 struct).

**Methods that move:**
- `addTab` → `tm.Add(sessionID)`
- `removeTab` → `tm.Remove(sessionID)`
- `tabDisplayName` → `tm.DisplayName(sessionID)`
- `startTabEdit/commitTabEdit/cancelTabEdit` → `tm.StartEdit/CommitEdit/CancelEdit`
- `switchTab` → `tm.SwitchCmd(delta, activeSessionID)`
- `tabHitTest` → `tm.HitTest(localX, activeSessionID, states, animFrame)`
- `tabBarView` → `tm.BarView(panelWidth, activeSessionID, states, animFrame)`
- `closeTabCmd` → `tm.CloseCmd(name)`

### 3c. Extract `attentionHandler` → `tui/attention_handler.go` (~295 lines)

**Consolidates:** `detector`, `autoApprover`, `notifier`, `telegramCh`, `sessionStates`, `stateStickUntil`, `autoApproveRuns` (7 fields → 1 struct).

**Core method:**
```go
func (ah *attentionHandler) RunCheck(mgr *session.Manager) []stateUpdate
```

Returns `[]stateUpdate` (sessionID, projectName, state). App applies these to sidebar/statusbar. Side effects (auto-approve, auto-confirm, notifications, telegram) happen inside RunCheck.

**Methods that move:**
- `checkAttention` (L1835-2038) → `ah.RunCheck(mgr)`
- `sendTelegramEvent` (L2045-2086) → `ah.sendTelegramEvent(...)`
- `aggregateProjectState` (L2091-2100) → `ah.AggregateProjectState(mgr, projectName)`

**Free functions stay in same file:** `isAttentionState`, `statePriority`, `stateToEventKind`, `attentionEventToState`.

**Integration point (in Update TickMsg handler):**
```go
case TickMsg:
    updates := a.attention.RunCheck(a.mgr)
    for _, u := range updates {
        a.statusbar.states[u.SessionID] = u.State
        a.sidebar.states[u.ProjectName] = a.attention.AggregateProjectState(a.mgr, u.ProjectName)
    }
    cmds = append(cmds, tickCmd())
```

### 3d. Extract mouse handling → `tui/mouse.go` (~176 lines)

Pure mechanical move. No new struct:
```go
func (a App) handleMouse(msg tea.MouseMsg) (App, tea.Cmd)
```

The entire `case tea.MouseMsg:` body (L532-708) becomes this method. Update() calls `return a.handleMouse(msg)`.

### Transformation order
1. Phase 3a (scrollState) — fewest cross-references
2. Phase 3b (tabManager) — depends on 3a for `scroll.CleanupSession` in removeTab
3. Phase 3c (attentionHandler) — depends on 3a/3b for field name changes
4. Phase 3d (mouse.go) — independent but easier after 3a-3c since field names stabilized

### Verification (after each sub-phase)
```
go build ./...
go test ./internal/tui/... -race -count=1
```

---

## Phase 4: Session Encapsulation

**Effort:** Medium | **Risk:** Low | **Files:** ~3

Unexport `Session` fields that callers access directly:

| Current (public) | New (method) | Callers |
|---|---|---|
| `s.Mu.RLock()` / `s.Mu.RUnlock()` | `s.WithRLock(fn)` or per-field methods | `tui/app.go:629,679,1305,1391` |
| `s.VT.Mode()` | `s.VTMode() vt10x.ModeFlag` | `tui/app.go:630,681,1306,1399` |
| `s.VT.Cell(col, row)` | `s.CellAt(col, row) vt10x.Glyph` | `tui/app.go:1408` |
| `s.VT.Cursor()` | `s.CursorPosition() (x, y int)` | `tui/app.go:1400` |
| `s.Cmd.Pid` | `s.PID() int` | `tui/app.go:1851` |
| `s.Width, s.Height` | Already has `s.Size()` | `tui/app.go:1398` |

### Transformation approach
1. Add new methods on `Session` that encapsulate lock + field access
2. Update callers in `tui/` to use new methods
3. Unexport fields (`VT`, `Mu`, `Cmd`, `Ptmx`, `Width`, `Height`)

### Verification
```
go build ./...
go test ./...
```

---

## Phase 5: Unified Notification Dispatch

**Effort:** Medium | **Risk:** Low | **New package:** `notification/channel.go`

### 5a. Define `Channel` interface (in `notification/`)

```go
type Channel interface {
    Notify(ctx context.Context, event Event) error
}

type Event struct {
    Project   string
    State     string    // "permission", "attention", "error", "done", "response"
    Detail    string
    Screen    []string  // optional screen content
}

type Dispatcher struct {
    channels []Channel
}

func (d *Dispatcher) Register(ch Channel) { d.channels = append(d.channels, ch) }
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) { ... }
```

### 5b. Adapt existing notifiers

- `notification.Notifier` (desktop) implements `Channel`
- Create `telegram.NotificationChannel` wrapper that implements `Channel` (wraps the bridge)

### 5c. Replace manual dispatch in App

Current: `App.checkAttention()` calls `a.notifier.Notify(...)` AND `a.sendTelegramEvent(...)` separately.
After: `attentionHandler` holds `*notification.Dispatcher` and calls `dispatcher.Dispatch()` once.

Adding a new notification channel (Slack, webhook) = implement `Channel` + register in `main.go`.

### Verification
```
go build ./...
go test ./...
```

---

## Phase 6: Centralized Session State Store

**Effort:** Medium | **Risk:** Low

### Problem
Session state is tracked in 4 parallel maps that must stay manually synchronized:
- `attentionHandler.sessionStates[id]` (canonical)
- `statusbar.states[id]`
- `sidebar.states[projectName]` (aggregated)
- `attentionHandler.stateStickUntil[id]`

Every transition must update all of these (~8 repetitions in the code).

### Solution

```go
type sessionStateStore struct {
    states      map[string]SessionState
    stickUntil  map[string]time.Time
    approveRuns map[string]int
    onChange    func(sessionID, projectName string, state SessionState)
}

func (s *sessionStateStore) Set(id, project string, state SessionState) {
    s.states[id] = state
    if s.onChange != nil {
        s.onChange(id, project, state) // propagates to sidebar + statusbar
    }
}
```

The `onChange` callback updates sidebar/statusbar, eliminating the repeated 3-line pattern.

---

## Execution Order Summary

| Phase | Depends On | Effort | Impact |
|-------|-----------|--------|--------|
| 1. Quick wins | None | Low | Fixes bugs, removes dead code |
| 2. Capabilities + BaseAgent | None | Medium | New agent in ~15 lines |
| 3a. scrollState extraction | None | Medium | -6 App fields, -180 lines from app.go |
| 3b. tabManager extraction | 3a | Medium | -6 App fields, -180 lines from app.go |
| 3c. attentionHandler extraction | 3a, 3b | Medium | -7 App fields, -295 lines from app.go |
| 3d. mouse.go extraction | 3a-3c | Low | -176 lines from app.go |
| 4. Session encapsulation | 3 | Medium | Cleaner API boundary |
| 5. Unified notification | 3c | Medium | Pluggable notification channels |
| 6. Centralized state store | 3c | Medium | Eliminates 4-way state sync |

**Total estimated impact on `app.go`:** 2,162 lines → ~1,370 lines, 35 fields → 20 fields.
