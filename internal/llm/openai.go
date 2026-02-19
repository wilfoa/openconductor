package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// OpenAIClient implements the Client interface using the OpenAI API.
type OpenAIClient struct {
	client openai.Client
	model  shared.ChatModel
}

// NewOpenAIClient creates a new OpenAIClient configured with the given API key.
// It uses the gpt-4o-mini model by default.
func NewOpenAIClient(apiKey string) *OpenAIClient {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAIClient{
		client: client,
		model:  shared.ChatModelGPT4oMini,
	}
}

// Classify sends the prompt to the OpenAI API and returns the raw text
// response.
func (o *OpenAIClient) Classify(ctx context.Context, prompt string) (string, error) {
	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: o.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return "", fmt.Errorf("openai classify: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai classify: no choices returned")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
