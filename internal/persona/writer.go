// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
)

const (
	markerStart    = "<!-- openconductor:persona:start -->"
	markerEnd      = "<!-- openconductor:persona:end -->"
	managedComment = "<!-- This section is managed by OpenConductor. Manual edits will be overwritten. -->"
)

// WritePersonaSection writes (or removes) the persona instruction block inside
// the agent's instruction file in the given repository. The managed section is
// delimited by HTML comment markers so it can be updated idempotently without
// disturbing user-written content in the same file.
func WritePersonaSection(
	repoPath string,
	agentType config.AgentType,
	persona config.PersonaType,
	customPersonas []config.CustomPersona,
) error {
	filename := TargetFile(agentType)
	if filename == "" {
		return fmt.Errorf("persona: unknown agent type %q", agentType)
	}
	filePath := filepath.Join(repoPath, filename)

	var personaText string
	if persona != config.PersonaNone {
		result := Resolve(persona, customPersonas)
		if !result.Found {
			return fmt.Errorf("persona: unknown persona %q", persona)
		}
		personaText = result.Instructions
	}

	logging.Info("writing persona section",
		"repo", repoPath,
		"agent", agentType,
		"persona", persona,
		"file", filePath,
	)

	return writeFile(filePath, personaText)
}

// writeFile merges the persona text into the file at filePath. It handles
// four cases: file does not exist, file has no markers, file has markers
// (replace), and persona is empty (removal).
func writeFile(filePath string, personaText string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return handleNoFile(filePath, personaText)
		}
		return fmt.Errorf("persona: reading %s: %w", filePath, err)
	}

	lines := strings.Split(string(data), "\n")
	startIdx, endIdx := findMarkers(lines)

	if personaText == "" {
		return handleRemoval(filePath, lines, startIdx, endIdx)
	}
	if startIdx >= 0 {
		return handleReplace(filePath, lines, startIdx, endIdx, personaText)
	}
	return handleAppend(filePath, string(data), personaText)
}

// findMarkers scans lines for the start and end persona markers. Returns -1
// for either index if the corresponding marker is not found.
func findMarkers(lines []string) (startIdx, endIdx int) {
	startIdx = -1
	endIdx = -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == markerStart {
			startIdx = i
		}
		if trimmed == markerEnd {
			endIdx = i
		}
	}
	return startIdx, endIdx
}

// buildMarkedSection returns the persona text wrapped in markers with a
// trailing newline.
func buildMarkedSection(personaText string) string {
	return markerStart + "\n" +
		managedComment + "\n" +
		personaText + "\n" +
		markerEnd + "\n"
}

// handleNoFile creates a new file containing just the persona section, or
// does nothing if personaText is empty (no file to create for PersonaNone).
func handleNoFile(filePath string, personaText string) error {
	if personaText == "" {
		return nil
	}
	content := buildMarkedSection(personaText)
	return atomicWrite(filePath, []byte(content), 0o644)
}

// handleRemoval removes the managed persona section from the file. If the
// file becomes empty after removal, it is deleted entirely.
func handleRemoval(filePath string, lines []string, startIdx, endIdx int) error {
	if startIdx < 0 {
		// No markers present -- nothing to remove.
		return nil
	}
	if endIdx < 0 {
		// Missing end marker -- treat start to EOF as the section.
		endIdx = len(lines) - 1
	}

	// Remove lines[startIdx..endIdx] inclusive.
	remaining := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	remaining = append(remaining, lines[:startIdx]...)
	remaining = append(remaining, lines[endIdx+1:]...)

	result := strings.TrimSpace(strings.Join(remaining, "\n"))
	if result == "" {
		return os.Remove(filePath)
	}
	return atomicWrite(filePath, []byte(result+"\n"), 0o644)
}

// handleReplace replaces the existing managed section (between markers) with
// updated persona text. If the end marker is missing, treats the section as
// extending to EOF (corruption recovery).
func handleReplace(filePath string, lines []string, startIdx, endIdx int, personaText string) error {
	if endIdx < 0 {
		endIdx = len(lines) - 1
	}

	var b strings.Builder
	for _, line := range lines[:startIdx] {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(buildMarkedSection(personaText))
	if endIdx+1 < len(lines) {
		after := strings.Join(lines[endIdx+1:], "\n")
		b.WriteString(after)
		// Ensure trailing newline if original had content after markers.
		if !strings.HasSuffix(after, "\n") {
			b.WriteByte('\n')
		}
	}

	return atomicWrite(filePath, []byte(b.String()), 0o644)
}

// handleAppend appends a new persona section to the end of an existing file.
func handleAppend(filePath string, existing string, personaText string) error {
	var b strings.Builder
	b.WriteString(existing)
	if !strings.HasSuffix(existing, "\n") {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(buildMarkedSection(personaText))

	return atomicWrite(filePath, []byte(b.String()), 0o644)
}

// atomicWrite writes content to path atomically by writing to a temp file in
// the same directory and renaming it into place. This prevents partial writes
// from corrupting the target file.
func atomicWrite(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".persona-*")
	if err != nil {
		return fmt.Errorf("persona: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Deferred cleanup removes the temp file on any error path. On success,
	// tmpPath is cleared so the deferred remove is a no-op.
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("persona: writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("persona: closing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("persona: setting permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("persona: renaming temp file: %w", err)
	}

	// Success -- prevent deferred cleanup.
	tmpPath = ""
	return nil
}
