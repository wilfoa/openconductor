// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── Health tracking ─────────────────────────────────────────────

func TestHealthy_InitialState(t *testing.T) {
	b := &Bot{}
	// A fresh bot with no errors and no polls should be healthy (both times
	// are zero, so the "never polled for > 1 minute" check doesn't fire
	// because lastSendOK is also zero).
	if !b.IsHealthy() {
		t.Error("expected fresh bot to be healthy")
	}
}

func TestHealthy_AfterPollOK(t *testing.T) {
	b := &Bot{}
	b.recordPollOK()
	if !b.IsHealthy() {
		t.Error("expected healthy after successful poll")
	}
}

func TestUnhealthy_AfterManyErrors(t *testing.T) {
	b := &Bot{}
	for i := 0; i < healthWarnThreshold; i++ {
		b.recordPollErr()
	}
	if b.IsHealthy() {
		t.Errorf("expected unhealthy after %d consecutive errors", healthWarnThreshold)
	}
}

func TestHealthy_ErrorCounterResets(t *testing.T) {
	b := &Bot{}
	// Accumulate errors just under the threshold.
	for i := 0; i < healthWarnThreshold-1; i++ {
		b.recordPollErr()
	}
	// A successful poll resets the counter.
	b.recordPollOK()
	if !b.IsHealthy() {
		t.Error("expected healthy after poll OK reset")
	}
}

func TestRecordSendOK_SetsTime(t *testing.T) {
	b := &Bot{}
	before := time.Now()
	b.recordSendOK()
	b.mu.Lock()
	ts := b.lastSendOK
	b.mu.Unlock()
	if ts.Before(before) {
		t.Error("expected lastSendOK to be set to now or later")
	}
}

// ── Backoff ─────────────────────────────────────────────────────

func TestBackoff_Doubles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &Bot{ctx: ctx}

	// Use a very small duration so the test is fast.
	next := b.backoff(1 * time.Millisecond)
	if next != 2*time.Millisecond {
		t.Errorf("expected 2ms, got %v", next)
	}
}

func TestBackoff_CapsAtMax(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &Bot{ctx: ctx}

	next := b.backoff(backoffMax)
	if next != backoffMax {
		t.Errorf("expected backoff capped at %v, got %v", backoffMax, next)
	}
}

func TestBackoff_RespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bot{ctx: ctx}

	done := make(chan struct{})
	go func() {
		// Request a long sleep — it should return early when cancelled.
		b.backoff(10 * time.Second)
		close(done)
	}()

	// Cancel after a short delay.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK — returned promptly.
	case <-time.After(2 * time.Second):
		t.Fatal("backoff did not respect context cancellation")
	}
}

// ── Supervisor ──────────────────────────────────────────────────

func TestSupervise_RestartsAfterPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &Bot{ctx: ctx}

	var calls atomic.Int32
	fn := func() {
		n := calls.Add(1)
		if n <= 2 {
			panic("test panic")
		}
		// Third call: exit normally to let the supervisor restart loop
		// detect context cancellation.
		cancel()
	}

	b.wg.Add(1)
	go b.supervise("test", fn)

	// Wait for the goroutine to finish.
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		got := calls.Load()
		if got < 3 {
			t.Errorf("expected at least 3 calls (2 panics + 1 normal), got %d", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("supervisor did not finish in time")
	}
}

func TestSupervise_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bot{ctx: ctx}

	// fn blocks forever (via context).
	fn := func() {
		<-ctx.Done()
	}

	b.wg.Add(1)
	go b.supervise("test", fn)

	// Cancel and expect the supervisor to exit.
	cancel()

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK.
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop after context cancel")
	}
}

func TestSupervise_RestartsAfterNormalReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &Bot{ctx: ctx}

	var calls atomic.Int32
	fn := func() {
		n := calls.Add(1)
		if n >= 3 {
			cancel()
		}
		// Return normally — supervisor should restart.
	}

	b.wg.Add(1)
	go b.supervise("test", fn)

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		got := calls.Load()
		if got < 3 {
			t.Errorf("expected at least 3 calls, got %d", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("supervisor did not finish in time")
	}
}

// ── Concurrent health tracking ──────────────────────────────────

func TestHealth_ConcurrentAccess(t *testing.T) {
	b := &Bot{}
	var wg sync.WaitGroup

	// Hammer health tracking from multiple goroutines to detect races.
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			b.recordPollOK()
		}()
		go func() {
			defer wg.Done()
			b.recordPollErr()
		}()
		go func() {
			defer wg.Done()
			b.recordSendOK()
			_ = b.IsHealthy()
		}()
	}
	wg.Wait()
	// No race detector panic = pass.
}
