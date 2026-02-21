// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"os"
	"path/filepath"
	"strings"
)

const maxCompletionVisible = 5

type completionModel struct {
	suggestions []string // full absolute paths of matching directories
	visible     bool     // whether dropdown is shown
	selected    int      // highlighted suggestion index
	lastScanned string   // last input value we scanned for (avoid redundant scans)
}

// scanDirectories reads the parent directory of partial and returns matching
// subdirectory paths (with trailing "/"). Returns nil on any error.
func scanDirectories(partial string) []string {
	if partial == "" {
		return nil
	}

	// Expand ~/
	expanded := partial
	if strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		expanded = filepath.Join(home, expanded[2:])
	}

	// If partial ends with "/", list children of that directory.
	// Otherwise, split into parent + prefix.
	var dir, prefix string
	if strings.HasSuffix(expanded, "/") {
		dir = expanded
		prefix = ""
	} else {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var matches []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden directories unless prefix starts with "."
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			matches = append(matches, filepath.Join(dir, name)+"/")
		}
		if len(matches) >= maxCompletionVisible {
			break
		}
	}

	return matches
}

// displayName returns just the directory basename for rendering in the dropdown.
func completionDisplayName(path string) string {
	clean := strings.TrimSuffix(path, "/")
	return filepath.Base(clean) + "/"
}
