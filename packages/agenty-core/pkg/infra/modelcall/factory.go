package modelcall

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"
)

func newCaller(ctx context.Context, model ModelCallConfig, config callConfig) (caller, error) {
	if strings.TrimSpace(model.APIKey) == "" {
		return nil, invalidRequest("model call has no API key")
	}
	if strings.TrimSpace(model.ModelCode) == "" {
		return nil, invalidRequest("model code must not be empty")
	}

	switch model.APIType {
	case APIOpenAI:
		client := newOpenAIClient(model, config)
		return &openAIResponsesCaller{client: &client, model: model}, nil
	case APIOpenAICompletions:
		client := newOpenAIClient(model, config)
		return &openAIChatCaller{client: &client, model: model}, nil
	case APIAnthropic:
		client := newAnthropicClient(model, config)
		return &anthropicCaller{client: &client, model: model}, nil
	case APIGemini:
		client, err := newGoogleClient(ctx, model, config)
		if err != nil {
			return nil, fmt.Errorf("modelcall: create Google GenAI client: %w", err)
		}
		return &googleCaller{client: client, model: model}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAPI, model.APIType)
	}
}

func nativeOpenAIResponsesProvider(model ModelCallConfig) bool {
	return model.Official && model.APIType == APIOpenAI
}

func newOpenAIClient(model ModelCallConfig, config callConfig) openai.Client {
	options := []openaioption.RequestOption{openaioption.WithAPIKey(model.APIKey)}
	if config.maxRetries != nil {
		options = append(options, openaioption.WithMaxRetries(*config.maxRetries))
	}
	if model.BaseURL != "" {
		options = append(options, openaioption.WithBaseURL(model.BaseURL))
	}
	if config.httpClient != nil {
		options = append(options, openaioption.WithHTTPClient(config.httpClient))
	}

	return openai.NewClient(options...)
}

func newAnthropicClient(model ModelCallConfig, config callConfig) anthropic.Client {
	options := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(model.APIKey)}
	if config.maxRetries != nil {
		options = append(options, anthropicoption.WithMaxRetries(*config.maxRetries))
	}
	if model.BaseURL != "" {
		options = append(options, anthropicoption.WithBaseURL(model.BaseURL))
	}
	if config.httpClient != nil {
		options = append(options, anthropicoption.WithHTTPClient(config.httpClient))
	}

	return anthropic.NewClient(options...)
}

func newGoogleClient(ctx context.Context, model ModelCallConfig, config callConfig) (*genai.Client, error) {
	clientConfig := &genai.ClientConfig{
		APIKey:     model.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: config.httpClient,
	}
	if model.BaseURL != "" {
		clientConfig.HTTPOptions.BaseURL = model.BaseURL
	}
	if config.maxRetries != nil {
		attempts := int32(*config.maxRetries + 1)
		clientConfig.HTTPOptions.RetryOptions = &genai.HTTPRetryOptions{Attempts: &attempts}
	}

	return genai.NewClient(ctx, clientConfig)
}
