// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/logging"
	"github.com/openconductorhq/openconductor/internal/session"
)

// DownloadFunc retrieves a file from Telegram by file_id. Returns the file
// bytes and the server file path (which contains the extension).
type DownloadFunc func(fileID string) ([]byte, string, error)

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
// not parse this field). Returns true if the message was forwarded to an agent.
func (h *handler) HandleInbound(text string, threadID int) bool {
	if text == "" {
		return false
	}

	if threadID == 0 {
		logging.Debug("telegram: inbound message has no thread ID (not a topic message)")
		return false
	}

	project := h.projectByTopic(threadID)
	if project == "" {
		logging.Debug("telegram: message in unknown topic", "thread_id", threadID)
		return false
	}

	sessions := h.mgr.GetSessionsByProject(project)
	if len(sessions) == 0 {
		logging.Debug("telegram: no active session for project", "project", project)
		return false
	}

	// Route to the most recently created session for this project.
	s := sessions[len(sessions)-1]

	writeWithEnter(s, text)
	logging.Info("telegram: forwarded message to agent", "project", project, "session", s.ID, "len", len(text))
	return true
}

// HandleCallback processes an inline keyboard callback (permission or question).
// It uses the Bot's raw API methods for answering and editing, which reliably
// handle Forum Topic messages (the library's Send/Request methods don't
// include message_thread_id).
func (h *handler) HandleCallback(b *Bot, query *tgbotapi.CallbackQuery) {
	data := query.Data
	parts := strings.SplitN(data, ":", 3)
	if len(parts) < 3 {
		logging.Debug("telegram: malformed callback data", "data", data)
		b.answerCallbackRaw(query.ID, "Invalid action")
		return
	}

	kind := parts[0] // "perm", "opt", or "reply"
	project := parts[1]
	action := parts[2]

	sessions := h.mgr.GetSessionsByProject(project)
	if len(sessions) == 0 {
		logging.Debug("telegram: callback for project with no session", "project", project, "kind", kind)
		b.answerCallbackRaw(query.ID, "No active session")
		return
	}
	// Route to the most recently created session for this project.
	s := sessions[len(sessions)-1]

	var actionLabel string

	switch kind {
	case "perm":
		adapter := h.getAdapter(project)
		if adapter == nil {
			b.answerCallbackRaw(query.ID, "Unknown agent")
			return
		}
		switch action {
		case "allow":
			// "Allow once" is the default selection — just confirm.
			s.Write(adapter.ApproveKeystroke())
			actionLabel = "Allowed once"
		case "allowall":
			ks := adapter.ApproveSessionKeystroke()
			if ks == nil {
				// Agent has no session-wide approval — fall back to single approve.
				s.Write(adapter.ApproveKeystroke())
			} else {
				writePermKeystroke(s, ks)
			}
			actionLabel = "Allowed always"
		case "deny":
			writePermKeystroke(s, adapter.DenyKeystroke())
			actionLabel = "Denied"
		default:
			b.answerCallbackRaw(query.ID, "Unknown action")
			return
		}

	case "opt":
		// Question option: navigate to the selected option and confirm.
		// Agents with selection-based dialogs (e.g. OpenCode) need arrow-key
		// navigation; others accept typed text.
		adapter := h.getAdapter(project)
		if qr, ok := adapter.(agent.QuestionResponder); ok {
			num, _ := strconv.Atoi(action)
			writePermKeystroke(s, qr.QuestionKeystroke(num))
		} else {
			writeWithEnter(s, action)
		}
		actionLabel = fmt.Sprintf("Selected: %s", action)

	case "reply":
		// Quick-reply action from Attention/Error keyboards.
		writeWithEnter(s, action)
		actionLabel = fmt.Sprintf("Replied: %s", action)

	default:
		b.answerCallbackRaw(query.ID, "Unknown callback type")
		return
	}

	logging.Info("telegram: callback handled", "project", project, "kind", kind, "action", action)

	// Answer the callback to dismiss the loading indicator.
	b.answerCallbackRaw(query.ID, actionLabel)

	// Edit the original message to show the action taken.
	if query.Message != nil {
		userName := ""
		if query.From != nil {
			userName = query.From.FirstName
		}
		// query.Message.Text is plain text (HTML tags stripped by Telegram).
		// Escape it before combining with new HTML to avoid parse errors
		// from literal < > & characters in the original content.
		newText := FormatActionTaken(html.EscapeString(query.Message.Text), actionLabel, userName)
		if err := b.editTopicMessage(query.Message.MessageID, newText, nil); err != nil {
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

// answerCallback is unused — kept as a comment for reference.
// Callback answering is now done via Bot.answerCallbackRaw() which uses
// raw API calls instead of the library's Request method.

// writeWithEnter writes text to the session's PTY, pauses for the agent's
// configured submit delay, then sends Enter (\r). The delay ensures the TUI's
// event loop processes the text characters before receiving the submit key.
// The delay comes from the agent adapter's SubmitDelay interface; agents that
// don't need a delay (e.g. Claude Code) return 0.
func writeWithEnter(s *session.Session, text string) {
	s.Write([]byte(text))
	if d := agent.GetSubmitDelay(s.Project.Agent); d > 0 {
		time.Sleep(d)
	}
	s.Write([]byte("\r"))
}

// writePermKeystroke writes a permission keystroke to the session's PTY. If
// the keystroke already ends with a submit character (\r or \n), it is written
// as-is (e.g. Claude Code's "y\n"). Otherwise, the keystroke is navigation
// for a selection dialog (e.g. OpenCode's arrow keys), so a SubmitDelay pause
// and Enter are appended to confirm the selection.
func writePermKeystroke(s *session.Session, ks []byte) {
	s.Write(ks)
	// If the keystroke already includes Enter/LF, the agent handles
	// confirmation internally (e.g. Claude Code's "n\n").
	if len(ks) > 0 && (ks[len(ks)-1] == '\r' || ks[len(ks)-1] == '\n') {
		return
	}
	// Navigation-only keystroke (e.g. arrow keys) — confirm with Enter.
	if d := agent.GetSubmitDelay(s.Project.Agent); d > 0 {
		time.Sleep(d)
	}
	s.Write([]byte("\r"))
}

// HandleInboundMedia processes an inbound photo or document from a Telegram
// Forum Topic. The file is downloaded, saved to <repo>/.openconductor/images/,
// and the path (plus optional caption) is forwarded to the agent's PTY.
func (h *handler) HandleInboundMedia(
	photo []rawPhotoSize,
	doc *rawDocument,
	caption string,
	threadID int,
	download DownloadFunc,
) bool {
	if threadID == 0 {
		logging.Debug("telegram: media message has no thread ID")
		return false
	}

	project := h.projectByTopic(threadID)
	if project == "" {
		logging.Debug("telegram: media in unknown topic", "thread_id", threadID)
		return false
	}

	sessions := h.mgr.GetSessionsByProject(project)
	if len(sessions) == 0 {
		logging.Debug("telegram: no active session for media", "project", project)
		return false
	}
	s := sessions[len(sessions)-1]

	// Determine which file to download.
	var fileID, fileName string
	if doc != nil {
		fileID = doc.FileID
		fileName = doc.FileName
	} else if len(photo) > 0 {
		// Use the largest photo (last in the array).
		fileID = photo[len(photo)-1].FileID
		fileName = "photo.jpg"
	}
	if fileID == "" {
		logging.Debug("telegram: media message with no file_id")
		return false
	}

	// Download the file.
	data, serverPath, err := download(fileID)
	if err != nil {
		logging.Error("telegram: failed to download media", "err", err, "project", project)
		return false
	}

	// Derive a filename with the correct extension.
	if ext := filepath.Ext(serverPath); ext != "" && filepath.Ext(fileName) == "" {
		fileName += ext
	}
	// Prefix with timestamp for uniqueness.
	ts := time.Now().Format("20060102_150405")
	fileName = ts + "_" + fileName

	// Save to <repo>/.openconductor/images/.
	ocDir := filepath.Join(s.Project.Repo, ".openconductor")
	imagesDir := filepath.Join(ocDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		logging.Error("telegram: failed to create images dir", "err", err, "dir", imagesDir)
		return false
	}
	// Ensure .openconductor/ is gitignored so images don't pollute the repo.
	ensureGitignore(ocDir)
	savePath := filepath.Join(imagesDir, fileName)
	if err := os.WriteFile(savePath, data, 0o644); err != nil {
		logging.Error("telegram: failed to save media", "err", err, "path", savePath)
		return false
	}

	// Build the relative path for the agent prompt.
	relPath := filepath.Join(".openconductor", "images", fileName)

	// Format the message for the agent.
	msg := agent.FormatImageInput(s.Project.Agent, relPath, caption)

	writeWithEnter(s, msg)
	logging.Info("telegram: forwarded media to agent",
		"project", project,
		"session", s.ID,
		"file", relPath,
		"bytes", len(data),
	)
	return true
}

// ensureGitignore creates a .gitignore inside the given directory (if one
// doesn't already exist) with a single "*" entry so the entire directory
// tree is excluded from git. This prevents downloaded Telegram images from
// polluting the project's repository.
func ensureGitignore(dir string) {
	gi := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gi); err == nil {
		return // already exists
	}
	_ = os.WriteFile(gi, []byte("# Auto-generated by OpenConductor\n*\n"), 0o644)
}

// PermissionKeyboard returns an inline keyboard for permission requests.
// Emoji prefixes provide visual color cues since Telegram buttons are unstyled.
func PermissionKeyboard(project string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🟢 Allow Once", FormatCallbackData("perm", project, "allow")),
			tgbotapi.NewInlineKeyboardButtonData("🟡 Allow Always", FormatCallbackData("perm", project, "allowall")),
			tgbotapi.NewInlineKeyboardButtonData("🔴 Deny", FormatCallbackData("perm", project, "deny")),
		),
	)
}

