// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package telegram provides a bidirectional Telegram bot bridge that gives
// users remote visibility into agent work and the ability to respond via
// Telegram Forum Topics.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
	"github.com/openconductorhq/openconductor/internal/session"
)

// Bot is the Telegram bot that bridges agent sessions to Forum Topics.
type Bot struct {
	api   *tgbotapi.BotAPI
	token string
	cfg   config.TelegramConfig
	mgr   *session.Manager
	state *topicState
	br    *bridge
	hdlr  *handler

	projects []config.Project

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBot creates a new Telegram bot. Call Start() to begin polling.
func NewBot(cfg config.TelegramConfig, mgr *session.Manager, projects []config.Project) (*Bot, error) {
	token := os.Getenv(cfg.BotTokenEnv)
	if token == "" {
		return nil, fmt.Errorf("telegram: env var %q is empty", cfg.BotTokenEnv)
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}

	logging.Info("telegram: authorized", "bot", api.Self.UserName)

	state := newTopicState()
	ctx, cancel := context.WithCancel(context.Background())

	return &Bot{
		api:      api,
		token:    token,
		cfg:      cfg,
		mgr:      mgr,
		state:    state,
		br:       newBridge(),
		hdlr:     newHandler(mgr, state, projects),
		projects: projects,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// EventChannel returns the channel for the TUI to send events on.
func (b *Bot) EventChannel() chan<- Event {
	return b.br.ch
}

// Start begins the polling and bridge goroutines. It creates Forum Topics
// for any projects that don't have one yet.
func (b *Bot) Start() error {
	// Load persisted topic state.
	if err := b.state.Load(); err != nil {
		logging.Debug("telegram: failed to load topic state (starting fresh)", "err", err)
	}

	// Ensure each project has a Forum Topic.
	for _, p := range b.projects {
		if b.state.Get(p.Name) != 0 {
			continue
		}
		topicID, err := b.createTopic(p.Name)
		if err != nil {
			logging.Error("telegram: failed to create topic", "project", p.Name, "err", err)
			continue
		}
		b.state.Set(p.Name, topicID)
		logging.Info("telegram: created topic", "project", p.Name, "topic_id", topicID)
	}

	// Persist any new topic IDs.
	if err := b.state.Save(); err != nil {
		logging.Error("telegram: failed to save topic state", "err", err)
	}

	// Start polling for incoming messages.
	b.wg.Add(1)
	go b.pollLoop()

	// Start bridge loop for outbound messages.
	b.wg.Add(1)
	go b.bridgeLoop()

	return nil
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() {
	b.cancel()
	b.api.StopReceivingUpdates()
	b.wg.Wait()
}

// rawUpdate is a partial Telegram Update parsed from raw JSON so we can
// extract fields the go-telegram-bot-api/v5 library does not expose
// (notably message_thread_id).
type rawUpdate struct {
	UpdateID      int             `json:"update_id"`
	Message       *rawMessage     `json:"message"`
	CallbackQuery json.RawMessage `json:"callback_query"`
}

// rawMessage captures the fields we need from an incoming message,
// including message_thread_id which the library's Message struct lacks.
type rawMessage struct {
	MessageID       int    `json:"message_id"`
	MessageThreadID int    `json:"message_thread_id"`
	Text            string `json:"text"`
	Chat            struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

// pollLoop receives Telegram updates via raw HTTP calls (not the library's
// GetUpdatesChan) so we can parse message_thread_id which the v5 library
// does not support.
func (b *Bot) pollLoop() {
	defer b.wg.Done()

	offset := 0
	client := &http.Client{Timeout: 40 * time.Second}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", url.PathEscape(b.token))

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		payload := map[string]interface{}{
			"offset":  offset,
			"timeout": 30,
		}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(b.ctx, http.MethodPost, apiURL, bytes.NewReader(body))
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			logging.Error("telegram: poll request error", "err", err)
			time.Sleep(2 * time.Second)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			logging.Error("telegram: poll error", "err", err)
			time.Sleep(2 * time.Second)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logging.Error("telegram: poll read error", "err", err)
			continue
		}

		var result struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil || !result.OK {
			logging.Error("telegram: poll parse error", "err", err, "ok", result.OK)
			time.Sleep(2 * time.Second)
			continue
		}

		// Parse updates from raw JSON.
		var rawUpdates []rawUpdate
		if err := json.Unmarshal(result.Result, &rawUpdates); err != nil {
			logging.Error("telegram: poll unmarshal updates error", "err", err)
			continue
		}

		// Also parse using the library types for callback queries (which
		// the library handles correctly).
		var libUpdates []tgbotapi.Update
		json.Unmarshal(result.Result, &libUpdates)

		for i, ru := range rawUpdates {
			if ru.UpdateID >= offset {
				offset = ru.UpdateID + 1
			}
			b.handleRawUpdate(ru, libUpdates[i])
		}
	}
}

// handleRawUpdate dispatches a single update using both raw and library-parsed data.
func (b *Bot) handleRawUpdate(raw rawUpdate, lib tgbotapi.Update) {
	// Callback queries: use the library's parsed type (it handles these correctly).
	if lib.CallbackQuery != nil {
		b.hdlr.HandleCallback(b.api, lib.CallbackQuery)
		return
	}

	// Text messages: use raw parsing to get message_thread_id.
	if raw.Message != nil && raw.Message.Chat.ID == b.cfg.ChatID {
		logging.Debug("telegram: inbound message",
			"text_len", len(raw.Message.Text),
			"thread_id", raw.Message.MessageThreadID,
			"chat_id", raw.Message.Chat.ID,
		)
		b.hdlr.HandleInbound(raw.Message.Text, raw.Message.MessageThreadID)
	}
}

// bridgeLoop reads events from the TUI and sends them to Telegram.
func (b *Bot) bridgeLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		case event, ok := <-b.br.Events():
			if !ok {
				return
			}
			if !b.br.shouldSend(event) {
				continue
			}
			b.sendEvent(event)
		}
	}
}

