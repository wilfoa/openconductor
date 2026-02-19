// Package llm provides a unified interface for interacting with LLM providers.
package llm

import "context"

// Client defines the interface for LLM classification requests.
type Client interface {
	// Classify sends a prompt to the LLM and returns the raw text response.
	Classify(ctx context.Context, prompt string) (string, error)
}
