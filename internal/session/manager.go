// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package session

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/openconductorhq/openconductor/internal/config"
)

// Manager holds all active sessions and tracks which one is currently
// displayed in the terminal panel. Multiple sessions can exist for the
// same project, each identified by a unique session ID.
type Manager struct {
	sessions        map[string]*Session // session ID → session
	projectSessions map[string][]string // project name → ordered list of session IDs
	nextInstance    map[string]int      // project name → next instance counter
	active          string              // active session ID
	mu              sync.RWMutex
}

// NewManager creates an empty session manager.
func NewManager() *Manager {
	return &Manager{
		sessions:        make(map[string]*Session),
		projectSessions: make(map[string][]string),
		nextInstance:    make(map[string]int),
	}
}

// sessionID generates a unique session ID for a project instance.
// Instance 1 returns just the project name; instance N>1 returns "name (N)".
func sessionID(projectName string, instance int) string {
	if instance == 1 {
		return projectName
	}
	return fmt.Sprintf("%s (%d)", projectName, instance)
}

// StartSession creates and starts a new session for the given project.
// Each call creates a fresh agent process — multiple sessions for the same
// project run in parallel.
func (m *Manager) StartSession(project config.Project, width, height int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := NewSession(project)
	if err != nil {
		return nil, fmt.Errorf("manager: %w", err)
	}

	// Assign instance number and session ID.
	m.nextInstance[project.Name]++
	instance := m.nextInstance[project.Name]
	id := sessionID(project.Name, instance)
	s.ID = id
	s.Instance = instance

	if err := s.Start(width, height); err != nil {
		m.nextInstance[project.Name]--
		return nil, fmt.Errorf("manager: %w", err)
	}

	m.sessions[id] = s
	m.projectSessions[project.Name] = append(m.projectSessions[project.Name], id)

	if m.active == "" {
		m.active = id
	}

	return s, nil
}

// StartSystemSession creates and starts a session that runs an arbitrary
// command instead of an agent. Used for system tabs like setup wizards.
// If a session with this name already exists, it is returned as-is.
func (m *Manager) StartSystemSession(name string, cmd *exec.Cmd, width, height int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[name]; ok {
		return s, nil
	}

	s := NewSystemSession(name)
	s.ID = name
	s.Instance = 1
	if err := s.StartCmd(cmd, width, height); err != nil {
		return nil, fmt.Errorf("manager: %w", err)
	}

	m.sessions[name] = s

	if m.active == "" {
		m.active = name
	}

	return s, nil
}

// GetSession returns the session with the given ID, or nil if none exists.
func (m *Manager) GetSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// GetSessionsByProject returns all sessions for the given project name,
// in creation order.
func (m *Manager) GetSessionsByProject(projectName string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := m.projectSessions[projectName]
	sessions := make([]*Session, 0, len(ids))
	for _, id := range ids {
		if s, ok := m.sessions[id]; ok {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// AllSessions returns all active sessions. Order is not guaranteed.
func (m *Manager) AllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// StopSession stops and removes the session with the given ID.
func (m *Manager) StopSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return
	}

	projectName := s.Project.Name
	s.Close()
	delete(m.sessions, id)

	// Remove from projectSessions.
	ids := m.projectSessions[projectName]
	for i, sid := range ids {
		if sid == id {
			m.projectSessions[projectName] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(m.projectSessions[projectName]) == 0 {
		delete(m.projectSessions, projectName)
	}

	if m.active == id {
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

// ActiveName returns the ID of the currently active session.
func (m *Manager) ActiveName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// SetActive switches the active session to the given session ID. Returns
// an error if no session exists with that ID.
func (m *Manager) SetActive(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return fmt.Errorf("no session with ID %q", id)
	}

	m.active = id
	return nil
}

// HasSessionsForProject reports whether any sessions exist for the given
// project name.
func (m *Manager) HasSessionsForProject(projectName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.projectSessions[projectName]) > 0
}

// InjectSession inserts a pre-built session into the manager, useful for
// testing without starting a real process. The session ID is used as the key.
func (m *Manager) InjectSession(id string, s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s.ID == "" {
		s.ID = id
	}
	if s.Instance == 0 {
		s.Instance = 1
	}

	m.sessions[id] = s
	projectName := s.Project.Name
	if projectName != "" {
		m.projectSessions[projectName] = append(m.projectSessions[projectName], id)
		if m.nextInstance[projectName] < s.Instance {
			m.nextInstance[projectName] = s.Instance
		}
	}
	if m.active == "" {
		m.active = id
	}
}

// Close stops all sessions and clears the manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range m.sessions {
		s.Close()
		delete(m.sessions, id)
	}

	m.projectSessions = make(map[string][]string)
	m.active = ""
}
