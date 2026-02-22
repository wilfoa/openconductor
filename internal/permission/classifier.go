// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/llm"
)

// classifierMinInterval is the minimum time between L2 calls per session.
const classifierMinInterval = 5 * time.Second

// Classifier uses an LLM to extract the permission category from terminal
// output when L1 pattern matching is inconclusive.
type Classifier struct {
	client      llm.Client
	minInterval time.Duration
	mu          sync.Mutex
	lastCall    map[string]time.Time
	lastBuffer  map[string]string
	lastResult  map[string]*ParsedPermission
}

// NewClassifier creates a Classifier backed by the given LLM client.
func NewClassifier(client llm.Client) *Classifier {
	return &Classifier{
		client:      client,
		minInterval: classifierMinInterval,
		lastCall:    make(map[string]time.Time),
		lastBuffer:  make(map[string]string),
		lastResult:  make(map[string]*ParsedPermission),
	}
}

// Classify sends the terminal output to the LLM and returns the parsed
// permission. Returns nil (with no error) when the LLM is throttled or when
// the buffer is unchanged since the last call.
func (c *Classifier) Classify(ctx context.Context, sessionName string, agentType config.AgentType, lines []string) (*ParsedPermission, error) {
	bufferKey := strings.Join(lines, "\n")

	c.mu.Lock()
	// Return cached result for identical buffer content.
	if prev, ok := c.lastBuffer[sessionName]; ok && prev == bufferKey {
		result := c.lastResult[sessionName]
		c.mu.Unlock()
		return result, nil
	}

	// Throttle: enforce minimum interval between calls.
	if last, ok := c.lastCall[sessionName]; ok {
		if time.Since(last) < c.minInterval {
			result := c.lastResult[sessionName]
			c.mu.Unlock()
			return result, nil
		}
	}

	c.lastCall[sessionName] = time.Now()
	c.lastBuffer[sessionName] = bufferKey
	c.mu.Unlock()

	prompt := buildClassifierPrompt(agentType, lines)
	raw, err := c.client.Classify(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("permission classify %q: %w", sessionName, err)
	}

	result := parseClassifierResponse(raw)

	c.mu.Lock()
	c.lastResult[sessionName] = result
	c.mu.Unlock()

	return result, nil
}

// llmResponse mirrors the JSON shape the LLM is asked to return.
type llmResponse struct {
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

// buildClassifierPrompt constructs the permission-classification prompt.
func buildClassifierPrompt(agentType config.AgentType, lines []string) string {
	joined := strings.Join(lines, "\n")
	safeList := BashSafeCommandList()

	return fmt.Sprintf(`You are analyzing a permission request from an AI coding agent (%s).

Based on the terminal output below, determine what permission is being requested.

Reply with a JSON object only — no explanation, no markdown fences:
{"category":"...","description":"...","confidence":0.0}

Valid categories:
- "file_read"   — reading or viewing file contents
- "file_edit"   — modifying existing file contents
- "file_create" — creating new files or directories
- "file_delete" — deleting files or directories
- "bash_safe"   — executing a safe shell command (only: %s)
- "bash_any"    — executing any other shell command
- "mcp_tools"   — calling an MCP (Model Context Protocol) tool
- "network"     — making a network request (HTTP, fetch, etc.)
- "unknown"     — cannot determine the permission type

Set confidence to 1.0 if certain, lower if uncertain. Only set bash_safe if the
command is explicitly one of the safe commands listed above.

Terminal output:
---
%s
---

JSON:`, agentType, safeList, joined)
}

// parseClassifierResponse extracts a ParsedPermission from the raw LLM output.
// Falls back to Unknown/low-confidence on any parse error.
func parseClassifierResponse(raw string) *ParsedPermission {
	// Strip any accidental markdown fences.
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Extract the JSON object.
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return &ParsedPermission{
			Category:    Unknown,
			Description: "could not parse LLM response",
			Confidence:  0,
			Source:      "llm",
		}
	}
	raw = raw[start : end+1]

	var resp llmResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return &ParsedPermission{
			Category:    Unknown,
			Description: "invalid LLM JSON: " + err.Error(),
			Confidence:  0,
			Source:      "llm",
		}
	}

	cat := Category(strings.ToLower(strings.TrimSpace(resp.Category)))
	switch cat {
	case FileRead, FileEdit, FileCreate, FileDelete,
		BashSafe, BashAny, MCPTools, Network, Unknown:
		// valid
	default:
		cat = Unknown
	}

	return &ParsedPermission{
		Category:    cat,
		Description: resp.Description,
		Confidence:  resp.Confidence,
		Source:      "llm",
	}
}
