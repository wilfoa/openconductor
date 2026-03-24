// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openconductorhq/openconductor/internal/config"
)

// helper: write a file in dir and return its path.
func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	return path
}

// helper: read a file, failing the test if it errors.
func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading test file %s: %v", path, err)
	}
	return string(data)
}

func TestWritePersonaSection_CreateNewFile(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaVibe, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "CLAUDE.md")
	content := readTestFile(t, path)

	if !strings.Contains(content, markerStart) {
		t.Error("missing start marker")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("missing end marker")
	}
	if !strings.Contains(content, managedComment) {
		t.Error("missing managed comment")
	}
	if !strings.Contains(content, "Vibe") {
		t.Error("missing persona instruction text")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("permissions = %o, want 644", perm)
	}
}

func TestWritePersonaSection_AppendToExisting(t *testing.T) {
	dir := t.TempDir()
	userContent := "# My Project\n\nSome existing content.\n"
	writeTestFile(t, dir, "CLAUDE.md", userContent)

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaPOC, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))

	// User content must be preserved.
	if !strings.Contains(content, "# My Project") {
		t.Error("user content lost")
	}
	if !strings.Contains(content, "Some existing content.") {
		t.Error("user content lost")
	}
	// Markers and persona text must be present.
	if !strings.Contains(content, markerStart) {
		t.Error("missing start marker")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("missing end marker")
	}
	if !strings.Contains(content, "POC") {
		t.Error("missing persona instruction text")
	}
}

func TestWritePersonaSection_ReplaceExisting(t *testing.T) {
	dir := t.TempDir()
	existing := "# Project\n\n" +
		markerStart + "\n" +
		managedComment + "\n" +
		"old persona text\n" +
		markerEnd + "\n" +
		"\n# Footer\n"
	writeTestFile(t, dir, "CLAUDE.md", existing)

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaScale, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))

	if strings.Contains(content, "old persona text") {
		t.Error("old persona text not replaced")
	}
	if !strings.Contains(content, "Scale") {
		t.Error("new persona text missing")
	}
	if !strings.Contains(content, "# Project") {
		t.Error("content before markers lost")
	}
	if !strings.Contains(content, "# Footer") {
		t.Error("content after markers lost")
	}
}

func TestWritePersonaSection_RemoveOnNone(t *testing.T) {
	dir := t.TempDir()
	existing := "# Project\n\n" +
		markerStart + "\n" +
		managedComment + "\n" +
		"persona text\n" +
		markerEnd + "\n" +
		"\n# Footer\n"
	writeTestFile(t, dir, "CLAUDE.md", existing)

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))

	if strings.Contains(content, markerStart) {
		t.Error("start marker should be removed")
	}
	if strings.Contains(content, markerEnd) {
		t.Error("end marker should be removed")
	}
	if strings.Contains(content, "persona text") {
		t.Error("persona text should be removed")
	}
	if !strings.Contains(content, "# Project") {
		t.Error("user content before markers lost")
	}
	if !strings.Contains(content, "# Footer") {
		t.Error("user content after markers lost")
	}
}

func TestWritePersonaSection_RemoveDeletesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	existing := markerStart + "\n" +
		managedComment + "\n" +
		"persona text\n" +
		markerEnd + "\n"
	writeTestFile(t, dir, "CLAUDE.md", existing)

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been deleted when only persona content remained")
	}
}

func TestWritePersonaSection_NoneNoFile(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not be created for PersonaNone")
	}
}

func TestWritePersonaSection_NoneNoMarkers(t *testing.T) {
	dir := t.TempDir()
	original := "# My Project\n\nUser content here.\n"
	writeTestFile(t, dir, "CLAUDE.md", original)

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))
	if content != original {
		t.Errorf("file should be unchanged.\ngot:\n%s\nwant:\n%s", content, original)
	}
}

func TestWritePersonaSection_CorruptedStartNoEnd(t *testing.T) {
	dir := t.TempDir()
	existing := "# Header\n\n" +
		markerStart + "\n" +
		"orphaned persona text\n" +
		"more orphaned text\n"
	writeTestFile(t, dir, "CLAUDE.md", existing)

	// Removal with corrupted markers (no end marker) should treat start-to-EOF.
	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaNone, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))

	if strings.Contains(content, markerStart) {
		t.Error("start marker should be removed")
	}
	if strings.Contains(content, "orphaned") {
		t.Error("orphaned text should be removed")
	}
	if !strings.Contains(content, "# Header") {
		t.Error("header content lost")
	}
}

func TestWritePersonaSection_PreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	existing := "# Project Title\n\nImportant notes here.\n\n" +
		markerStart + "\n" +
		managedComment + "\n" +
		"old instructions\n" +
		markerEnd + "\n\n" +
		"## User Section\n\nMore user content.\n"
	writeTestFile(t, dir, "CLAUDE.md", existing)

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaVibe, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))

	if !strings.Contains(content, "# Project Title") {
		t.Error("content before markers lost")
	}
	if !strings.Contains(content, "Important notes here.") {
		t.Error("content before markers lost")
	}
	if !strings.Contains(content, "## User Section") {
		t.Error("content after markers lost")
	}
	if !strings.Contains(content, "More user content.") {
		t.Error("content after markers lost")
	}
	if strings.Contains(content, "old instructions") {
		t.Error("old instructions should be replaced")
	}
	if !strings.Contains(content, "Vibe") {
		t.Error("new persona text missing")
	}
}

