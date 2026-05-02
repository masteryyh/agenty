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
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/utils"
	"gorm.io/gorm"
)

const (
	searchDefaultLimit            = 10
	searchRRFK                    = 60
	searchRerankCandidateLimit    = 30
	workspaceFileSnippetLineRange = 2
)

type SearchService struct {
	db               *gorm.DB
	knowledgeService *KnowledgeService
	webSearchService *WebSearchService
	providers        map[models.APIType]providers.Provider
}

var (
	searchService *SearchService
	searchOnce    sync.Once
)

func GetSearchService() *SearchService {
	searchOnce.Do(func() {
		searchService = &SearchService{
			db:               conn.GetDB(),
			knowledgeService: GetKnowledgeService(),
			webSearchService: GetWebSearchService(),
			providers: map[models.APIType]providers.Provider{
				models.APITypeOpenAI:       providers.NewOpenAIProvider(),
				models.APITypeOpenAILegacy: providers.NewOpenAILegacyProvider(),
				models.APITypeAnthropic:    providers.NewAnthropicProvider(),
				models.APITypeKimi:         providers.NewKimiProvider(),
				models.APITypeGemini:       providers.NewGeminiProvider(),
				models.APITypeBigModel:     providers.NewBigModelProvider(),
				models.APITypeQwen:         providers.NewQwenProvider(),
			},
		}
	})
	return searchService
}

func (s *SearchService) Search(ctx context.Context, sc models.SearchContext, req models.SearchRequest) (*models.SearchResponse, error) {
	if err := models.ValidateSearchRequest(req); err != nil {
		return nil, err
	}

	var kbSpecs, fileSpecs, webSpecs []models.SearchSpec
	for _, spec := range req.Searches {
		if spec.Limit <= 0 {
			spec.Limit = searchDefaultLimit
		}
		spec.Channel = strings.TrimSpace(spec.Channel)
		spec.Query = strings.TrimSpace(spec.Query)
		switch spec.Channel {
		case "knowledge_base":
			kbSpecs = append(kbSpecs, spec)
		case "workspace_files":
			fileSpecs = append(fileSpecs, spec)
		case "web_search":
			webSpecs = append(webSpecs, spec)
		default:
			continue
		}
	}

	resp := &models.SearchResponse{}
	var sets []models.SearchCandidateSet

	if len(kbSpecs) > 0 {
		ch, candidates := s.executeKBSpecs(ctx, sc, kbSpecs)
		resp.KnowledgeBase = ch
		sets = append(sets, candidates...)
	}
	if len(fileSpecs) > 0 {
		ch, candidates := s.executeFileSpecs(ctx, sc, fileSpecs)
		resp.WorkspaceFiles = ch
		sets = append(sets, candidates...)
	}
	if len(webSpecs) > 0 {
		ch, candidates := s.executeWebSpecs(ctx, webSpecs)
		resp.WebSearch = ch
		sets = append(sets, candidates...)
	}

	resp.RankedResults = globalWeightedRRF(sets, searchRerankCandidateLimit)
	if len(s.providers) > 0 && len(resp.RankedResults) > 1 {
		if reranked, notes := s.rerankCandidates(ctx, collectAllQueries(req.Searches), resp.RankedResults); len(reranked) > 0 {
			resp.RankedResults = reranked
			resp.EvaluatorNotes = notes
		}
	}
	applyGlobalOrder(resp)
	resp.OverallQuality = overallQuality(resp.KnowledgeBase, resp.WorkspaceFiles, resp.WebSearch)
	return resp, nil
}

