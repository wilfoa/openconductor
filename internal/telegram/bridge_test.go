// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"testing"
	"time"
)

func TestScreenFingerprint_SkipsBlanks(t *testing.T) {
	lines := []string{"hello", "", "  ", "world"}
	fp := screenFingerprint(lines)
	if fp != "hello\nworld\n" {
		t.Fatalf("expected 'hello\\nworld\\n', got %q", fp)
	}
}

func TestScreenFingerprint_AllBlank(t *testing.T) {
	lines := []string{"", "  ", "   "}
	fp := screenFingerprint(lines)
	if fp != "" {
		t.Fatalf("expected empty string, got %q", fp)
	}
}

func TestBridge_SendAndReceive(t *testing.T) {
	br := newBridge()
	e := Event{Project: "proj1", Kind: EventResponse, Screen: []string{"output"}}
	br.Send(e)

	select {
	case got := <-br.Events():
		if got.Project != "proj1" {
			t.Fatalf("expected proj1, got %q", got.Project)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBridge_SendDropsWhenFull(t *testing.T) {
	br := newBridge()
	// Fill the channel (capacity 64).
	for i := 0; i < 64; i++ {
		br.Send(Event{Project: "proj"})
	}
	// This should not block — it drops silently.
	br.Send(Event{Project: "dropped"})
	// Channel should have exactly 64 events.
	if len(br.ch) != 64 {
		t.Fatalf("expected 64 events in channel, got %d", len(br.ch))
	}
}

func TestBridge_ShouldSend_RateLimit(t *testing.T) {
	br := newBridge()
	e := Event{Project: "proj1", Screen: []string{"content"}}

	// First send always passes.
	if !br.shouldSend(e) {
		t.Fatal("expected first send to pass")
	}

	// Same project within minSendInterval should be blocked.
	if br.shouldSend(e) {
		t.Fatal("expected rate-limited send to be blocked")
	}
}

func TestBridge_ShouldSend_DedupSameContent(t *testing.T) {
	br := newBridge()
	e := Event{Project: "proj1", Screen: []string{"content"}}

	br.shouldSend(e) // register first send

	// Wait past the rate limit.
	br.mu.Lock()
	br.lastSent["proj1"] = sentRecord{
		content: screenFingerprint(e.Screen),
		at:      time.Now().Add(-5 * time.Second),
	}
	br.mu.Unlock()

	// Same content should be deduped even past rate limit.
	if br.shouldSend(e) {
		t.Fatal("expected identical content to be deduped")
	}
}

func TestBridge_ShouldSend_DifferentContentPasses(t *testing.T) {
	br := newBridge()
	e1 := Event{Project: "proj1", Screen: []string{"content A"}}
	e2 := Event{Project: "proj1", Screen: []string{"content B"}}

	br.shouldSend(e1)

	// Backdate the last send past rate limit.
	br.mu.Lock()
	br.lastSent["proj1"] = sentRecord{
		content: screenFingerprint(e1.Screen),
		at:      time.Now().Add(-5 * time.Second),
	}
	br.mu.Unlock()

	if !br.shouldSend(e2) {
		t.Fatal("expected different content to pass after rate limit")
	}
}

func TestBridge_ShouldSend_DifferentProjectsIndependent(t *testing.T) {
	br := newBridge()
	e1 := Event{Project: "proj1", Screen: []string{"content"}}
	e2 := Event{Project: "proj2", Screen: []string{"content"}}

	if !br.shouldSend(e1) {
		t.Fatal("expected first project to pass")
	}
	// Different project should pass even with same content.
	if !br.shouldSend(e2) {
		t.Fatal("expected different project to pass independently")
	}
}
