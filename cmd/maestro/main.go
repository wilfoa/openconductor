package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/amir/maestro/internal/attention"
	"github.com/amir/maestro/internal/bootstrap"
	"github.com/amir/maestro/internal/config"
	"github.com/amir/maestro/internal/llm"
	"github.com/amir/maestro/internal/notification"
	"github.com/amir/maestro/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "bootstrap":
			runBootstrap(os.Args[2:])
			return
		case "--help", "-h":
			printUsage()
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	runTUI()
}

func printUsage() {
	fmt.Println("Usage: maestro [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (no command)    Launch the TUI")
	fmt.Println("  bootstrap       Bootstrap agent config files for a repository")
	fmt.Println()
	fmt.Println("Bootstrap usage:")
	fmt.Println("  maestro bootstrap <repo-path> [--agent <type>]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --agent    Agent type: claude-code (default), codex, gemini")
}

func runBootstrap(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: missing repository path")
		fmt.Fprintln(os.Stderr, "Usage: maestro bootstrap <repo-path> [--agent <type>]")
		os.Exit(1)
	}

	repoPath := args[0]
	agentType := "claude-code"

	// Parse remaining flags.
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --agent requires a value")
				os.Exit(1)
			}
			i++
			agentType = args[i]
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown flag %q\n", args[i])
			os.Exit(1)
		}
	}

	fmt.Printf("Bootstrapping %s for agent %q...\n", repoPath, agentType)
	if err := bootstrap.Bootstrap(repoPath, agentType); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}

// newLLMClient creates an LLM client from the config if provider and API key
// are configured. Returns nil if not configured or if the API key env var is
// empty.
func newLLMClient(cfg *config.Config) llm.Client {
	if cfg.LLM.Provider == "" || cfg.LLM.APIKey == "" {
		return nil
	}

	apiKey := os.Getenv(cfg.LLM.APIKey)
	if apiKey == "" {
		return nil
	}

	model := cfg.LLM.Model

	switch cfg.LLM.Provider {
	case "anthropic":
		return llm.NewAnthropicClient(apiKey, model)
	case "openai":
		return llm.NewOpenAIClient(apiKey, model)
	case "google":
		client, err := llm.NewGoogleClient(context.Background(), apiKey, model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create Google LLM client: %v\n", err)
			return nil
		}
		return client
	default:
		fmt.Fprintf(os.Stderr, "Warning: unknown LLM provider %q\n", cfg.LLM.Provider)
		return nil
	}
}

func runTUI() {
	configPath := config.DefaultConfigPath()
	cfg := config.LoadOrDefault(configPath)

	app := tui.NewApp(cfg, configPath)

	// Wire L2 LLM classifier if configured.
	if client := newLLMClient(cfg); client != nil {
		app.SetClassifier(attention.NewClassifier(client))
	}

	// Wire desktop notifications.
	app.SetNotifier(notification.New(cfg.Notifications.Enabled, cfg.Notifications.Cooldown))

	p := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Graceful shutdown: catch SIGINT/SIGTERM and tell bubbletea to quit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		p.Send(tea.Quit())
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
