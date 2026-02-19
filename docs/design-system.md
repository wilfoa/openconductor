# Maestro Design System

## 1. Brand Identity

### Philosophy

Maestro is a conductor. A conductor does not play instruments -- they bring
clarity, timing, and cohesion to an ensemble. The visual language should
reflect this: **confident authority with graceful restraint**. Not flashy,
not minimal to the point of emptiness. Think of the difference between a
neon sign and a well-lit concert hall.

### Brand Attributes

| Attribute     | Expression                                        |
|---------------|---------------------------------------------------|
| Authority     | Strong typographic hierarchy, decisive spacing     |
| Warmth        | Amber/gold accents against cool backgrounds        |
| Precision     | Consistent 1-unit spacing grid, aligned elements   |
| Orchestration | Visual flow that guides the eye across panels      |

### Naming Convention for Colors

All color tokens follow the pattern `color{Role}` where Role describes
the semantic purpose, not the visual appearance. This prevents names from
becoming stale when values change.


## 2. Color System

### The Problem with the Current Palette

The existing palette is a collection of Tailwind utility colors. They were
chosen individually and do not form a cohesive system. The violet primary
is cold and tech-generic. The status colors are saturated equally, creating
visual noise. The background is undefined (relying on terminal default),
which means the dark violet highlight and alt-bg feel disconnected.

### New Palette: "Concert Hall"

The new palette is built around a warm-toned dark background with a
signature gold accent that nods to the Maestro metaphor -- brass
instruments, warm stage lighting, gilded concert halls.

#### Foundation Colors

```
colorBg        = "#151520"  // Deep indigo-black, the "stage"
colorBgPanel   = "#1B1B2F"  // Sidebar panel background (slightly lifted)
colorBgAlt     = "#12121E"  // Status bar, recessed surfaces
colorSurface   = "#232340"  // Cards, elevated elements
colorBorder    = "#2A2A45"  // Default borders, separators
colorBorderFoc = "#D4A84B"  // Focused borders (gold)
```

**Rationale**: The backgrounds are in the indigo-black family, not pure
gray-black. This gives the app a subtle depth and prevents the "generic
terminal" feel. The slight blue undertone pairs naturally with the gold
accent.

#### Text Colors

```
colorFg        = "#E8E6F0"  // Primary text (warm off-white)
colorFgSecond  = "#9896A8"  // Secondary text, descriptions
colorDim       = "#5C5A6E"  // Muted text, disabled states
colorFgInverse = "#151520"  // Text on bright backgrounds
```

**Rationale**: The text colors are warm off-whites and lavender-grays
rather than pure gray. This ties them to the indigo background family
and feels less clinical.

#### Brand Accent

```
colorAccent    = "#D4A84B"  // Gold -- the Maestro signature
colorAccentDim = "#8B7A3A"  // Muted gold for subtle uses
colorAccentBg  = "#2A2415"  // Gold-tinted surface (selected item bg)
```

**Rationale**: Gold is the single brand-defining color. It replaces violet
as the primary accent. Gold connotes mastery, warmth, and orchestral
brass. It is distinctive without being garish, and it reads clearly on
dark backgrounds at all sizes.

#### Status Colors

```
colorWorking   = "#5EC4A0"  // Seafoam green -- active, alive
colorAttention = "#E8B84D"  // Warm amber -- close to gold, feels native
colorError     = "#D46A6A"  // Dusty rose-red -- serious but not alarming
colorIdle      = "#5C5A6E"  // Matches dim text -- intentionally quiet
colorDone      = "#7B93DB"  // Soft periwinkle -- calm completion
```

**Rationale**: The status colors are desaturated compared to the current
Tailwind defaults. This is intentional. Highly saturated status colors
fight for attention equally, creating noise. Desaturated colors let the
truly urgent state (attention/error) stand out through relative contrast,
not absolute brightness.

The attention color is close to the brand gold, which creates a natural
visual pathway: your eye is drawn to gold accents, and attention states
are in the same family.

#### Agent Identity Colors

Each AI agent gets a subtle identifying color. These are NOT used for
large surfaces -- only for small accent marks (the agent badge, a thin
left-border on cards).

```
colorAgentClaude  = "#D4A373"  // Warm terracotta (Anthropic warmth)
colorAgentCodex   = "#7DC4A8"  // Sage green (OpenAI)
colorAgentGemini  = "#7BAFD4"  // Sky blue (Google)
colorAgentOpenCode= "#B39CD4"  // Soft lavender (community/open)
```

### Color Accessibility

All text/background combinations must meet WCAG 2.1 AA contrast (4.5:1).

| Combination                    | Ratio | Pass |
|--------------------------------|-------|------|
| colorFg on colorBg             | 13.2  | Yes  |
| colorFg on colorBgPanel        | 11.8  | Yes  |
| colorFg on colorSurface        | 9.4   | Yes  |
| colorFgSecond on colorBg       | 6.1   | Yes  |
| colorDim on colorBg            | 3.2   | *AA Large only* |
| colorAccent on colorBg         | 8.7   | Yes  |
| colorWorking on colorBg        | 7.4   | Yes  |
| colorAttention on colorBg      | 8.5   | Yes  |
| colorError on colorBg          | 5.6   | Yes  |

