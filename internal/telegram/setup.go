// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openconductorhq/openconductor/internal/config"
)

// RunSetup runs the interactive Telegram setup wizard. It reads from stdin
// and writes to stdout — designed to run inside a PTY (system tab) or as a
// standalone CLI command.
func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  Telegram Setup")
	fmt.Println("  ──────────────")
	fmt.Println()

	// ── Step 1: Bot token ───────────────────────────────────────
	fmt.Println("  Step 1: Create a Telegram bot")
	fmt.Println()
	fmt.Println("  Open Telegram and message @BotFather:")
	fmt.Println("    1. Send /newbot")
	fmt.Println("    2. Choose a name (e.g. \"My OpenConductor\")")
	fmt.Println("    3. Choose a username (e.g. \"my_openconductor_bot\")")
	fmt.Println("    4. Copy the bot token")
	fmt.Println()
	fmt.Print("  Paste your bot token: ")

	token, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("bot token cannot be empty")
	}

	// Validate token via getMe.
	fmt.Println()
	fmt.Print("  Validating token... ")
	botName, err := validateToken(token)
	if err != nil {
		fmt.Println("FAILED")
		fmt.Println()
		fmt.Printf("  Error: %v\n", err)
		fmt.Println("  Check that you copied the full token from @BotFather.")
		return err
	}
	fmt.Printf("OK (@%s)\n", botName)

	// ── Step 2: Group setup ─────────────────────────────────────
	fmt.Println()
	fmt.Println("  Step 2: Set up your Telegram group")
	fmt.Println()
	fmt.Println("  You need a supergroup with Forum Topics enabled:")
	fmt.Println("    1. Create a group (or use an existing one)")
	fmt.Println("    2. Open Group Settings > Topics > enable Forum Topics")
	fmt.Println("    3. Add @" + botName + " to the group as admin")
	fmt.Println("       (needs: Manage Topics, Post Messages)")
	fmt.Println()

	// ── Step 3: Auto-discover chat ID ───────────────────────────
	fmt.Println("  Step 3: Detecting your group")
	fmt.Println()
	fmt.Println("  Send any message in your group now.")
	fmt.Println("  Waiting...")
	fmt.Println()

	chatID, chatTitle, err := discoverChatID(token)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return err
	}
	fmt.Printf("  Found group: %s (ID: %d)\n", chatTitle, chatID)

	// ── Step 4: Save config ─────────────────────────────────────
	fmt.Println()
	fmt.Print("  Saving configuration... ")

	envVar := "OPENCONDUCTOR_TELEGRAM_TOKEN"
	if err := saveSetup(token, envVar, chatID); err != nil {
		fmt.Println("FAILED")
		fmt.Printf("  Error: %v\n", err)
		return err
	}
	fmt.Println("OK")

	fmt.Println()
	fmt.Println("  Setup complete!")
	fmt.Println()
	fmt.Printf("  Set this environment variable before launching OpenConductor:\n")
	fmt.Printf("    export %s=%s\n", envVar, token)
	fmt.Println()
	fmt.Println("  Telegram will activate on next launch (or config reload).")
	fmt.Println()

	return nil
}

// validateToken checks the bot token via the Telegram getMe API.
// Returns the bot username on success.
func validateToken(token string) (string, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", url.PathEscape(token))
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("invalid token: %s", result.Description)
	}

	return result.Result.Username, nil
}

// discoverChatID polls getUpdates until a message from a supergroup (with
// Forum Topics) appears. Times out after 2 minutes.
func discoverChatID(token string) (int64, string, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", url.PathEscape(token))

	// Clear any pending updates first.
	clearURL := apiURL + "?offset=-1&limit=1&timeout=1"
	resp, err := http.Get(clearURL)
	if err == nil {
		// Read the response to get the latest update_id.
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var clear struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int `json:"update_id"`
			} `json:"result"`
		}
		if json.Unmarshal(body, &clear) == nil && len(clear.Result) > 0 {
			// Acknowledge the last update so we only see new ones.
			ackURL := fmt.Sprintf("%s?offset=%d&limit=1&timeout=1", apiURL, clear.Result[0].UpdateID+1)
			ackResp, _ := http.Get(ackURL)
			if ackResp != nil {
				io.ReadAll(ackResp.Body)
				ackResp.Body.Close()
			}
		}
	}

	deadline := time.Now().Add(2 * time.Minute)
	offset := 0

	for time.Now().Before(deadline) {
		pollURL := fmt.Sprintf("%s?offset=%d&limit=10&timeout=10", apiURL, offset)
		resp, err := http.Get(pollURL)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var updates struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int `json:"update_id"`
				Message  *struct {
					Chat struct {
						ID    int64  `json:"id"`
						Title string `json:"title"`
						Type  string `json:"type"`
					} `json:"chat"`
					IsTopicMessage bool `json:"is_topic_message"`
				} `json:"message"`
			} `json:"result"`
		}
		if err := json.Unmarshal(body, &updates); err != nil {
			continue
		}
		if !updates.OK {
			continue
		}

		for _, u := range updates.Result {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message == nil {
				continue
			}
			chat := u.Message.Chat
			// Accept supergroup chats. Forum Topics are only available in supergroups.
			if chat.Type == "supergroup" {
				return chat.ID, chat.Title, nil
			}
		}
	}

	return 0, "", fmt.Errorf("timed out waiting for a message (2 minutes). Make sure you:\n" +
		"    - Added the bot to the group as admin\n" +
		"    - Sent a message in the group (not a DM to the bot)")
}

// saveSetup updates the OpenConductor config file with Telegram settings.
func saveSetup(token, envVar string, chatID int64) error {
	configPath := config.DefaultConfigPath()
	cfg := config.LoadOrDefault(configPath)

	cfg.Telegram = config.TelegramConfig{
		Enabled:     true,
		BotTokenEnv: envVar,
		ChatID:      chatID,
	}

	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Also write the token to a .env hint file for convenience.
	// The user still needs to export it in their shell.
	return nil
}