func (s *SearchService) executeKBSpecs(ctx context.Context, sc models.SearchContext, specs []models.SearchSpec) (*models.SearchChannelResponse, []models.SearchCandidateSet) {
	if s.knowledgeService == nil {
		return &models.SearchChannelResponse{
			Results:     []models.KBSearchChannelResult{},
			QueriesUsed: map[string]string{},
			Quality:     "error",
			Message:     "Knowledge base service is unavailable.",
		}, nil
	}

	seen := make(map[string]struct{})
	var results []models.KBSearchChannelResult
	queriesUsed := make(map[string]string)
	var errors []string
	var sets []models.SearchCandidateSet

	for _, spec := range specs {
		queriesUsed[spec.ID] = spec.Query
		hits, err := s.knowledgeService.HybridSearch(ctx, sc.AgentID, spec.Query, spec.Limit)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", spec.ID, err))
			if len(hits) == 0 {
				continue
			}
		}
		set := models.SearchCandidateSet{Weight: 1.0}
		for i, h := range hits {
			id := fmt.Sprintf("kb:%s:%d", h.ItemID, h.ChunkIndex)
			candidate := models.RankedSearchResult{
				ID:         id,
				SearchID:   spec.ID,
				Channel:    "knowledge_base",
				Ranker:     "kb_hybrid",
				ItemID:     h.ItemID.String(),
				ItemTitle:  h.ItemTitle,
				Title:      h.ItemTitle,
				Category:   string(h.Category),
				ChunkIndex: h.ChunkIndex,
				Content:    h.Content,
				RawScore:   h.Score,
			}
			set.Candidates = append(set.Candidates, candidate)
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			results = append(results, models.KBSearchChannelResult{
				SearchID:   spec.ID,
				ItemID:     h.ItemID.String(),
				ItemTitle:  h.ItemTitle,
				Category:   string(h.Category),
				ChunkIndex: h.ChunkIndex,
				Content:    h.Content,
				Score:      h.Score,
			})
			_ = i
		}
		sets = append(sets, set)
	}

	quality, message := assessQuality(len(results), errors, "knowledge base", "Consider trying different keywords, using workspace_files for project files, or using web_search for external information.")
	return &models.SearchChannelResponse{Results: results, QueriesUsed: queriesUsed, Quality: quality, Message: message, Errors: errors}, sets
}

func (s *SearchService) executeWebSpecs(ctx context.Context, specs []models.SearchSpec) (*models.SearchChannelResponse, []models.SearchCandidateSet) {
	if s.webSearchService == nil {
		return &models.SearchChannelResponse{
			Results:     []models.WebSearchChannelResult{},
			QueriesUsed: map[string]string{},
			Quality:     "error",
			Message:     "Web search is not configured. Please add a web search API key in system settings.",
		}, nil
	}

	seen := make(map[string]struct{})
	var results []models.WebSearchChannelResult
	queriesUsed := make(map[string]string)
	var errors []string
	var sets []models.SearchCandidateSet

	for _, spec := range specs {
		queriesUsed[spec.ID] = spec.Query
		hits, err := s.webSearchService.Search(ctx, spec.Query, spec.Limit)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", spec.ID, err))
			continue
		}
		set := models.SearchCandidateSet{Weight: 0.9}
		for _, h := range hits {
			id := "web:" + h.URL
			candidate := models.RankedSearchResult{
				ID:       id,
				SearchID: spec.ID,
				Channel:  "web_search",
				Ranker:   "web_search",
				Title:    h.Title,
				URL:      h.URL,
				Content:  h.Snippet,
			}
			set.Candidates = append(set.Candidates, candidate)
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			results = append(results, models.WebSearchChannelResult{SearchID: spec.ID, Title: h.Title, URL: h.URL, Content: h.Snippet})
		}
		sets = append(sets, set)
	}

	quality, message := assessQuality(len(results), errors, "web", "Try broadening or rephrasing the web query.")
	return &models.SearchChannelResponse{Results: results, QueriesUsed: queriesUsed, Quality: quality, Message: message, Errors: errors}, sets
}

