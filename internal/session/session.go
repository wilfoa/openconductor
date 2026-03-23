// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package session manages agent process lifecycles, wrapping each agent in a
// PTY with vt10x terminal emulation.
package session

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
)

// State represents the lifecycle state of a session.
type State int

const (
	StateIdle State = iota
	StateRunning
	StateStopped
)

// Session wraps a single agent process running in a PTY with vt10x terminal
// emulation. Multiple sessions can exist for the same project, each running
// an independent agent process.
type Session struct {
	ID       string // unique session identifier (e.g. "proj", "proj (2)")
	Instance int    // instance number (1, 2, 3...)
	Project  config.Project
	Agent    agent.AgentAdapter
	Ptmx     *os.File
	Cmd      *os.Process
	VT       vt10x.Terminal
	State    State

	// outputFilter, when non-nil, preprocesses raw PTY output before it
	// reaches the vt10x terminal emulator. Used to strip escape sequences
	// that vt10x cannot parse correctly (e.g. kitty keyboard protocol).
	outputFilter func([]byte) []byte

	// scrollCapture collects lines that scroll off the top of the vt10x
	// grid during VT.Write(). The App's checkScrollback() drains these.
	scrollCapture []ScrollCapturedLine

	Mu            sync.RWMutex
	Width, Height int
	closed        bool
}

// ScrollCapturedLine holds a single terminal row captured as it scrolled
// off the top of the vt10x grid.
type ScrollCapturedLine struct {
	Text   string
	Glyphs []vt10x.Glyph
}

// NewSession creates a Session for the given project. It looks up the
// appropriate AgentAdapter from the registry but does not start the process.
func NewSession(project config.Project) (*Session, error) {
	adapter, err := agent.Get(project.Agent)
	if err != nil {
		return nil, fmt.Errorf("creating session for %q: %w", project.Name, err)
	}

	s := &Session{
		Project: project,
		Agent:   adapter,
		State:   StateIdle,
	}

	// If the agent implements OutputFilter, create a per-session filter
	// function to preprocess PTY output before vt10x.
	if f := agent.GetOutputFilter(project.Agent); f != nil {
		s.outputFilter = f.NewOutputFilter()
	}

	return s, nil
}

// NewSystemSession creates a Session that runs an arbitrary command instead
// of an agent. Used for system tabs (e.g. Telegram setup wizard).
func NewSystemSession(name string) *Session {
	return &Session{
		Project: config.Project{Name: name},
		State:   StateIdle,
	}
}

// Start launches the agent process in a PTY with the given dimensions.
// opts is forwarded to the agent adapter's Command method.
func (s *Session) Start(width, height int, opts agent.LaunchOptions) error {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	s.Mu.Lock()
	defer s.Mu.Unlock()

	s.Width = width
	s.Height = height

	// Create the vt10x terminal emulator.
	s.VT = vt10x.New(vt10x.WithSize(width, height))

	// Build the command from the agent adapter.
	cmd := s.Agent.Command(s.Project.Repo, opts)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	// Start the command in a PTY.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("starting PTY for %q: %w", s.Project.Name, err)
	}

	s.Ptmx = ptmx
	s.Cmd = cmd.Process
	s.State = StateRunning
	s.closed = false

	return nil
}

// StartCmd launches an arbitrary command in a PTY. Used for system sessions
// that don't go through the agent adapter (e.g. setup wizards).
func (s *Session) StartCmd(cmd *exec.Cmd, width, height int) error {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	s.Mu.Lock()
	defer s.Mu.Unlock()

	s.Width = width
	s.Height = height
	s.VT = vt10x.New(vt10x.WithSize(width, height))

	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("starting PTY for system session: %w", err)
	}

	s.Ptmx = ptmx
	s.Cmd = cmd.Process
	s.State = StateRunning
	s.closed = false

	return nil
}