func TestWritePersonaSection_CustomPersona(t *testing.T) {
	dir := t.TempDir()
	customPersonas := []config.CustomPersona{
		{
			Name:         "security",
			Label:        "Security",
			Instructions: "Always review for vulnerabilities.\nRun SAST on every change.",
			AutoApprove:  config.ApprovalSafe,
		},
	}

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaType("security"), customPersonas)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readTestFile(t, filepath.Join(dir, "CLAUDE.md"))

	if !strings.Contains(content, "Always review for vulnerabilities.") {
		t.Error("custom persona instructions missing")
	}
	if !strings.Contains(content, "Run SAST on every change.") {
		t.Error("custom persona instructions missing")
	}
	if !strings.Contains(content, markerStart) {
		t.Error("missing start marker")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("missing end marker")
	}
}

func TestWritePersonaSection_UnknownPersona(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaSection(dir, config.AgentClaudeCode, config.PersonaType("nonexistent"), nil)
	if err == nil {
		t.Fatal("expected error for unknown persona, got nil")
	}
	if !strings.Contains(err.Error(), "unknown persona") {
		t.Errorf("error should mention unknown persona, got: %v", err)
	}
}

func TestWritePersonaSection_UnknownAgent(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaSection(dir, config.AgentType("unknown-agent"), config.PersonaVibe, nil)
	if err == nil {
		t.Fatal("expected error for unknown agent type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown agent type") {
		t.Errorf("error should mention unknown agent type, got: %v", err)
	}
}

func TestFindMarkers(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		wantStart int
		wantEnd   int
	}{
		{
			name:      "both found",
			lines:     []string{"header", markerStart, "content", markerEnd, "footer"},
			wantStart: 1,
			wantEnd:   3,
		},
		{
			name:      "only start",
			lines:     []string{"header", markerStart, "content"},
			wantStart: 1,
			wantEnd:   -1,
		},
		{
			name:      "neither found",
			lines:     []string{"header", "content", "footer"},
			wantStart: -1,
			wantEnd:   -1,
		},
		{
			name:      "empty file",
			lines:     []string{},
			wantStart: -1,
			wantEnd:   -1,
		},
		{
			name:      "markers with surrounding whitespace",
			lines:     []string{"  " + markerStart + "  ", "content", "\t" + markerEnd + "\t"},
			wantStart: 0,
			wantEnd:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := findMarkers(tt.lines)
			if start != tt.wantStart {
				t.Errorf("startIdx = %d, want %d", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("endIdx = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

func TestBuildMarkedSection(t *testing.T) {
	text := "Some persona instructions."
	section := buildMarkedSection(text)

	if !strings.HasPrefix(section, markerStart+"\n") {
		t.Error("section should start with start marker")
	}
	if !strings.Contains(section, managedComment) {
		t.Error("section should contain managed comment")
	}
	if !strings.Contains(section, text) {
		t.Error("section should contain persona text")
	}
	if !strings.HasSuffix(section, markerEnd+"\n") {
		t.Error("section should end with end marker and trailing newline")
	}

	// Verify ordering: start marker, managed comment, text, end marker.
	startPos := strings.Index(section, markerStart)
	commentPos := strings.Index(section, managedComment)
	textPos := strings.Index(section, text)
	endPos := strings.Index(section, markerEnd)

	if !(startPos < commentPos && commentPos < textPos && textPos < endPos) {
		t.Error("section elements are in wrong order")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.md")
	content := []byte("hello, world\n")

	err := atomicWrite(path, content, 0o644)
	if err != nil {
		t.Fatalf("atomicWrite failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("permissions = %o, want 644", perm)
	}

	// Verify no temp files are left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".persona-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")

	// Write initial content.
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("writing initial file: %v", err)
	}

	// Overwrite with atomicWrite.
	newContent := []byte("updated content\n")
	if err := atomicWrite(path, newContent, 0o644); err != nil {
		t.Fatalf("atomicWrite failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != string(newContent) {
		t.Errorf("content = %q, want %q", got, newContent)
	}
}

func TestWritePersonaSection_OpenCodeAgent(t *testing.T) {
	dir := t.TempDir()

	err := WritePersonaSection(dir, config.AgentOpenCode, config.PersonaVibe, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// OpenCode should write to AGENTS.md.
	path := filepath.Join(dir, "AGENTS.md")
	content := readTestFile(t, path)

	if !strings.Contains(content, markerStart) {
		t.Error("missing start marker in AGENTS.md")
	}
	if !strings.Contains(content, "Vibe") {
		t.Error("missing persona text in AGENTS.md")
	}

	// CLAUDE.md should NOT exist.
	claudePath := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); !os.IsNotExist(err) {
		t.Error("CLAUDE.md should not be created for OpenCode agent")
	}
}
