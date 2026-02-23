// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/openconductorhq/openconductor/internal/config"
)

// topicState manages the mapping between project names and Telegram Forum
// Topic IDs. It persists to ~/.openconductor/telegram_state.json.
type topicState struct {
	mu     sync.RWMutex
	topics map[string]int // project name → topic message_thread_id
	path   string
}

func newTopicState() *topicState {
	return &topicState{
		topics: make(map[string]int),
		path:   filepath.Join(config.DefaultConfigDir(), "telegram_state.json"),
	}
}

// Load reads the topic state from disk. Missing file is not an error.
func (s *topicState) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.topics)
}

// Save writes the topic state to disk.
func (s *topicState) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(s.topics)
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o644)
}

// Get returns the topic ID for a project, or 0 if not set.
func (s *topicState) Get(project string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.topics[project]
}

// Set stores a topic ID for a project.
func (s *topicState) Set(project string, topicID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topics[project] = topicID
}

// Projects returns all project names with stored topic IDs.
func (s *topicState) Projects() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.topics))
	for name := range s.topics {
		names = append(names, name)
	}
	return names
}