func (s *SearchService) executeFileSpecs(ctx context.Context, sc models.SearchContext, specs []models.SearchSpec) (*models.SearchChannelResponse, []models.SearchCandidateSet) {
	queriesUsed := make(map[string]string)
	for _, spec := range specs {
		queriesUsed[spec.ID] = spec.Query
	}
	if strings.TrimSpace(sc.Cwd) == "" {
		return &models.SearchChannelResponse{
			Results:     []models.WorkspaceFileSearchResult{},
			QueriesUsed: queriesUsed,
			Quality:     "error",
			Message:     "workspace_files requires the current session to have a cwd. Set cwd before using this channel.",
		}, nil
	}

	root, err := utils.GetCleanPath(sc.Cwd, true)
	if err != nil {
		return &models.SearchChannelResponse{Results: []models.WorkspaceFileSearchResult{}, QueriesUsed: queriesUsed, Quality: "error", Message: fmt.Sprintf("failed to resolve cwd: %v", err)}, nil
	}
	if err := validateSearchRoot(root); err != nil {
		return &models.SearchChannelResponse{Results: []models.WorkspaceFileSearchResult{}, QueriesUsed: queriesUsed, Quality: "error", Message: err.Error()}, nil
	}

	seen := make(map[string]struct{})
	var results []models.WorkspaceFileSearchResult
	var errors []string
	var sets []models.SearchCandidateSet

	for _, spec := range specs {
		hits, err := searchWorkspaceFiles(ctx, root, spec)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", spec.ID, err))
			continue
		}
		set := models.SearchCandidateSet{Weight: 1.1}
		for _, h := range hits {
			set.Candidates = append(set.Candidates, h)
			if _, exists := seen[h.ID]; exists {
				continue
			}
			seen[h.ID] = struct{}{}
			results = append(results, models.WorkspaceFileSearchResult{
				SearchID:     h.SearchID,
				Path:         h.Path,
				RelativePath: h.RelativePath,
				StartLine:    h.StartLine,
				EndLine:      h.EndLine,
				Content:      h.Content,
				Score:        h.RawScore,
			})
		}
		sets = append(sets, set)
	}

	quality, message := assessQuality(len(results), errors, "workspace files", "Try a more specific filename, symbol, or phrase from the project.")
	return &models.SearchChannelResponse{Results: results, QueriesUsed: queriesUsed, Quality: quality, Message: message, Errors: errors}, sets
}

func validateSearchRoot(path string) error {
	cleanPath := filepath.Clean(path)
	if _, ok := consts.BlockingPaths[cleanPath]; ok {
		return fmt.Errorf("path %s is not allowed because it may block the process", path)
	}

	lowerPath := strings.ToLower(cleanPath)
	for _, prefix := range consts.SensitiveFileToolPathPrefixes {
		if isSearchPathUnderPrefix(lowerPath, prefix) {
			return fmt.Errorf("path %s is a sensitive system path", path)
		}
	}
	return nil
}

func isSearchPathUnderPrefix(path, prefix string) bool {
	normalizedPath := strings.ReplaceAll(path, "\\", "/")
	normalizedPrefix := strings.ReplaceAll(prefix, "\\", "/")
	return normalizedPath == normalizedPrefix || strings.HasPrefix(normalizedPath, normalizedPrefix+"/")
}

func searchWorkspaceFiles(ctx context.Context, root string, spec models.SearchSpec) ([]models.RankedSearchResult, error) {
	terms := normalizedSearchTerms(spec.Query)
	if len(terms) == 0 {
		return nil, nil
	}

	var candidates []models.RankedSearchResult
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipWorkspaceDir(entry.Name()) || matchesAnyGlob(rel, spec.ExcludeGlobs) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		if len(spec.IncludeGlobs) > 0 && !matchesAnyGlob(rel, spec.IncludeGlobs) {
			return nil
		}
		if matchesAnyGlob(rel, spec.ExcludeGlobs) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() <= 0 || info.Size() > consts.WorkspaceSearchMaxFileSize {
			return nil
		}
		clean, err := filepath.EvalSymlinks(path)
		if err != nil || !isPathWithin(clean, root) {
			return nil
		}
		fileCandidates, err := scoreWorkspaceFile(spec, root, clean, rel, terms)
		if err != nil {
			return nil
		}
		candidates = append(candidates, fileCandidates...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].RawScore == candidates[j].RawScore {
			return candidates[i].RelativePath < candidates[j].RelativePath
		}
		return candidates[i].RawScore > candidates[j].RawScore
	})
	if len(candidates) > spec.Limit {
		candidates = candidates[:spec.Limit]
	}
	return candidates, nil
}

