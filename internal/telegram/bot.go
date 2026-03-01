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

// Resilience constants.
const (
	// backoffMin is the initial retry delay after an API error.
	backoffMin = 2 * time.Second
	// backoffMax caps the exponential backoff.
	backoffMax = 60 * time.Second
	// heartbeatInterval is how often we ping Telegram to verify the bot is alive.
	heartbeatInterval = 5 * time.Minute
	// healthWarnThreshold: log a warning after this many consecutive poll errors.
	healthWarnThreshold = 5
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

	// Health tracking (protected by mu).
	mu              sync.Mutex
	startedAt       time.Time // when Start() was called
	lastPollOK      time.Time // last successful getUpdates
	lastSendOK      time.Time // last successful sendMessage
	consecutiveErrs int       // consecutive poll failures
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

	b.mu.Lock()
	b.startedAt = time.Now()
	b.mu.Unlock()

	// Start polling for incoming messages (supervised with auto-restart).
	b.wg.Add(1)
	go b.supervise("pollLoop", b.pollLoop)

	// Start bridge loop for outbound messages (supervised with auto-restart).
	b.wg.Add(1)
	go b.supervise("bridgeLoop", b.bridgeLoop)

	// Start periodic heartbeat to detect dead bot tokens / API issues.
	b.wg.Add(1)
	go b.heartbeatLoop()

	return nil
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() {
	b.cancel()
	b.api.StopReceivingUpdates()
	b.wg.Wait()
}

// IsHealthy reports whether the bot is operating normally. Returns false if
// the poll loop hasn't succeeded recently or has accumulated many errors.
func (b *Bot) IsHealthy() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.consecutiveErrs >= healthWarnThreshold {
		return false
	}
	// If we've never polled successfully and the bot has been running for
	// over a minute, something is wrong. Before Start() is called (or in
	// the first minute), we give the benefit of the doubt.
	if b.lastPollOK.IsZero() && !b.startedAt.IsZero() && time.Since(b.startedAt) > time.Minute {
		return false
	}
	return true
}

// recordPollOK marks a successful poll, resetting the error counter.
func (b *Bot) recordPollOK() {
	b.mu.Lock()
	b.lastPollOK = time.Now()
	b.consecutiveErrs = 0
	b.mu.Unlock()
}

// recordPollErr increments the consecutive error counter and logs warnings
// at threshold boundaries.
func (b *Bot) recordPollErr() {
	b.mu.Lock()
	b.consecutiveErrs++
	n := b.consecutiveErrs
	b.mu.Unlock()

	if n == healthWarnThreshold {
		logging.Error("telegram: poll loop unhealthy",
			"consecutive_errors", n,
			"threshold", healthWarnThreshold,
		)
	}
}

// recordSendOK marks a successful outbound message.
func (b *Bot) recordSendOK() {
	b.mu.Lock()
	b.lastSendOK = time.Now()
	b.mu.Unlock()
}

// supervise runs fn in a loop with panic recovery and exponential backoff
// restart. If fn returns normally or panics, it is restarted after a delay
// unless the context is cancelled. name is used for logging.
func (b *Bot) supervise(name string, fn func()) {
	defer b.wg.Done()

	delay := backoffMin
	for {
		if b.ctx.Err() != nil {
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.Error("telegram: panic in "+name, "recover", fmt.Sprintf("%v", r))
				}
			}()
			fn()
		}()

		// fn returned (or panicked). If context is done, exit cleanly.
		if b.ctx.Err() != nil {
			return
		}

		// Unexpected exit — restart with backoff.
		logging.Error("telegram: "+name+" exited unexpectedly, restarting", "delay", delay)
		select {
		case <-time.After(delay):
		case <-b.ctx.Done():
			return
		}
		delay *= 2
		if delay > backoffMax {
			delay = backoffMax
		}
	}
}

