// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openconductorhq/openconductor/internal/attention"
	"github.com/openconductorhq/openconductor/internal/bootstrap"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/llm"
	"github.com/openconductorhq/openconductor/internal/logging"
	"github.com/openconductorhq/openconductor/internal/notification"
	"github.com/openconductorhq/openconductor/internal/permission"
	"github.com/openconductorhq/openconductor/internal/telegram"
	"github.com/openconductorhq/openconductor/internal/tui"
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
		case "telegram":
			runTelegram(args[1:])
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
	fmt.Println("Usage: openconductor [flags] [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (no command)       Launch the TUI")
	fmt.Println("  bootstrap          Bootstrap agent config files for a repository")
	fmt.Println("  telegram setup     Set up Telegram bot integration")
	fmt.Println()
	fmt.Println("Global flags:")
	fmt.Println("  --debug    Enable verbose debug logging to ~/.openconductor/openconductor.log")
	fmt.Println()
	fmt.Println("Bootstrap usage:")
	fmt.Println("  openconductor bootstrap <repo-path> [--agent <type>]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --agent    Agent type: claude-code (default), codex, gemini")
}

func runTelegram(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: openconductor telegram setup")
		os.Exit(1)
	}

	switch args[0] {
	case "setup":
		if err := telegram.RunSetup(); err != nil {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown telegram subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: openconductor telegram setup")
		os.Exit(1)
	}
}

func runBootstrap(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: missing repository path")
		fmt.Fprintln(os.Stderr, "Usage: openconductor bootstrap <repo-path> [--agent <type>]")
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
	// verbose debug messages. Log file: ~/.openconductor/openconductor.log.
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

	// Load saved state (open tabs from previous session).
	statePath := config.DefaultStatePath()
	restoredState := config.LoadState(statePath)
	if len(restoredState.OpenTabs) > 0 {
		logging.Info("restoring tabs from previous session",
			"tabs", len(restoredState.OpenTabs),
			"active", restoredState.ActiveTab,
		)
	}

	app := tui.NewApp(cfg, configPath, &restoredState)
	app.SetStatePath(statePath)

	// Wire L2 LLM classifier and auto-approver if an LLM is configured.
	// Both the attention classifier and permission classifier share the same
	// underlying LLM client to avoid redundant provider setup.
	if client := newLLMClient(cfg); client != nil {
		app.SetClassifier(attention.NewClassifier(client))
		logging.Info("LLM classifier enabled", "provider", cfg.LLM.Provider)

		// Build the permission detector (L1 + L2) and wire the auto-approver.
		permClassifier := permission.NewClassifier(client)
		permDetector := permission.NewDetector(permClassifier)
		app.SetAutoApprover(attention.NewAutoApprover(permDetector))
		logging.Info("auto-approver enabled")
	} else {
		// No LLM configured: still enable the auto-approver with L1-only
		// detection so pattern-matched permissions can be auto-approved.
		permDetector := permission.NewDetector(nil)
		app.SetAutoApprover(attention.NewAutoApprover(permDetector))
	}

	// Wire desktop notifications.
	app.SetNotifier(notification.New(cfg.Notifications.Enabled, cfg.Notifications.Cooldown))

	// Wire Telegram bot bridge if enabled.
	var tgBot *telegram.Bot
	if cfg.Telegram.Enabled {
		bot, err := telegram.NewBot(cfg.Telegram, app.SessionManager(), cfg.Projects)
		if err != nil {
			logging.Error("telegram: failed to create bot", "err", err)
			fmt.Fprintf(os.Stderr, "Warning: Telegram bot failed to start: %v\n", err)
		} else {
			app.SetTelegramChannel(bot.EventChannel())
			tgBot = bot
			logging.Info("telegram bot configured")
		}
	}

	p := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Enable kitty keyboard protocol for enhanced key reporting (e.g., Shift+Enter).
	// Mode flags: 1=disambiguate, 2=report event types, 4=report alternate keys, 8=report all keys
	// We use mode 1 (disambiguate) which enables CSI u sequences for modified keys.
	// See: https://sw.kovidgoyal.net/kitty/keyboard-protocol/
	os.Stdout.WriteString("\x1b[>1u")

	// Graceful shutdown: catch SIGINT/SIGTERM and tell bubbletea to quit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logging.Info("received signal, shutting down", "signal", sig.String())
		p.Send(tea.Quit())
	}()

	// Start Telegram bot (polling + bridge loops).
	if tgBot != nil {
		if err := tgBot.Start(); err != nil {
			logging.Error("telegram: failed to start bot", "err", err)
			fmt.Fprintf(os.Stderr, "Warning: Telegram bot failed to start: %v\n", err)
		} else {
			logging.Info("telegram bot started")
		}
	}

	logging.Info("starting TUI")
	finalModel, err := p.Run()
	// Save state regardless of how the program exited. The Ctrl+C path
	// already saves in the Update handler, but signal-based exits and
	// errors may bypass it. SaveState is idempotent.
	if final, ok := finalModel.(tui.App); ok {
		final.SaveStatePublic()
	}
	if err != nil {
		// Disable kitty keyboard protocol before exiting.
		os.Stdout.WriteString("\x1b[<u")
		if tgBot != nil {
			tgBot.Stop()
		}
		logging.Error("TUI exited with error", "err", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	// Disable kitty keyboard protocol on clean exit.
	os.Stdout.WriteString("\x1b[<u")
	if tgBot != nil {
		tgBot.Stop()
		logging.Info("telegram bot stopped")
	}
	logging.Info("openconductor exited cleanly")
}
