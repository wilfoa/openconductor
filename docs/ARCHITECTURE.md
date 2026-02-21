# OpenConductor — Architecture

## System Overview

```
┌─────────────────────────────────────────┐
│              OpenConductor                     │
│                                          │
│  ┌──────────────┐  ┌────────────────┐   │
│  │  TUI Layer   │  │  Notification  │   │
│  │  (Bubble Tea │  │  Service       │   │
│  │   framework) │  │  (macOS notif) │   │
│  └──────┬───────┘  └───────▲────────┘   │
│         │                  │             │
│  ┌──────▼──────────────────┴──────────┐ │
│  │        Session Manager             │ │
│  │                                     │ │
│  │  - project registry                │ │
│  │  - PTY allocation per project      │ │
│  │  - output monitoring (detect idle) │ │
│  │  - input forwarding                │ │
│  └──────┬─────────┬─────────┬─────────┘ │
│         │         │         │            │
│     ┌───▼──┐  ┌───▼──┐  ┌──▼───┐       │
│     │ PTY  │  │ PTY  │  │ PTY  │       │
│     │      │  │      │  │      │       │
│     │claude│  │codex │  │gemini│       │
│     │ code │  │ cli  │  │ cli  │       │
│     └──────┘  └──────┘  └──────┘       │
└─────────────────────────────────────────┘
```

## Tech Stack

| Component      | Choice                              | Why                                                  |
|----------------|-------------------------------------|------------------------------------------------------|
| Language       | Go                                  | Best PTY support, single binary, great TUI ecosystem |
| TUI framework  | Bubble Tea (charmbracelet)          | Mature, Elm-architecture, large community            |
| PTY management | creack/pty                          | Standard Go PTY library                              |
| Config         | YAML files in ~/.openconductor/     | Simple, human-editable                               |
| Notifications  | gen2brain/beeep or native osascript | Cross-platform desktop notifications                 |
| Persistence    | SQLite via modernc.org/sqlite       | Project state, session history                       |

---

## Component Breakdown

### 1. TUI Layer (Bubble Tea)

The outermost shell. Manages the visual layout:

- **Sidebar model**: list of projects with status indicators, notification badges
- **Terminal model**: renders the active project's PTY output
- **Status bar model**: summary of all project states
- **Keybindings**: switch projects, send input, open workflows

Elm architecture means:
- `Update(msg) → (Model, Cmd)` — pure state transitions
- `View(model) → string` — render current state
- Messages flow from PTY monitor → session manager → TUI

### 2. Session Manager

The core orchestrator. Manages the lifecycle of agent sessions:

```go
type SessionManager struct {
    projects   map[string]*Project
    sessions   map[string]*AgentSession
    detector   *AttentionDetector
    notifier   *NotificationService
    store      *SQLiteStore
}

type Project struct {
    ID         string
    Name       string
    RepoPath   string
    AgentType  string   // "claude-code" | "codex" | "gemini-cli"
    Config     ProjectConfig
}

type AgentSession struct {
    ProjectID  string
    PTY        *os.File
    Process    *os.Process
    Buffer     *TerminalBuffer  // rendered text (VT100-parsed)
    State      SessionState     // idle | working | needs-attention
    Attention  *AttentionType   // why it needs attention (if applicable)
    StartedAt  time.Time
}
```

Responsibilities:
- Spawn agent CLI processes in PTYs
- Forward user input to the active session's PTY
- Route PTY output to the TUI for rendering
- Feed PTY output to the attention detector
- Persist session state to SQLite

### 3. PTY Management

Each project gets its own PTY:

```go
func spawnAgent(project *Project) (*AgentSession, error) {
    // Determine the CLI command based on agent type
    cmd := agentCommand(project.AgentType) // e.g., "claude", "codex", "gemini"

    // Start in the project's repo directory
    cmd.Dir = project.RepoPath

    // Allocate PTY
    ptyFile, err := pty.Start(cmd)

    // Start VT100 parser to maintain clean text buffer
    buffer := NewTerminalBuffer(ptyFile)

    return &AgentSession{
        PTY:     ptyFile,
        Process: cmd.Process,
        Buffer:  buffer,
        State:   Working,
    }, nil
}
```

The VT100 parser renders raw PTY output (with ANSI escape codes) into a clean text buffer. This buffer is what both the TUI and attention detector consume.

### 4. Attention Detection System

See [ATTENTION_DETECTION.md](./ATTENTION_DETECTION.md) for full design.

