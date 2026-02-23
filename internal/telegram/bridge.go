// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"strings"
	"sync"
	"time"

	"github.com/openconductorhq/openconductor/internal/logging"
)

// EventKind categorizes what happened in an agent session.
type EventKind int

const (
	EventResponse   EventKind = iota // agent finished responding (Working → Idle)
	EventPermission                  // agent needs a permission decision
	EventQuestion                    // agent is asking a question with options
	EventAttention                   // agent needs generic user input
	EventError                       // agent hit an error
	EventDone                        // task completed
)

// Event is sent from the TUI to the Telegram bot when a state transition
// occurs for a session.
type Event struct {
	Project   string // project name (used for topic lookup)
	SessionID string // session ID (e.g. "proj" or "proj (2)")
	Kind      EventKind
	Detail    string   // human-readable description from attention detection
	Screen    []string // current visible terminal lines
}

// bridge manages the outbound event flow from the TUI to Telegram, including
// deduplication and rate limiting.
type bridge struct {
	ch       chan Event
	lastSent map[string]sentRecord // project → last sent content
	mu       sync.Mutex
}

type sentRecord struct {
	content string
	at      time.Time
}

// minSendInterval is the minimum time between messages for the same project.
const minSendInterval = 3 * time.Second

func newBridge() *bridge {
	return &bridge{
		ch:       make(chan Event, 64),
		lastSent: make(map[string]sentRecord),
	}
}

// Send queues an event for delivery to Telegram. Non-blocking; drops if full.
func (b *bridge) Send(e Event) {
	select {
	case b.ch <- e:
	default:
		logging.Debug("telegram bridge: event dropped (channel full)", "project", e.Project)
	}
}

// Events returns the channel that the bot reads from.
func (b *bridge) Events() <-chan Event {
	return b.ch
}

// shouldSend checks rate limiting and deduplication. Returns true if this
// event should be sent to Telegram.
func (b *bridge) shouldSend(e Event) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	content := screenFingerprint(e.Screen)
	now := time.Now()

	if last, ok := b.lastSent[e.Project]; ok {
		// Rate limit: skip if too recent.
		if now.Sub(last.at) < minSendInterval {
			return false
		}
		// Dedup: skip if content is identical.
		if content == last.content {
			return false
		}
	}

	b.lastSent[e.Project] = sentRecord{content: content, at: now}
	return true
}

// screenFingerprint produces a simple string hash of screen content for
// deduplication. We join non-empty lines; this is cheap and sufficient.
func screenFingerprint(lines []string) string {
	var sb strings.Builder
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			sb.WriteString(trimmed)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
