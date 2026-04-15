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
	"strings"
	"sync"

	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
)

type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type WebSearchService struct{}

var (
	webSearchService *WebSearchService
	webSearchOnce    sync.Once
)

func GetWebSearchService() *WebSearchService {
	webSearchOnce.Do(func() {
		webSearchService = &WebSearchService{}
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

type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
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

	tavilyResp, err := conn.Post[tavilyResponse](ctx, conn.HTTPRequest{
		URL:     "https://api.tavily.com/search",
		Headers: map[string]string{"Authorization": "Bearer " + apiKey},
		Body:    reqBody,
	})
	if err != nil {
		return nil, fmt.Errorf("Tavily API request failed: %w", err)
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

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type braveWeb struct {
	Results []braveResult `json:"results"`
}

type braveResponse struct {
	Web braveWeb `json:"web"`
}

func (s *WebSearchService) searchBrave(ctx context.Context, apiKey, query string, limit int) ([]WebSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Brave API key is not configured")
	}
	if limit <= 0 {
		limit = 5
	}

	braveResp, err := conn.Get[braveResponse](ctx, conn.HTTPRequest{
		URL:    "https://api.search.brave.com/res/v1/web/search",
		Params: map[string]string{"q": query, "count": fmt.Sprintf("%d", limit)},
		Headers: map[string]string{
			"Accept":               "application/json",
			"X-Subscription-Token": apiKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Brave API request failed: %w", err)
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

type firecrawlItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

type firecrawlResponse struct {
	Data []firecrawlItem `json:"data"`
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

	endpoint := strings.TrimRight(baseURL, "/") + "/v1/search"
	reqBody := map[string]any{
		"query": query,
		"limit": limit,
	}

	fcResp, err := conn.Post[firecrawlResponse](ctx, conn.HTTPRequest{
		URL:     endpoint,
		Headers: map[string]string{"Authorization": "Bearer " + apiKey},
		Body:    reqBody,
	})
	if err != nil {
		return nil, fmt.Errorf("Firecrawl API request failed: %w", err)
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
