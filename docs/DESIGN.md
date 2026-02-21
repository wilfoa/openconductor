# OpenConductor — Design Document

## What is OpenConductor?

A terminal multiplexer purpose-built for coding agents. Like tmux is to shell sessions, OpenConductor is to agent sessions.

OpenConductor is the **executor** that manages agent **workers**. It does NOT replace any agent CLI — it wraps them, manages them, and provides a better UX for working with multiple agents in parallel.

### The Problem

Today, developers working with multiple coding agents (Claude Code, Codex, Gemini CLI) iterate terminal tabs in a round-robin manner — writing the next prompt to an agent, moving to the next tab, and so on. There is no push-based notification when an agent finishes or needs attention.

### The Solution

OpenConductor gives users:

1. **Awareness** — stop tab-cycling. A sidebar shows which agents need you NOW.
2. **Agent bootstrapping** — when setting up a project, OpenConductor offers to configure the agent's repo with recommended workflows (CLAUDE.md, MCP servers for Linear/JIRA, planning-first instructions).
3. **Workflow templates** — opinionated but optional. Suggest a 70/30 planning/implementation ratio. Help users be better "conductors" of their agent orchestra.

### The Metaphor

| Concept        | Executor world               | OpenConductor world                            |
|----------------|------------------------------|------------------------------------------|
| Conductor      | Celery / Temporal / k8s      | OpenConductor                                  |
| Worker         | Python process / container   | Claude Code / Codex / Gemini CLI         |
| Job            | Task payload                 | Project + prompt                         |
| Job status     | pending / running / complete | idle / working / needs-attention          |
| Result         | Return value                 | Diff / conversation output               |
| Queue          | Redis / SQS                  | Sidebar project list                     |

---

## Supported Agents

OpenConductor is agent-agnostic. It manages any CLI-based coding agent via PTY.

### Licensing Summary

| Aspect                      | Claude Code              | Codex CLI            | Gemini CLI                      |
|-----------------------------|--------------------------|----------------------|---------------------------------|
| **License**                 | Proprietary              | Apache 2.0           | Apache 2.0                      |
| **Embed/bundle CLI?**       | No                       | Yes, with attribution| Yes, with attribution           |
| **Spawn as subprocess?**    | No (blocked Jan 2026)    | Yes, first-class     | Technically yes, fragile        |
| **Official SDK?**           | Agent SDK (API key auth) | Codex SDK + MCP      | GenAI SDKs (no agent loop)      |
| **Commercial use?**         | Via Agent SDK only       | Yes                  | Yes (paid tier for EU users)    |
| **Anti-competition clause** | Can't build competing AI | Can't build competing AI | Can't build competing AI    |

**OpenConductor's legal position:** OpenConductor is a terminal manager, not a harness. The user runs their own agent CLI with their own credentials. OpenConductor allocates a PTY and provides a better window manager. This is analogous to iTerm2 or tmux — not a third-party harness.

---

## User Workflow

### The 70/30 Planning/Implementation Ratio

The key insight: the quality of task descriptions determines whether an agent can work autonomously. OpenConductor encourages users to invest 70% of effort in meticulous planning with the agent, then let the agent execute the remaining 30%.

### Workflow Steps

1. User opens OpenConductor, sees their projects in the sidebar
2. User activates a project — this spawns (or resumes) the agent CLI in a managed PTY
3. User and agent **plan together** interactively (the 70%)
4. Plan gets broken into tasks, persisted to Linear/JIRA
5. User **approves** the plan
6. User tells the agent to **start working** on the tasks (the 30%)
7. User switches to another project while this agent works
8. OpenConductor **notifies** the user when the agent finishes or needs input
9. User reviews, approves, and the agent picks up the next task

### Important: Human is Always the Trigger

OpenConductor does NOT autonomously run agents. The human activates a project, chooses to plan or execute, and reviews results. The agent never acts without human initiation.

---

## Task Management Integration

### Philosophy

Linear/JIRA is **durable storage for the plan**, not a communication channel. The agent writes tasks during planning, reads them back during execution — potentially days later, in a fresh context window. Humans get transparency as a side effect.

Tasks are the **contract between planning-phase and execution-phase**.

### Backend Adapter Pattern

OpenConductor supports a closed set of task management backends through an adapter interface:

```
Task Service (normalized model)
    │
    ├── Linear Adapter
    ├── JIRA Adapter
    ├── GitHub Issues Adapter
    └── Shortcut Adapter
```

### Normalized Task Status

Every backend maps to/from this normalized set:

- `backlog` — not started
- `planned` — plan exists, not yet approved
- `ready` — approved, agent can pick it up
- `in_progress` — agent is working
- `review` — agent done, human needs to review
- `done` — human approved
- `rejected` — human rejected, needs rework

### Status Sniffer-and-Mapper

Instead of asking users to manually map statuses, OpenConductor:

1. Fetches workflow states from the backend (e.g., Linear team states)
2. Sends them to a small LLM with the normalized status set
3. LLM returns the mapping automatically
4. Mapping is persisted in project config

Runs once on project setup. No user intervention needed.

---

## UI Design

### TUI Layout

```
┌─────────────────────────────────────────────────────┐
│                    OpenConductor TUI                       │
│                                                      │
│  ┌───────────┐  ┌────────────────────────────────┐  │
│  │ Projects  │  │                                │  │
│  │           │  │  Claude Code session            │  │
│  │  ● app-v2 │  │  (project: app-v2)             │  │
│  │    codex  │  │                                │  │
│  │           │  │  I've finished refactoring the │  │
│  │  🔔 api   │  │  auth middleware. Here's what  │  │
│  │    claude │  │  I changed: ...                │  │
│  │           │  │                                │  │
│  │  ○ infra  │  │  > _                           │  │
│  │    gemini │  │                                │  │
│  │           │  │                                │  │
│  └───────────┘  └────────────────────────────────┘  │
│                                                      │
│  api: waiting for input · app-v2: working · infra: idle │
└─────────────────────────────────────────────────────┘
```

- **Left sidebar**: project list with status indicators and notification badges
- **Main panel**: active agent's terminal session (full PTY passthrough)
- **Status bar**: per-project status summary
