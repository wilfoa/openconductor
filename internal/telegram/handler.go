// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
	"github.com/openconductorhq/openconductor/internal/session"
)

// ptySubmitDelay is the pause between writing text and sending Enter (\r) to
// the PTY. TUI apps (Bubble Tea in particular) process stdin reads in batches
// through their event loop. Without a delay the text and Enter may land in the
// same read, and the Enter can be handled before the input component has
// committed the preceding characters — causing the submission to be silently
// ignored. 50 ms is long enough for the TUI to run at least one Update cycle.
const ptySubmitDelay = 50 * time.Millisecond

// handler routes incoming Telegram messages and callback queries to the
// appropriate agent session.
type handler struct {
	mgr      *session.Manager
	state    *topicState
	projects []config.Project
}

func newHandler(mgr *session.Manager, state *topicState, projects []config.Project) *handler {
	return &handler{
		mgr:      mgr,
		state:    state,
		projects: projects,
	}
}

// HandleInbound processes an inbound text message from a Telegram Forum Topic.
// threadID is the message_thread_id extracted from raw JSON (the library does
// not parse this field).
func (h *handler) HandleInbound(text string, threadID int) {
	if text == "" {
		return
	}

	if threadID == 0 {
		logging.Debug("telegram: inbound message has no thread ID (not a topic message)")
		return
	}

	project := h.projectByTopic(threadID)
	if project == "" {
		logging.Debug("telegram: message in unknown topic", "thread_id", threadID)
		return
	}

	sessions := h.mgr.GetSessionsByProject(project)
	if len(sessions) == 0 {
		logging.Debug("telegram: no active session for project", "project", project)
		return
	}

	// Route to the most recently created session for this project.
	s := sessions[len(sessions)-1]

	writeWithEnter(s, text)
	logging.Info("telegram: forwarded message to agent", "project", project, "session", s.ID, "len", len(text))
}

// HandleCallback processes an inline keyboard callback (permission or question).
func (h *handler) HandleCallback(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	data := query.Data
	parts := strings.SplitN(data, ":", 3)
	if len(parts) < 3 {
		logging.Debug("telegram: malformed callback data", "data", data)
		h.answerCallback(bot, query.ID, "Invalid action")
		return
	}

	kind := parts[0] // "perm" or "opt"
	project := parts[1]
	action := parts[2]

	sessions := h.mgr.GetSessionsByProject(project)
	if len(sessions) == 0 {
		h.answerCallback(bot, query.ID, "No active session")
		return
	}
	// Route to the most recently created session for this project.
	s := sessions[len(sessions)-1]

	var actionLabel string

	switch kind {
	case "perm":
		adapter := h.getAdapter(project)
		if adapter == nil {
			h.answerCallback(bot, query.ID, "Unknown agent")
			return
		}
		switch action {
		case "allow":
			s.Write(adapter.ApproveKeystroke())
			actionLabel = "Allowed once"
		case "allowall":
			ks := adapter.ApproveSessionKeystroke()
			if ks == nil {
				ks = adapter.ApproveKeystroke()
			}
			s.Write(ks)
			actionLabel = "Allowed always"
		case "deny":
			s.Write(adapter.DenyKeystroke())
			actionLabel = "Denied"
		default:
			h.answerCallback(bot, query.ID, "Unknown action")
			return
		}

	case "opt":
		// Question option: send the number, then Enter.
		writeWithEnter(s, action)
		actionLabel = fmt.Sprintf("Selected: %s", action)

	case "reply":
		// Quick-reply action from Attention/Error keyboards.
		writeWithEnter(s, action)
		actionLabel = fmt.Sprintf("Replied: %s", action)

	default:
		h.answerCallback(bot, query.ID, "Unknown callback type")
		return
	}

	logging.Info("telegram: callback handled", "project", project, "kind", kind, "action", action)

	// Answer the callback to dismiss the loading indicator.
	h.answerCallback(bot, query.ID, actionLabel)

	// Edit the original message to show the action taken.
	if query.Message != nil {
		userName := ""
		if query.From != nil {
			userName = query.From.FirstName
		}
		newText := FormatActionTaken(query.Message.Text, actionLabel, userName)
		edit := tgbotapi.NewEditMessageText(
			query.Message.Chat.ID,
			query.Message.MessageID,
			newText,
		)
		edit.ParseMode = "HTML"
		if _, err := bot.Send(edit); err != nil {
			logging.Debug("telegram: failed to edit message after callback", "err", err)
		}
	}
}

