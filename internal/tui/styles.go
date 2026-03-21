// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import "github.com/charmbracelet/lipgloss"

var (
	// ── Color Palette ─────────────────────────────────────────────
	// Warm gold accent with warm-shifted neutrals.
	// Desaturated status colors for dark-background legibility.

	colorPrimary  = lipgloss.Color("#D4A053") // warm gold accent
	colorInfo     = lipgloss.Color("#7BAFCC") // steel blue
	colorSuccess  = lipgloss.Color("#7EC699") // sage green
	colorWarning  = lipgloss.Color("#E5C07B") // muted gold
	colorDanger   = lipgloss.Color("#E06C75") // muted rose
	colorQuestion = lipgloss.Color("#56B6C2") // teal — agent asking a question

	colorFg     = lipgloss.Color("#E2DCD5") // warm off-white (primary text)
	colorDimFg  = lipgloss.Color("#A89F91") // warm gray (secondary text)
	colorMuted  = lipgloss.Color("#6D6560") // dark warm gray (tertiary)
	colorSubtle = lipgloss.Color("#3D3835") // charcoal (borders)

	colorHighlight = lipgloss.Color("#2A2420") // selected item bg
	colorBgAlt     = lipgloss.Color("#1E1B18") // status bar bg

	// ── Layout ────────────────────────────────────────────────────

	defaultSidebarWidth = 26
	minSidebarWidth     = 20
	screenPadding       = 1 // horizontal padding on each side of the screen
	statusBarRows       = 1 // status bar is always 1 row
)

