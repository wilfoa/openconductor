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
	"github.com/amir/maestro/internal/logging"
	"github.com/amir/maestro/internal/notification"
	"github.com/amir/maestro/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Parse global flags before subcommands.
	debug := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--debug" {
			debug = true
			args = append(args[:i], args[i+1:]...)
			i--
		}
	}

	if len(args) > 0 {
		switch args[0] {
		case "bootstrap":
			runBootstrap(args[1:])
			return
		case "--help", "-h":
			printUsage()
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
			printUsage()
			os.Exit(1)
		}
	}

	runTUI(debug)
}

func printUsage() {
	fmt.Println("Usage: maestro [flags] [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (no command)    Launch the TUI")
	fmt.Println("  bootstrap       Bootstrap agent config files for a repository")
	fmt.Println()
	fmt.Println("Global flags:")
	fmt.Println("  --debug    Enable verbose debug logging to ~/.maestro/maestro.log")
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

func runTUI(debug bool) {
	// Initialize file logger. Always logs at info level; --debug adds
	// verbose debug messages. Log file: ~/.maestro/maestro.log.
	if err := logging.Init(logging.Options{Debug: debug}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to init logger: %v\n", err)
	}
	defer logging.Close()
	defer logging.RecoverPanic()

	configPath := config.DefaultConfigPath()
	cfg := config.LoadOrDefault(configPath)
	logging.Info("config loaded",
		"path", configPath,
		"projects", len(cfg.Projects),
	)

	app := tui.NewApp(cfg, configPath)

	// Wire L2 LLM classifier if configured.
	if client := newLLMClient(cfg); client != nil {
		app.SetClassifier(attention.NewClassifier(client))
		logging.Info("LLM classifier enabled", "provider", cfg.LLM.Provider)
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
		sig := <-sigCh
		logging.Info("received signal, shutting down", "signal", sig.String())
		p.Send(tea.Quit())
	}()

	logging.Info("starting TUI")
	if _, err := p.Run(); err != nil {
		logging.Error("TUI exited with error", "err", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	logging.Info("maestro exited cleanly")
}
