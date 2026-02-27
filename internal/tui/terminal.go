// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"

	"github.com/openconductorhq/openconductor/internal/logging"
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

	// Scrollback state. The buffer captures glyph rows as they scroll off
	// the top of the vt10x viewport. scrollOffset tracks how many lines
	// the user has scrolled up from the live view (0 = live).
	scrollback   *scrollbackBuffer
	scrollOffset int

	// scrollPinned is true when the user has scrolled up into history and
	// wants the view to stay pinned to the same content. When new lines
	// are pushed to the scrollback, the offset is auto-adjusted to
	// compensate. Set to false when the user scrolls down (toward live)
	// so the offset can freely reach 0 without fighting auto-adjustment.
	scrollPinned bool

	// altScreen is true when the underlying session is running in
	// alternate screen mode (TUI apps like OpenCode). In alt-screen mode,
	// the viewport is a self-contained screen (header + content + footer),
	// so scrollback rendering shows ONLY scrollback lines instead of
	// mixing scrollback with live viewport rows.
	altScreen bool
}

func newTerminalModel() terminalModel {
	return terminalModel{
		mu:         &sync.RWMutex{},
		focused:    true,
		active:     false,
		scrollback: newScrollbackBuffer(defaultScrollbackCapacity),
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

// Update handles terminal-level messages. In the OpenConductor app flow,
// this method is NOT called — the App handles session I/O at a higher level
// (sessionOutputMsg + timer-based runScrollCheck). This Update path exists
// for standalone terminalModel use (StartShell/StartCommand) and is
// exercised by scrollback_test.go.
func (m terminalModel) Update(msg tea.Msg) (terminalModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ptyOutputMsg:
		if !m.active {
			return m, nil
		}
		m.mu.Lock()
		m.captureAndWrite(msg)
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

	if m.scrollOffset > 0 {
		return m.viewScrollback()
	}
	return m.viewLive()
}

// viewLive renders the current vt10x viewport (scrollOffset == 0).
func (m terminalModel) viewLive() string {
	// Read the cursor position so we can render a visible block cursor.
	cursorCol, cursorRow := -1, -1
	if m.vt.CursorVisible() {
		cur := m.vt.Cursor()
		cursorCol = cur.X
		cursorRow = cur.Y
	}

	var sb strings.Builder
	for row := 0; row < m.height; row++ {
		cc := -1
		if row == cursorRow {
			cc = cursorCol
		}
		m.renderViewportRow(&sb, row, cc)
		if row < m.height-1 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// viewScrollback renders scrollback content when the user has scrolled up
// (scrollOffset > 0).
//
// For alt-screen sessions (TUI apps), the viewport is a self-contained
// screen (header + content + footer), so we show ONLY scrollback lines.
// Mixing scrollback with live viewport rows would duplicate content that
// was captured into scrollback but is still visible on the current screen.
//
// For regular terminal sessions, scrollback and viewport are contiguous,
// so we render a mix: scrollback lines at top, live viewport rows at bottom.
func (m terminalModel) viewScrollback() string {
	if m.altScreen {
		logging.Debug("scrollback: rendering alt-screen only",
			"offset", m.scrollOffset,
			"sb_len", m.scrollback.Len(),
			"height", m.height,
		)
		return m.viewScrollbackOnly()
	}
	logging.Debug("scrollback: rendering mixed (NOT alt-screen)",
		"offset", m.scrollOffset,
		"sb_len", m.scrollback.Len(),
		"height", m.height,
	)
	return m.viewScrollbackMixed()
}

// viewScrollbackOnly renders a window of scrollback-only content, used for
// alt-screen TUI sessions. The entire viewport is filled from the scrollback
// buffer; no live viewport rows are mixed in.
//
// scrollOffset=1 shows the most recent page of scrollback. Larger offsets
// scroll further into history. Blank lines pad the top when the scrollback
// has fewer lines than the viewport height.
func (m terminalModel) viewScrollbackOnly() string {
	sbLen := m.scrollback.Len()
	offset := m.scrollOffset
	if offset > sbLen {
		offset = sbLen
	}
	if sbLen == 0 {
		return ""
	}

	// The window covers [startIdx, startIdx+height). Older lines are at the
	// top. startIdx can go negative when the user scrolls past the oldest
	// content — those positions are rendered as blank lines.
	startIdx := sbLen - m.height - offset + 1

	var sb strings.Builder
	for i := 0; i < m.height; i++ {
		lineIdx := startIdx + i
		if lineIdx >= 0 && lineIdx < sbLen {
			glyphs := m.scrollback.Line(lineIdx)
			if glyphs != nil {
				m.renderGlyphRow(&sb, glyphs)
			}
		}
		if i < m.height-1 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// viewScrollbackMixed renders a mix of scrollback buffer lines and live
// viewport rows. Used for regular (non-alt-screen) terminal sessions where
// scrollback and viewport content are contiguous.
func (m terminalModel) viewScrollbackMixed() string {
	sbLen := m.scrollback.Len()
	offset := m.scrollOffset
	if offset > sbLen {
		offset = sbLen
	}

	fromScrollback := offset
	fromViewport := m.height - fromScrollback

	var sb strings.Builder
	rendered := 0

	// Render scrollback lines (older content at top).
	for i := 0; i < fromScrollback; i++ {
		lineIdx := sbLen - offset + i
		glyphs := m.scrollback.Line(lineIdx)
		if glyphs != nil {
			m.renderGlyphRow(&sb, glyphs)
		}
		rendered++
		if rendered < m.height {
			sb.WriteRune('\n')
		}
	}

	// Render live viewport rows from the top (no cursor in scrollback view).
	for row := 0; row < fromViewport; row++ {
		m.renderViewportRow(&sb, row, -1)
		rendered++
		if rendered < m.height {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

// renderViewportRow writes one row from the live vt10x grid into sb,
// including SGR escape sequences for color and attributes. cursorCol is
// the column where the cursor should be drawn (with reverse video);
// pass -1 when no cursor should appear on this row.
func (m terminalModel) renderViewportRow(sb *strings.Builder, row int, cursorCol int) {
	var curFG vt10x.Color = vt10x.DefaultFG
	var curBG vt10x.Color = vt10x.DefaultBG
	var curMode int16

	for col := 0; col < m.width; col++ {
		g := m.vt.Cell(col, row)
		ch := g.Char
		if ch == 0 {
			ch = ' '
		}

		// At the cursor position, toggle reverse video so the cell
		// appears as a visible block cursor (swapped FG/BG).
		if col == cursorCol {
			g.Mode ^= int16(vtAttrReverse)
		}

		if g.FG != curFG || g.BG != curBG || g.Mode != curMode {
			sb.WriteString(glyphSGR(g))
			curFG = g.FG
			curBG = g.BG
			curMode = g.Mode
		}
		sb.WriteRune(ch)

		// After drawing the cursor cell, force a fresh SGR on the next
		// cell so the reverse doesn't bleed.
		if col == cursorCol {
			curFG = vt10x.Color(0xFFFFFFFF)
			curBG = vt10x.Color(0xFFFFFFFF)
			curMode = -1
		}
	}

	if curFG != vt10x.DefaultFG || curBG != vt10x.DefaultBG || curMode != 0 {
		sb.WriteString("\x1b[0m")
	}
}

// renderGlyphRow writes a stored scrollback line into sb. If the line is
// shorter than the current terminal width, the remainder is padded with
// spaces. If longer, it is truncated.
func (m terminalModel) renderGlyphRow(sb *strings.Builder, glyphs []vt10x.Glyph) {
	var curFG vt10x.Color = vt10x.DefaultFG
	var curBG vt10x.Color = vt10x.DefaultBG
	var curMode int16

	renderWidth := len(glyphs)
	if renderWidth > m.width {
		renderWidth = m.width
	}

	for col := 0; col < renderWidth; col++ {
		g := glyphs[col]
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

	// Pad if scrollback line is shorter than current width.
	if renderWidth < m.width {
		if curFG != vt10x.DefaultFG || curBG != vt10x.DefaultBG || curMode != 0 {
			sb.WriteString("\x1b[0m")
			curFG = vt10x.DefaultFG
			curBG = vt10x.DefaultBG
			curMode = 0
		}
		for col := renderWidth; col < m.width; col++ {
			sb.WriteRune(' ')
		}
	}

	if curFG != vt10x.DefaultFG || curBG != vt10x.DefaultBG || curMode != 0 {
		sb.WriteString("\x1b[0m")
	}
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
	if g.Mode&vtAttrReverse != 0 {
		sb.WriteString(";7")
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

// ── Scrollback capture ──────────────────────────────────────────

// captureAndWrite snapshots the current viewport, writes data to vt10x,
// then detects how many lines scrolled off and pushes them to the
// scrollback buffer. Must be called with mu held.
func (m *terminalModel) captureAndWrite(data []byte) {
	if m.vt == nil {
		return
	}

	// Snapshot all rows as glyph slices + text before the write.
	oldGlyphs := m.snapshotGlyphs()
	oldTexts := make([]string, len(oldGlyphs))
	for i, row := range oldGlyphs {
		oldTexts[i] = glyphsToText(row)
	}

	// Write to vt10x — this may destroy the top rows via scrollUp().
	m.vt.Write(data)

	// Build new text snapshot for shift detection.
	newTexts := make([]string, m.height)
	for row := 0; row < m.height; row++ {
		newTexts[row] = m.viewportRowText(row)
	}

	// Detect how many lines scrolled off the top.
	shift := detectScrollShift(oldTexts, newTexts)

	if shift > 0 {
		// Find where the shifted region starts. In a normal terminal scroll,
		// this is row 0. In a TUI with a fixed header, the top rows may be
		// identical (header), and the actual shift starts further down.
		firstDiff := 0
		for firstDiff < len(oldTexts) && firstDiff < len(newTexts) && oldTexts[firstDiff] == newTexts[firstDiff] {
			firstDiff++
		}

		// Push the scrolled-off rows from old[firstDiff..firstDiff+shift-1].
		end := firstDiff + shift
		if end > len(oldGlyphs) {
			end = len(oldGlyphs)
		}
		for i := firstDiff; i < end; i++ {
			m.scrollback.Push(oldGlyphs[i])
		}
	}
}

// snapshotGlyphs copies every cell from the current vt10x viewport into
// freshly allocated glyph slices. Must be called with mu held.
func (m *terminalModel) snapshotGlyphs() []scrollbackLine {
	rows := make([]scrollbackLine, m.height)
	for row := 0; row < m.height; row++ {
		glyphs := make(scrollbackLine, m.width)
		for col := 0; col < m.width; col++ {
			glyphs[col] = m.vt.Cell(col, row)
		}
		rows[row] = glyphs
	}
	return rows
}

// viewportRowText returns the text content of a single viewport row,
// trimmed of trailing spaces. Must be called with mu held.
func (m *terminalModel) viewportRowText(row int) string {
	var sb strings.Builder
	sb.Grow(m.width)
	for col := 0; col < m.width; col++ {
		g := m.vt.Cell(col, row)
		if g.Char == 0 {
			sb.WriteRune(' ')
		} else {
			sb.WriteRune(g.Char)
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

// ── Scroll navigation ───────────────────────────────────────────

// ScrollBy adjusts the scroll offset by delta lines (positive = scroll up
// into history, negative = scroll down toward live). Clamps to valid range.
//
// Scrolling up sets scrollPinned so that new output auto-adjusts the offset
// to keep the view on the same content. Scrolling down clears scrollPinned
// so the user can freely reach offset 0 (live view) without fighting the
// auto-adjustment from runScrollCheck.
func (m *terminalModel) ScrollBy(delta int) {
	m.scrollOffset += delta
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if max := m.scrollback.Len(); m.scrollOffset > max {
		m.scrollOffset = max
	}

	if delta > 0 {
		m.scrollPinned = true
	} else if delta < 0 {
		m.scrollPinned = false
	}

	// Reaching offset 0 means we're back to live view — unpin.
	if m.scrollOffset == 0 {
		m.scrollPinned = false
	}
}

// ScrollToBottom resets the scroll offset to 0, returning to the live view.
func (m *terminalModel) ScrollToBottom() {
	m.scrollOffset = 0
	m.scrollPinned = false
}

// InScrollMode returns true when the user has scrolled away from the live
// viewport (scrollOffset > 0).
func (m *terminalModel) InScrollMode() bool {
	return m.scrollOffset > 0
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
	// ── Bracketed paste ────────────────────────────────────────────
	// When Bubble Tea detects bracketed paste, it strips the markers
	// and sets Paste=true. Re-wrap the content so the child process
	// sees a proper paste event (critical for image path drag-and-drop).
	if msg.Paste {
		content := []byte(string(msg.Runes))
		buf := make([]byte, 0, len(content)+12)
		buf = append(buf, "\x1b[200~"...)
		buf = append(buf, content...)
		buf = append(buf, "\x1b[201~"...)
		return buf
	}

	// ── Simple byte keys (may have Alt modifier) ───────────────────
	var raw []byte
	switch msg.Type {
	case tea.KeyRunes:
		raw = []byte(string(msg.Runes))
	case tea.KeyEnter:
		raw = []byte{'\r'}
	case tea.KeyBackspace:
		raw = []byte{0x7f}
	case tea.KeyTab:
		raw = []byte{'\t'}
	case tea.KeySpace:
		raw = []byte{' '}
	case tea.KeyEscape:
		return []byte{0x1b}

	// ── Navigation keys (Alt → xterm modifier param ;3) ────────────
	case tea.KeyUp:
		if msg.Alt {
			return []byte("\x1b[1;3A")
		}
		return []byte("\x1b[A")
	case tea.KeyDown:
		if msg.Alt {
			return []byte("\x1b[1;3B")
		}
		return []byte("\x1b[B")
	case tea.KeyRight:
		if msg.Alt {
			return []byte("\x1b[1;3C")
		}
		return []byte("\x1b[C")
	case tea.KeyLeft:
		if msg.Alt {
			return []byte("\x1b[1;3D")
		}
		return []byte("\x1b[D")
	case tea.KeyHome:
		if msg.Alt {
			return []byte("\x1b[1;3H")
		}
		return []byte("\x1b[H")
	case tea.KeyEnd:
		if msg.Alt {
			return []byte("\x1b[1;3F")
		}
		return []byte("\x1b[F")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")

	// ── Shift+key combinations ─────────────────────────────────────
	case tea.KeyShiftTab:
		return []byte("\x1b[Z")
	case tea.KeyShiftUp:
		return []byte("\x1b[1;2A")
	case tea.KeyShiftDown:
		return []byte("\x1b[1;2B")
	case tea.KeyShiftRight:
		return []byte("\x1b[1;2C")
	case tea.KeyShiftLeft:
		return []byte("\x1b[1;2D")
	case tea.KeyShiftHome:
		return []byte("\x1b[1;2H")
	case tea.KeyShiftEnd:
		return []byte("\x1b[1;2F")

	// ── Ctrl+Shift+key combinations ────────────────────────────────
	case tea.KeyCtrlShiftUp:
		return []byte("\x1b[1;6A")
	case tea.KeyCtrlShiftDown:
		return []byte("\x1b[1;6B")
	case tea.KeyCtrlShiftRight:
		return []byte("\x1b[1;6C")
	case tea.KeyCtrlShiftLeft:
		return []byte("\x1b[1;6D")
	case tea.KeyCtrlShiftHome:
		return []byte("\x1b[1;6H")
	case tea.KeyCtrlShiftEnd:
		return []byte("\x1b[1;6F")

	// ── Control keys ───────────────────────────────────────────────
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
	case tea.KeyCtrlG:
		return []byte{0x07}
	case tea.KeyCtrlH:
		return []byte{0x08}
	case tea.KeyCtrlK:
		return []byte{0x0b}
	case tea.KeyCtrlL:
		return []byte{0x0c}
	case tea.KeyCtrlN:
		return []byte{0x0e}
	case tea.KeyCtrlO:
		return []byte{0x0f}
	case tea.KeyCtrlP:
		return []byte{0x10}
	case tea.KeyCtrlQ:
		return []byte{0x11}
	case tea.KeyCtrlR:
		return []byte{0x12}
	case tea.KeyCtrlT:
		return []byte{0x14}
	case tea.KeyCtrlU:
		return []byte{0x15}
	case tea.KeyCtrlV:
		return []byte{0x16}
	case tea.KeyCtrlW:
		return []byte{0x17}
	case tea.KeyCtrlX:
		return []byte{0x18}
	case tea.KeyCtrlY:
		return []byte{0x19}
	case tea.KeyCtrlZ:
		return []byte{0x1a}

	default:
		s := msg.String()
		if s != "" &&
			!strings.HasPrefix(s, "ctrl+") &&
			!strings.HasPrefix(s, "alt+") &&
			!strings.HasPrefix(s, "shift+") {
			return []byte(s)
		}
		return nil
	}

	// For simple byte keys (Runes, Enter, Backspace, Tab, Space),
	// prepend ESC when the Alt modifier is active.
	if msg.Alt && raw != nil {
		return append([]byte{0x1b}, raw...)
	}
	return raw
}

// mouseToBytes encodes a Bubble Tea mouse event as an xterm mouse escape
// sequence for forwarding to a child PTY. x and y are 0-indexed coordinates
// relative to the child terminal area. sgrMode and motionMode indicate which
// encoding and event types the child requested. Returns nil if the event
// should not be forwarded.
func mouseToBytes(msg tea.MouseMsg, x, y int, sgrMode, motionMode bool) []byte {
	col := x + 1 // SGR uses 1-indexed coordinates
	row := y + 1

	btn := -1
	suffix := byte('M') // 'M' for press/motion, 'm' for release

	switch msg.Action {
	case tea.MouseActionPress:
		switch msg.Button {
		case tea.MouseButtonLeft:
			btn = 0
		case tea.MouseButtonMiddle:
			btn = 1
		case tea.MouseButtonRight:
			btn = 2
		case tea.MouseButtonWheelUp:
			btn = 64
		case tea.MouseButtonWheelDown:
			btn = 65
		default:
			return nil
		}
	case tea.MouseActionRelease:
		switch msg.Button {
		case tea.MouseButtonLeft:
			btn = 0
		case tea.MouseButtonMiddle:
			btn = 1
		case tea.MouseButtonRight:
			btn = 2
		default:
			btn = 0
		}
		suffix = 'm'
	case tea.MouseActionMotion:
		if !motionMode {
			return nil
		}
		switch msg.Button {
		case tea.MouseButtonLeft:
			btn = 32
		case tea.MouseButtonMiddle:
			btn = 33
		case tea.MouseButtonRight:
			btn = 34
		default:
			btn = 35
		}
	default:
		return nil
	}

	if btn < 0 {
		return nil
	}

	if sgrMode {
		return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", btn, col, row, suffix))
	}

	// X10/normal encoding: \x1b[M<button+32><col+32><row+32>
	if col+32 > 255 || row+32 > 255 {
		return nil
	}
	if msg.Action == tea.MouseActionRelease {
		btn = 3
	}
	return []byte{'\x1b', '[', 'M', byte(btn + 32), byte(col + 32), byte(row + 32)}
}

// unknownCSIBytes extracts the raw bytes from a bubbletea unknownCSISequenceMsg.
// bubbletea v1.3.10 emits kitty keyboard protocol sequences (e.g. Shift+Enter
// as \x1b[13;2u) that it doesn't recognise as the unexported type
// unknownCSISequenceMsg (underlying type []byte). Since the type is unexported
// we detect it via reflection on the type name and extract the byte slice.
// Returns nil for any other message type.
func unknownCSIBytes(msg tea.Msg) []byte {
	rv := reflect.ValueOf(msg)
	if !rv.IsValid() {
		return nil
	}
	if reflect.TypeOf(msg).Name() != "unknownCSISequenceMsg" {
		return nil
	}
	if rv.Kind() != reflect.Slice {
		return nil
	}
	return rv.Bytes()
}

// parseKittyCSI attempts to parse a kitty keyboard protocol CSI u sequence
// into a tea.KeyMsg. The CSI u format is: \x1b [ <codepoint> ; <modifier> u
// where modifier = (shift | alt<<1 | ctrl<<2 | super<<3) + 1.
//
// Returns (keyMsg, true) for sequences that map to app-level shortcuts
// (Ctrl+C, Ctrl+S, Ctrl+J, Ctrl+K). All other sequences return (_, false)
// and should be forwarded to the PTY.
func parseKittyCSI(raw []byte) (tea.KeyMsg, bool) {
	// Minimum: \x1b [ <digit> u = 4 bytes.
	if len(raw) < 4 || raw[0] != '\x1b' || raw[1] != '[' || raw[len(raw)-1] != 'u' {
		return tea.KeyMsg{}, false
	}

	params := string(raw[2 : len(raw)-1]) // between '[' and 'u'
	parts := strings.SplitN(params, ";", 2)

	codepoint, err := strconv.Atoi(parts[0])
	if err != nil {
		return tea.KeyMsg{}, false
	}

	modifier := 1 // default: no modifier
	if len(parts) > 1 {
		modifier, err = strconv.Atoi(parts[1])
		if err != nil {
			return tea.KeyMsg{}, false
		}
	}

	// Decode modifier bits (encoded as bits + 1).
	modBits := modifier - 1
	hasCtrl := modBits&4 != 0
	hasAlt := modBits&2 != 0

	if !hasCtrl {
		return tea.KeyMsg{}, false
	}

	// Map Ctrl+letter codepoints to tea.KeyType.
	switch rune(codepoint) {
	case 'c':
		return tea.KeyMsg{Type: tea.KeyCtrlC, Alt: hasAlt}, true
	case 's':
		return tea.KeyMsg{Type: tea.KeyCtrlS, Alt: hasAlt}, true
	case 'j':
		return tea.KeyMsg{Type: tea.KeyCtrlJ, Alt: hasAlt}, true
	case 'k':
		return tea.KeyMsg{Type: tea.KeyCtrlK, Alt: hasAlt}, true
	default:
		return tea.KeyMsg{}, false
	}
}
