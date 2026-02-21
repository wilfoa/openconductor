// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package notification provides desktop notification support for agent
// attention events.
package notification

import (
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

// Notifier sends desktop notifications with per-project cooldown to avoid
// notification fatigue.
type Notifier struct {
	enabled  bool
	cooldown time.Duration
	mu       sync.Mutex
	lastSent map[string]time.Time
}

// New creates a new Notifier. If enabled is false, all calls to Notify are
// no-ops. The cooldownSeconds parameter controls the minimum interval between
// notifications for the same project.
func New(enabled bool, cooldownSeconds int) *Notifier {
	return &Notifier{
		enabled:  enabled,
		cooldown: time.Duration(cooldownSeconds) * time.Second,
		lastSent: make(map[string]time.Time),
	}
}

// Notify sends a desktop notification for the given project and attention type.
// It respects the per-project cooldown period and silently drops notifications
// that arrive too soon after the previous one for the same project.
func (n *Notifier) Notify(project string, attnType string, detail string) {
	if !n.enabled {
		return
	}

	n.mu.Lock()
	now := time.Now()
	if last, ok := n.lastSent[project]; ok {
		if now.Sub(last) < n.cooldown {
			n.mu.Unlock()
			return
		}
	}
	n.lastSent[project] = now
	n.mu.Unlock()

	title := fmt.Sprintf("OpenConductor: %s", project)
	message := fmt.Sprintf("[%s] %s", attnType, detail)

	// Best-effort notification; errors are intentionally discarded since
	// desktop notifications are non-critical.
	_ = beeep.Notify(title, message, "")
}
