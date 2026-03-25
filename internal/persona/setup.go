// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/openconductorhq/openconductor/internal/config"
)

var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// RunSetup runs the interactive persona manager. It reads from stdin
// and writes to stdout — designed to run inside a PTY (system tab) or as a
// standalone CLI command.
func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)
	configPath := config.DefaultConfigPath()

	for {
		cfg := config.LoadOrDefault(configPath)

		fmt.Println()
		fmt.Println("  Persona Manager")
		fmt.Println("  ───────────────")
		fmt.Println()
		fmt.Println("  Built-in personas (read-only):")
		fmt.Println("    vibe     Move fast, skip tests, auto-approve")
		fmt.Println("    poc      Working demos, basic tests")
		fmt.Println("    scale    TDD, production quality, thorough")
		fmt.Println()
		fmt.Println("  Custom personas:")

		if len(cfg.Personas) == 0 {
			fmt.Println("    (none)")
		} else {
			for _, p := range cfg.Personas {
				fmt.Printf("    %-16s %s\n", p.Name, p.Label)
			}
		}

		fmt.Println()
		fmt.Println("  [c] Create  [e] Edit  [d] Delete  [q] Quit")
		fmt.Println()
		fmt.Print("  > ")

		choice := readLine(reader)
		switch strings.ToLower(choice) {
		case "c":
			if err := createPersona(reader, cfg, configPath); err != nil {
				fmt.Printf("\n  Error: %v\n", err)
			}
		case "e":
			if err := editPersona(reader, cfg, configPath); err != nil {
				fmt.Printf("\n  Error: %v\n", err)
			}
		case "d":
			if err := deletePersona(reader, cfg, configPath); err != nil {
				fmt.Printf("\n  Error: %v\n", err)
			}
		case "q":
			fmt.Println()
			return nil
		default:
			fmt.Println("\n  Unknown option. Use c, e, d, or q.")
		}
	}
}

// createPersona prompts for a new custom persona and saves it to config.
func createPersona(reader *bufio.Reader, cfg *config.Config, configPath string) error {
	fmt.Println()
	fmt.Println("  Create Persona")
	fmt.Println("  ──────────────")
	fmt.Println()

	// Name (slug).
	fmt.Print("  Name (slug): ")
	name := readLine(reader)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if !isValidSlug(name) {
		return fmt.Errorf("invalid name %q: must match [a-z][a-z0-9-]*", name)
	}
	if !isNameAvailable(name, cfg.Personas) {
		return fmt.Errorf("name %q is already in use (built-in or custom)", name)
	}

	// Label.
	fmt.Print("  Label: ")
	label := readLine(reader)
	if label == "" {
		return fmt.Errorf("label cannot be empty")
	}

	// Instructions (multi-line).
	fmt.Println("  Instructions (enter text, then a line containing only END):")
	instructions := readMultiLine(reader)
	if instructions == "" {
		return fmt.Errorf("instructions cannot be empty")
	}

	// Auto-approve level.
	fmt.Print("  Auto-approve level (off/safe/full) [off]: ")
	approveStr := readLine(reader)
	if approveStr == "" {
		approveStr = "off"
	}
	var approve config.ApprovalLevel
	switch strings.ToLower(approveStr) {
	case "off":
		approve = config.ApprovalOff
	case "safe":
		approve = config.ApprovalSafe
	case "full":
		approve = config.ApprovalFull
	default:
		return fmt.Errorf("unknown auto-approve level %q: use off, safe, or full", approveStr)
	}

	cfg.Personas = append(cfg.Personas, config.CustomPersona{
		Name:         name,
		Label:        label,
		Instructions: instructions,
		AutoApprove:  approve,
	})

	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Created persona %q.\n", name)
	return nil
}

