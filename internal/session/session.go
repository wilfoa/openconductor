// Package session manages agent process lifecycles, wrapping each agent in a
// PTY with vt10x terminal emulation.
package session

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
)

// State represents the lifecycle state of a session.
type State int

const (
	StateIdle State = iota
	StateRunning
	StateStopped
)

// Session wraps a single agent process running in a PTY with vt10x terminal
// emulation.
type Session struct {
	Project config.Project
	Agent   agent.AgentAdapter
	Ptmx    *os.File
	Cmd     *os.Process
	VT      vt10x.Terminal
	State   State

	Mu            sync.RWMutex
	Width, Height int
	closed        bool
}

// NewSession creates a Session for the given project. It looks up the
// appropriate AgentAdapter from the registry but does not start the process.
func NewSession(project config.Project) (*Session, error) {
	adapter, err := agent.Get(project.Agent)
	if err != nil {
		return nil, fmt.Errorf("creating session for %q: %w", project.Name, err)
	}

	return &Session{
		Project: project,
		Agent:   adapter,
		State:   StateIdle,
	}, nil
}

// Start launches the agent process in a PTY with the given dimensions.
func (s *Session) Start(width, height int) error {
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
	cmd := s.Agent.Command(s.Project.Repo, agent.LaunchOptions{})
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

// Write sends data to the PTY (i.e. user keyboard input).
func (s *Session) Write(data []byte) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if s.Ptmx != nil && !s.closed {
		s.Ptmx.Write(data)
	}
}

// GetScreenLines returns the current visible terminal content as a slice of
// strings, one per row.
func (s *Session) GetScreenLines() []string {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if s.VT == nil {
		return nil
	}

	lines := make([]string, s.Height)
	for row := 0; row < s.Height; row++ {
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

			// Write to the vt10x terminal emulator.
			s.Mu.Lock()
			if s.VT != nil {
				s.VT.Write(data)
			}
			s.Mu.Unlock()

			ch <- data
		}
	}()

	return ch
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
