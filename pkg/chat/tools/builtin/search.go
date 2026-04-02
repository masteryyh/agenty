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

package builtin

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/services"
)

type SearchTool struct {
	knowledgeService *services.KnowledgeService
	webSearchService *services.WebSearchService
	evaluator        *services.SearchEvaluator
}

func (t *SearchTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name: "search",
		Description: `Unified multi-channel, multi-strategy search tool. Submit an array of search specs; each spec has a unique id, a channel, a query, and an optional per-spec limit. The same channel may appear multiple times with different queries to implement multi-strategy retrieval (e.g., keyword + semantic + HyDE in one call).

Available channels:
- "knowledge_base": Searches all knowledge base categories (llm_memory, session_memory, user_document) using hybrid vector + BM25 + keyword retrieval.
- "web_search": Searches the internet via the configured provider (Brave / Tavily / Firecrawl). Only available when a web search API key is configured in system settings.

Query format guidance per channel and strategy:
- knowledge_base + semantic (vector): Natural language question, e.g., "How did Google perform in Q3 2025?"
- knowledge_base + keyword (BM25): Refined keywords, e.g., "Google Q3 2025 revenue earnings net profit"
- knowledge_base + HyDE: After reviewing initial results, write a hypothetical passage that would answer the question (based on actual results, not imagined). Add this as a new entry with a distinct id on a second call.
- web_search: Search-engine-style query, e.g., "Google Q3 2025 annual report revenue"

Recommended workflow:
1. First call: Submit keyword and semantic queries to knowledge_base (different ids).
2. Review the returned quality and message per channel.
3. If quality is "medium" or "low", add a HyDE query in a second call.
4. Only fall back to web_search when knowledge_base quality is "low" or "no_results".

Results are returned as a JSON object grouped by channel. Each channel section includes results, the queries used, a quality rating (high/medium/low/no_results/error), and an improvement suggestion message.`,
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"searches": {
					Type:        "array",
					Description: `Array of search specs. Each element specifies one search operation.`,
					Items: &tools.ParameterProperty{
						Type: "object",
						Properties: map[string]tools.ParameterProperty{
							"id": {
								Type:        "string",
								Description: `Unique identifier for this search spec (e.g., "kb_kw", "kb_sem", "web1"). Returned with results to identify which spec produced each result.`,
							},
							"channel": {
								Type:        "string",
								Description: `Search channel: "knowledge_base" or "web_search".`,
							},
							"query": {
								Type:        "string",
								Description: `Search query string tailored to the channel and strategy (see tool description for format guidance).`,
							},
							"limit": {
								Type:        "integer",
								Description: `Maximum number of results to return for this spec. Defaults to 10.`,
							},
						},
						Required: []string{"id", "channel", "query"},
					},
				},
			},
			Required: []string{"searches"},
		},
	}
}

type searchSpec struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

type searchArgs struct {
	Searches []searchSpec `json:"searches"`
}

type kbResult struct {
	SearchID   string  `json:"search_id"`
	ItemID     string  `json:"item_id"`
	ItemTitle  string  `json:"item_title,omitempty"`
	Category   string  `json:"category"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

type webResult struct {
	SearchID string `json:"search_id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Content  string `json:"content"`
}

type channelResponse struct {
	Results     any               `json:"results"`
	QueriesUsed map[string]string `json:"queries_used"`
	Quality     string            `json:"quality"`
	Message     string            `json:"message"`
}

type searchResponse struct {
	KnowledgeBase  *channelResponse `json:"knowledge_base,omitempty"`
	WebSearch      *channelResponse `json:"web_search,omitempty"`
	OverallQuality string           `json:"overall_quality"`
	EvaluatorNotes string           `json:"evaluator_notes,omitempty"`
}

func (t *SearchTool) Execute(ctx context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if len(args.Searches) == 0 {
		return "", fmt.Errorf("searches array cannot be empty")
	}

	var kbSpecs, webSpecs []searchSpec
	for _, s := range args.Searches {
		if strings.TrimSpace(s.Query) == "" {
			return "", fmt.Errorf("search spec %q has empty query", s.ID)
		}
		if s.Limit <= 0 {
			s.Limit = 10
		}
		switch s.Channel {
		case "knowledge_base":
			kbSpecs = append(kbSpecs, s)
		case "web_search":
			webSpecs = append(webSpecs, s)
		default:
			return "", fmt.Errorf("unknown channel %q in spec %q", s.Channel, s.ID)
		}
	}

	resp := &searchResponse{}

	if len(kbSpecs) > 0 {
		resp.KnowledgeBase = t.executeKBSpecs(ctx, tcc, kbSpecs)
	}
	if len(webSpecs) > 0 {
		resp.WebSearch = t.executeWebSpecs(ctx, tcc, webSpecs)
	}

	resp.OverallQuality = overallQuality(resp.KnowledgeBase, resp.WebSearch)

	if t.evaluator != nil && strings.TrimSpace(tcc.ModelCode) != "" {
		notes := t.buildEvaluatorNotes(ctx, tcc.ModelCode, resp)
		resp.EvaluatorNotes = notes
	}

	out, err := json.MarshalString(resp)
	if err != nil {
		return "", fmt.Errorf("failed to serialize search response: %w", err)
	}
	return out, nil
}

