package session

import (
	"fmt"
	"sync"

	"github.com/amir/maestro/internal/config"
)

// Manager holds all active sessions and tracks which one is currently
// displayed in the terminal panel.
type Manager struct {
	sessions map[string]*Session
	active   string
	mu       sync.RWMutex
}

// NewManager creates an empty session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// StartSession creates and starts a session for the given project. If a
// session already exists for that project name it is returned as-is.
func (m *Manager) StartSession(project config.Project, width, height int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[project.Name]; ok {
		return s, nil
	}

	s, err := NewSession(project)
	if err != nil {
		return nil, fmt.Errorf("manager: %w", err)
	}

	if err := s.Start(width, height); err != nil {
		return nil, fmt.Errorf("manager: %w", err)
	}

	m.sessions[project.Name] = s

	if m.active == "" {
		m.active = project.Name
	}

	return s, nil
}

// GetSession returns the session for the given project name, or nil if none
// exists.
func (m *Manager) GetSession(name string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[name]
}

// StopSession stops and removes the session for the given project name.
func (m *Manager) StopSession(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[name]; ok {
		s.Close()
		delete(m.sessions, name)
	}

	if m.active == name {
		m.active = ""
	}
}

// ActiveSession returns the currently active session, or nil if none is
// active.
func (m *Manager) ActiveSession() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[m.active]
}

// ActiveName returns the name of the currently active session.
func (m *Manager) ActiveName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// SetActive switches the active session to the given project name. Returns
// an error if no session exists for that name.
func (m *Manager) SetActive(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[name]; !ok {
		return fmt.Errorf("no session for project %q", name)
	}

	m.active = name
	return nil
}

// Close stops all sessions and clears the manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, s := range m.sessions {
		s.Close()
		delete(m.sessions, name)
	}

	m.active = ""
}