// sendEvent formats and sends an event to the appropriate Telegram topic.
func (b *Bot) sendEvent(e Event) {
	topicID := b.state.Get(e.Project)
	if topicID == 0 {
		logging.Debug("telegram: no topic for project", "project", e.Project)
		return
	}

	var messages []string
	var keyboard *tgbotapi.InlineKeyboardMarkup

	switch e.Kind {
	case EventResponse:
		messages = FormatResponse(e.Project, e.Screen)
	case EventPermission:
		messages = FormatPermission(e.Project, e.Detail, e.Screen)
		kb := PermissionKeyboard(e.Project)
		keyboard = &kb
	case EventQuestion:
		messages = FormatQuestion(e.Project, e.Screen)
		options := ParseQuestionOptions(e.Screen)
		if len(options) > 0 {
			kb := QuestionKeyboard(e.Project, options)
			keyboard = &kb
		}
	case EventAttention:
		messages = FormatAttention(e.Project, e.Detail, e.Screen)
		kb := AttentionKeyboard(e.Project)
		keyboard = &kb
	case EventError:
		messages = FormatError(e.Project, e.Detail, e.Screen)
		kb := ErrorKeyboard(e.Project)
		keyboard = &kb
	case EventDone:
		messages = FormatDone(e.Project, e.Screen)
	}

	if len(messages) == 0 {
		return
	}

	for i, text := range messages {
		var kb *tgbotapi.InlineKeyboardMarkup
		if i == len(messages)-1 {
			kb = keyboard
		}
		if err := b.sendToTopic(topicID, text, kb); err != nil {
			logging.Error("telegram: failed to send message", "project", e.Project, "err", err)
			return
		}
	}

	logging.Debug("telegram: sent message", "project", e.Project, "kind", e.Kind, "parts", len(messages))
}

// sendToTopic sends a message to a specific Forum Topic using the raw API
// (the go-telegram-bot-api v5 library does not support message_thread_id).
func (b *Bot) sendToTopic(topicID int, text string, keyboard *tgbotapi.InlineKeyboardMarkup) error {
	payload := map[string]interface{}{
		"chat_id":                  b.cfg.ChatID,
		"message_thread_id":        topicID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}

	return b.rawAPICall("sendMessage", payload)
}

// editTopicMessage edits a message in a topic.
func (b *Bot) editTopicMessage(messageID int, text string, keyboard *tgbotapi.InlineKeyboardMarkup) error {
	payload := map[string]interface{}{
		"chat_id":    b.cfg.ChatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}

	return b.rawAPICall("editMessageText", payload)
}

// createTopic creates a new Forum Topic for a project via the raw Telegram API.
func (b *Bot) createTopic(name string) (int, error) {
	payload := map[string]interface{}{
		"chat_id": b.cfg.ChatID,
		"name":    name,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/createForumTopic", url.PathEscape(b.token))
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageThreadID int    `json:"message_thread_id"`
			Name            string `json:"name"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("parsing response: %w", err)
	}
	if !result.OK {
		return 0, fmt.Errorf("API error: %s", result.Description)
	}

	return result.Result.MessageThreadID, nil
}

// rawAPICall makes a raw POST request to the Telegram Bot API.
func (b *Bot) rawAPICall(method string, payload map[string]interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", url.PathEscape(b.token), method)
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API %s returned %d: %s", method, resp.StatusCode, string(respBody))
	}

	return nil
}

// FormatCallbackData creates callback data strings with size safety.
func FormatCallbackData(kind string, project string, action string) string {
	data := kind + ":" + project + ":" + action
	// Telegram callback data is limited to 64 bytes.
	if len(data) > 64 {
		// Truncate project name to fit.
		maxProj := 64 - len(kind) - len(action) - 2
		if maxProj < 1 {
			maxProj = 1
		}
		data = kind + ":" + project[:maxProj] + ":" + action
	}
	return data
}

// intToStr is a helper for payload building.
func intToStr(i int) string {
	return strconv.Itoa(i)
}