func scoreWorkspaceFile(spec models.SearchSpec, root, path, rel string, terms []string) ([]models.RankedSearchResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, nil
	}

	content := string(data)
	lowerRel := strings.ToLower(filepath.ToSlash(rel))
	lowerBase := strings.ToLower(filepath.Base(rel))
	phrase := strings.ToLower(strings.TrimSpace(spec.Query))
	pathScore := 0.0
	if strings.Contains(lowerRel, phrase) {
		pathScore += 8
	}
	if strings.Contains(lowerBase, phrase) {
		pathScore += 10
	}
	for _, term := range terms {
		if strings.Contains(lowerRel, term) {
			pathScore += 2
		}
		if strings.Contains(lowerBase, term) {
			pathScore += 3
		}
	}

	lines := strings.Split(content, "\n")
	var hits []models.RankedSearchResult
	usedWindows := make(map[string]struct{})
	for i, line := range lines {
		lowerLine := strings.ToLower(line)
		lineScore := 0.0
		if phrase != "" && strings.Contains(lowerLine, phrase) {
			lineScore += 10
		}
		for _, term := range terms {
			lineScore += float64(strings.Count(lowerLine, term))
		}
		if lineScore <= 0 {
			continue
		}
		lineScore += pathScore
		start := max(1, i+1-workspaceFileSnippetLineRange)
		end := min(len(lines), i+1+workspaceFileSnippetLineRange)
		windowKey := fmt.Sprintf("%d:%d", start, end)
		if _, exists := usedWindows[windowKey]; exists {
			continue
		}
		usedWindows[windowKey] = struct{}{}
		snippet := strings.Join(lines[start-1:end], "\n")
		id := fmt.Sprintf("file:%s:%d:%d", filepath.ToSlash(rel), start, end)
		hits = append(hits, models.RankedSearchResult{
			ID:           id,
			SearchID:     spec.ID,
			Channel:      "workspace_files",
			Ranker:       "file_content",
			Title:        rel,
			Path:         path,
			RelativePath: filepath.ToSlash(rel),
			StartLine:    start,
			EndLine:      end,
			Content:      snippet,
			RawScore:     lineScore,
		})
	}
	if len(hits) == 0 && pathScore > 0 {
		end := min(len(lines), 5)
		if end == 0 {
			return nil, nil
		}
		hits = append(hits, models.RankedSearchResult{
			ID:           fmt.Sprintf("file:%s:1:%d", filepath.ToSlash(rel), end),
			SearchID:     spec.ID,
			Channel:      "workspace_files",
			Ranker:       "file_name",
			Title:        rel,
			Path:         path,
			RelativePath: filepath.ToSlash(rel),
			StartLine:    1,
			EndLine:      end,
			Content:      strings.Join(lines[:end], "\n"),
			RawScore:     pathScore,
		})
	}
	return hits, nil
}

func normalizedSearchTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	seen := make(map[string]struct{})
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, " \t\n\r\"'`.,:;()[]{}<>")
		if len(f) < 2 {
			continue
		}
		if _, exists := seen[f]; exists {
			continue
		}
		seen[f] = struct{}{}
		terms = append(terms, f)
	}
	return terms
}

func shouldSkipWorkspaceDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".idea", ".vscode", "node_modules", "vendor", "dist", "build", "target", "tmp", "temp", ".cache":
		return true
	default:
		return false
	}
}

