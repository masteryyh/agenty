/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
)

const (
	defaultTavilyExtractURL       = "https://api.tavily.com/extract"
	defaultBraveLLMContextURL     = "https://api.search.brave.com/res/v1/llm/context"
	defaultFirecrawlBaseURL       = "https://api.firecrawl.dev"
	defaultWebFetchMaxContentSize = 512 * 1024
)

type WebFetchResponse struct {
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type WebFetchService struct {
	httpClient       *http.Client
	systemService    *SystemService
	tavilyExtractURL string
	braveContextURL  string
	firecrawlBaseURL string
	maxContentSize   int64
}

var (
	webFetchService *WebFetchService
	webFetchOnce    sync.Once
)

func GetWebFetchService() *WebFetchService {
	webFetchOnce.Do(func() {
		webFetchService = &WebFetchService{
			httpClient:       conn.GetHTTPClient(),
			systemService:    GetSystemService(),
			tavilyExtractURL: defaultTavilyExtractURL,
			braveContextURL:  defaultBraveLLMContextURL,
			firecrawlBaseURL: defaultFirecrawlBaseURL,
			maxContentSize:   defaultWebFetchMaxContentSize,
		}
	})
	return webFetchService
}

func (s *WebFetchService) Fetch(ctx context.Context, rawURL string) (*WebFetchResponse, error) {
	targetURL, err := normalizeFetchURL(rawURL)
	if err != nil {
		return nil, err
	}

	systemService := s.systemService
	if systemService == nil {
		systemService = GetSystemService()
	}
	settings, err := systemService.getOrCreate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system settings: %w", err)
	}
	return s.fetchWithSettings(ctx, settings, targetURL)
}

func (s *WebFetchService) fetchWithSettings(ctx context.Context, settings *models.SystemSettings, targetURL string) (*WebFetchResponse, error) {
	switch s.selectProvider(settings) {
	case models.WebSearchProviderTavily:
		return s.fetchTavily(ctx, settings.TavilyAPIKey, targetURL)
	case models.WebSearchProviderFirecrawl:
		return s.fetchFirecrawl(ctx, settings.FirecrawlAPIKey, settings.FirecrawlBaseURL, targetURL)
	case models.WebSearchProviderBrave:
		return s.fetchBrave(ctx, settings.BraveAPIKey, targetURL)
	default:
		return s.fetchDirect(ctx, targetURL)
	}
}

func (s *WebFetchService) selectProvider(settings *models.SystemSettings) models.WebSearchProvider {
	if settings == nil {
		return ""
	}
	switch settings.WebSearchProvider {
	case models.WebSearchProviderTavily:
		return models.WebSearchProviderTavily
	case models.WebSearchProviderFirecrawl:
		return models.WebSearchProviderFirecrawl
	case models.WebSearchProviderBrave:
		return models.WebSearchProviderBrave
	default:
		return ""
	}
}

type tavilyExtractResponse struct {
	Results       []tavilyExtractResult       `json:"results"`
	FailedResults []tavilyExtractFailedResult `json:"failed_results"`
}

type tavilyExtractResult struct {
	URL        string `json:"url"`
	RawContent string `json:"raw_content"`
}

type tavilyExtractFailedResult struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}

func (s *WebFetchService) fetchTavily(ctx context.Context, apiKey, targetURL string) (*WebFetchResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Tavily API key is not configured")
	}

	endpoint := s.tavilyExtractURL
	if endpoint == "" {
		endpoint = defaultTavilyExtractURL
	}
	resp, err := webFetchPostJSON[tavilyExtractResponse](ctx, s.httpClient, endpoint, map[string]string{
		"Authorization": "Bearer " + apiKey,
	}, map[string]any{
		"urls":          targetURL,
		"format":        "markdown",
		"extract_depth": "basic",
	})
	if err != nil {
		return nil, fmt.Errorf("Tavily API request failed: %w", err)
	}
	if len(resp.Results) == 0 {
		if len(resp.FailedResults) > 0 && resp.FailedResults[0].Error != "" {
			return nil, fmt.Errorf("Tavily failed to fetch URL: %s", resp.FailedResults[0].Error)
		}
		return nil, fmt.Errorf("Tavily returned no content")
	}

	result := resp.Results[0]
	return &WebFetchResponse{
		URL:         firstNonEmpty(result.URL, targetURL),
		ContentType: "markdown",
		Content:     result.RawContent,
	}, nil
}

type firecrawlScrapeResponse struct {
	Success bool                `json:"success"`
	Data    firecrawlScrapeData `json:"data"`
	Error   string              `json:"error"`
}

type firecrawlScrapeData struct {
	Markdown string                  `json:"markdown"`
	HTML     string                  `json:"html"`
	RawHTML  string                  `json:"rawHtml"`
	Metadata firecrawlScrapeMetadata `json:"metadata"`
	Warning  string                  `json:"warning"`
}

type firecrawlScrapeMetadata struct {
	Title       string `json:"title"`
	SourceURL   string `json:"sourceURL"`
	URL         string `json:"url"`
	StatusCode  int    `json:"statusCode"`
	ContentType string `json:"contentType"`
}

