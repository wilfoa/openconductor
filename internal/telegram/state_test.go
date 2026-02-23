// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTopicState_SetAndGet(t *testing.T) {
	s := newTopicState()
	s.Set("project-a", 123)
	s.Set("project-b", 456)

	if got := s.Get("project-a"); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
	if got := s.Get("project-b"); got != 456 {
		t.Fatalf("expected 456, got %d", got)
	}
	if got := s.Get("nonexistent"); got != 0 {
		t.Fatalf("expected 0 for missing project, got %d", got)
	}
}

func TestTopicState_Projects(t *testing.T) {
	s := newTopicState()
	s.Set("alpha", 1)
	s.Set("beta", 2)

	projects := s.Projects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	// Check both are present (order not guaranteed).
	found := map[string]bool{}
	for _, p := range projects {
		found[p] = true
	}
	if !found["alpha"] || !found["beta"] {
		t.Fatalf("expected alpha and beta, got %v", projects)
	}
}

func TestTopicState_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Save.
	s1 := newTopicState()
	s1.path = path
	s1.Set("proj-1", 111)
	s1.Set("proj-2", 222)
	if err := s1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected state file to exist")
	}

	// Load into a new instance.
	s2 := newTopicState()
	s2.path = path
	if err := s2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := s2.Get("proj-1"); got != 111 {
		t.Fatalf("expected 111 after load, got %d", got)
	}
	if got := s2.Get("proj-2"); got != 222 {
		t.Fatalf("expected 222 after load, got %d", got)
	}
}

func TestTopicState_LoadMissingFileOK(t *testing.T) {
	s := newTopicState()
	s.path = "/nonexistent/path/state.json"
	if err := s.Load(); err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(s.topics) != 0 {
		t.Fatal("expected empty topics after loading missing file")
	}
}

func TestTopicState_SaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "deep", "state.json")

	s := newTopicState()
	s.path = path
	s.Set("proj", 42)
	if err := s.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected nested state file to exist")
	}
}
