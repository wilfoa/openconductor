// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package attention provides LLM-based classification of terminal output to
// detect agent states that require user attention.
package attention

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openconductorhq/openconductor/internal/llm"
)

// validStates is the set of valid classification results.
var validStates = map[string]bool{
	"WAITING_INPUT":    true,
	"NEEDS_PERMISSION": true,
	"DONE":             true,
	"ERROR":            true,
	"WORKING":          true,
	"STUCK":            true,
}

// Classifier uses an LLM client to classify the state of a terminal session
// based on its recent output.
type Classifier struct {
	client         llm.Client
	minInterval    time.Duration
	workingBackoff time.Duration
	mu             sync.Mutex
	lastCall       map[string]time.Time
	lastResult     map[string]string
	lastBuffer     map[string]string
}

// NewClassifier creates a new Classifier with the given LLM client.
// It enforces a minimum interval of 5 seconds between calls per session and a
// 15-second backoff when the last result was WORKING.
func NewClassifier(client llm.Client) *Classifier {
	return &Classifier{
		client:         client,
		minInterval:    5 * time.Second,
		workingBackoff: 15 * time.Second,
		lastCall:       make(map[string]time.Time),
		lastResult:     make(map[string]string),
		lastBuffer:     make(map[string]string),
	}
}

// ClassifyResult holds the parsed output from the LLM classifier.
type ClassifyResult struct {
	// State is the agent state classification (WAITING_INPUT, WORKING, etc.).
	State string
	// ImagePaths are file paths to images the agent created or referenced,
	// extracted by the LLM from natural language output. May be empty.
	ImagePaths []string
}

// Classify sends the terminal text to the LLM and parses the response.
// The result includes the agent state classification and any image file paths
// the LLM detected in the output.
//
// Throttling rules:
//   - Skips if called sooner than minInterval since the last call for this session.
//   - Skips if the buffer content has not changed since the last call.
//   - Applies a longer backoff when the previous result was WORKING.
func (c *Classifier) Classify(ctx context.Context, sessionName string, lastLines []string) (ClassifyResult, error) {
	c.mu.Lock()

	now := time.Now()
	bufferKey := strings.Join(lastLines, "\n")

	// Skip if buffer unchanged since last call.
	if prev, ok := c.lastBuffer[sessionName]; ok && prev == bufferKey {
		result := c.lastResult[sessionName]
		c.mu.Unlock()
		return ClassifyResult{State: result}, nil
	}

	// Apply throttling: minimum interval between calls.
	if last, ok := c.lastCall[sessionName]; ok {
		interval := c.minInterval
		// Apply longer backoff if last result was WORKING.
		if c.lastResult[sessionName] == "WORKING" {
			interval = c.workingBackoff
		}
		if now.Sub(last) < interval {
			result := c.lastResult[sessionName]
			c.mu.Unlock()
			return ClassifyResult{State: result}, nil
		}
	}

	// Record the call time and buffer before releasing the lock.
	c.lastCall[sessionName] = now
	c.lastBuffer[sessionName] = bufferKey
	c.mu.Unlock()

	prompt := buildPrompt(lastLines)

	result, err := c.client.Classify(ctx, prompt)
	if err != nil {
		return ClassifyResult{}, fmt.Errorf("attention classify %q: %w", sessionName, err)
	}

	state := parseState(result)
	imagePaths := parseImagePaths(result)

	c.mu.Lock()
	c.lastResult[sessionName] = state
	c.mu.Unlock()

	return ClassifyResult{State: state, ImagePaths: imagePaths}, nil
}

// buildPrompt constructs the classification prompt from terminal lines.
func buildPrompt(lines []string) string {
	joined := strings.Join(lines, "\n")
	return `You are analyzing the terminal output of a coding agent.
Based on the terminal output below, classify the agent's current state.

Reply with exactly one of:
- WAITING_INPUT: Agent is waiting for the user to type something
- NEEDS_PERMISSION: Agent is asking for permission to perform an action
- DONE: Agent has finished its current task
- ERROR: Agent encountered an error
- WORKING: Agent is still actively working
- STUCK: Agent appears to be looping or making no progress

Also, if the agent created or references any image files (.png, .jpg, .jpeg, .gif, .webp, .svg), list their file paths. If none, write "Images: none".

Terminal output:
---
` + joined + `
---

Classification:

Images:`
}

// parseState extracts and validates the classification label from the LLM
// response. If the response does not contain a recognized state, it defaults
// to "WORKING".
func parseState(response string) string {
	// Try to find a valid state in the response.
	trimmed := strings.TrimSpace(response)

	// Check the full trimmed response first (most common case).
	upper := strings.ToUpper(trimmed)
	if validStates[upper] {
		return upper
	}

	// The LLM might return something like "WAITING_INPUT" with extra text.
	// Look for any valid state in the response.
	for state := range validStates {
		if strings.Contains(upper, state) {
			return state
		}
	}

	// Default to WORKING if we cannot parse the response.
	return "WORKING"
}

// parseImagePaths extracts image file paths from the LLM response.
// Looks for lines after "Images:" that contain file paths with image extensions.
func parseImagePaths(response string) []string {
	// Find the "Images:" section in the response.
	idx := strings.Index(strings.ToLower(response), "images:")
	if idx < 0 {
		return nil
	}

	section := response[idx+len("images:"):]
	trimmed := strings.TrimSpace(section)

	// "none" means no images detected.
	if strings.HasPrefix(strings.ToLower(trimmed), "none") {
		return nil
	}

	// Split remaining lines and collect paths.
	var paths []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.Trim(line, "`\"'")
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(strings.ToLower(line), "none") {
			continue
		}

		// Only keep lines that look like file paths with image extensions.
		lower := strings.ToLower(line)
		for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"} {
			if strings.HasSuffix(lower, ext) {
				paths = append(paths, line)
				break
			}
		}
	}

	return paths
}