// heartbeatLoop periodically pings Telegram (getMe) to verify the bot token
// is still valid. If the call fails repeatedly, it logs escalating warnings.
func (b *Bot) heartbeatLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if err := b.rawAPICall("getMe", map[string]interface{}{}); err != nil {
				logging.Error("telegram: heartbeat failed (getMe)", "err", err)
				b.recordPollErr()
			} else {
				logging.Debug("telegram: heartbeat OK")
			}
		}
	}
}

// backoff sleeps for a duration with exponential backoff, respecting context
// cancellation. Returns the next backoff duration.
func (b *Bot) backoff(current time.Duration) time.Duration {
	select {
	case <-time.After(current):
	case <-b.ctx.Done():
	}
	next := current * 2
	if next > backoffMax {
		next = backoffMax
	}
	return next
}

// rawUpdate is a partial Telegram Update parsed from raw JSON so we can
// extract fields the go-telegram-bot-api/v5 library does not expose
// (notably message_thread_id).
type rawUpdate struct {
	UpdateID      int             `json:"update_id"`
	Message       *rawMessage     `json:"message"`
	CallbackQuery json.RawMessage `json:"callback_query"`
}

// rawCallbackQuery is a minimal callback query parsed from raw JSON.
// Used as a fallback when the go-telegram-bot-api v5 library cannot parse
// newer Telegram Bot API callback query formats (e.g., MaybeInaccessibleMessage
// in the message field, introduced in Bot API 7.0 after the library's release).
type rawCallbackQuery struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	From *struct {
		FirstName string `json:"first_name"`
	} `json:"from"`
	Message *struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
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

	// Media fields for image/document forwarding to agents.
	Photo    []rawPhotoSize `json:"photo,omitempty"`
	Document *rawDocument   `json:"document,omitempty"`
	Caption  string         `json:"caption,omitempty"`
}

// rawPhotoSize is a subset of Telegram's PhotoSize. When a user sends a
// photo, Telegram provides multiple resolutions. The last element is the
// largest (highest resolution).
type rawPhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size,omitempty"`
}

// rawDocument is a subset of Telegram's Document for file messages.
type rawDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int    `json:"file_size,omitempty"`
}

// pollLoop receives Telegram updates via raw HTTP calls (not the library's
// GetUpdatesChan) so we can parse message_thread_id which the v5 library
// does not support.
func (b *Bot) pollLoop() {
	offset := 0
	delay := backoffMin
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
			b.recordPollErr()
			delay = b.backoff(delay)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			logging.Error("telegram: poll error", "err", err)
			b.recordPollErr()
			delay = b.backoff(delay)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logging.Error("telegram: poll read error", "err", err)
			b.recordPollErr()
			delay = b.backoff(delay)
			continue
		}

		var result struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil || !result.OK {
			logging.Error("telegram: poll parse error", "err", err, "ok", result.OK)
			b.recordPollErr()
			delay = b.backoff(delay)
			continue
		}

		// Successful poll — reset backoff and record health.
		b.recordPollOK()
		delay = backoffMin

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
	// Callback queries: try the library's parsed type first, then fall back
	// to raw JSON parsing. The go-telegram-bot-api v5.5.1 predates Bot API
	// 7.0 which changed callback_query.message to MaybeInaccessibleMessage.
	// The library may leave CallbackQuery nil for newer update formats.
	if lib.CallbackQuery != nil {
		logging.Debug("telegram: callback received (library)", "data", lib.CallbackQuery.Data)
		b.handleCallback(lib.CallbackQuery)
		return
	}
	if len(raw.CallbackQuery) > 2 { // >2 excludes "" and "null"
		var rcq rawCallbackQuery
		if err := json.Unmarshal(raw.CallbackQuery, &rcq); err == nil && rcq.ID != "" {
			logging.Debug("telegram: callback received (raw fallback)", "data", rcq.Data)
			// Build a library CallbackQuery from the raw fields so we can
			// reuse the same handler for both paths.
			libCQ := &tgbotapi.CallbackQuery{
				ID:   rcq.ID,
				Data: rcq.Data,
			}
			if rcq.From != nil {
				libCQ.From = &tgbotapi.User{FirstName: rcq.From.FirstName}
			}
			if rcq.Message != nil {
				libCQ.Message = &tgbotapi.Message{
					MessageID: rcq.Message.MessageID,
					Chat:      &tgbotapi.Chat{ID: rcq.Message.Chat.ID},
					Text:      rcq.Message.Text,
				}
			}
			b.handleCallback(libCQ)
			return
		}
	}

	// Messages: use raw parsing to get message_thread_id (library v5 omits it).
	if raw.Message != nil && raw.Message.Chat.ID == b.cfg.ChatID {
		logging.Debug("telegram: inbound message",
			"text_len", len(raw.Message.Text),
			"has_photo", len(raw.Message.Photo) > 0,
			"has_document", raw.Message.Document != nil,
			"thread_id", raw.Message.MessageThreadID,
			"chat_id", raw.Message.Chat.ID,
		)

		// Photo or document messages: download and forward to agent.
		if len(raw.Message.Photo) > 0 || raw.Message.Document != nil {
			if b.hdlr.HandleInboundMedia(
				raw.Message.Photo,
				raw.Message.Document,
				raw.Message.Caption,
				raw.Message.MessageThreadID,
				b.downloadFile,
			) {
				b.reactToMessage(raw.Message.MessageID, "📸")
			}
			return
		}

		// Text messages.
		if b.hdlr.HandleInbound(raw.Message.Text, raw.Message.MessageThreadID) {
			b.reactToMessage(raw.Message.MessageID, "👀")
		}
	}
}