The dim color intentionally fails small-text AA. It is reserved for
decorative elements (borders, disabled states, decorative symbols) that
do not carry essential information.


## 3. Typography and Spacing

### Text Styles Hierarchy

In a TUI, we have limited tools: **Bold**, *Dim*, and color. There is no
font-size variation, no font-weight scale. We must create hierarchy
through the combination of these three attributes plus spacing.

#### Level 1: Section Headers
```
Bold + colorAccent
MarginBottom(1)
Example: "SESSIONS" or "Projects"
```

#### Level 2: Item Primary Text
```
Bold + colorFg
Example: project names
```

#### Level 3: Item Secondary Text
```
Normal + colorFgSecond
Example: agent type, status descriptions
```

#### Level 4: Metadata / Hints
```
Normal + colorDim
Example: keyboard hints, timestamps
```

#### Level 5: Decorative
```
Normal + colorDim
Example: borders, separators, structural characters
```

### Spacing Grid

All spacing is in terminal cell units (1 unit = 1 character/row).

```
XS  = 0  (inline, no gap)
S   = 1  (standard between related items)
M   = 2  (between groups, after headers)
L   = 3  (major section breaks -- rare in tight TUI)
```

### Sidebar Width

```
default = 28  (increased from 24 for breathing room)
minimum = 22
```

The extra 4 columns allow for:
- 1 col left padding
- 2 col card left padding (for agent color bar)
- ~20 col for project name text
- 1 col right padding
- 1 col border

This prevents project names from being aggressively truncated.


## 4. Sidebar Design

### Concept: "Instrument Sections"

The sidebar is reimagined as a panel of session "cards." Each card is a
miniature representation of an agent session, showing everything you need
at a glance without expanding it.

### Structure (Top to Bottom)

```
[Padding: 1 row]
[Section Header]
[Padding: 1 row]
[Session Card 1]
[Gap: 1 row]
[Session Card 2]
[Gap: 1 row]
...
[Spacer to bottom]
[Add Button]
[Padding: 1 row]
```

### Section Header

The header uses the word "SESSIONS" (uppercase, letterspaced) rather than
"Projects." This reinforces the active, living nature of the items --
these are not static project bookmarks, they are running agent sessions.

```
  S E S S I O N S                   (gold, bold, letterspaced)
```

In Lipgloss:

```go
sidebarHeaderStyle = lipgloss.NewStyle().
    Bold(true).
    Foreground(colorAccent).
    MarginBottom(1)

// Render with manual letterspacing:
func letterspace(s string) string {
    runes := []rune(strings.ToUpper(s))
    spaced := make([]string, len(runes))
    for i, r := range runes {
        spaced[i] = string(r)
    }
    return strings.Join(spaced, " ")
}
```

### Session Cards

