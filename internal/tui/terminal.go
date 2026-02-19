package tui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// vt10x attribute flags — mirrored from unexported constants in vt10x/state.go.
const (
	vtAttrReverse   = 1 << 0
	vtAttrUnderline = 1 << 1
	vtAttrBold      = 1 << 2
	vtAttrItalic    = 1 << 4
	vtAttrBlink     = 1 << 5
)

// ptyOutputMsg carries raw bytes read from the PTY.
type ptyOutputMsg []byte

// ptyExitedMsg signals the PTY process has exited.
type ptyExitedMsg struct{ err error }

type terminalModel struct {
	ptmx    *os.File
	cmd     *exec.Cmd
	vt      vt10x.Terminal
	mu      *sync.RWMutex
	width   int
	height  int
	focused bool
	active  bool
}

func newTerminalModel() terminalModel {
	return terminalModel{
		mu:      &sync.RWMutex{},
		focused: true,
		active:  false,
	}
}

func (m *terminalModel) StartShell(width, height int) tea.Cmd {
	m.width = width
	m.height = height

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	return m.startProcess(exec.Command(shell), width, height)
}

func (m *terminalModel) StartCommand(args []string, dir string, width, height int) tea.Cmd {
	m.width = width
	m.height = height

	if len(args) == 0 {
		return nil
	}
	cmd := exec.Command(args[0], args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}

	return m.startProcess(cmd, width, height)
}

func (m *terminalModel) startProcess(cmd *exec.Cmd, width, height int) tea.Cmd {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	vt := vt10x.New(vt10x.WithSize(width, height))
	m.vt = vt

	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return func() tea.Msg { return ptyExitedMsg{err: err} }
	}

	m.ptmx = ptmx
	m.cmd = cmd
	m.active = true

	return m.readLoop()
}

func (m *terminalModel) readLoop() tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := m.ptmx.Read(buf)
		if err != nil {
			return ptyExitedMsg{err: err}
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		return ptyOutputMsg(data)
	}
}

func (m terminalModel) Update(msg tea.Msg) (terminalModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ptyOutputMsg:
		if !m.active {
			return m, nil
		}
		m.mu.Lock()
		m.vt.Write(msg)
		m.mu.Unlock()
		return m, m.readLoop()

	case ptyExitedMsg:
		m.active = false
		return m, nil

	case tea.KeyMsg:
		if !m.active || !m.focused {
			return m, nil
		}
		data := keyToBytes(msg)
		if data != nil && m.ptmx != nil {
			m.ptmx.Write(data)
		}
		return m, nil
	}

	return m, nil
}