// editPersona shows a numbered list of custom personas, lets the user pick
// one, then shows pre-filled values and prompts for changes.
func editPersona(reader *bufio.Reader, cfg *config.Config, configPath string) error {
	if len(cfg.Personas) == 0 {
		fmt.Println("\n  No custom personas to edit.")
		return nil
	}

	fmt.Println()
	fmt.Println("  Edit Persona")
	fmt.Println("  ────────────")
	fmt.Println()

	for i, p := range cfg.Personas {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.Label)
	}

	fmt.Println()
	fmt.Print("  Pick a number: ")
	numStr := readLine(reader)
	idx, err := strconv.Atoi(numStr)
	if err != nil || idx < 1 || idx > len(cfg.Personas) {
		return fmt.Errorf("invalid selection %q", numStr)
	}
	idx-- // zero-based

	p := &cfg.Personas[idx]
	fmt.Println()

	// Label.
	fmt.Printf("  Label [%s]: ", p.Label)
	newLabel := readLine(reader)
	if newLabel != "" {
		p.Label = newLabel
	}

	// Instructions.
	fmt.Println("  Current instructions:")
	for _, line := range strings.Split(p.Instructions, "\n") {
		fmt.Printf("    %s\n", line)
	}
	fmt.Println("  New instructions (enter text then END, or just END to keep):")
	newInstructions := readMultiLine(reader)
	if newInstructions != "" {
		p.Instructions = newInstructions
	}

	// Auto-approve level.
	current := string(p.AutoApprove)
	if current == "" {
		current = "off"
	}
	fmt.Printf("  Auto-approve level (off/safe/full) [%s]: ", current)
	approveStr := readLine(reader)
	if approveStr != "" {
		switch strings.ToLower(approveStr) {
		case "off":
			p.AutoApprove = config.ApprovalOff
		case "safe":
			p.AutoApprove = config.ApprovalSafe
		case "full":
			p.AutoApprove = config.ApprovalFull
		default:
			return fmt.Errorf("unknown auto-approve level %q: use off, safe, or full", approveStr)
		}
	}

	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Updated persona %q.\n", p.Name)
	return nil
}

// deletePersona shows a numbered list of custom personas, lets the user pick
// one, confirms, then removes it from config.
func deletePersona(reader *bufio.Reader, cfg *config.Config, configPath string) error {
	if len(cfg.Personas) == 0 {
		fmt.Println("\n  No custom personas to delete.")
		return nil
	}

	fmt.Println()
	fmt.Println("  Delete Persona")
	fmt.Println("  ──────────────")
	fmt.Println()

	for i, p := range cfg.Personas {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.Label)
	}

	fmt.Println()
	fmt.Print("  Pick a number: ")
	numStr := readLine(reader)
	idx, err := strconv.Atoi(numStr)
	if err != nil || idx < 1 || idx > len(cfg.Personas) {
		return fmt.Errorf("invalid selection %q", numStr)
	}
	idx-- // zero-based

	name := cfg.Personas[idx].Name
	fmt.Printf("  Delete %q? (y/n): ", name)
	confirm := readLine(reader)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("  Cancelled.")
		return nil
	}

	cfg.Personas = append(cfg.Personas[:idx], cfg.Personas[idx+1:]...)

	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Deleted persona %q.\n", name)
	return nil
}

// readLine reads a single line from the reader and returns it trimmed.
func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// readMultiLine reads lines until a line containing only "END" is entered.
// Returns the joined text (excluding the END sentinel).
func readMultiLine(reader *bufio.Reader) string {
	var lines []string
	for {
		line, _ := reader.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if trimmed == "END" {
			break
		}
		lines = append(lines, strings.TrimRight(line, "\r\n"))
	}
	return strings.Join(lines, "\n")
}

// isValidSlug checks that name matches the pattern [a-z][a-z0-9-]*.
func isValidSlug(name string) bool {
	return slugRegex.MatchString(name)
}

// isNameAvailable checks that name is not a built-in persona and not already
// present in the existing custom personas list.
func isNameAvailable(name string, existing []config.CustomPersona) bool {
	if config.BuiltinPersonaNames[config.PersonaType(name)] {
		return false
	}
	for _, p := range existing {
		if p.Name == name {
			return false
		}
	}
	return true
}