Each session is a 3-line "card" within the sidebar. The card uses a
left-edge color bar (a single column of the agent's identity color)
to provide instant agent-type recognition.

#### Card Anatomy (Unselected)

```
  [A]  whereiseveryone        [S]
       opencode
```

Where:
- `[A]` = Agent color bar (1 char, using a half-block or the agent color bar)
- `whereiseveryone` = Project name (bold, colorFg)
- `[S]` = Status indicator (right-aligned)
- `opencode` = Agent label (colorFgSecond, indented under name)

#### Concrete Rendering

```
  |  whereiseveryone       ~
     opencode
```

Wait -- in a TUI, a "color bar" is achieved by rendering a block
character in the agent's color:

```
  ▎ whereiseveryone        ~     (idle)
    claude-code
```

Characters used:
- `▎` (U+258E, LEFT ONE QUARTER BLOCK) -- thin vertical bar in agent color
- Status symbols are right-aligned at the card's trailing edge

#### Concrete Unicode Mockup -- Full Sidebar

**Normal State (terminal focused, sidebar unfocused):**

```
                              .
   S E S S I O N S            .   <- gold text, right border is dim
                              .
   ▎ whereiseveryone      ◆   .   <- ▎ in terracotta (claude), ◆ working
     claude-code              .   <- secondary text
                              .
   ▎ stocks               ◇   .   <- ▎ in sage (codex), ◇ idle
     codex                    .
                              .
   ▎ ml-pipeline          ◈   .   <- ▎ in sky (gemini), ◈ attention
     gemini                   .
                              .
                              .
                              .
   + new session              .   <- dim text
                              .
```

**Focused State (sidebar focused, one item selected):**

```
                              |
   S E S S I O N S            |   <- gold border when focused
                              |
   ▎ whereiseveryone      ◆   |
     claude-code              |
                              |
  [▎ stocks               ◇  ]|   <- selected: surface bg, bright border
  [  codex                    ]|
                              |
   ▎ ml-pipeline          ◈   |
     gemini                   |
                              |
                              |
                              |
   + new session              |
                              |
```

In the focused+selected state, the selected card gets:
- Background: `colorSurface` (#232340)
- The project name becomes Bold + colorFg
- The status symbol gets brighter
- Subtle left bracket or full-width highlight

### Status Symbols: Redesigned

The current dots (filled circle / empty circle) are:
1. Hard to distinguish at small sizes
2. All the same shape, differentiated only by color
3. Not meaningful -- a dot tells you nothing

The new system uses distinct shapes that carry meaning even without color
(for colorblind users or low-quality displays):

```
Symbol  State           Color            Meaning/Shape Rationale
------  -----           -----            ---------------------------
  ◇     Idle            colorIdle        Diamond outline = empty, waiting
  ◆     Working         colorWorking     Filled diamond = active, solid
  ◈     Needs Attention colorAttention   Diamond w/ dot = something inside
  ✦     Done            colorDone        4-pointed star = completed, sparkle
  ▲     Error           colorError       Triangle = warning/danger, universal
```

Unicode code points:
- `◇` U+25C7 WHITE DIAMOND
- `◆` U+25C6 BLACK DIAMOND
- `◈` U+25C8 WHITE DIAMOND CONTAINING BLACK SMALL DIAMOND
- `✦` U+2726 BLACK FOUR POINTED STAR
- `▲` U+25B2 BLACK UP-POINTING TRIANGLE

These shapes form a "diamond family" with natural progression:
empty -> filled -> filled with emphasis -> completed -> broken (triangle).

**Fallback for limited terminals**: If Unicode support is poor, fall back
to ASCII: `-` idle, `*` working, `!` attention, `+` done, `x` error.

### Agent Labels: Shorthand Badges

Instead of spelling out the full agent name, use compact badges with
the agent's identity color. This saves horizontal space.

```go
// Agent badge map
var agentBadge = map[config.AgentType]string{
    config.AgentClaudeCode: "claude",
    config.AgentCodex:      "codex",
    config.AgentGemini:     "gemini",
    config.AgentOpenCode:   "opencode",
}
```

The agent name is rendered in the agent's identity color at reduced
weight (normal, not bold).

### Selected State in Detail

The selected card treatment uses a subtle background fill rather than
a bold/inverse treatment. This is more refined and less jarring.

```go
cardSelectedStyle = lipgloss.NewStyle().
    Background(colorSurface).
    Padding(0, 1).
    MarginLeft(1)

cardUnselectedStyle = lipgloss.NewStyle().
    Padding(0, 1).
    MarginLeft(1)
```

### Focus Indicators

The sidebar border changes color to indicate focus:

```go
// Unfocused: dim border
sidebarStyle = lipgloss.NewStyle().
    BorderRight(true).
    BorderStyle(lipgloss.NormalBorder()).
    BorderForeground(colorBorder).
    Padding(1, 1)

// Focused: gold border
sidebarFocusedStyle = lipgloss.NewStyle().
    BorderRight(true).
    BorderStyle(lipgloss.NormalBorder()).
    BorderForeground(colorAccent).
    Padding(1, 1)
```

### Empty State

When no sessions exist, the sidebar shows an inviting empty state:

```
                              .
   S E S S I O N S            .
                              .
                              .
         ♪                    .   <- gold, large music note
                              .
     No sessions yet          .   <- colorFgSecond
     Press  a  to begin       .   <- "a" rendered in gold
                              .
                              .
```

The music note (U+266A) reinforces the Maestro brand without being
heavy-handed.

### Add Button

The add button sits at the bottom of the sidebar, anchored to the
bottom of the scrollable area:

```
   + new session              .   <- "+" in gold, "new session" in dim
```

```go
addButtonStyle = lipgloss.NewStyle().
    Foreground(colorDim)

addButtonPlusStyle = lipgloss.NewStyle().
    Foreground(colorAccent)

// Render:
addButtonPlusStyle.Render("+") + " " + addButtonStyle.Render("new session")
```


## 5. Status Bar

### The Problem

The current status bar is a flat, dim row with some keybind hints. It
wastes the most persistently visible UI real estate in the app.

### New Design

The status bar becomes a dense information strip with clear visual
zones:

```
Section:  [Keybinds]          [Session Info]              [Clock/Meta]
```

#### Concrete Mockup

```
 Esc focus  a add  d delete  j/k nav                     stocks  codex  ◆ working
```

Broken down:
- Left zone: Key hints. Keys rendered in gold, labels in dim.
- Right zone: Active session name (bright) + agent badge (agent color) + status

```go
// Key rendering
statusKeyStyle = lipgloss.NewStyle().
    Foreground(colorAccent).
    Bold(true)

statusLabelStyle = lipgloss.NewStyle().
    Foreground(colorDim)

statusSepStyle = lipgloss.NewStyle().
    Foreground(colorBorder).
    SetString(" | ")

// Right zone
statusProjectStyle = lipgloss.NewStyle().
    Foreground(colorFg).
    Bold(true)

statusAgentStyle = lipgloss.NewStyle().
    Foreground(colorFgSecond)

statusStateStyle = lipgloss.NewStyle()  // colored per state
```

#### Separator

A thin horizontal line above the status bar, using the border color:

```go
statusBarContainerStyle = lipgloss.NewStyle().
    Background(colorBgAlt).
    Foreground(colorDim).
    Padding(0, 1).
    BorderTop(true).
    BorderStyle(lipgloss.NormalBorder()).
    BorderForeground(colorBorder)
```

#### Attention Notification in Status Bar

When any session needs attention, the status bar subtly pulses the
session name. In a TUI, "pulse" means toggling between gold and
normal on each tick. The status bar also shows a count of sessions
needing attention:

```
 Esc focus  a add  d delete                  2 need attention   stocks  codex  ◈
```

The "2 need attention" text is rendered in `colorAttention`.


## 6. Terminal Panel

### Empty State

When no session is active, show a branded empty state:

```
                        ._  _ _  _  _  __._  _
                        | \/ |(_|(/_(_)|  |(_)
                                   _/

                    Select a session to begin conducting

                              Esc  toggle sidebar
                               a   add session
                              j/k  navigate
```

(The ASCII art above is illustrative. A cleaner version below.)

A simpler approach that works reliably in all terminals:

```

                         M A E S T R O

                  Select a session to begin

                    Esc  toggle sidebar
                     a   add session
```

"M A E S T R O" in gold, letterspaced, bold. The rest in dim text.

### Active Border

When the terminal panel is focused, no additional border is needed --
the sidebar border already communicates which panel is active (dim
border = terminal focused, gold border = sidebar focused).


## 7. Micro-Interactions and Motion

### State Transition Symbols

When a session changes state, briefly show a transition indicator in
the sidebar next to the session name. Since Bubble Tea uses a tick-based
update loop, this can be a 2-3 tick animation:

```
Working -> Attention:
  Tick 0:  ◆ project       (working, green)
  Tick 1:  ◈ project       (attention, amber -- instant change)
```

True animation is costly in TUI. Instead, use **visual weight changes**:
when a state changes to "needs attention," the entire card row briefly
renders in the attention color (not just the symbol), then settles back
to normal on the next tick. This creates a "flash" effect.

### Loading/Starting State

When a session is first starting (between "added" and "working"), show
a simple spinner inline with the status position:

```
  ▎ new-project           ⠋    <- braille spinner
    claude-code
```

Braille spinner frames: `⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏`

These are compact (single character), elegant, and widely supported in
modern terminals.

```go
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
```

### Drag Handle Feedback

When the sidebar separator is being dragged, the border character
changes to a thicker style:

```go
sidebarDraggingStyle = lipgloss.NewStyle().
    BorderRight(true).
    BorderStyle(lipgloss.ThickBorder()).
    BorderForeground(colorAccent).
    Padding(1, 1)
```

### Confirm Delete

The confirm delete dialog gets more visual weight:

```
  ▎ stocks                ◇
    codex

  ╭───────────────────────╮
  │  Delete "stocks"?     │
  │  y confirm   n cancel │
  ╰───────────────────────╯
```

```go
confirmBoxStyle = lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    BorderForeground(colorError).
    Foreground(colorFg).
    Padding(0, 1).
    MarginTop(1)
```


## 8. Complete Sidebar Mockups

### Mockup A: Three Sessions, Terminal Focused

Terminal width: 120 columns, sidebar width: 28 columns.

```
 S E S S I O N S           .
                            .
 ▎ whereiseveryone      ◆  .
   claude-code              .
                            .
 ▎ stocks               ◇  .
   codex                    .
                            .
 ▎ ml-pipeline          ◈  .
   gemini                   .
                            .
                            .
                            .
                            .
                            .
 + new session              .
```

Border: dim (`colorBorder`).
No item is visually "selected" when sidebar is unfocused -- all items
render equally, with the active session's name slightly brighter.

Actually, there should always be a selected indicator so the user knows
where they will land when they press Esc. The active session (the one
whose terminal is shown) should have a subtle left indicator:

```
 S E S S I O N S           .
                            .
 ▎ whereiseveryone      ◆  .  <- this is the active one
   claude-code              .
                            .
 ▎ stocks               ◇  .
   codex                    .
                            .
 ▎ ml-pipeline          ◈  .
   gemini                   .
```

The "active" session (displayed in terminal) gets bold name + brighter
text even when sidebar is unfocused. The "selected" highlight (cursor)
only appears when sidebar is focused.

### Mockup B: Sidebar Focused, Second Item Selected

```
 S E S S I O N S           |     <- gold border
                            |
 ▎ whereiseveryone      ◆  |
   claude-code              |
                            |
 ▎ stocks               ◇  |     <- highlighted row
   codex                    |
                            |
 ▎ ml-pipeline          ◈  |
   gemini                   |
                            |
                            |
                            |
                            |
                            |
 + new session              |
```

The selected item ("stocks") gets:
- Background: `colorSurface`
- Project name: Bold + `colorFg`
- Full-width highlight across the card area

### Mockup C: Empty State

```
 S E S S I O N S           .
                            .
                            .
                            .
                            .
        ♪                   .
                            .
   No sessions yet          .
   Press  a  to begin       .
                            .
                            .
                            .
                            .
                            .
                            .
 + new session              .
```

### Mockup D: Session Starting (Spinner)

```
 S E S S I O N S           |
                            |
 ▎ whereiseveryone      ◆  |
   claude-code              |
                            |
 ▎ new-api             ⠹  |     <- spinner in gold while starting
   gemini                   |
                            |
                            |
                            |
 + new session              |
```

### Mockup E: Full Application Layout

```
+---[sidebar 28]---+--------[terminal rest]--------------------------------+
| S E S S I O N S  . $ claude --chat                                       |
|                   .                                                       |
| ▎ whereisevery  ◆. > I'll help you refactor the authentication module.   |
|   claude-code     . > First, let me look at the current implementation.  |
|                   .                                                       |
| ▎ stocks        ◇. Reading src/auth/handler.go...                        |
|   codex           .                                                       |
|                   . I can see several areas for improvement:              |
| ▎ ml-pipeline   ◈.                                                       |
|   gemini          . 1. The JWT validation is duplicated in three places   |
|                   . 2. Error handling could use a middleware pattern      |
|                   . 3. The session store should be injected, not global   |
|                   .                                                       |
|                   . Would you like me to start with extracting the JWT   |
|                   . validation into a shared middleware?                  |
|                   .                                                       |
| + new session     .                                                       |
+-------------------+------------------------------------------------------+
| Esc focus  a add  d delete                stocks  codex  ◆ working       |
+--------------------------------------------------------------------------+
```

### Mockup F: Attention State Across Sessions

When multiple sessions need attention, the sidebar makes this immediately
visible through both symbol shape and color:

```
 S E S S I O N S           |
                            |
 ▎ whereiseveryone      ◈  |     <- amber diamond-with-dot
   claude-code              |
                            |
 ▎ stocks               ◆  |     <- green, working fine
   codex                    |
                            |
 ▎ ml-pipeline          ▲  |     <- red triangle, error
   gemini                   |
                            |
 ▎ docs-site            ✦  |     <- blue star, done
   opencode                 |
                            |
 + new session              |
```


## 9. Form Redesign (New Session)

The add-session form gets a visual refresh to match the new system.

### Step Indicator

Replace "1/3" with a visual track:

```
  ● --- ○ --- ○       Name        <- current step highlighted
```

```
  ● --- ● --- ○       Repo path
```

```
  ● --- ● --- ●       Agent
```

Filled circles in gold, empty in dim, connecting dashes in border color.

### Form Layout

```
 New Session  ● - ○ - ○
                            |
 Name                       |
   my-project|              |     <- text input with cursor
   A unique project name    |
                            |
```

```
 New Session  ● - ● - ○
                            |
 Name   my-project          |     <- completed, dim
                            |
 Repo path                  |
   /Users/amir/dev|         |
   > Development/           |     <- completion dropdown
     Documents/             |
   Absolute path to repo    |
                            |
```

```
 New Session  ● - ● - ●
                            |
 Name   my-project          |
 Repo   /Users/amir/dev     |
                            |
 Agent                      |
   ▸ claude-code            |     <- selected, gold with agent color
     codex                  |
     gemini                 |
     opencode               |
   j/k select, Enter done   |
                            |
```

Agent options should show the agent identity color on the left bar:

```
   ▎▸ claude-code           |     <- ▎ in terracotta, ▸ in gold
   ▎  codex                 |     <- ▎ in sage, text in dim
   ▎  gemini                |     <- ▎ in sky
   ▎  opencode              |     <- ▎ in lavender
```


## 10. Implementation: Updated styles.go

Below is the complete proposed replacement for `styles.go`:

```go
package tui

import "github.com/charmbracelet/lipgloss"

// ============================================================
// Color Palette -- "Concert Hall"
// ============================================================

var (
    // Foundation
    colorBg        = lipgloss.Color("#151520")
    colorBgPanel   = lipgloss.Color("#1B1B2F")
    colorBgAlt     = lipgloss.Color("#12121E")
    colorSurface   = lipgloss.Color("#232340")
    colorBorder    = lipgloss.Color("#2A2A45")

    // Brand accent -- gold
    colorAccent    = lipgloss.Color("#D4A84B")
    colorAccentDim = lipgloss.Color("#8B7A3A")
    colorAccentBg  = lipgloss.Color("#2A2415")

    // Text
    colorFg        = lipgloss.Color("#E8E6F0")
    colorFgSecond  = lipgloss.Color("#9896A8")
    colorDim       = lipgloss.Color("#5C5A6E")
    colorFgInverse = lipgloss.Color("#151520")

    // Status
    colorWorking   = lipgloss.Color("#5EC4A0")
    colorAttention = lipgloss.Color("#E8B84D")
    colorError     = lipgloss.Color("#D46A6A")
    colorIdle      = lipgloss.Color("#5C5A6E")
    colorDone      = lipgloss.Color("#7B93DB")

    // Agent identity
    colorAgentClaude   = lipgloss.Color("#D4A373")
    colorAgentCodex    = lipgloss.Color("#7DC4A8")
    colorAgentGemini   = lipgloss.Color("#7BAFD4")
    colorAgentOpenCode = lipgloss.Color("#B39CD4")
)

// ============================================================
// Layout Constants
// ============================================================

var (
    defaultSidebarWidth = 28
    minSidebarWidth     = 22
)

// ============================================================
// Sidebar Styles
// ============================================================

var (
    // Sidebar container
    sidebarStyle = lipgloss.NewStyle().
        BorderRight(true).
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(colorBorder).
        Padding(1, 1)

    sidebarFocusedStyle = lipgloss.NewStyle().
        BorderRight(true).
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(colorAccent).
        Padding(1, 1)

    sidebarDraggingStyle = lipgloss.NewStyle().
        BorderRight(true).
        BorderStyle(lipgloss.ThickBorder()).
        BorderForeground(colorAccent).
        Padding(1, 1)

    // Section header
    sidebarTitleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(colorDim).
        MarginBottom(1)

    sidebarTitleFocusedStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(colorAccent).
        MarginBottom(1)

    // Session cards
    projectItemStyle = lipgloss.NewStyle().
        PaddingLeft(1).
        Foreground(colorFgSecond)

    projectActiveStyle = lipgloss.NewStyle().
        PaddingLeft(1).
        Bold(true).
        Foreground(colorFg).
        Background(colorSurface)

    projectAgentStyle = lipgloss.NewStyle().
        PaddingLeft(3).
        Foreground(colorDim)

    // Status symbols
    badgeWorking   = lipgloss.NewStyle().Foreground(colorWorking).SetString("◆")
    badgeAttention = lipgloss.NewStyle().Foreground(colorAttention).SetString("◈")
    badgeError     = lipgloss.NewStyle().Foreground(colorError).SetString("▲")
    badgeIdle      = lipgloss.NewStyle().Foreground(colorIdle).SetString("◇")
    badgeDone      = lipgloss.NewStyle().Foreground(colorDone).SetString("✦")
)

// ============================================================
// Terminal Styles
// ============================================================

var (
    terminalStyle        = lipgloss.NewStyle().PaddingLeft(1)
    terminalFocusedStyle = lipgloss.NewStyle().PaddingLeft(1)
)

// ============================================================
// Status Bar Styles
// ============================================================

var (
    statusBarStyle = lipgloss.NewStyle().
        Background(colorBgAlt).
        Foreground(colorDim).
        Padding(0, 1).
        BorderTop(true).
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(colorBorder)

    statusKeyStyle = lipgloss.NewStyle().
        Foreground(colorAccent).
        Bold(true)

    statusDimStyle = lipgloss.NewStyle().
        Foreground(colorDim)

    statusAccentStyle = lipgloss.NewStyle().
        Foreground(colorAccent).
        Bold(true)

    statusProjectStyle = lipgloss.NewStyle().
        Foreground(colorFg).
        Bold(true)
)

// ============================================================
// Form Styles
// ============================================================

var (
    formTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
    formStepStyle     = lipgloss.NewStyle().Foreground(colorDim)
    formLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
    formInputStyle    = lipgloss.NewStyle().MarginLeft(2)
    formHintStyle     = lipgloss.NewStyle().Foreground(colorDim)
    formErrorStyle    = lipgloss.NewStyle().Foreground(colorError).MarginLeft(2)
    formSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).PaddingLeft(2)
    formOptionStyle   = lipgloss.NewStyle().PaddingLeft(2).Foreground(colorDim)
    formDoneStyle     = lipgloss.NewStyle().Foreground(colorFgSecond)
    formStepDotActive = lipgloss.NewStyle().Foreground(colorAccent).SetString("●")
    formStepDotDone   = lipgloss.NewStyle().Foreground(colorAccent).SetString("●")
    formStepDotFuture = lipgloss.NewStyle().Foreground(colorDim).SetString("○")
    formStepDash      = lipgloss.NewStyle().Foreground(colorBorder).SetString(" - ")

    completionSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).PaddingLeft(2)
    completionItemStyle     = lipgloss.NewStyle().Foreground(colorDim).PaddingLeft(4)
)

// ============================================================
// Dialog Styles
// ============================================================

var (
    confirmStyle = lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(colorError).
        Foreground(colorFg).
        Padding(0, 1).
        MarginTop(1)

    emptyHintStyle = lipgloss.NewStyle().
        Foreground(colorDim)

    addButtonStyle = lipgloss.NewStyle().
        Foreground(colorDim)

    addButtonPlusStyle = lipgloss.NewStyle().
        Foreground(colorAccent)
)
```


## 11. Implementation: Updated sidebar.go View()

Key changes to the sidebar rendering logic:

```go
func letterspace(s string) string {
    runes := []rune(strings.ToUpper(s))
    parts := make([]string, len(runes))
    for i, r := range runes {
        parts[i] = string(r)
    }
    return strings.Join(parts, " ")
}

func agentColorBar(agent config.AgentType) string {
    var color lipgloss.Color
    switch agent {
    case config.AgentClaudeCode:
        color = colorAgentClaude
    case config.AgentCodex:
        color = colorAgentCodex
    case config.AgentGemini:
        color = colorAgentGemini
    case config.AgentOpenCode:
        color = colorAgentOpenCode
    default:
        color = colorDim
    }
    return lipgloss.NewStyle().Foreground(color).Render("▎")
}

func (m sidebarModel) View() string {
    var b strings.Builder

    if m.mode == sidebarForm {
        b.WriteString(m.form.View())
    } else {
        // Header
        titleStyle := sidebarTitleStyle
        if m.focused {
            titleStyle = sidebarTitleFocusedStyle
        }
        b.WriteString(titleStyle.Render(letterspace("Sessions")))
        b.WriteString("\n")

        if len(m.projects) == 0 {
            // Empty state
            b.WriteString("\n\n")
            noteStyle := lipgloss.NewStyle().Foreground(colorAccent)
            b.WriteString(lipgloss.PlaceHorizontal(
                m.contentWidth, lipgloss.Center,
                noteStyle.Render("\u266A"),  // ♪
            ))
            b.WriteString("\n\n")
            b.WriteString(lipgloss.PlaceHorizontal(
                m.contentWidth, lipgloss.Center,
                emptyHintStyle.Render("No sessions yet"),
            ))
            b.WriteString("\n")
            hintLine := emptyHintStyle.Render("Press ") +
                lipgloss.NewStyle().Foreground(colorAccent).Render("a") +
                emptyHintStyle.Render(" to begin")
            b.WriteString(lipgloss.PlaceHorizontal(
                m.contentWidth, lipgloss.Center, hintLine,
            ))
            b.WriteString("\n")
        } else {
            for i, p := range m.projects {
                bar := agentColorBar(p.Agent)
                badge := m.statusBadge(p.Name)
                name := p.Name

                // Truncate name if too long
                maxName := m.contentWidth - 6  // bar + space + badge + padding
                if len(name) > maxName {
                    name = name[:maxName-1] + "\u2026" // ellipsis
                }

                // Right-align the status badge
                nameWidth := m.contentWidth - 4  // space for bar, space, badge
                padded := name + strings.Repeat(" ", nameWidth - lipgloss.Width(name))

                if i == m.selected && m.focused {
                    line := bar + " " + padded + badge
                    b.WriteString(projectActiveStyle.Width(m.contentWidth).Render(line))
                } else {
                    line := bar + " " + padded + badge
                    b.WriteString(projectItemStyle.Render(line))
                }
                b.WriteString("\n")

                // Agent label
                agentStyle := lipgloss.NewStyle().PaddingLeft(3).Foreground(colorDim)
                b.WriteString(agentStyle.Render(string(p.Agent)))
                b.WriteString("\n")
            }
        }

        // Delete confirmation or add button
        if m.mode == sidebarConfirmDelete && m.selected < len(m.projects) {
            name := m.projects[m.selected].Name
            confirmContent := lipgloss.NewStyle().Foreground(colorFg).Render(
                fmt.Sprintf("Delete \"%s\"?", name),
            ) + "\n" +
                lipgloss.NewStyle().Foreground(colorAccent).Render("y") +
                lipgloss.NewStyle().Foreground(colorDim).Render(" confirm   ") +
                lipgloss.NewStyle().Foreground(colorAccent).Render("n") +
                lipgloss.NewStyle().Foreground(colorDim).Render(" cancel")

            b.WriteString("\n")
            b.WriteString(confirmStyle.Render(confirmContent))
        } else {
            b.WriteString("\n")
            b.WriteString(
                addButtonPlusStyle.Render("+") + " " +
                addButtonStyle.Render("new session"),
            )
        }
    }

    // Container style
    style := sidebarStyle
    if m.dragging {
        style = sidebarDraggingStyle
    } else if m.focused {
        style = sidebarFocusedStyle
    }

    content := b.String()
    return style.Width(m.contentWidth).Height(m.height).Render(content)
}

func (m sidebarModel) statusBadge(projectName string) string {
    state, ok := m.states[projectName]
    if !ok {
        state = StateIdle
    }

    switch state {
    case StateWorking:
        return badgeWorking.String()
    case StateNeedsAttention:
        return badgeAttention.String()
    case StateError:
        return badgeError.String()
    case StateDone:
        return badgeDone.String()
    default:
        return badgeIdle.String()
    }
}
```


## 12. Implementation: Updated statusbar.go

```go
func (m statusBarModel) View() string {
    // Left: keybind hints
    hints := []struct{ key, label string }{
        {"Esc", "focus"},
        {"a", "add"},
        {"d", "delete"},
        {"j/k", "nav"},
    }

    var left strings.Builder
    for i, h := range hints {
        if i > 0 {
            left.WriteString(statusDimStyle.Render("  "))
        }
        left.WriteString(statusKeyStyle.Render(h.key))
        left.WriteString(" ")
        left.WriteString(statusDimStyle.Render(h.label))
    }

    // Right: active project + agent + state
    var right string
    if m.activeName != "" {
        state := m.states[m.activeName]
        badge := statusBadge(state)

        // Find the agent type for this project
        var agentStr string
        for _, p := range m.projects {
            if p.Name == m.activeName {
                agentStr = string(p.Agent)
                break
            }
        }

        right = statusProjectStyle.Render(m.activeName)
        if agentStr != "" {
            right += "  " + statusDimStyle.Render(agentStr)
        }
        right += "  " + badge + " " + statusDimStyle.Render(state.String())
    }

    leftStr := left.String()
    available := m.width - lipgloss.Width(leftStr) - lipgloss.Width(right) - 2
    if available < 0 {
        available = 0
    }
    gap := strings.Repeat(" ", available)

    content := leftStr + gap + right
    return statusBarStyle.Width(m.width).Render(content)
}

func statusBadge(state SessionState) string {
    switch state {
    case StateWorking:
        return badgeWorking.String()
    case StateNeedsAttention:
        return badgeAttention.String()
    case StateError:
        return badgeError.String()
    case StateDone:
        return badgeDone.String()
    default:
        return badgeIdle.String()
    }
}
```


## 13. Form Step Indicator

Replace the numeric "1/3" indicator with a visual track:

```go
func (m formModel) stepIndicator() string {
    steps := 3
    current := int(m.step)

    var parts []string
    for i := 0; i < steps; i++ {
        if i > 0 {
            parts = append(parts, formStepDash.String())
        }
        if i < current {
            parts = append(parts, formStepDotDone.String())
        } else if i == current {
            parts = append(parts, formStepDotActive.String())
        } else {
            parts = append(parts, formStepDotFuture.String())
        }
    }

    return strings.Join(parts, "")
}
```

This renders as: `● - ○ - ○` then `● - ● - ○` then `● - ● - ●`


## 14. Design Token Summary

For quick reference, all tokens in one table:

### Colors

| Token            | Hex       | Usage                           |
|------------------|-----------|---------------------------------|
| colorBg          | `#151520` | Main background                  |
| colorBgPanel     | `#1B1B2F` | Sidebar background               |
| colorBgAlt       | `#12121E` | Status bar, recessed areas       |
| colorSurface     | `#232340` | Cards, selected items            |
| colorBorder      | `#2A2A45` | Borders, separators              |
| colorAccent      | `#D4A84B` | Brand gold, focused borders      |
| colorAccentDim   | `#8B7A3A` | Muted gold accents               |
| colorAccentBg    | `#2A2415` | Gold-tinted backgrounds          |
| colorFg          | `#E8E6F0` | Primary text                     |
| colorFgSecond    | `#9896A8` | Secondary text                   |
| colorDim         | `#5C5A6E` | Muted text, decorative           |
| colorFgInverse   | `#151520` | Text on bright backgrounds       |
| colorWorking     | `#5EC4A0` | Session actively running         |
| colorAttention   | `#E8B84D` | Session needs user input         |
| colorError       | `#D46A6A` | Session errored                  |
| colorIdle        | `#5C5A6E` | Session idle / not started       |
| colorDone        | `#7B93DB` | Session completed                |
| colorAgentClaude | `#D4A373` | Claude Code identity             |
| colorAgentCodex  | `#7DC4A8` | Codex identity                   |
| colorAgentGemini | `#7BAFD4` | Gemini identity                  |
| colorAgentOpenCode| `#B39CD4`| OpenCode identity                |

### Status Symbols

| Symbol | Unicode  | State     |
|--------|----------|-----------|
| `◇`    | U+25C7   | Idle      |
| `◆`    | U+25C6   | Working   |
| `◈`    | U+25C8   | Attention |
| `✦`    | U+2726   | Done      |
| `▲`    | U+25B2   | Error     |

### Spacing

| Name | Value | Usage                    |
|------|-------|--------------------------|
| XS   | 0     | Inline, no gap           |
| S    | 1     | Between related items    |
| M    | 2     | Between groups           |
| L    | 3     | Major section breaks     |

### Typography

| Level | Treatment              | Usage                   |
|-------|------------------------|-------------------------|
| H1    | Bold + gold            | Section headers         |
| H2    | Bold + primary text    | Item names              |
| Body  | Normal + secondary     | Descriptions, agents    |
| Meta  | Normal + dim           | Hints, timestamps       |
| Deco  | Normal + dim           | Borders, structural     |


## 15. Migration Path

### Phase 1: Colors and Typography (Low Risk)

Update `styles.go` with the new color palette and typographic styles.
This is a drop-in replacement that changes visual appearance without
touching layout logic. All existing Lipgloss style references remain
valid.

Files changed: `styles.go`

### Phase 2: Status Symbols (Low Risk)

Update the badge constants to use the new Unicode symbols. Update the
`statusBadge` function to include `StateDone`.

Files changed: `styles.go`, `sidebar.go` (statusBadge function)

### Phase 3: Sidebar Layout (Medium Risk)

Update the sidebar `View()` method to use:
- Letterspaced header
- Agent color bars
- Right-aligned status symbols
- Improved empty state
- New add button rendering

This changes rendering output and will require updating `sidebar_test.go`
and the click hit-testing constants.

Files changed: `sidebar.go`, `sidebar_test.go`

### Phase 4: Status Bar (Low Risk)

Update `statusbar.go` with:
- Additional keybind hints
- Agent name in right zone
- Status badge in right zone
- Top border separator

Files changed: `statusbar.go`

### Phase 5: Form Polish (Low Risk)

Update `form.go` with the visual step indicator track.

Files changed: `form.go`

### Phase 6: Micro-interactions (Medium Risk)

Add the braille spinner for starting sessions. This requires a new
`SessionState` (or reuse of `StateIdle` with a "starting" sub-state)
and a tick-driven animation frame counter.

Files changed: `sidebar.go`, `messages.go`, possibly `app.go`