func (m terminalModel) View() string {
	if !m.active {
		line1 := emptyHintStyle.Render("No active session")
		line2 := emptyHintStyle.Render("Select a project to start")
		content := line1 + "\n" + line2
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.vt == nil {
		return ""
	}

	var sb strings.Builder
	for row := 0; row < m.height; row++ {
		var curFG vt10x.Color = vt10x.DefaultFG
		var curBG vt10x.Color = vt10x.DefaultBG
		var curMode int16

		for col := 0; col < m.width; col++ {
			g := m.vt.Cell(col, row)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}

			if g.FG != curFG || g.BG != curBG || g.Mode != curMode {
				sb.WriteString(glyphSGR(g))
				curFG = g.FG
				curBG = g.BG
				curMode = g.Mode
			}

			sb.WriteRune(ch)
		}

		// Reset at end of line to prevent attribute bleeding.
		if curFG != vt10x.DefaultFG || curBG != vt10x.DefaultBG || curMode != 0 {
			sb.WriteString("\x1b[0m")
		}
		if row < m.height-1 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// glyphSGR returns an SGR escape sequence that sets the terminal attributes
// to match the given vt10x glyph's foreground, background, and mode.
func glyphSGR(g vt10x.Glyph) string {
	var sb strings.Builder
	sb.WriteString("\x1b[0") // reset as base

	if g.Mode&vtAttrBold != 0 {
		sb.WriteString(";1")
	}
	if g.Mode&vtAttrItalic != 0 {
		sb.WriteString(";3")
	}
	if g.Mode&vtAttrUnderline != 0 {
		sb.WriteString(";4")
	}
	if g.Mode&vtAttrBlink != 0 {
		sb.WriteString(";5")
	}

	if g.FG != vt10x.DefaultFG {
		sb.WriteByte(';')
		sb.WriteString(colorSGR(g.FG, true))
	}
	if g.BG != vt10x.DefaultBG {
		sb.WriteByte(';')
		sb.WriteString(colorSGR(g.BG, false))
	}

	sb.WriteByte('m')
	return sb.String()
}

// colorSGR converts a vt10x Color to an SGR parameter string.
func colorSGR(c vt10x.Color, fg bool) string {
	n := uint32(c)

	// Special defaults (DefaultFG, DefaultBG, DefaultCursor) are >= 1<<24.
	if n >= 1<<24 {
		if fg {
			return "39"
		}
		return "49"
	}

	// Standard ANSI 0-7.
	if n < 8 {
		if fg {
			return strconv.Itoa(30 + int(n))
		}
		return strconv.Itoa(40 + int(n))
	}

	// Bright ANSI 8-15.
	if n < 16 {
		if fg {
			return strconv.Itoa(90 + int(n) - 8)
		}
		return strconv.Itoa(100 + int(n) - 8)
	}

	// 256-color palette 16-255.
	if n < 256 {
		if fg {
			return "38;5;" + strconv.Itoa(int(n))
		}
		return "48;5;" + strconv.Itoa(int(n))
	}

	// True color RGB (256 – 16777215).
	r := (n >> 16) & 0xFF
	g := (n >> 8) & 0xFF
	b := n & 0xFF
	if fg {
		return "38;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b))
	}
	return "48;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b))
}

func (m *terminalModel) SetSize(width, height int) {
	if width < 1 || height < 1 {
		return
	}
	m.width = width
	m.height = height
	if m.active && m.ptmx != nil {
		pty.Setsize(m.ptmx, &pty.Winsize{
			Rows: uint16(height),
			Cols: uint16(width),
		})
	}
	m.mu.Lock()
	if m.vt != nil {
		m.vt.Resize(width, height)
	}
	m.mu.Unlock()
}

func (m *terminalModel) GetScreenLines() []string {
	if !m.active || m.vt == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	lines := make([]string, m.height)
	for row := 0; row < m.height; row++ {
		var sb strings.Builder
		for col := 0; col < m.width; col++ {
			g := m.vt.Cell(col, row)
			if g.Char == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(g.Char)
			}
		}
		lines[row] = strings.TrimRight(sb.String(), " ")
	}
	return lines
}

func (m *terminalModel) Close() {
	if m.ptmx != nil {
		m.ptmx.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Signal(os.Interrupt)
	}
}

func keyToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyCtrlA:
		return []byte{0x01}
	case tea.KeyCtrlB:
		return []byte{0x02}
	case tea.KeyCtrlD:
		return []byte{0x04}
	case tea.KeyCtrlE:
		return []byte{0x05}
	case tea.KeyCtrlF:
		return []byte{0x06}
	case tea.KeyCtrlK:
		return []byte{0x0b}
	case tea.KeyCtrlL:
		return []byte{0x0c}
	case tea.KeyCtrlN:
		return []byte{0x0e}
	case tea.KeyCtrlP:
		return []byte{0x10}
	case tea.KeyCtrlR:
		return []byte{0x12}
	case tea.KeyCtrlU:
		return []byte{0x15}
	case tea.KeyCtrlW:
		return []byte{0x17}
	default:
		s := msg.String()
		if s != "" && !strings.HasPrefix(s, "ctrl+") && !strings.HasPrefix(s, "alt+") {
			return []byte(s)
		}
		return nil
	}
}