// AttentionKeyboard returns an inline keyboard with common quick replies
// for when the agent needs user input.
func AttentionKeyboard(project string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🟡 yes", FormatCallbackData("reply", project, "yes")),
			tgbotapi.NewInlineKeyboardButtonData("🟡 no", FormatCallbackData("reply", project, "no")),
			tgbotapi.NewInlineKeyboardButtonData("🟡 continue", FormatCallbackData("reply", project, "continue")),
			tgbotapi.NewInlineKeyboardButtonData("⏭ skip", FormatCallbackData("reply", project, "skip")),
		),
	)
}

// ErrorKeyboard returns an inline keyboard for error states.
func ErrorKeyboard(project string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 retry", FormatCallbackData("reply", project, "retry")),
			tgbotapi.NewInlineKeyboardButtonData("⏭ skip", FormatCallbackData("reply", project, "skip")),
			tgbotapi.NewInlineKeyboardButtonData("🔴 abort", FormatCallbackData("reply", project, "abort")),
		),
	)
}

// QuestionKeyboard returns an inline keyboard for question options.
// Each option gets a purple circle prefix for visual distinction.
func QuestionKeyboard(project string, options []string) tgbotapi.InlineKeyboardMarkup {
	var buttons []tgbotapi.InlineKeyboardButton
	for _, opt := range options {
		// Extract just the leading number (before "." or ")").
		num := extractLeadingNumber(strings.TrimSpace(opt))
		buttons = append(buttons,
			tgbotapi.NewInlineKeyboardButtonData("🟣 "+opt, FormatCallbackData("opt", project, num)),
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

// ParseQuestionOptions extracts numbered options from the question dialog
// at the bottom of OpenCode's screen. It scans backward from the dialog
// footer ("enter submit  esc dismiss") to collect only the actual options,
// avoiding false matches on numbered items in earlier conversation content.
func ParseQuestionOptions(lines []string) []string {
	// Find the question dialog footer — scan backward.
	footerIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		lower := strings.ToLower(strings.TrimSpace(lines[i]))
		if (strings.Contains(lower, "enter submit") || strings.Contains(lower, "enter confirm")) &&
			strings.Contains(lower, "esc dismiss") {
			footerIdx = i
			break
		}
	}

	// If no dialog footer found, try a broader scan for "select" + "dismiss".
	if footerIdx < 0 {
		for i := len(lines) - 1; i >= 0; i-- {
			lower := strings.ToLower(strings.TrimSpace(lines[i]))
			if strings.Contains(lower, "select") && strings.Contains(lower, "dismiss") {
				footerIdx = i
				break
			}
		}
	}

	if footerIdx < 0 {
		return nil
	}

	// Scan backward from the footer to collect numbered options.
	// Stop when we hit a line that's not an option, description, or blank
	// (i.e., we've left the dialog area).
	var options []string
	for i := footerIdx - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		// Strip leading box-drawing border characters (│, ┃) from OpenCode dialogs.
		stripped := strings.TrimLeft(trimmed, "│┃|▏▎ ")
		stripped = strings.TrimSpace(stripped)
		if stripped == "" {
			continue
		}
		// Check if this is a numbered option.
		if len(stripped) >= 2 && stripped[0] >= '1' && stripped[0] <= '9' && (stripped[1] == '.' || stripped[1] == ')') {
			options = append(options, stripped)
			continue
		}
		// Indented text (description line for a previous option) — skip but keep scanning.
		if strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "│") || strings.HasPrefix(trimmed, "┃") {
			continue
		}
		// Non-option, non-indented line — we've left the dialog. Stop.
		break
	}

	// Reverse to get ascending order (1, 2, 3...).
	for i, j := 0, len(options)-1; i < j; i, j = i+1, j-1 {
		options[i], options[j] = options[j], options[i]
	}
	return options
}
