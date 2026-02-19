package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/amir/maestro/internal/config"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Force TrueColor output so ANSI background codes are emitted in tests.
	lipgloss.DefaultRenderer().SetColorProfile(termenv.TrueColor)
}

func makeTestApp() App {
	cfg := &config.Config{
		Projects: []config.Project{
			{Name: "stocks", Repo: "/tmp/stocks", Agent: config.AgentOpenCode},
			{Name: "Where is everyone", Repo: "/tmp/whereiseveryone", Agent: config.AgentClaudeCode},
		},
	}

	app := NewApp(cfg, "")
	app.width = 80
	app.height = 12
	app.ready = true
	app.sidebar.states["stocks"] = StateWorking
	app.statusbar.states["stocks"] = StateWorking
	app.sidebar.states["Where is everyone"] = StateNeedsAttention
	app.statusbar.states["Where is everyone"] = StateNeedsAttention
	app.addTab("Where is everyone")
	app.layout()
	return app
}

// bgAltSubstring is the ANSI parameter for colorBgAlt (#1E1B18 = rgb(30,27,24)).
const bgAltSubstring = "48;2;30;27;24"

func TestVisualRender(t *testing.T) {
	app := makeTestApp()

	// Case 1: Left tab active
	app.active = 0
	app.sidebar.selected = 0
	app.statusbar.activeName = "stocks"

	fmt.Println("\n=== LEFT TAB ACTIVE ===")
	fmt.Printf("Tab header : %q\n", app.tabHeaderView())
	fmt.Printf("Tab border : %q\n", app.tabBorderView())
	fmt.Println("--- rendered ---")
	fmt.Println(app.tabHeaderView())
	fmt.Println(app.tabBorderView())

	// Case 2: Right tab active
	app.active = 1
	app.sidebar.selected = 1
	app.statusbar.activeName = "Where is everyone"

	fmt.Println("\n=== RIGHT TAB ACTIVE ===")
	fmt.Printf("Tab header : %q\n", app.tabHeaderView())
	fmt.Printf("Tab border : %q\n", app.tabBorderView())
	fmt.Println("--- rendered ---")
	fmt.Println(app.tabHeaderView())
	fmt.Println(app.tabBorderView())

	// Full view
	fmt.Println("\n=== FULL VIEW (right active) ===")
	fmt.Println(app.View())
}

func TestTabVisualInvariants(t *testing.T) {
	app := makeTestApp()

	t.Run("no separator character between tabs", func(t *testing.T) {
		app.active = 0
		header := app.tabHeaderView()
		stripped := stripAnsi(header)
		if strings.Contains(stripped, "│") {
			t.Error("tab header should not contain │ separator")
		}
	})

	t.Run("active tab has no bgAlt", func(t *testing.T) {
		app.active = 0
		header := app.tabHeaderView()
		// Active tab is the first segment. Find it by the bracket style.
		bracketIdx := strings.Index(header, "[●")
		if bracketIdx < 0 {
			t.Fatal("[● not found in header")
		}
		// The first ANSI escape should be the active style — no bgAlt.
		firstEscEnd := strings.Index(header, "m")
		firstEsc := header[:firstEscEnd+1]
		if strings.Contains(firstEsc, bgAltSubstring) {
			t.Errorf("active tab style has bgAlt: %q", firstEsc)
		}
	})

	t.Run("inactive tab has bgAlt", func(t *testing.T) {
		app.active = 0
		header := app.tabHeaderView()
		// Inactive tab contains "Where is everyone" text.
		nameIdx := strings.Index(header, "Where is everyone")
		if nameIdx < 0 {
			t.Fatal("inactive name not found")
		}
		// Look backwards for closest ANSI escape.
		prefix := header[:nameIdx]
		lastEsc := strings.LastIndex(prefix, "\x1b[")
		escEnd := strings.Index(prefix[lastEsc:], "m")
		style := prefix[lastEsc : lastEsc+escEnd+1]
		if !strings.Contains(style, bgAltSubstring) {
			t.Errorf("inactive style missing bgAlt: %q", style)
		}
	})

	t.Run("border gap under active left tab", func(t *testing.T) {
		app.active = 0
		border := app.tabBorderView()
		stripped := stripAnsi(border)
		t.Logf("border: %q", stripped)

		// Starts with spaces (gap under active left tab).
		if strings.HasPrefix(stripped, "─") {
			t.Error("border starts with ─ but left tab is active")
		}
		// Has ─ somewhere (under inactive right tab).
		if !strings.Contains(stripped, "─") {
			t.Error("no ─ under inactive tab")
		}
	})

	t.Run("border gap under active right tab", func(t *testing.T) {
		app.active = 1
		border := app.tabBorderView()
		stripped := stripAnsi(border)
		t.Logf("border: %q", stripped)

		// Starts with ─ (under inactive left tab).
		if !strings.HasPrefix(stripped, "─") {
			t.Error("border should start with ─")
		}
		// Ends with spaces (gap under active right tab).
		if !strings.HasSuffix(stripped, " ") {
			t.Error("border should end with spaces")
		}
	})

	t.Run("border has no bgAlt", func(t *testing.T) {
		app.active = 0
		border := app.tabBorderView()
		if strings.Contains(border, bgAltSubstring) {
			t.Error("border should not have bgAlt — just ─ chars on default bg")
		}
	})

	t.Run("single tab — full gap", func(t *testing.T) {
		singleCfg := &config.Config{
			Projects: []config.Project{
				{Name: "solo", Repo: "/tmp/solo", Agent: config.AgentClaudeCode},
			},
		}
		single := NewApp(singleCfg, "")
		single.width = 80
		single.height = 12
		single.ready = true
		single.layout()

		border := single.tabBorderView()
		stripped := stripAnsi(border)
		if strings.Contains(stripped, "─") {
			t.Errorf("single active tab should have no ─, got: %q", stripped)
		}
	})

	t.Run("tab widths match panel", func(t *testing.T) {
		app.active = 0
		header := app.tabHeaderView()
		border := app.tabBorderView()

		sbWidth := app.sidebar.Width()
		panelWidth := app.width - sbWidth

		if w := lipgloss.Width(header); w != panelWidth {
			t.Errorf("header width = %d, expected %d", w, panelWidth)
		}
		if w := lipgloss.Width(border); w != panelWidth {
			t.Errorf("border width = %d, expected %d", w, panelWidth)
		}
	})

	t.Run("full view line widths", func(t *testing.T) {
		app.active = 1
		app.sidebar.selected = 1
		app.statusbar.activeName = "Where is everyone"

		view := app.View()
		lines := strings.Split(view, "\n")

		for i, line := range lines {
			w := lipgloss.Width(line)
			if w != app.width && w != 0 {
				t.Errorf("line %d width = %d, expected %d: %q", i, w, app.width, line)
			}
		}
	})
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