Two-layer pipeline:
- **Layer 1 (Heuristics)**: free, instant, catches obvious cases
- **Layer 2 (LLM)**: cheap text model, catches nuanced cases

### 5. Configuration System

```
~/.openconductor/
├── config.yaml              # global settings
└── projects/
    ├── my-saas-app.yaml     # per-project config
    └── internal-tools.yaml
```

Project config:

```yaml
name: "my-saas-app"
repo: "/Users/amir/Development/my-saas-app"
agent: "claude-code"

task_backend:
  type: "linear"
  api_key_env: "LINEAR_API_KEY"
  team_id: "TEAM-xxx"
  status_mapping:           # auto-generated by sniffer
    backlog: "Backlog"
    planned: "Todo"
    ready: "Todo"
    in_progress: "In Progress"
    review: "In Review"
    done: "Done"
    rejected: "Cancelled"

notifications:
  on_needs_attention: true
  on_error: true
  method: "macos"
```

### 6. Notification Service

Triggered by the attention detector when a session transitions to `needs-attention`:

```go
type NotificationService struct {
    method string // "macos" | "beeep" | "webhook"
}

func (n *NotificationService) Notify(project string, attention AttentionType) {
    title := fmt.Sprintf("OpenConductor: %s", project)
    body := attentionMessage(attention)
    // e.g., "Agent finished task, ready for review"
    // e.g., "Agent is asking a question"

    switch n.method {
    case "macos":
        exec.Command("osascript", "-e",
            fmt.Sprintf(`display notification "%s" with title "%s"`, body, title)).Run()
    case "beeep":
        beeep.Notify(title, body, "")
    }
}
```

### 7. Persistence (SQLite)

Stores:
- Project registry and config cache
- Session history (when started, tokens used, outcomes)
- Attention event log (for analytics: how often do agents need help?)
- Task cache (local mirror of Linear/JIRA tasks)

---

## Data Flow

```
User keystroke
    │
    ▼
TUI (Bubble Tea) ──► if sidebar focused: navigate projects
    │                  if terminal focused: forward to PTY
    │
    ▼
Session Manager ──► writes to active session's PTY stdin
    │
    ▼
Agent CLI (in PTY) ──► processes input, produces output
    │
    ▼
PTY stdout ──► VT100 parser ──► TerminalBuffer (clean text)
    │                               │
    ├──► TUI renders terminal       │
    │    panel from buffer          │
    │                               ▼
    │                          Attention Detector
    │                               │
    │                          Layer 1: heuristics
    │                               │
    │                          (uncertain?)
    │                               │
    │                          Layer 2: LLM classify
    │                               │
    │                               ▼
    │                          State change → needs-attention
    │                               │
    │                               ├──► Update sidebar badge
    │                               └──► Send desktop notification
    │
    ▼
(cycle continues)
```

---

## Agent Adapter Pattern

Each agent type has a thin adapter for agent-specific concerns:

```go
type AgentAdapter interface {
    // The CLI command to spawn
    Command(project *Project) *exec.Cmd

    // Agent-specific heuristics for attention detection
    // (supplements the generic detector)
    AttentionHints(lastLines []string) *AttentionType

    // How to bootstrap a new project with this agent
    Bootstrap(repoPath string) error
}
```

Implementations:
- `ClaudeCodeAdapter` — spawns `claude`, detects `>` prompt, bootstraps with `CLAUDE.md`
- `CodexAdapter` — spawns `codex`, detects Codex prompt, bootstraps with `.codex/instructions.md`
- `GeminiAdapter` — spawns `gemini`, detects Gemini prompt, bootstraps with `GEMINI.md`

---

## Task Backend Adapter Pattern

```go
type TaskBackend interface {
    Name() string
    Connect(config BackendConfig) error

    CreateTask(task NewTask) (*Task, error)
    GetTask(externalID string) (*Task, error)
    UpdateTask(externalID string, updates TaskUpdates) (*Task, error)
    ListTasks(filter TaskFilter) ([]*Task, error)

    // Status mapping
    MapStatusToExternal(status TaskStatus) string
    MapStatusFromExternal(externalStatus string) TaskStatus

    // Auto-detect workflow states for sniffer-and-mapper
    FetchWorkflowStates() ([]string, error)
}
```

Supported backends (closed set):
- Linear (MVP)
- JIRA
- GitHub Issues
- Shortcut
