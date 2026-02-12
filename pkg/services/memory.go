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
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const rrfK = 60

type MemoryService struct {
	db  *gorm.DB
	cfg *config.EmbeddingConfig
}

var (
	memoryService *MemoryService
	memoryOnce    sync.Once
)

func GetMemoryService() *MemoryService {
	memoryOnce.Do(func() {
		memoryService = &MemoryService{
			db:  conn.GetDB(),
			cfg: config.GetConfigManager().GetConfig().Embedding,
		}
	})
	return memoryService
}

func (s *MemoryService) IsEnabled() bool {
	return s.cfg != nil && s.cfg.APIKey != ""
}

func (s *MemoryService) embed(ctx context.Context, text string) ([]float32, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("embedding service is not configured")
	}

	client := conn.GetOpenAIClient(s.cfg.BaseURL, s.cfg.APIKey)
	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: s.cfg.Model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		Dimensions: param.NewOpt(int64(s.cfg.Dimensions)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	vec := lo.Map(resp.Data[0].Embedding, func(v float64, _ int) float32 {
		return float32(v)
	})
	return normalizeVector(vec), nil
}

func normalizeVector(vec []float32) []float32 {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	return lo.Map(vec, func(v float32, _ int) float32 {
		return float32(float64(v) / norm)
	})
}

func (s *MemoryService) SaveMemory(ctx context.Context, content string) (*models.MemoryDto, error) {
	embedding, err := s.embed(ctx, content)
	if err != nil {
		slog.ErrorContext(ctx, "failed to embed memory content", "error", err)
		return nil, fmt.Errorf("failed to embed content: %w", err)
	}

	memory := &models.Memory{
		Content:   content,
		Embedding: pgvector.NewVector(embedding),
	}

	if err := s.db.WithContext(ctx).Create(memory).Error; err != nil {
		slog.ErrorContext(ctx, "failed to save memory", "error", err)
		return nil, fmt.Errorf("failed to save memory: %w", err)
	}

	return memory.ToDto(), nil
}

func (s *MemoryService) SearchMemory(ctx context.Context, query string, limit int) ([]models.MemorySearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	searchLimit := limit * 3

	vectorResults, err := s.vectorSearch(ctx, query, searchLimit)
	if err != nil {
		slog.ErrorContext(ctx, "vector search failed", "error", err)
	}

	fullTextResults, err := s.fullTextSearch(ctx, query, searchLimit)
	if err != nil {
		slog.ErrorContext(ctx, "full text search failed", "error", err)
	}

	keywordResults, err := s.keywordSearch(ctx, query, searchLimit)
	if err != nil {
		slog.ErrorContext(ctx, "keyword search failed", "error", err)
	}

	merged := rrfMerge(limit, vectorResults, fullTextResults, keywordResults)
	return merged, nil
}

func (s *MemoryService) vectorSearch(ctx context.Context, query string, limit int) ([]rankedItem, error) {
	embedding, err := s.embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	var memories []models.Memory
	queryVec := pgvector.NewVector(embedding)
	err = s.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Clauses(clause.OrderBy{
			Expression: clause.Expr{SQL: "embedding <=> ?", Vars: []any{queryVec}},
		}).
		Limit(limit).
		Find(&memories).Error
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	return lo.Map(memories, func(m models.Memory, i int) rankedItem {
		return rankedItem{id: m.ID, memory: m.ToDto(), rank: i + 1}
	}), nil
}

func (s *MemoryService) fullTextSearch(ctx context.Context, query string, limit int) ([]rankedItem, error) {
	tsQuery := strings.Join(strings.Fields(query), " | ")

	var memories []models.Memory
	err := s.db.WithContext(ctx).
		Where("deleted_at IS NULL AND to_tsvector('simple', content) @@ to_tsquery('simple', ?)", tsQuery).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "ts_rank(to_tsvector('simple', content), to_tsquery('simple', ?)) DESC",
				Vars: []any{tsQuery},
			},
		}).
		Limit(limit).
		Find(&memories).Error
	if err != nil {
		return nil, fmt.Errorf("full text search failed: %w", err)
	}

	return lo.Map(memories, func(m models.Memory, i int) rankedItem {
		return rankedItem{id: m.ID, memory: m.ToDto(), rank: i + 1}
	}), nil
}

func (s *MemoryService) keywordSearch(ctx context.Context, query string, limit int) ([]rankedItem, error) {
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	patterns := lo.Map(words, func(w string, _ int) string {
		return "%" + w + "%"
	})

	tx := s.db.WithContext(ctx).Where("deleted_at IS NULL")
	orConditions := s.db.Where("content ILIKE ?", patterns[0])
	for _, p := range patterns[1:] {
		orConditions = orConditions.Or("content ILIKE ?", p)
	}
	tx = tx.Where(orConditions)

	var memories []models.Memory
	if err := tx.Limit(limit).Find(&memories).Error; err != nil {
		return nil, fmt.Errorf("keyword search failed: %w", err)
	}

	return lo.Map(memories, func(m models.Memory, i int) rankedItem {
		return rankedItem{id: m.ID, memory: m.ToDto(), rank: i + 1}
	}), nil
}

type rankedItem struct {
	id     uuid.UUID
	memory *models.MemoryDto
	rank   int
}

func rrfMerge(limit int, resultSets ...[]rankedItem) []models.MemorySearchResult {
	scores := make(map[uuid.UUID]float64)
	items := make(map[uuid.UUID]*models.MemoryDto)

	for _, results := range resultSets {
		for _, item := range results {
			scores[item.id] += 1.0 / float64(rrfK+item.rank)
			if _, exists := items[item.id]; !exists {
				items[item.id] = item.memory
			}
		}
	}

	type scored struct {
		id    uuid.UUID
		score float64
	}

	sorted := lo.Map(lo.Keys(scores), func(id uuid.UUID, _ int) scored {
		return scored{id: id, score: scores[id]}
	})
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	return lo.Map(sorted, func(s scored, _ int) models.MemorySearchResult {
		return models.MemorySearchResult{
			Memory: items[s.id],
			Score:  s.score,
		}
	})
}
