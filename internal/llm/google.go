package llm

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// GoogleClient implements the Client interface using the Google Gemini API.
type GoogleClient struct {
	client *genai.Client
	model  string
}

// NewGoogleClient creates a new GoogleClient configured with the given API
// key. If model is empty, it defaults to gemini-2.0-flash.
func NewGoogleClient(ctx context.Context, apiKey, model string) (*GoogleClient, error) {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("creating google genai client: %w", err)
	}
	return &GoogleClient{
		client: client,
		model:  model,
	}, nil
}

// Classify sends the prompt to the Gemini API and returns the raw text
// response.
func (g *GoogleClient) Classify(ctx context.Context, prompt string) (string, error) {
	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt), nil)
	if err != nil {
		return "", fmt.Errorf("google classify: %w", err)
	}

	text := resp.Text()
	if text == "" {
		return "", fmt.Errorf("google classify: empty response")
	}

	return strings.TrimSpace(text), nil
}