// Write sends data to the PTY (i.e. user keyboard input).
func (s *Session) Write(data []byte) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if s.Ptmx != nil && !s.closed {
		n, err := s.Ptmx.Write(data)
		if err != nil {
			logging.Error("session: PTY write failed",
				"session", s.ID,
				"bytes", len(data),
				"written", n,
				"error", err,
			)
		}
	} else {
		logging.Warn("session: Write skipped (ptmx nil or closed)",
			"session", s.ID,
			"ptmxNil", s.Ptmx == nil,
			"closed", s.closed,
			"bytes", len(data),
		)
	}
}

// GetScreenLines returns the current visible terminal content as a slice of
// strings, one per row. For alt-screen apps (like OpenCode), all rows are
// returned since the app manages the entire screen. For non-alt-screen
// sessions (like Claude Code), rows below the cursor position are truncated
// because they contain stale content from previous renders that the app
// didn't clear.
func (s *Session) GetScreenLines() []string {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if s.VT == nil {
		return nil
	}

	altScreen := s.VT.Mode()&vt10x.ModeAltScreen != 0
	cursorY := s.VT.Cursor().Y

	// For non-alt-screen sessions, only return rows up to the cursor.
	// Content below the cursor is stale — the app wrote output above
	// and never cleared what was previously rendered below.
	h := s.Height
	if !altScreen && cursorY+1 < h {
		h = cursorY + 1
	}

	lines := make([]string, h)
	for row := 0; row < h; row++ {
		var sb strings.Builder
		for col := 0; col < s.Width; col++ {
			g := s.VT.Cell(col, row)
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

// Resize changes the PTY and vt10x terminal dimensions.
func (s *Session) Resize(w, h int) {
	if w < 1 || h < 1 {
		return
	}

	s.Mu.Lock()
	defer s.Mu.Unlock()

	s.Width = w
	s.Height = h

	if s.Ptmx != nil && !s.closed {
		pty.Setsize(s.Ptmx, &pty.Winsize{
			Rows: uint16(h),
			Cols: uint16(w),
		})
	}

	if s.VT != nil {
		s.VT.Resize(w, h)
	}
}

// ReadLoop starts a goroutine that reads from the PTY and sends data on the
// returned channel. The channel is closed when the PTY read returns an error
// (typically because the process exited).
func (s *Session) ReadLoop() <-chan []byte {
	ch := make(chan []byte, 64)

	go func() {
		defer close(ch)
		buf := make([]byte, 4096)
		for {
			s.Mu.RLock()
			ptmx := s.Ptmx
			closed := s.closed
			s.Mu.RUnlock()

			if ptmx == nil || closed {
				return
			}

			n, err := ptmx.Read(buf)
			if err != nil {
				return
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			// If the agent provides an output filter, apply it before
			// vt10x sees the data. This strips escape sequences that
			// vt10x would misparse (e.g. kitty keyboard protocol).
			if s.outputFilter != nil {
				data = s.outputFilter(data)
			}

			// Write to the vt10x terminal emulator, capturing any
			// lines that scroll off the top of the grid.
			s.Mu.Lock()
			if s.VT != nil {
				s.captureScrollOff(data)
			}
			s.Mu.Unlock()

			ch <- data
		}
	}()

	return ch
}

// captureScrollOff snapshots the top rows before VT.Write(), writes the data,
// then detects how many lines scrolled off by comparing the old top rows with
// the new screen. Any rows that scrolled off are saved to s.scrollCapture.
//
// Must be called with s.Mu held (write lock).
func (s *Session) captureScrollOff(data []byte) {
	vt := s.VT
	cursor := vt.Cursor()
	h := s.Height

	// For alt-screen apps (like OpenCode), skip the capture — their TUI
	// manages the full screen and scrollback is handled by pushAltScreenDiff.
	// Only non-alt-screen apps (like Claude Code) need this.
	if vt.Mode()&vt10x.ModeAltScreen != 0 {
		vt.Write(data)
		return
	}
	_ = cursor

	// Snapshot the top rows before write. We save up to height rows because
	// a large write can scroll the entire screen.
	preTexts := make([]string, h)
	preGlyphs := make([][]vt10x.Glyph, h)
	w := s.Width
	for row := 0; row < h; row++ {
		glyphs := make([]vt10x.Glyph, w)
		var sb strings.Builder
		for col := 0; col < w; col++ {
			g := vt.Cell(col, row)
			glyphs[col] = g
			if g.Char == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(g.Char)
			}
		}
		preTexts[row] = strings.TrimRight(sb.String(), " ")
		preGlyphs[row] = glyphs
	}

	// Perform the actual write.
	vt.Write(data)

	// Detect how many rows scrolled off by finding where old content
	// ended up in the new screen. Check multiple old rows against the
	// new screen to find the shift.
	scrolled := 0
	for shift := 1; shift < h; shift++ {
		// Check if old[shift] == new[0]: the old row at position 'shift'
		// is now at position 0, meaning 'shift' rows scrolled off.
		if preTexts[shift] == "" {
			continue
		}
		var sb strings.Builder
		for col := 0; col < w; col++ {
			g := vt.Cell(col, 0)
			if g.Char == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(g.Char)
			}
		}
		newRow0 := strings.TrimRight(sb.String(), " ")
		if preTexts[shift] == newRow0 {
			scrolled = shift
			break
		}
	}

	// Fallback: if we couldn't find a matching row (entire screen changed),
	// check how many old rows are NOT in the new screen at all. This handles
	// bursts larger than the screen height where no old content survives.
	if scrolled == 0 {
		newTexts := make(map[string]struct{}, h)
		for row := 0; row < h; row++ {
			var sb strings.Builder
			for col := 0; col < w; col++ {
				g := vt.Cell(col, row)
				if g.Char == 0 {
					sb.WriteRune(' ')
				} else {
					sb.WriteRune(g.Char)
				}
			}
			t := strings.TrimRight(sb.String(), " ")
			if t != "" {
				newTexts[t] = struct{}{}
			}
		}
		// Count old non-blank rows that are completely gone from the new screen.
		for i := 0; i < h; i++ {
			if preTexts[i] == "" {
				continue
			}
			if _, exists := newTexts[preTexts[i]]; !exists {
				scrolled++
			} else {
				break // Found a surviving row — stop counting.
			}
		}
	}

	// Push scrolled-off rows to the capture buffer. Blank lines are
	// preserved — they serve as meaningful separators in CLI agent output
	// (e.g. spacing between Claude Code conversation sections).
	if scrolled > 0 {
		for i := 0; i < scrolled; i++ {
			s.scrollCapture = append(s.scrollCapture, ScrollCapturedLine{
				Text:   preTexts[i],
				Glyphs: preGlyphs[i],
			})
		}
	}
}

// DrainScrollCapture returns and clears all lines captured during VT.Write()
// scroll-off events. Called by the App's checkScrollback() to push these
// lines into the scrollback buffer.
//
// Must be called with s.Mu held (at least read lock).
func (s *Session) DrainScrollCapture() []ScrollCapturedLine {
	if len(s.scrollCapture) == 0 {
		return nil
	}
	lines := s.scrollCapture
	s.scrollCapture = nil
	return lines
}

// Size returns the current terminal dimensions.
func (s *Session) Size() (width, height int) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.Width, s.Height
}

// GetVT returns the vt10x terminal and current dimensions. The caller should
// treat the returned Terminal as shared state protected by the session's
// internal lock -- use GetScreenLines for safe reads.
func (s *Session) GetVT() (vt vt10x.Terminal, width, height int) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.VT, s.Width, s.Height
}

// IsRunning reports whether the session has an active PTY process.
func (s *Session) IsRunning() bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return !s.closed && s.Ptmx != nil
}

// Close terminates the agent process and releases PTY resources.
func (s *Session) Close() {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	if s.Ptmx != nil {
		s.Ptmx.Close()
	}
	if s.Cmd != nil {
		s.Cmd.Signal(os.Interrupt)
	}

	s.State = StateStopped
}
