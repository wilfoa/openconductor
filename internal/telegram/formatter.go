// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"fmt"
	"html"
	"strings"
)

// maxMessageLen is Telegram's maximum message length. We leave room for HTML tags.
const maxMessageLen = 4000

// FormatResponse formats an agent response (Working → Idle transition).
func FormatResponse(project string, screen []string) []string {
	body := cleanScreen(screen)
	if body == "" {
		return nil
	}
	header := fmt.Sprintf("<b>%s</b>\n\n", html.EscapeString(project))
	return splitMessage(header, body)
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
	return splitMessage(header, body)
}

// FormatQuestion formats a question from the agent.
func FormatQuestion(project string, screen []string) []string {
	body := cleanScreen(screen)
	if body == "" {
		return nil
	}
	header := fmt.Sprintf("<b>%s</b>  \xe2\x9d\x93\n\n", html.EscapeString(project)) // question emoji
	return splitMessage(header, body)
}

// FormatAttention formats a generic attention-needed message.
func FormatAttention(project string, detail string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xe2\x9a\xa0\xef\xb8\x8f\n\n", html.EscapeString(project)) // warning emoji
	if detail != "" {
		header += fmt.Sprintf("%s\n\n", html.EscapeString(detail))
	}
	if body == "" {
		return []string{header}
	}
	return splitMessage(header, body)
}

// FormatError formats an error notification.
func FormatError(project string, detail string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xF0\x9F\x94\xB4\n\n", html.EscapeString(project)) // red circle emoji
	if detail != "" {
		header += fmt.Sprintf("%s\n\n", html.EscapeString(detail))
	}
	if body == "" {
		return []string{header}
	}
	return splitMessage(header, body)
}

// FormatDone formats a task-complete notification.
func FormatDone(project string, screen []string) []string {
	body := cleanScreen(screen)
	header := fmt.Sprintf("<b>%s</b>  \xe2\x9c\x85\n\n", html.EscapeString(project)) // checkmark emoji
	if body == "" {
		return []string{header}
	}
	return splitMessage(header, body)
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
// blanks and wrapping in a <pre> block.
func cleanScreen(lines []string) string {
	// Trim leading and trailing empty lines.
	start, end := 0, len(lines)-1
	for start <= end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end >= start && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	if start > end {
		return ""
	}

	var sb strings.Builder
	for i := start; i <= end; i++ {
		sb.WriteString(html.EscapeString(lines[i]))
		if i < end {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// splitMessage splits a long message into Telegram-safe chunks. The header
// is included only in the first chunk. Each chunk is wrapped in <pre> tags
// (except the header portion).
func splitMessage(header string, body string) []string {
	// If it fits in one message, send as-is.
	full := header + "<pre>" + body + "</pre>"
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
			return header + "<pre>"
		}
		return "<pre>"
	}

	for _, line := range bodyLines {
		prefix := chunkPrefix()

		// Check if adding this line would exceed the limit.
		addition := len(prefix) + chunk.Len() + len(line) + len("</pre>") + 1
		if chunk.Len() > 0 && addition > maxMessageLen {
			// Flush current chunk with its prefix.
			messages = append(messages, prefix+chunk.String()+"</pre>")
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
		messages = append(messages, chunkPrefix()+chunk.String()+"</pre>")
	}

	return messages
}
