// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"fmt"
	"html"
	"strings"
	"unicode"
)

// maxMessageLen is Telegram's maximum message length. We leave room for HTML tags.
const maxMessageLen = 4000

// replyHint is appended to actionable messages to inform the user they can
// reply directly in the Telegram thread to send text to the agent.
const replyHint = "\n\n<i>Reply in this thread to respond to the agent</i>"

// FormatResponse formats an agent response (Working → Idle transition).
func FormatResponse(project string, screen []string) []string {
	body := cleanScreen(screen)
	if body == "" {
		return nil
	}
	header := fmt.Sprintf("<b>%s</b>\n\n", html.EscapeString(project))
	msgs := splitMessage(header, body, false)
	if len(msgs) > 0 {
		msgs[len(msgs)-1] += replyHint
	}
	return msgs
}

// FormatPermission formats a permission request.
func FormatPermission(project string, detail string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xF0\x9F\x94\x92\n\n", html.EscapeString(project)) // lock emoji
	if detail != "" {
		header += fmt.Sprintf("<code>%s</code>\n\n", html.EscapeString(detail))
	}
	if body == "" {
		return []string{header}
	}
	return splitMessage(header, body, true)
}

// FormatQuestion formats a question from the agent.
func FormatQuestion(project string, screen []string) []string {
	body := cleanScreen(screen)
	if body == "" {
		return nil
	}
	header := fmt.Sprintf("<b>%s</b>  \xe2\x9d\x93\n\n", html.EscapeString(project)) // question emoji
	return splitMessage(header, body, true)
}

// FormatAttention formats a generic attention-needed message.
func FormatAttention(project string, detail string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xe2\x9a\xa0\xef\xb8\x8f\n\n", html.EscapeString(project)) // warning emoji
	if detail != "" {
		header += fmt.Sprintf("%s\n\n", html.EscapeString(detail))
	}
	if body == "" {
		return []string{header + replyHint}
	}
	msgs := splitMessage(header, body, false)
	if len(msgs) > 0 {
		msgs[len(msgs)-1] += replyHint
	}
	return msgs
}

// FormatError formats an error notification.
func FormatError(project string, detail string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xF0\x9F\x94\xB4\n\n", html.EscapeString(project)) // red circle emoji
	if detail != "" {
		header += fmt.Sprintf("%s\n\n", html.EscapeString(detail))
	}
	if body == "" {
		return []string{header + replyHint}
	}
	msgs := splitMessage(header, body, true)
	if len(msgs) > 0 {
		msgs[len(msgs)-1] += replyHint
	}
	return msgs
}

// FormatDone formats a task-complete notification.
func FormatDone(project string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xe2\x9c\x85\n\n", html.EscapeString(project)) // checkmark emoji
	footer := "\n\n<i>Reply in this thread to start a new task</i>"
	if body == "" {
		return []string{header + footer}
	}
	msgs := splitMessage(header, body, false)
	if len(msgs) > 0 {
		msgs[len(msgs)-1] += footer
	}
	return msgs
}

// FormatActionTaken edits a permission/question message to show what happened.
func FormatActionTaken(original string, action string, user string) string {
	suffix := fmt.Sprintf("\n\n<i>%s</i>", html.EscapeString(action))
	if user != "" {
		suffix += fmt.Sprintf(" by %s", html.EscapeString(user))
	}
	return original + suffix
}

// cleanScreen extracts meaningful text from terminal screen lines, trimming
// blanks, removing decorative TUI borders, and wrapping in a <pre> block.
func cleanScreen(lines []string) string {
	// Filter out blank and decorative lines.
	var filtered []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			filtered = append(filtered, line)
			continue
		}
		if isDecorativeLine(line) {
			continue
		}
		filtered = append(filtered, line)
	}

	// Trim leading and trailing empty lines.
	start, end := 0, len(filtered)-1
	for start <= end && strings.TrimSpace(filtered[start]) == "" {
		start++
	}
	for end >= start && strings.TrimSpace(filtered[end]) == "" {
		end--
	}
	if start > end {
		return ""
	}

	var sb strings.Builder
	for i := start; i <= end; i++ {
		sb.WriteString(html.EscapeString(filtered[i]))
		if i < end {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// isDecorativeLine returns true if a line consists entirely of box-drawing
// characters and whitespace. These are TUI panel borders (e.g. ╹▀▀▀▀▀▀▀▀)
// that carry no text content and should not be sent to Telegram.
func isDecorativeLine(line string) bool {
	hasBoxChar := false
	for _, r := range line {
		if isBoxDrawing(r) {
			hasBoxChar = true
		} else if !unicode.IsSpace(r) {
			return false
		}
	}
	return hasBoxChar
}

// isBoxDrawing returns true for Unicode box-drawing and block-element
// characters commonly used in TUI borders.
func isBoxDrawing(r rune) bool {
	// U+2500–U+257F: Box Drawing
	if r >= 0x2500 && r <= 0x257F {
		return true
	}
	// U+2580–U+259F: Block Elements (▀ ▄ █ ░ ▒ ▓ etc.)
	if r >= 0x2580 && r <= 0x259F {
		return true
	}
	return false
}

// splitMessage splits a long message into Telegram-safe chunks. The header
// is included only in the first chunk. When codeBlock is true, each chunk's
// body is wrapped in <pre> tags (monospace); otherwise the body is sent as
// plain HTML text (normal proportional font).
func splitMessage(header string, body string, codeBlock bool) []string {
	open, close_ := "", ""
	if codeBlock {
		open, close_ = "<pre>", "</pre>"
	}

	// If it fits in one message, send as-is.
	full := header + open + body + close_
	if len(full) <= maxMessageLen {
		return []string{full}
	}

	// Split body on newline boundaries.
	bodyLines := strings.Split(body, "\n")
	var messages []string
	var chunk strings.Builder
	isFirst := true

	// chunkPrefix returns the correct prefix for the current chunk.
	chunkPrefix := func() string {
		if isFirst {
			return header + open
		}
		return open
	}

	for _, line := range bodyLines {
		prefix := chunkPrefix()

		// Check if adding this line would exceed the limit.
		addition := len(prefix) + chunk.Len() + len(line) + len(close_) + 1
		if chunk.Len() > 0 && addition > maxMessageLen {
			// Flush current chunk with its prefix.
			messages = append(messages, prefix+chunk.String()+close_)
			chunk.Reset()
			isFirst = false
			// Start a new chunk with this line.
			chunk.WriteString(line)
		} else {
			if chunk.Len() > 0 {
				chunk.WriteByte('\n')
			}
			chunk.WriteString(line)
		}
	}

	// Flush remaining.
	if chunk.Len() > 0 {
		messages = append(messages, chunkPrefix()+chunk.String()+close_)
	}

	return messages
}