func matchesAnyGlob(rel string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		matched, err := filepath.Match(pattern, rel)
		if err == nil && matched {
			return true
		}
		matched, err = filepath.Match(pattern, base)
		if err == nil && matched {
			return true
		}
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/")+"/") {
			return true
		}
	}
	return false
}

func isPathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}

func globalWeightedRRF(sets []models.SearchCandidateSet, limit int) []models.RankedSearchResult {
	scores := make(map[string]float64)
	items := make(map[string]models.RankedSearchResult)
	for _, set := range sets {
		for i, candidate := range set.Candidates {
			rank := i + 1
			scores[candidate.ID] += set.Weight / float64(searchRRFK+rank)
			if existing, exists := items[candidate.ID]; !exists || candidate.RawScore > existing.RawScore {
				items[candidate.ID] = candidate
			}
		}
	}
	result := make([]models.RankedSearchResult, 0, len(scores))
	for id, score := range scores {
		candidate := items[id]
		candidate.FusedScore = score
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].FusedScore == result[j].FusedScore {
			return result[i].ID < result[j].ID
		}
		return result[i].FusedScore > result[j].FusedScore
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (s *SearchService) rerankCandidates(ctx context.Context, query string, candidates []models.RankedSearchResult) ([]models.RankedSearchResult, string) {
	if len(candidates) == 0 {
		return nil, ""
	}
	input := make([]models.SearchRerankCandidate, 0, len(candidates))
	candidateMap := make(map[string]models.RankedSearchResult, len(candidates))
	for _, c := range candidates {
		input = append(input, models.SearchRerankCandidate{
			ID:       c.ID,
			Channel:  c.Channel,
			Title:    utils.FirstNonEmpty(c.Title, c.ItemTitle, c.RelativePath),
			Source:   utils.FirstNonEmpty(c.URL, c.RelativePath, c.ItemID),
			Content:  utils.Truncate(c.Content, 1200),
			RRFScore: c.FusedScore,
			RawScore: c.RawScore,
		})
		candidateMap[c.ID] = c
	}
	reranked, err := s.fusionRerank(ctx, query, input, len(candidates))
	if err != nil {
		slog.WarnContext(ctx, "search fusion rerank failed", "error", err)
		return nil, ""
	}
	ordered := make([]models.RankedSearchResult, 0, len(reranked.OrderedIDs))
	seen := make(map[string]struct{})
	for _, id := range reranked.OrderedIDs {
		candidate, ok := candidateMap[id]
		if !ok {
			continue
		}
		candidate.RerankScore = reranked.Scores[id]
		ordered = append(ordered, candidate)
		seen[id] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, exists := seen[candidate.ID]; !exists {
			ordered = append(ordered, candidate)
		}
	}
	return ordered, reranked.Summary
}

func (s *SearchService) fusionRerank(ctx context.Context, query string, candidates []models.SearchRerankCandidate, limit int) (*models.SearchFusionRerankResult, error) {
	if len(candidates) == 0 {
		return &models.SearchFusionRerankResult{Scores: map[string]float64{}}, nil
	}
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}

	model, provider, err := s.resolveLightModel(ctx)
	if err != nil {
		return nil, err
	}

	candidatesJSON, err := json.MarshalString(candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank candidates: %w", err)
	}

	p := s.providerFor(provider.Type)
	resp, err := p.Chat(ctx, &providers.ChatRequest{
		Model:    model.Code,
		Messages: []providers.Message{{Role: models.RoleUser, Content: fmt.Sprintf(consts.SearchFusionRerankPrompt, query, candidatesJSON)}},
		BaseURL:  provider.BaseURL,
		APIKey:   provider.APIKey,
		APIType:  provider.Type,
		ResponseFormat: &providers.ResponseFormat{
			Type: "json_object",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM rerank failed: %w", err)
	}

	var result models.SearchFusionRerankResult
	if err := decodeJSONResponse(resp.Content, &result); err != nil {
		return nil, fmt.Errorf("failed to parse rerank JSON: %w", err)
	}
	if result.Scores == nil {
		result.Scores = map[string]float64{}
	}
	if len(result.OrderedIDs) > limit {
		result.OrderedIDs = result.OrderedIDs[:limit]
	}
	return &result, nil
}

func (s *SearchService) providerFor(apiType models.APIType) providers.Provider {
	if p, ok := s.providers[apiType]; ok {
		return p
	}
	return s.providers[models.APITypeOpenAI]
}

func (s *SearchService) resolveLightModel(ctx context.Context) (*models.Model, *models.ModelProvider, error) {
	model, err := gorm.G[models.Model](s.db).
		Where("light = true AND deleted_at IS NULL").
		Order("created_at ASC").
		First(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("no light model found: %w", err)
	}

	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil || provider.APIKey == "" {
		return nil, nil, fmt.Errorf("light model provider not configured: %w", err)
	}
	return &model, &provider, nil
}

func decodeJSONResponse[T any](content string, out *T) error {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	return json.UnmarshalString(content, out)
}

func applyGlobalOrder(resp *models.SearchResponse) {
	order := make(map[string]int, len(resp.RankedResults))
	for i, r := range resp.RankedResults {
		order[r.ID] = i
	}
	if resp.KnowledgeBase != nil {
		if results, ok := resp.KnowledgeBase.Results.([]models.KBSearchChannelResult); ok {
			sort.SliceStable(results, func(i, j int) bool {
				return searchOrder(order, fmt.Sprintf("kb:%s:%d", results[i].ItemID, results[i].ChunkIndex)) < searchOrder(order, fmt.Sprintf("kb:%s:%d", results[j].ItemID, results[j].ChunkIndex))
			})
			resp.KnowledgeBase.Results = results
		}
	}
	if resp.WorkspaceFiles != nil {
		if results, ok := resp.WorkspaceFiles.Results.([]models.WorkspaceFileSearchResult); ok {
			sort.SliceStable(results, func(i, j int) bool {
				left := fmt.Sprintf("file:%s:%d:%d", results[i].RelativePath, results[i].StartLine, results[i].EndLine)
				right := fmt.Sprintf("file:%s:%d:%d", results[j].RelativePath, results[j].StartLine, results[j].EndLine)
				return searchOrder(order, left) < searchOrder(order, right)
			})
			resp.WorkspaceFiles.Results = results
		}
	}
	if resp.WebSearch != nil {
		if results, ok := resp.WebSearch.Results.([]models.WebSearchChannelResult); ok {
			sort.SliceStable(results, func(i, j int) bool {
				return searchOrder(order, "web:"+results[i].URL) < searchOrder(order, "web:"+results[j].URL)
			})
			resp.WebSearch.Results = results
		}
	}
}

func searchOrder(order map[string]int, id string) int {
	if v, ok := order[id]; ok {
		return v
	}
	return len(order) + 1
}

func collectAllQueries(specs []models.SearchSpec) string {
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		parts = append(parts, spec.Query)
	}
	return strings.Join(parts, " | ")
}

func assessQuality(count int, errors []string, channel, suggestion string) (string, string) {
	if count == 0 && len(errors) > 0 {
		return "error", fmt.Sprintf("No %s results returned because searches failed.", channel)
	}
	if count == 0 {
		return "no_results", fmt.Sprintf("No matching results found in %s. %s", channel, suggestion)
	}
	if count < 3 {
		return "medium", fmt.Sprintf("Limited %s results found. %s", channel, suggestion)
	}
	if len(errors) > 0 {
		return "medium", fmt.Sprintf("Some %s searches failed, but usable results were found.", channel)
	}
	return "high", ""
}

func overallQuality(channels ...*models.SearchChannelResponse) string {
	qualities := map[string]int{"high": 3, "medium": 2, "low": 1, "no_results": 0, "error": 0}
	best := -1
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		if v, ok := qualities[ch.Quality]; ok && v > best {
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
