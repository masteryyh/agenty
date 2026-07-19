package conn

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"
)

func GetOpenAIClient(baseUrl, apiKey string) *openai.Client {
	client := openai.NewClient(
		openaiOption.WithBaseURL(baseUrl),
		openaiOption.WithAPIKey(apiKey),
		openaiOption.WithHTTPClient(GetHTTPClient()),
	)
	return &client
}

func GetAnthropicClient(baseURL, apiKey string) anthropic.Client {
	opts := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(apiKey),
		anthropicOption.WithHTTPClient(GetHTTPClient()),
	}
	if baseURL != "" {
		opts = append(opts, anthropicOption.WithBaseURL(baseURL))
	}
	return anthropic.NewClient(opts...)
}

func GetGeminiClient(ctx context.Context, baseURL, apiKey string) (*genai.Client, error) {
	cc := &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: GetHTTPClient(),
	}
	if baseURL != "" {
		cc.HTTPOptions = genai.HTTPOptions{BaseURL: baseURL}
	}

	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return client, nil
}
