// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestScanDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "alpha"), 0o755)
	os.MkdirAll(filepath.Join(dir, "beta"), 0o755)
	os.MkdirAll(filepath.Join(dir, "another"), 0o755)

	results := scanDirectories(filepath.Join(dir, "a"))
	if len(results) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(results), results)
	}
	// Should match "alpha" and "another" (both start with "a")
	for _, r := range results {
		base := filepath.Base(filepath.Clean(r))
		if base != "alpha" && base != "another" {
			t.Fatalf("unexpected match: %s", r)
		}
	}
}

func TestScanDirectoriesFiltersFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "subfile"), []byte("data"), 0o644)

	results := scanDirectories(filepath.Join(dir, "sub"))
	if len(results) != 1 {
		t.Fatalf("expected 1 match (dir only), got %d: %v", len(results), results)
	}
	if filepath.Base(filepath.Clean(results[0])) != "subdir" {
		t.Fatalf("expected subdir, got %s", results[0])
	}
}

func TestScanDirectoriesExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	results := scanDirectories("~/")
	if results == nil {
		t.Fatal("expected results for ~/")
	}
	// All results should be under home dir.
	for _, r := range results {
		if !filepath.HasPrefix(r, home) {
			t.Fatalf("result %q not under home %q", r, home)
		}
	}
}

func TestScanDirectoriesNoMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "alpha"), 0o755)

	results := scanDirectories(filepath.Join(dir, "zzz"))
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d: %v", len(results), results)
	}
}

func TestCompletionTabAccepts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "myrepo"), 0o755)
	os.MkdirAll(filepath.Join(dir, "other"), 0o755)

	m := newTestForm()
	m.nameInput.SetValue("test")
	m.step = stepRepo
	m.repoInput.SetValue(filepath.Join(dir, "my"))

	// Manually trigger scan (simulates what Update does after value change).
	m.completion.suggestions = scanDirectories(m.repoInput.Value())
	m.completion.visible = len(m.completion.suggestions) > 0
	m.completion.selected = 0

	if !m.completion.visible {
		t.Fatal("expected completion to be visible")
	}
	if len(m.completion.suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(m.completion.suggestions))
	}

	// Tab accepts the suggestion.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	expected := filepath.Join(dir, "myrepo") + "/"
	if m.repoInput.Value() != expected {
		t.Fatalf("expected %q, got %q", expected, m.repoInput.Value())
	}
}

func TestCompletionDisplayName(t *testing.T) {
	name := completionDisplayName("/home/user/projects/myrepo/")
	if name != "myrepo/" {
		t.Fatalf("expected 'myrepo/', got %q", name)
	}
}
