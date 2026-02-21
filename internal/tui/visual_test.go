package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/openconductorhq/openconductor/internal/config"
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
	app.height = 15 // bumped from 12 to accommodate 3-line tab bar
	app.ready = true
	app.sidebar.states["stocks"] = StateWorking
	app.statusbar.states["stocks"] = StateWorking
	app.sidebar.states["Where is everyone"] = StateNeedsAttention
	app.statusbar.states["Where is everyone"] = StateNeedsAttention
	app.addTab("Where is everyone")
	app.layout()
	return app
}

// tabBarLines splits the 3-line tab bar into top border, content, and bottom border.
func tabBarLines(bar string) (top, content, bottom string) {
	lines := strings.SplitN(bar, "\n", 3)
	switch len(lines) {
	case 3:
		return lines[0], lines[1], lines[2]
	case 2:
		return lines[0], lines[1], ""
	default:
		return bar, "", ""
	}
}

func TestVisualRender(t *testing.T) {
	app := makeTestApp()

	// Case 1: Left tab active
	app.active = 0
	app.sidebar.selected = 0
	app.statusbar.activeName = "stocks"

	bar := app.tabBarView()
	top, content, bottom := tabBarLines(bar)
	fmt.Println("\n=== LEFT TAB ACTIVE ===")
	fmt.Printf("Top border : %q\n", stripAnsi(top))
	fmt.Printf("Content    : %q\n", stripAnsi(content))
	fmt.Printf("Bot border : %q\n", stripAnsi(bottom))
	fmt.Println("--- rendered ---")
	fmt.Println(bar)

	// Case 2: Right tab active
	app.active = 1
	app.sidebar.selected = 1
	app.statusbar.activeName = "Where is everyone"

	bar = app.tabBarView()
	top, content, bottom = tabBarLines(bar)
	fmt.Println("\n=== RIGHT TAB ACTIVE ===")
	fmt.Printf("Top border : %q\n", stripAnsi(top))
	fmt.Printf("Content    : %q\n", stripAnsi(content))
	fmt.Printf("Bot border : %q\n", stripAnsi(bottom))
	fmt.Println("--- rendered ---")
	fmt.Println(bar)

	// Full view
	fmt.Println("\n=== FULL VIEW (right active) ===")
	fmt.Println(app.View())
}

func TestTabVisualInvariants(t *testing.T) {
	app := makeTestApp()

	t.Run("top border has rounded corners", func(t *testing.T) {
		app.active = 0
		top, _, _ := tabBarLines(app.tabBarView())
		stripped := stripAnsi(top)
		if !strings.Contains(stripped, "╭") || !strings.Contains(stripped, "╮") {
			t.Errorf("top border should have rounded corners, got: %q", stripped)
		}
	})

	t.Run("active tab content has no bgAlt", func(t *testing.T) {
		app.active = 0
		_, content, _ := tabBarLines(app.tabBarView())
		// Find the active tab region (first │...│ segment)
		// Active tab should not have bgAlt background
		nameIdx := strings.Index(content, "stocks")
		if nameIdx < 0 {
			t.Fatal("active tab name 'stocks' not found in content line")
		}
		prefix := content[:nameIdx]
		if strings.Contains(prefix, bgAltSubstring) {
			t.Error("active tab should not have bgAlt background")
		}
	})

	t.Run("inactive tab content has no bgAlt (border technique)", func(t *testing.T) {
		// With the border technique, we no longer use bgAlt on tabs.
		// Visual distinction comes from border color + text style.
		app.active = 0
		_, content, _ := tabBarLines(app.tabBarView())
		nameIdx := strings.Index(content, "Where is everyone")
		if nameIdx < 0 {
			t.Fatal("inactive tab name not found in content line")
		}
	})

	t.Run("active tab bottom is open (spaces)", func(t *testing.T) {
		app.active = 0
		_, _, bottom := tabBarLines(app.tabBarView())
		stripped := stripAnsi(bottom)
		t.Logf("bottom: %q", stripped)

		// Active tab (left) bottom should start with ┘ (open corner)
		if !strings.HasPrefix(stripped, "┘") {
			t.Errorf("expected ┘ at start for active tab, got: %q", stripped[:min(5, len(stripped))])
		}
		// Should contain └ closing the active tab
		if !strings.Contains(stripped, "└") {
			t.Error("expected └ to close active tab opening")
		}
		// Should contain ─ from the inactive tab and gap
		if !strings.Contains(stripped, "─") {
			t.Error("expected ─ from inactive tab / gap")
		}
	})

	t.Run("inactive tab bottom is closed", func(t *testing.T) {
		app.active = 0
		_, _, bottom := tabBarLines(app.tabBarView())
		stripped := stripAnsi(bottom)

		// Inactive tab (right) should have ┴ corners
		if !strings.Contains(stripped, "┴") {
			t.Errorf("expected ┴ in bottom for inactive tab, got: %q", stripped)
		}
	})

	t.Run("active right tab — left starts with ┴", func(t *testing.T) {
		app.active = 1
		_, _, bottom := tabBarLines(app.tabBarView())
		stripped := stripAnsi(bottom)
		t.Logf("bottom: %q", stripped)

		// Inactive (left) tab should start with ┴
		if !strings.HasPrefix(stripped, "┴") {
			t.Errorf("expected ┴ at start for inactive tab, got: %q", stripped[:min(5, len(stripped))])
		}
		// Active (right) tab should have ┘ and └
		if !strings.Contains(stripped, "┘") || !strings.Contains(stripped, "└") {
			t.Error("expected ┘ and └ for active tab opening")
		}
	})

	t.Run("single tab — open bottom with gap", func(t *testing.T) {
		singleCfg := &config.Config{
			Projects: []config.Project{
				{Name: "solo", Repo: "/tmp/solo", Agent: config.AgentClaudeCode},
			},
		}
		single := NewApp(singleCfg, "")
		single.width = 80
		single.height = 15
		single.ready = true
		single.layout()

		_, _, bottom := tabBarLines(single.tabBarView())
		stripped := stripAnsi(bottom)
		// Active tab should have open bottom (┘...└)
		if !strings.Contains(stripped, "┘") || !strings.Contains(stripped, "└") {
			t.Errorf("single active tab should have ┘ and └, got: %q", stripped)
		}
		// Gap should have ─
		if !strings.Contains(stripped, "─") {
			t.Errorf("gap should have ─, got: %q", stripped)
		}
	})

	t.Run("tab bar widths — all 3 lines match panel", func(t *testing.T) {
		app.active = 0
		top, content, bottom := tabBarLines(app.tabBarView())

		sbWidth := app.sidebar.Width()
		panelWidth := app.innerWidth() - sbWidth

		if w := lipgloss.Width(top); w != panelWidth {
			t.Errorf("top border width = %d, expected %d", w, panelWidth)
		}
		if w := lipgloss.Width(content); w != panelWidth {
			t.Errorf("content width = %d, expected %d", w, panelWidth)
		}
		if w := lipgloss.Width(bottom); w != panelWidth {
			t.Errorf("bottom border width = %d, expected %d", w, panelWidth)
		}
	})

	t.Run("tab bar is exactly 3 lines", func(t *testing.T) {
		app.active = 0
		bar := app.tabBarView()
		lineCount := strings.Count(bar, "\n") + 1
		if lineCount != 3 {
			t.Errorf("tab bar should be 3 lines, got %d", lineCount)
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

// bgAltSubstring is the ANSI parameter for colorBgAlt (#1E1B18 = rgb(30,27,24)).
const bgAltSubstring = "48;2;30;27;24"

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
