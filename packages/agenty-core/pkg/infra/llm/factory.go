package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
)

type factoryConfig struct {
	httpClient *http.Client
}

type Option func(*factoryConfig) error

func WithHTTPClient(client *http.Client) Option {
	return func(config *factoryConfig) error {
		if client == nil {
			return invalidRequest("HTTP client must not be nil")
		}

		config.httpClient = client
		return nil
	}
}

func NewCaller(
	ctx context.Context,
	provider catalog.Provider,
	model catalog.Model,
	options ...Option,
) (agentloop.Caller, error) {
	config := factoryConfig{}
	for _, option := range options {
		if err := option(&config); err != nil {
			return nil, fmt.Errorf("llm: apply caller option: %w", err)
		}
	}

	if strings.TrimSpace(provider.APIKey) == "" {
		return nil, invalidRequest("provider %q has no API key", provider.Code)
	}
	if model.Code.IsZero() {
		return nil, invalidRequest("model code must not be empty")
	}

	switch provider.Type {
	case catalog.APIOpenAI:
		client := newOpenAIClient(provider, config)
		return &openAIResponsesCaller{
			client:       &client,
			model:        model,
			nativeOpenAI: nativeOpenAIResponsesProvider(provider),
		}, nil
	case catalog.APIOpenAICompletions:
		client := newOpenAIClient(provider, config)
		return &openAIChatCaller{client: &client, model: model}, nil
	case catalog.APIAnthropic:
		client := newAnthropicClient(provider, config)
		return &anthropicCaller{client: &client, model: model}, nil
	case catalog.APIGemini:
		client, err := newGoogleClient(ctx, provider, config)
		if err != nil {
			return nil, fmt.Errorf("llm: create Google GenAI client: %w", err)
		}
		return &googleCaller{client: client, model: model}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAPI, provider.Type)
	}
}

func nativeOpenAIResponsesProvider(provider catalog.Provider) bool {
	if provider.Code.String() != "openai" {
		return false
	}
	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		return true
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(parsed.Scheme, "https") &&
		strings.EqualFold(parsed.Hostname(), "api.openai.com") &&
		strings.TrimRight(parsed.EscapedPath(), "/") == "/v1" &&
		parsed.RawQuery == "" && parsed.Fragment == ""
}

func newOpenAIClient(provider catalog.Provider, config factoryConfig) openai.Client {
	options := []openaioption.RequestOption{openaioption.WithAPIKey(provider.APIKey)}
	if provider.BaseURL != "" {
		options = append(options, openaioption.WithBaseURL(provider.BaseURL))
	}
	if config.httpClient != nil {
		options = append(options, openaioption.WithHTTPClient(config.httpClient))
	}

	return openai.NewClient(options...)
}

func newAnthropicClient(provider catalog.Provider, config factoryConfig) anthropic.Client {
	options := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(provider.APIKey)}
	if provider.BaseURL != "" {
		options = append(options, anthropicoption.WithBaseURL(provider.BaseURL))
	}
	if config.httpClient != nil {
		options = append(options, anthropicoption.WithHTTPClient(config.httpClient))
	}

	return anthropic.NewClient(options...)
}

func newGoogleClient(ctx context.Context, provider catalog.Provider, config factoryConfig) (*genai.Client, error) {
	clientConfig := &genai.ClientConfig{
		APIKey:     provider.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: config.httpClient,
	}
	if provider.BaseURL != "" {
		clientConfig.HTTPOptions.BaseURL = provider.BaseURL
	}

	return genai.NewClient(ctx, clientConfig)
}
