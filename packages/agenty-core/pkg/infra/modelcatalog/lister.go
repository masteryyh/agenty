// Package modelcatalog retrieves and normalizes provider model listings.
package modelcatalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const (
	defaultAnthropicVersion = "2023-06-01"
	defaultModelsURL        = "models"
	defaultAnthropicModels  = "v1/models"
	maxResponseBytes        = 16 << 20
	maxPaginationPages      = 100
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Lister calls a provider's configured model endpoint and adapts its response
// to the provider-neutral AvailableModel shape.
type Lister struct {
	client httpDoer
}

// NewLister creates a model lister. A bounded timeout is used when no client is
// supplied so an unavailable provider cannot block the core indefinitely.
func NewLister(client *http.Client) *Lister {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Lister{client: client}
}

var defaultLister = NewLister(nil)

// List retrieves and normalizes the model catalog using the built-in HTTP
// client. ProviderService uses this entry point so callers do not need to
// construct or inject an infrastructure dependency.
func List(ctx context.Context, provider catalog.Provider) ([]catalog.AvailableModel, error) {
	return defaultLister.List(ctx, provider)
}

// List retrieves all available models for provider. Provider-specific pages are
// followed transparently until the upstream indicates that the list is done.
func (l *Lister) List(ctx context.Context, provider catalog.Provider) ([]catalog.AvailableModel, error) {
	if l == nil || l.client == nil {
		return nil, fmt.Errorf("model catalog HTTP client is not configured")
	}

	switch provider.Type {
	case catalog.APIOpenAI, catalog.APIOpenAICompletions:
		return l.listOpenAICompatible(ctx, provider)
	case catalog.APIAnthropic:
		return l.listAnthropic(ctx, provider)
	case catalog.APIGemini:
		return l.listGemini(ctx, provider)
	default:
		return nil, fmt.Errorf("unsupported provider API type %q", provider.Type)
	}
}

type anthropicListResponse struct {
	Data    []anthropicModel `json:"data"`
	HasMore bool             `json:"has_more"`
	LastID  string           `json:"last_id"`
}

type anthropicModel struct {
	ID             string                 `json:"id"`
	DisplayName    string                 `json:"display_name"`
	MaxInputTokens int                    `json:"max_input_tokens"`
	MaxTokens      int64                  `json:"max_tokens"`
	Capabilities   *anthropicCapabilities `json:"capabilities"`
}

type capabilitySupport struct {
	Supported bool `json:"supported"`
}

type anthropicCapabilities struct {
	ImageInput *capabilitySupport `json:"image_input"`
	PDFInput   *capabilitySupport `json:"pdf_input"`
	Thinking   *capabilitySupport `json:"thinking"`
	Effort     *anthropicEffort   `json:"effort"`
}

type anthropicEffort struct {
	Supported bool              `json:"supported"`
	Low       capabilitySupport `json:"low"`
	Medium    capabilitySupport `json:"medium"`
	High      capabilitySupport `json:"high"`
	XHigh     capabilitySupport `json:"xhigh"`
	Max       capabilitySupport `json:"max"`
}

func (l *Lister) listAnthropic(ctx context.Context, provider catalog.Provider) ([]catalog.AvailableModel, error) {
	models := make([]catalog.AvailableModel, 0)
	seenCursors := make(map[string]struct{})
	var afterID string

	for range maxPaginationPages {
		query := url.Values{}
		if afterID != "" {
			query.Set("after_id", afterID)
		}

		var response anthropicListResponse
		if err := l.getJSON(ctx, provider, query, &response); err != nil {
			return nil, err
		}

		for index, item := range response.Data {
			efforts := anthropicReasoningEfforts(item.Capabilities)
			multiModal := item.Capabilities != nil &&
				((item.Capabilities.ImageInput != nil && item.Capabilities.ImageInput.Supported) ||
					(item.Capabilities.PDFInput != nil && item.Capabilities.PDFInput.Supported))
			model, err := normalizeModel(
				provider,
				len(models)+index,
				item.ID,
				item.DisplayName,
				item.MaxInputTokens,
				item.MaxTokens,
				multiModal,
				efforts,
			)
			if err != nil {
				return nil, err
			}
			models = append(models, model)
		}

		if !response.HasMore {
			return models, nil
		}
		if response.LastID == "" {
			return nil, fmt.Errorf("provider %q returned has_more without last_id", provider.Code)
		}
		if _, ok := seenCursors[response.LastID]; ok {
			return nil, fmt.Errorf("provider %q returned a repeated pagination cursor", provider.Code)
		}
		seenCursors[response.LastID] = struct{}{}
		afterID = response.LastID
	}

	return nil, fmt.Errorf("provider %q exceeded model pagination limit", provider.Code)
}

func anthropicReasoningEfforts(capabilities *anthropicCapabilities) []shared.ReasoningEffort {
	if capabilities == nil {
		return []shared.ReasoningEffort{}
	}
	if capabilities.Effort != nil {
		effort := capabilities.Effort
		supported := make([]shared.ReasoningEffort, 0, len(shared.StandardReasoningEfforts()))
		if effort.Low.Supported {
			supported = append(supported, shared.ReasoningLow)
		}
		if effort.Medium.Supported {
			supported = append(supported, shared.ReasoningMedium)
		}
		if effort.High.Supported {
			supported = append(supported, shared.ReasoningHigh)
		}
		if effort.XHigh.Supported {
			supported = append(supported, shared.ReasoningXHigh)
		}
		if effort.Max.Supported {
			supported = append(supported, shared.ReasoningMax)
		}
		if len(supported) > 0 {
			return supported
		}
		if effort.Supported {
			return shared.StandardReasoningEfforts()
		}
		return []shared.ReasoningEffort{}
	}
	if capabilities.Thinking != nil && capabilities.Thinking.Supported {
		return shared.StandardReasoningEfforts()
	}
	return []shared.ReasoningEffort{}
}

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	ContextLength       int                    `json:"context_length"`
	MaxCompletionTokens int64                  `json:"max_completion_tokens"`
	Architecture        openRouterArchitecture `json:"architecture"`
	TopProvider         openRouterTopProvider  `json:"top_provider"`
	Reasoning           *openRouterReasoning   `json:"reasoning"`
}

type openRouterArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

type openRouterTopProvider struct {
	ContextLength       int   `json:"context_length"`
	MaxCompletionTokens int64 `json:"max_completion_tokens"`
}

type openRouterReasoning struct {
	SupportedEfforts []string `json:"supported_efforts"`
}

func (l *Lister) listOpenAICompatible(ctx context.Context, provider catalog.Provider) ([]catalog.AvailableModel, error) {
	var response openRouterResponse
	if err := l.getJSON(ctx, provider, nil, &response); err != nil {
		return nil, err
	}

	isOpenRouter := provider.Code.String() == "openrouter"
	models := make([]catalog.AvailableModel, 0, len(response.Data))
	for index, item := range response.Data {
		if isOpenRouter && !slices.Contains(item.Architecture.OutputModalities, "text") {
			continue
		}

		contextWindow := item.ContextLength
		if contextWindow <= 0 {
			contextWindow = item.TopProvider.ContextLength
		}
		maxOutputTokens := item.MaxCompletionTokens
		if maxOutputTokens <= 0 {
			maxOutputTokens = item.TopProvider.MaxCompletionTokens
		}
		var efforts []shared.ReasoningEffort
		if item.Reasoning != nil {
			if item.Reasoning.SupportedEfforts == nil {
				efforts = shared.StandardReasoningEfforts()
			} else {
				efforts = parseReasoningEfforts(item.Reasoning.SupportedEfforts)
			}
		} else {
			efforts = []shared.ReasoningEffort{}
		}
		model, err := normalizeModel(
			provider,
			index,
			item.ID,
			item.Name,
			contextWindow,
			maxOutputTokens,
			containsNonTextInput(item.Architecture.InputModalities),
			efforts,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, nil
}

type geminiListResponse struct {
	Models        []geminiModel `json:"models"`
	NextPageToken string        `json:"nextPageToken"`
}

type geminiModel struct {
	Name                       string   `json:"name"`
	BaseModelID                string   `json:"baseModelId"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int64    `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Thinking                   *bool    `json:"thinking"`
}

func (l *Lister) listGemini(ctx context.Context, provider catalog.Provider) ([]catalog.AvailableModel, error) {
	models := make([]catalog.AvailableModel, 0)
	seenTokens := make(map[string]struct{})
	var pageToken string

	for range maxPaginationPages {
		query := url.Values{}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}

		var response geminiListResponse
		if err := l.getJSON(ctx, provider, query, &response); err != nil {
			return nil, err
		}

		for _, item := range response.Models {
			if !slices.Contains(item.SupportedGenerationMethods, "generateContent") {
				continue
			}

			code := strings.TrimPrefix(item.BaseModelID, "models/")
			if code == "" {
				code = strings.TrimPrefix(item.Name, "models/")
			}
			efforts := []shared.ReasoningEffort{}
			if item.Thinking != nil && *item.Thinking {
				efforts = []shared.ReasoningEffort{
					shared.ReasoningLow,
					shared.ReasoningMedium,
					shared.ReasoningHigh,
				}
			}

			model, err := normalizeModel(
				provider,
				len(models),
				code,
				item.DisplayName,
				item.InputTokenLimit,
				item.OutputTokenLimit,
				false,
				efforts,
			)
			if err != nil {
				return nil, err
			}
			models = append(models, model)
		}

		if response.NextPageToken == "" {
			return models, nil
		}
		if _, ok := seenTokens[response.NextPageToken]; ok {
			return nil, fmt.Errorf("provider %q returned a repeated pagination token", provider.Code)
		}
		seenTokens[response.NextPageToken] = struct{}{}
		pageToken = response.NextPageToken
	}

	return nil, fmt.Errorf("provider %q exceeded model pagination limit", provider.Code)
}

func (l *Lister) getJSON(ctx context.Context, provider catalog.Provider, query url.Values, target any) error {
	endpoint, err := endpointURL(provider)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		values := endpoint.Query()
		for key, items := range query {
			values.Del(key)
			for _, item := range items {
				values.Add(key, item)
			}
		}
		endpoint.RawQuery = values.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build model list request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	switch provider.Type {
	case catalog.APIAnthropic:
		request.Header.Set("x-api-key", provider.APIKey)
		request.Header.Set("anthropic-version", defaultAnthropicVersion)
	case catalog.APIGemini:
		values := request.URL.Query()
		values.Set("key", provider.APIKey)
		request.URL.RawQuery = values.Encode()
	default:
		request.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	response, err := l.client.Do(request)
	if err != nil {
		return fmt.Errorf("request provider model list: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read provider model list response: %w", err)
	}
	if int64(len(body)) > maxResponseBytes {
		return fmt.Errorf("provider model list response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("provider model list returned HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode provider model list response: %w", err)
	}
	return nil
}

func endpointURL(provider catalog.Provider) (*url.URL, error) {
	baseURL, err := url.Parse(strings.TrimSpace(provider.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid provider base URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" || baseURL.Host == "" {
		return nil, fmt.Errorf("provider base URL must be an absolute HTTP(S) URL")
	}

	modelsURL := strings.TrimSpace(provider.ModelsURL)
	if modelsURL == "" {
		modelsURL = defaultModelsURL
		if provider.Type == catalog.APIAnthropic {
			modelsURL = defaultAnthropicModels
		}
	}
	relative, err := url.Parse(modelsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid provider models URL: %w", err)
	}
	if relative.IsAbs() || relative.Host != "" || strings.HasPrefix(relative.Path, "//") {
		return nil, fmt.Errorf("provider models URL must be relative to the base URL")
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + strings.TrimLeft(relative.Path, "/")
	baseURL.RawPath = ""
	if relative.RawQuery != "" {
		baseURL.RawQuery = relative.RawQuery
	}
	baseURL.Fragment = ""
	return baseURL, nil
}

func normalizeModel(
	provider catalog.Provider,
	index int,
	code string,
	name string,
	contextWindow int,
	maxOutputTokens int64,
	multiModal bool,
	reasoningEfforts []shared.ReasoningEffort,
) (catalog.AvailableModel, error) {
	code = strings.TrimSpace(code)
	modelCode, err := shared.NewModelCode(code)
	if err != nil {
		return catalog.AvailableModel{}, fmt.Errorf("provider %q model %d has invalid id: %w", provider.Code, index, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = code
	}
	if contextWindow <= 0 {
		contextWindow = catalog.DefaultAvailableModelContextWindow
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = catalog.DefaultAvailableModelMaxOutputTokens
	}
	if reasoningEfforts == nil {
		reasoningEfforts = []shared.ReasoningEffort{}
	}
	return catalog.AvailableModel{
		Code:             modelCode,
		Name:             name,
		ContextWindow:    contextWindow,
		MaxOutputTokens:  maxOutputTokens,
		MultiModal:       multiModal,
		Reasoning:        len(reasoningEfforts) > 0,
		ReasoningEfforts: reasoningEfforts,
	}, nil
}

func parseReasoningEfforts(values []string) []shared.ReasoningEffort {
	available := make(map[shared.ReasoningEffort]struct{}, len(values))
	for _, value := range values {
		effort := shared.ReasoningEffort(strings.ToLower(strings.TrimSpace(value)))
		if effort.Valid() && effort.Enabled() {
			available[effort] = struct{}{}
		}
	}
	result := make([]shared.ReasoningEffort, 0, len(available))
	for _, effort := range shared.StandardReasoningEfforts() {
		if _, ok := available[effort]; ok {
			result = append(result, effort)
		}
	}
	return result
}

func containsNonTextInput(modalities []string) bool {
	for _, modality := range modalities {
		if strings.TrimSpace(strings.ToLower(modality)) != "" && strings.ToLower(modality) != "text" {
			return true
		}
	}
	return false
}