// Derived layout values — computed from the styles above so that changes
// to padding, borders, or margins propagate automatically. These are set
// in init() after the style vars are initialised.
var (
	// terminalPadLeft is the left-padding the terminal style applies.
	// Used to translate screen X to PTY X and to compute terminal width.
	terminalPadLeft int

	// sidebarHPad is the total horizontal padding of the sidebar style
	// (left + right). Used to compute the inner width available for
	// sidebar content.
	sidebarHPad int

	// sidebarTopPadding is the top padding of the sidebar style. Used
	// for mouse click hit-testing against sidebar items.
	sidebarTopPadding int

	// activeProjectBorderW is the horizontal frame size (borders) of
	// the selected project item style. Used to compute the available
	// content width inside the accent-bar selection indicator.
	activeProjectBorderW int

	// tabCloseRegion is the width of the clickable close-button region
	// at the right end of each tab (right padding + "✕" + space).
	tabCloseRegion int

	// statusBarHPad is the total horizontal padding of the status bar
	// style. Used to compute the gap between left and right content.
	statusBarHPad int

	// Minimum host terminal dimensions below which the app shows a
	// "terminal too small" overlay instead of the normal UI. These are
	// the outer dimensions (the full terminal window), not the inner
	// content area.
	minAppWidth  = 60
	minAppHeight = 10

	// ── Sidebar ───────────────────────────────────────────────────

	sidebarStyle = lipgloss.NewStyle().
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorSubtle).
			Padding(1, 1)

	sidebarFocusedStyle = lipgloss.NewStyle().
				BorderRight(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorPrimary).
				Padding(1, 1)

	sidebarDraggingStyle = lipgloss.NewStyle().
				BorderRight(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorWarning).
				Padding(1, 1)

	sidebarTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorDimFg).
				MarginBottom(1)

	sidebarTitleFocusedStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(colorPrimary).
					MarginBottom(1)

	// ── Project Items ─────────────────────────────────────────────

	projectItemStyle      = lipgloss.NewStyle().Foreground(colorDimFg)
	projectAgentStyle     = lipgloss.NewStyle().PaddingLeft(3).Foreground(colorMuted)
	projectSeparatorStyle = lipgloss.NewStyle().Foreground(colorSubtle)

	// Selected project: gold ▎ left accent bar with highlight background.
	// Both name and agent lines render as a single block so the accent
	// spans the full item height. Content uses colorFg (off-white) for
	// readability; the gold accent bar provides the selection signal.
	projectAccentBorder = lipgloss.Border{
		Left: "▎",
	}

	projectActiveStyle = lipgloss.NewStyle().
				Border(projectAccentBorder, false, false, false, true).
				BorderForeground(colorPrimary).
				Foreground(colorFg).
				Bold(true).
				Background(colorHighlight)

	// ── Status Badges ─────────────────────────────────────────────
	// Green ● = online (steady idle, breathing working).
	// Red ● = error / attention analysis issue.
	// Gold ◆ = needs user attention.
	// Orange ! = needs permission decision.
	// Teal ? = agent asking a question.
	// Blue ✓ = task done.

	colorSuccessMid = lipgloss.Color("#5E9A78") // mid green for breathing mid-frame
	colorPermission = lipgloss.Color("#FF8C00") // orange: permission request

	badgeOnline     = lipgloss.NewStyle().Foreground(colorSuccess).SetString("●")    // green: agent online
	badgeAttention  = lipgloss.NewStyle().Foreground(colorWarning).SetString("◆")    // gold: needs input
	badgePermission = lipgloss.NewStyle().Foreground(colorPermission).SetString("!") // orange: needs permission
	badgeAsking     = lipgloss.NewStyle().Foreground(colorQuestion).SetString("?")   // teal: agent question
	badgeError      = lipgloss.NewStyle().Foreground(colorDanger).SetString("●")
	badgeDone       = lipgloss.NewStyle().Foreground(colorInfo).SetString("✓")

	// Breathing cycle for StateWorking: ● bright → • mid → · dim → • mid.
	// Each frame is 600ms, full cycle is 2.4s.
	breathingBadgeStyles = [4]lipgloss.Style{
		lipgloss.NewStyle().Foreground(colorSuccess).SetString("●"),    // frame 0: full, bright
		lipgloss.NewStyle().Foreground(colorSuccessMid).SetString("•"), // frame 1: shrinking, mid
		lipgloss.NewStyle().Foreground(colorMuted).SetString("·"),      // frame 2: smallest, dim
		lipgloss.NewStyle().Foreground(colorSuccessMid).SetString("•"), // frame 3: growing, mid
	}

	// ── Tab Bar (lipgloss border technique) ──────────────────────
	// Inspired by github.com/charmbracelet/lipgloss/examples/layout/main.go
	// See .opencode/skills/lipgloss-guide/SKILL.md for details.
	//
	// Active tab:   open bottom (space) merges into terminal below, gold border.
	// Inactive tab: closed bottom (─) forms continuous border line, subtle border.
	// Gap:          bottom-only border fills remaining width with ─.

	activeTabBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      " ",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┘",
		BottomRight: "└",
	}

	inactiveTabBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┴",
		BottomRight: "┴",
	}

	tabStyle = lipgloss.NewStyle().
			Border(inactiveTabBorder, true).
			BorderForeground(colorSubtle).
			Foreground(colorDimFg).
			Padding(0, 1)

	tabActiveStyle = lipgloss.NewStyle().
			Border(activeTabBorder, true).
			BorderForeground(colorPrimary).
			Background(colorHighlight).
			Foreground(colorPrimary).
			Bold(true).
			Padding(0, 1)

	tabGapStyle = lipgloss.NewStyle().
			Border(inactiveTabBorder, true).
			BorderTop(false).
			BorderLeft(false).
			BorderRight(false).
			BorderForeground(colorSubtle)

	// ── Terminal Panel ────────────────────────────────────────────

	terminalStyle        = lipgloss.NewStyle().PaddingLeft(1)
	terminalFocusedStyle = lipgloss.NewStyle().PaddingLeft(1)

	// ── Status Bar ────────────────────────────────────────────────

	statusBarStyle      = lipgloss.NewStyle().Background(colorBgAlt).Foreground(colorDimFg).Padding(0, 1)
	statusKeyStyle      = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	statusDimStyle      = lipgloss.NewStyle().Foreground(colorMuted)
	statusAccentStyle   = lipgloss.NewStyle().Foreground(colorFg).Bold(true)
	statusExitHintStyle = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)

	// ── Forms ─────────────────────────────────────────────────────

	formTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	formStepStyle     = lipgloss.NewStyle().Foreground(colorMuted)
	formLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	formInputStyle    = lipgloss.NewStyle().MarginLeft(2)
	formHintStyle     = lipgloss.NewStyle().Foreground(colorMuted)
	formErrorStyle    = lipgloss.NewStyle().Foreground(colorDanger).MarginLeft(2)
	formSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).PaddingLeft(2)
	formOptionStyle   = lipgloss.NewStyle().PaddingLeft(2).Foreground(colorDimFg)
	formDoneStyle     = lipgloss.NewStyle().Foreground(colorDimFg)

	// ── Completion ────────────────────────────────────────────────

	completionSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).PaddingLeft(2)
	completionItemStyle     = lipgloss.NewStyle().Foreground(colorDimFg).PaddingLeft(4)

	// ── Misc ──────────────────────────────────────────────────────

	confirmStyle   = lipgloss.NewStyle().Foreground(colorDanger)
	emptyHintStyle = lipgloss.NewStyle().Foreground(colorMuted)
	addButtonStyle = lipgloss.NewStyle().Foreground(colorPrimary)
)

func init() {
	// Derive layout values from the styles so that changes to padding,
	// borders, or margins propagate automatically to coordinate
	// calculations and hit-testing.
	terminalPadLeft = terminalStyle.GetPaddingLeft()
	sidebarHPad = sidebarStyle.GetHorizontalPadding()
	sidebarTopPadding = sidebarStyle.GetPaddingTop()
	activeProjectBorderW = projectActiveStyle.GetHorizontalBorderSize()
	statusBarHPad = statusBarStyle.GetHorizontalPadding()

	// Tab close region: right padding + " ✕" (space + icon).
	// The close icon "✕" is 1 visual column; the space before it is 1.
	closeIconWidth := lipgloss.Width(" ✕")
	tabRightPad := tabStyle.GetPaddingRight()
	tabCloseRegion = tabRightPad + closeIconWidth
}