// projectByTopic looks up which project a topic ID belongs to.
func (h *handler) projectByTopic(threadID int) string {
	for _, p := range h.projects {
		if h.state.Get(p.Name) == threadID {
			return p.Name
		}
	}
	return ""
}

// getAdapter returns the agent adapter for a project.
func (h *handler) getAdapter(project string) agent.AgentAdapter {
	for _, p := range h.projects {
		if p.Name == project {
			a, err := agent.Get(p.Agent)
			if err != nil {
				return nil
			}
			return a
		}
	}
	return nil
}

func (h *handler) answerCallback(bot *tgbotapi.BotAPI, callbackID string, text string) {
	cb := tgbotapi.NewCallback(callbackID, text)
	if _, err := bot.Request(cb); err != nil {
		logging.Debug("telegram: failed to answer callback", "err", err)
	}
}

// writeWithEnter writes text to the session's PTY, pauses for ptySubmitDelay,
// then sends Enter (\r). The delay ensures the TUI's event loop processes the
// text characters before receiving the submit key.
func writeWithEnter(s *session.Session, text string) {
	s.Write([]byte(text))
	time.Sleep(ptySubmitDelay)
	s.Write([]byte("\r"))
}

// PermissionKeyboard returns an inline keyboard for permission requests.
func PermissionKeyboard(project string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Allow Once", FormatCallbackData("perm", project, "allow")),
			tgbotapi.NewInlineKeyboardButtonData("Allow Always", FormatCallbackData("perm", project, "allowall")),
			tgbotapi.NewInlineKeyboardButtonData("Deny", FormatCallbackData("perm", project, "deny")),
		),
	)
}

// AttentionKeyboard returns an inline keyboard with common quick replies
// for when the agent needs user input.
func AttentionKeyboard(project string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("yes", FormatCallbackData("reply", project, "yes")),
			tgbotapi.NewInlineKeyboardButtonData("no", FormatCallbackData("reply", project, "no")),
			tgbotapi.NewInlineKeyboardButtonData("continue", FormatCallbackData("reply", project, "continue")),
			tgbotapi.NewInlineKeyboardButtonData("skip", FormatCallbackData("reply", project, "skip")),
		),
	)
}

// ErrorKeyboard returns an inline keyboard for error states.
func ErrorKeyboard(project string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("retry", FormatCallbackData("reply", project, "retry")),
			tgbotapi.NewInlineKeyboardButtonData("skip", FormatCallbackData("reply", project, "skip")),
			tgbotapi.NewInlineKeyboardButtonData("abort", FormatCallbackData("reply", project, "abort")),
		),
	)
}

// QuestionKeyboard returns an inline keyboard for question options.
func QuestionKeyboard(project string, options []string) tgbotapi.InlineKeyboardMarkup {
	var buttons []tgbotapi.InlineKeyboardButton
	for _, opt := range options {
		// Extract just the leading number (before "." or ")").
		num := extractLeadingNumber(strings.TrimSpace(opt))
		buttons = append(buttons,
			tgbotapi.NewInlineKeyboardButtonData(opt, FormatCallbackData("opt", project, num)),
		)
	}
	return tgbotapi.NewInlineKeyboardMarkup(buttons)
}

// extractLeadingNumber returns the leading digits from a string.
// E.g. "1. Foo" → "1", "12) Bar" → "12".
func extractLeadingNumber(s string) string {
	for i, c := range s {
		if c < '0' || c > '9' {
			return s[:i]
		}
	}
	return s
}

// ParseQuestionOptions extracts numbered options from screen lines.
// Matches lines starting with "1.", "2.", "1)", "2)", etc.
func ParseQuestionOptions(lines []string) []string {
	var options []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) >= 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && (trimmed[1] == '.' || trimmed[1] == ')') {
			options = append(options, trimmed)
		}
	}
	return options
}