// bridgeLoop reads events from the TUI and sends them to Telegram.
func (b *Bot) bridgeLoop() {
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
		// No keyboard for generic attention events — the agent needs user
		// input but there's no specific question. The "Reply in this thread"
		// hint in the message body covers this. Specific events (Permission,
		// Question, Error) have their own keyboards.
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

	b.recordSendOK()
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

// reactToMessage adds an emoji reaction to a message. This provides visual
// feedback that the bot received and processed the user's message.
func (b *Bot) reactToMessage(messageID int, emoji string) {
	payload := map[string]interface{}{
		"chat_id":    b.cfg.ChatID,
		"message_id": messageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	}
	if err := b.rawAPICall("setMessageReaction", payload); err != nil {
		logging.Debug("telegram: failed to react to message", "message_id", messageID, "err", err)
	}
}

// downloadFile retrieves a file from Telegram's servers by its file_id.
// Returns the file bytes and the original file path (which contains the
// extension, e.g. "photos/file_42.jpg"). The caller is responsible for
// saving the bytes to disk.
func (b *Bot) downloadFile(fileID string) ([]byte, string, error) {
	dlURL, err := b.api.GetFileDirectURL(fileID)
	if err != nil {
		return nil, "", fmt.Errorf("getFileDirectURL: %w", err)
	}

	resp, err := http.Get(dlURL) //nolint:gosec // URL is from Telegram's API
	if err != nil {
		return nil, "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download file: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read file body: %w", err)
	}

	// Extract the file path from the download URL for extension detection.
	// URL format: https://api.telegram.org/file/bot<token>/<file_path>
	filePath := ""
	if u, err := url.Parse(dlURL); err == nil {
		filePath = u.Path
	}

	return data, filePath, nil
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

// handleCallback processes a callback query using the handler for routing
// and raw API calls for answering and editing. This avoids relying on the
// go-telegram-bot-api library's Request/Send methods which may not handle
// Forum Topic messages correctly.
func (b *Bot) handleCallback(query *tgbotapi.CallbackQuery) {
	b.hdlr.HandleCallback(b, query)
}

// answerCallbackRaw answers a callback query using a raw API call.
func (b *Bot) answerCallbackRaw(callbackID, text string) {
	payload := map[string]interface{}{
		"callback_query_id": callbackID,
		"text":              text,
	}
	if err := b.rawAPICall("answerCallbackQuery", payload); err != nil {
		logging.Debug("telegram: failed to answer callback", "callback_id", callbackID, "err", err)
	}
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
