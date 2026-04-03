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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/models"
)

type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type WebSearchService struct {
	client *http.Client
}

var (
	webSearchService *WebSearchService
	webSearchOnce    sync.Once
)

func GetWebSearchService() *WebSearchService {
	webSearchOnce.Do(func() {
		webSearchService = &WebSearchService{
			client: &http.Client{Timeout: 30 * time.Second},
		}
	})
	return webSearchService
}

func (s *WebSearchService) Search(ctx context.Context, query string, limit int) ([]WebSearchResult, error) {
	settings, err := GetSystemService().getOrCreate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system settings: %w", err)
	}

	switch settings.WebSearchProvider {
	case models.WebSearchProviderTavily:
		return s.searchTavily(ctx, settings.TavilyAPIKey, query, limit)
	case models.WebSearchProviderBrave:
		return s.searchBrave(ctx, settings.BraveAPIKey, query, limit)
	case models.WebSearchProviderFirecrawl:
		return s.searchFirecrawl(ctx, settings.FirecrawlAPIKey, settings.FirecrawlBaseURL, query, limit)
	case models.WebSearchProviderDisabled, "":
		return nil, fmt.Errorf("web search is disabled")
	default:
		return nil, fmt.Errorf("unsupported web search provider: %s", settings.WebSearchProvider)
	}
}

func (s *WebSearchService) IsEnabled(ctx context.Context) bool {
	settings, err := GetSystemService().getOrCreate(ctx)
	if err != nil {
		return false
	}
	return settings.WebSearchProvider != models.WebSearchProviderDisabled && settings.WebSearchProvider != ""
}

func (s *WebSearchService) searchTavily(ctx context.Context, apiKey, query string, limit int) ([]WebSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Tavily API key is not configured")
	}
	if limit <= 0 {
		limit = 5
	}

	reqBody := map[string]any{
		"query":       query,
		"max_results": limit,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Tavily API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Tavily response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tavily API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var tavilyResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &tavilyResp); err != nil {
		return nil, fmt.Errorf("failed to parse Tavily response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(tavilyResp.Results))
	for _, r := range tavilyResp.Results {
		results = append(results, WebSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}
	return results, nil
}

func (s *WebSearchService) searchBrave(ctx context.Context, apiKey, query string, limit int) ([]WebSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Brave API key is not configured")
	}
	if limit <= 0 {
		limit = 5
	}

	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d", url.QueryEscape(query), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Brave API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Brave response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Brave API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(respBody, &braveResp); err != nil {
		return nil, fmt.Errorf("failed to parse Brave response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, WebSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}
	return results, nil
}

func (s *WebSearchService) searchFirecrawl(ctx context.Context, apiKey, baseURL, query string, limit int) ([]WebSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Firecrawl API key is not configured")
	}
	if baseURL == "" {
		baseURL = "https://api.firecrawl.dev"
	}
	if limit <= 0 {
		limit = 5
	}

	reqBody := map[string]any{
		"query": query,
		"limit": limit,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/v1/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Firecrawl API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Firecrawl response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Firecrawl API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var fcResp struct {
		Data []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Markdown    string `json:"markdown"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &fcResp); err != nil {
		return nil, fmt.Errorf("failed to parse Firecrawl response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(fcResp.Data))
	for _, r := range fcResp.Data {
		snippet := r.Description
		if snippet == "" && len(r.Markdown) > 500 {
			snippet = r.Markdown[:500]
		} else if snippet == "" {
			snippet = r.Markdown
		}
		results = append(results, WebSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		})
	}
	return results, nil
}
