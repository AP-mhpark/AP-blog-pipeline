package llm

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const defaultModel = anthropic.ModelClaudeSonnet5

// Client wraps the Anthropic Messages API for text generation.
type Client struct {
	api anthropic.Client
}

// NewClient creates a Client. If apiKey is empty, the underlying SDK falls
// back to the ANTHROPIC_API_KEY environment variable.
func NewClient(apiKey string) *Client {
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &Client{api: anthropic.NewClient(opts...)}
}

// GenerateText sends a system+user prompt pair to Claude and returns the
// concatenated text of the response.
func (c *Client) GenerateText(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	message, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     defaultModel,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("generate text: %w", err)
	}

	var out string
	for _, block := range message.Content {
		if text := block.AsText(); text.Text != "" {
			out += text.Text
		}
	}
	return out, nil
}