func (s *WebFetchService) fetchFirecrawl(ctx context.Context, apiKey, baseURL, targetURL string) (*WebFetchResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Firecrawl API key is not configured")
	}

	resp, err := webFetchPostJSON[firecrawlScrapeResponse](ctx, s.httpClient, firecrawlScrapeEndpoint(s.firecrawlBaseURL, baseURL), map[string]string{
		"Authorization": "Bearer " + apiKey,
	}, map[string]any{
		"url":             targetURL,
		"formats":         []string{"markdown", "html"},
		"onlyMainContent": true,
	})
	if err != nil {
		return nil, fmt.Errorf("Firecrawl API request failed: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("Firecrawl failed to fetch URL: %s", firstNonEmpty(resp.Error, resp.Data.Warning, "unknown error"))
	}

	contentType := "markdown"
	content := resp.Data.Markdown
	if content == "" {
		contentType = "html"
		content = firstNonEmpty(resp.Data.HTML, resp.Data.RawHTML)
	}
	if content == "" {
		return nil, fmt.Errorf("Firecrawl returned no content")
	}

	return &WebFetchResponse{
		URL:         firstNonEmpty(resp.Data.Metadata.SourceURL, resp.Data.Metadata.URL, targetURL),
		ContentType: contentType,
		Title:       resp.Data.Metadata.Title,
		Content:     content,
		StatusCode:  resp.Data.Metadata.StatusCode,
	}, nil
}

func firecrawlScrapeEndpoint(defaultBaseURL, configuredBaseURL string) string {
	baseURL := configuredBaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if baseURL == "" {
		baseURL = defaultFirecrawlBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v2") {
		return baseURL + "/scrape"
	}
	return baseURL + "/v2/scrape"
}

type braveContextResponse struct {
	Grounding braveGrounding         `json:"grounding"`
	Sources   map[string]braveSource `json:"sources"`
}

type braveGrounding struct {
	Generic []braveContextItem `json:"generic"`
}

type braveContextItem struct {
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Snippets []string `json:"snippets"`
}

type braveSource struct {
	Title    string   `json:"title"`
	Hostname string   `json:"hostname"`
	Age      []string `json:"age"`
}

func (s *WebFetchService) fetchBrave(ctx context.Context, apiKey, targetURL string) (*WebFetchResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Brave API key is not configured")
	}

	endpoint := s.braveContextURL
	if endpoint == "" {
		endpoint = defaultBraveLLMContextURL
	}
	resp, err := webFetchGetJSON[braveContextResponse](ctx, s.httpClient, endpoint, map[string]string{
		"q":                                targetURL,
		"count":                            "5",
		"maximum_number_of_urls":           "1",
		"maximum_number_of_tokens":         "8192",
		"maximum_number_of_tokens_per_url": "8192",
		"context_threshold_mode":           "disabled",
	}, map[string]string{
		"Accept":               "application/json",
		"X-Subscription-Token": apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("Brave API request failed: %w", err)
	}
	if len(resp.Grounding.Generic) == 0 {
		return nil, fmt.Errorf("Brave returned no content")
	}

	item := selectBraveContextItem(resp.Grounding.Generic, targetURL)
	content := strings.Join(item.Snippets, "\n\n")
	if content == "" {
		return nil, fmt.Errorf("Brave returned no snippets")
	}
	title := item.Title
	if source, ok := resp.Sources[item.URL]; ok && title == "" {
		title = source.Title
	}
	return &WebFetchResponse{
		URL:         firstNonEmpty(item.URL, targetURL),
		ContentType: "snippets",
		Title:       title,
		Content:     content,
	}, nil
}

func selectBraveContextItem(items []braveContextItem, targetURL string) braveContextItem {
	normalizedTarget := strings.TrimRight(targetURL, "/")
	for _, item := range items {
		if strings.TrimRight(item.URL, "/") == normalizedTarget {
			return item
		}
	}
	return items[0]
}

func (s *WebFetchService) fetchDirect(ctx context.Context, targetURL string) (*WebFetchResponse, error) {
	httpClient := s.httpClient
	if httpClient == nil {
		httpClient = conn.GetHTTPClient()
	}
	maxContentSize := s.maxContentSize
	if maxContentSize <= 0 {
		maxContentSize = defaultWebFetchMaxContentSize
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("direct URL request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxContentSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	truncated := int64(len(body)) > maxContentSize
	if truncated {
		body = body[:maxContentSize]
	}
	return &WebFetchResponse{
		URL:         resp.Request.URL.String(),
		ContentType: firstNonEmpty(resp.Header.Get("Content-Type"), "text/html"),
		Content:     strings.ToValidUTF8(string(body), "\uFFFD"),
		StatusCode:  resp.StatusCode,
		Truncated:   truncated,
	}, nil
}

func webFetchPostJSON[T any](ctx context.Context, httpClient *http.Client, rawURL string, headers map[string]string, body any) (T, error) {
	return webFetchDoJSON[T](ctx, httpClient, http.MethodPost, rawURL, nil, headers, body)
}

func webFetchGetJSON[T any](ctx context.Context, httpClient *http.Client, rawURL string, params map[string]string, headers map[string]string) (T, error) {
	return webFetchDoJSON[T](ctx, httpClient, http.MethodGet, rawURL, params, headers, nil)
}

func webFetchDoJSON[T any](ctx context.Context, httpClient *http.Client, method, rawURL string, params map[string]string, headers map[string]string, body any) (T, error) {
	var zero T
	if httpClient == nil {
		httpClient = conn.GetHTTPClient()
	}

	requestURL, err := buildFetchRequestURL(rawURL, params)
	if err != nil {
		return zero, err
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return zero, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return zero, fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result T
	if err := json.Unmarshal(respBody, &result); err != nil {
		return zero, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func buildFetchRequestURL(rawURL string, params map[string]string) (string, error) {
	if len(params) == 0 {
		return rawURL, nil
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	q := parsedURL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	parsedURL.RawQuery = q.Encode()
	return parsedURL.String(), nil
}

func normalizeFetchURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("url cannot be empty")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("url must use http or https scheme")
	}
	if parsedURL.Host == "" {
		return "", fmt.Errorf("url host cannot be empty")
	}
	return parsedURL.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