func (t *SearchTool) executeKBSpecs(ctx context.Context, tcc tools.ToolCallContext, specs []searchSpec) *channelResponse {
	if t.knowledgeService == nil {
		return &channelResponse{
			Results:     []kbResult{},
			QueriesUsed: map[string]string{},
			Quality:     "error",
			Message:     "Knowledge base service is unavailable.",
		}
	}

	seen := make(map[string]struct{})
	var results []kbResult
	queriesUsed := make(map[string]string)

	for _, spec := range specs {
		queriesUsed[spec.ID] = spec.Query
		hits, err := t.knowledgeService.HybridSearch(ctx, tcc.AgentID, spec.Query, spec.Limit)
		if err != nil {
			continue
		}
		for _, h := range hits {
			dedupeKey := fmt.Sprintf("%s:%d", h.ItemID, h.ChunkIndex)
			if _, exists := seen[dedupeKey]; exists {
				continue
			}
			seen[dedupeKey] = struct{}{}
			results = append(results, kbResult{
				SearchID:   spec.ID,
				ItemID:     h.ItemID.String(),
				ItemTitle:  h.ItemTitle,
				Category:   string(h.Category),
				ChunkIndex: h.ChunkIndex,
				Content:    h.Content,
				Score:      h.Score,
			})
		}
	}

	quality, message := assessKBQuality(results)
	return &channelResponse{
		Results:     results,
		QueriesUsed: queriesUsed,
		Quality:     quality,
		Message:     message,
	}
}

func (t *SearchTool) executeWebSpecs(ctx context.Context, tcc tools.ToolCallContext, specs []searchSpec) *channelResponse {
	if t.webSearchService == nil {
		return &channelResponse{
			Results:     []webResult{},
			QueriesUsed: map[string]string{},
			Quality:     "error",
			Message:     "Web search is not configured. Please add a web search API key in system settings.",
		}
	}

	seen := make(map[string]struct{})
	var results []webResult
	queriesUsed := make(map[string]string)

	for _, spec := range specs {
		queriesUsed[spec.ID] = spec.Query
		hits, err := t.webSearchService.Search(ctx, spec.Query, spec.Limit)
		if err != nil {
			continue
		}
		for _, h := range hits {
			if _, exists := seen[h.URL]; exists {
				continue
			}
			seen[h.URL] = struct{}{}
			results = append(results, webResult{
				SearchID: spec.ID,
				Title:    h.Title,
				URL:      h.URL,
				Content:  h.Snippet,
			})
		}
	}

	quality, message := assessWebQuality(results)
	return &channelResponse{
		Results:     results,
		QueriesUsed: queriesUsed,
		Quality:     quality,
		Message:     message,
	}
}

func (t *SearchTool) buildEvaluatorNotes(ctx context.Context, modelCode string, resp *searchResponse) string {
	var parts []string

	if resp.KnowledgeBase != nil {
		if results, ok := resp.KnowledgeBase.Results.([]kbResult); ok && len(results) > 0 {
			var sb strings.Builder
			for i, r := range results {
				if i >= 5 {
					break
				}
				fmt.Fprintf(&sb, "[kb:%s] %s\n", r.Category, r.Content)
			}
			allQueries := collectQueries(resp.KnowledgeBase.QueriesUsed)
			eval, err := t.evaluator.Evaluate(ctx, modelCode, allQueries, sb.String())
			if err == nil && eval != nil {
				resp.KnowledgeBase.Quality = string(eval.Quality)
				if eval.Summary != "" {
					resp.KnowledgeBase.Message = eval.Summary
				}
				parts = append(parts, fmt.Sprintf("knowledge_base: %s", eval.Reasoning))
			}
		}
	}

	if resp.WebSearch != nil {
		if results, ok := resp.WebSearch.Results.([]webResult); ok && len(results) > 0 {
			var sb strings.Builder
			for i, r := range results {
				if i >= 5 {
					break
				}
				fmt.Fprintf(&sb, "[%s] %s\n%s\n", r.Title, r.URL, r.Content)
			}
			allQueries := collectQueries(resp.WebSearch.QueriesUsed)
			eval, err := t.evaluator.Evaluate(ctx, modelCode, allQueries, sb.String())
			if err == nil && eval != nil {
				resp.WebSearch.Quality = string(eval.Quality)
				if eval.Summary != "" {
					resp.WebSearch.Message = eval.Summary
				}
				parts = append(parts, fmt.Sprintf("web_search: %s", eval.Reasoning))
			}
		}
	}

	resp.OverallQuality = overallQuality(resp.KnowledgeBase, resp.WebSearch)
	return strings.Join(parts, "; ")
}

func collectQueries(m map[string]string) string {
	var parts []string
	for _, q := range m {
		parts = append(parts, q)
	}
	return strings.Join(parts, " | ")
}

func assessKBQuality(results []kbResult) (string, string) {
	switch {
	case len(results) == 0:
		return "no_results", "No matching results found in knowledge base. Consider: (1) trying different keywords or a more specific query, (2) adding relevant documents via /knowledge add, (3) using web_search channel."
	case len(results) < 3:
		return "medium", "Limited results found. Consider using additional query strategies (keyword + semantic + HyDE) or supplementing with web_search."
	default:
		return "high", ""
	}
}

func assessWebQuality(results []webResult) (string, string) {
	switch {
	case len(results) == 0:
		return "no_results", "No web results returned. The query may be too specific or the search provider may have rate-limited the request. Try rephrasing the query."
	case len(results) < 3:
		return "medium", "Few web results returned. Consider broadening the query."
	default:
		return "high", ""
	}
}

func overallQuality(kb, web *channelResponse) string {
	qualities := map[string]int{"high": 3, "medium": 2, "low": 1, "no_results": 0, "error": 0}
	best := -1
	if kb != nil {
		if v, ok := qualities[kb.Quality]; ok && v > best {
			best = v
		}
	}
	if web != nil {
		if v, ok := qualities[web.Quality]; ok && v > best {
			best = v
		}
	}
	switch best {
	case 3:
		return "high"
	case 2:
		return "medium"
	case 1:
		return "low"
	default:
		return "no_results"
	}
}
