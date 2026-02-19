package tui

import "github.com/charmbracelet/lipgloss"

var (
	// ── Color Palette ─────────────────────────────────────────────
	// Warm gold accent with warm-shifted neutrals.
	// Desaturated status colors for dark-background legibility.

	colorPrimary = lipgloss.Color("#D4A053") // warm gold accent
	colorInfo    = lipgloss.Color("#7BAFCC") // steel blue
	colorSuccess = lipgloss.Color("#7EC699") // sage green
	colorWarning = lipgloss.Color("#E5C07B") // muted gold
	colorDanger  = lipgloss.Color("#E06C75") // muted rose

	colorFg     = lipgloss.Color("#E2DCD5") // warm off-white (primary text)
	colorDimFg  = lipgloss.Color("#A89F91") // warm gray (secondary text)
	colorMuted  = lipgloss.Color("#6D6560") // dark warm gray (tertiary)
	colorSubtle = lipgloss.Color("#3D3835") // charcoal (borders)

	colorHighlight = lipgloss.Color("#2A2420") // selected item bg
	colorBgAlt     = lipgloss.Color("#1E1B18") // status bar bg

	// ── Layout ────────────────────────────────────────────────────

	defaultSidebarWidth = 24
	minSidebarWidth     = 20

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

	projectItemStyle   = lipgloss.NewStyle().Foreground(colorDimFg)
	projectActiveStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Background(colorHighlight)
	projectAgentStyle  = lipgloss.NewStyle().PaddingLeft(3).Foreground(colorMuted)

	// ── Status Badges ─────────────────────────────────────────────
	// Distinct shapes for accessibility (not just color).

	badgeWorking   = lipgloss.NewStyle().Foreground(colorSuccess).SetString("●")
	badgeAttention = lipgloss.NewStyle().Foreground(colorWarning).SetString("◆")
	badgeError     = lipgloss.NewStyle().Foreground(colorDanger).SetString("●")
	badgeIdle      = lipgloss.NewStyle().Foreground(colorMuted).SetString("○")
	badgeDone      = lipgloss.NewStyle().Foreground(colorInfo).SetString("✓")

	// ── Tab Header (above terminal panel) ─────────────────────────

	tabHeaderStyle = lipgloss.NewStyle().
			Background(colorBgAlt).
			Foreground(colorDimFg).
			Padding(0, 1)
	tabActiveStyle   = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true) // no bg — blends into terminal
	tabInactiveStyle = lipgloss.NewStyle().Foreground(colorDimFg).Background(colorBgAlt)
	tabSepStyle      = lipgloss.NewStyle() // unused — bg contrast is the separator
	tabDimStyle      = lipgloss.NewStyle().Foreground(colorMuted)

	// ── Terminal Panel ────────────────────────────────────────────

	terminalStyle        = lipgloss.NewStyle().PaddingLeft(1)
	terminalFocusedStyle = lipgloss.NewStyle().PaddingLeft(1)

	// ── Status Bar ────────────────────────────────────────────────

	statusBarStyle    = lipgloss.NewStyle().Background(colorBgAlt).Foreground(colorDimFg).Padding(0, 1)
	statusKeyStyle    = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	statusDimStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	statusAccentStyle = lipgloss.NewStyle().Foreground(colorFg).Bold(true)

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
