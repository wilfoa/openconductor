package bootstrap

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*
var templateFS embed.FS

// TemplateData holds the data passed to bootstrap templates.
type TemplateData struct {
	ProjectName string
	RepoPath    string
	Language    string
}

// Bootstrap initializes agent-specific configuration files in the given
// repository path. The agentType selects which set of files to create:
// "claude-code", "codex", or "gemini". Existing files are never overwritten.
func Bootstrap(repoPath string, agentType string) error {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("accessing repo path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", absPath)
	}

	data := TemplateData{
		ProjectName: filepath.Base(absPath),
		RepoPath:    absPath,
		Language:    detectLanguage(absPath),
	}

	switch agentType {
	case "claude-code":
		if err := renderTemplate("templates/claude_md.tmpl", filepath.Join(absPath, "CLAUDE.md"), data); err != nil {
			return err
		}
		if err := renderTemplate("templates/mcp_json.tmpl", filepath.Join(absPath, ".mcp.json"), data); err != nil {
			return err
		}
	case "codex":
		codexDir := filepath.Join(absPath, ".codex")
		if err := os.MkdirAll(codexDir, 0o755); err != nil {
			return fmt.Errorf("creating .codex directory: %w", err)
		}
		if err := renderTemplate("templates/codex_instructions.tmpl", filepath.Join(codexDir, "instructions.md"), data); err != nil {
			return err
		}
	case "gemini":
		geminiDir := filepath.Join(absPath, ".gemini")
		if err := os.MkdirAll(geminiDir, 0o755); err != nil {
			return fmt.Errorf("creating .gemini directory: %w", err)
		}
		if err := renderTemplate("templates/gemini_md.tmpl", filepath.Join(absPath, "GEMINI.md"), data); err != nil {
			return err
		}
		if err := renderTemplate("templates/gemini_settings.tmpl", filepath.Join(geminiDir, "settings.json"), data); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown agent type: %q (supported: claude-code, codex, gemini)", agentType)
	}

	return nil
}

// renderTemplate parses the named embedded template, executes it with data,
// and writes the result to destPath. If destPath already exists, the file is
// skipped and a message is printed.
func renderTemplate(tmplName string, destPath string, data TemplateData) error {
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("  skip %s (already exists)\n", destPath)
		return nil
	}

	content, err := templateFS.ReadFile(tmplName)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", tmplName, err)
	}

	tmpl, err := template.New(filepath.Base(tmplName)).Parse(string(content))
	if err != nil {
		return fmt.Errorf("parsing template %s: %w", tmplName, err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", destPath, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("executing template %s: %w", tmplName, err)
	}

	fmt.Printf("  created %s\n", destPath)
	return nil
}

// detectLanguage inspects the repository for well-known project files and
// returns the primary language name. Falls back to "unknown" if nothing is
// recognized.
func detectLanguage(repoPath string) string {
	markers := []struct {
		file     string
		language string
	}{
		{"go.mod", "Go"},
		{"Cargo.toml", "Rust"},
		{"package.json", "JavaScript/TypeScript"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
		{"Gemfile", "Ruby"},
		{"pom.xml", "Java"},
		{"build.gradle", "Java"},
		{"mix.exs", "Elixir"},
		{"Package.swift", "Swift"},
	}

	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(repoPath, m.file)); err == nil {
			return m.language
		}
	}

	return "unknown"
}
