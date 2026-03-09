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
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/db"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/lo"
)

const (
	rrfK = 60
)

type MemoryService struct {
	sqlDB   *sql.DB
	queries *db.Queries
	cfg     *config.EmbeddingConfig
}

var (
	memoryService *MemoryService
	memoryOnce    sync.Once
)

func GetMemoryService() *MemoryService {
	memoryOnce.Do(func() {
		sqlDB := conn.GetSQLDB()
		memoryService = &MemoryService{
			sqlDB:   sqlDB,
			queries: db.New(sqlDB),
			cfg:     config.GetConfigManager().GetConfig().Embedding,
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
		Dimensions: param.NewOpt(int64(1536)),
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

func (s *MemoryService) SaveMemory(ctx context.Context, agentID uuid.UUID, content string) (*models.MemoryDto, error) {
	embedding, err := s.embed(ctx, content)
	if err != nil {
		slog.ErrorContext(ctx, "failed to embed memory content", "error", err)
		return nil, fmt.Errorf("failed to embed content: %w", err)
	}

	row, err := s.queries.CreateMemory(ctx, db.CreateMemoryParams{
		AgentID:   agentID,
		Content:   content,
		Embedding: pgvector.NewVector(embedding),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save memory", "error", err)
		return nil, fmt.Errorf("failed to save memory: %w", err)
	}

	return models.MemoryRowToDto(row), nil
}

func (s *MemoryService) SearchMemory(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]models.MemorySearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	searchLimit := limit * 3

	vectorResults, err := s.vectorSearch(ctx, agentID, query, searchLimit)
	if err != nil {
		slog.ErrorContext(ctx, "vector search failed", "error", err)
	}

	fullTextResults, err := s.fullTextSearch(ctx, agentID, query, searchLimit)
	if err != nil {
		slog.ErrorContext(ctx, "full text search failed", "error", err)
	}

	keywordResults, err := s.keywordSearch(ctx, agentID, query, searchLimit)
	if err != nil {
		slog.ErrorContext(ctx, "keyword search failed", "error", err)
	}

	merged := rrfMerge(limit, vectorResults, fullTextResults, keywordResults)
	return merged, nil
}

func (s *MemoryService) vectorSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]rankedItem, error) {
	embedding, err := s.embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	rows, err := s.queries.VectorSearchMemories(ctx, db.VectorSearchMemoriesParams{
		AgentID:   agentID,
		Embedding: pgvector.NewVector(embedding),
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	return lo.Map(rows, func(r db.VectorSearchMemoriesRow, i int) rankedItem {
		return rankedItem{
			id:     r.ID,
			memory: &models.MemoryDto{ID: r.ID, AgentID: r.AgentID, Content: r.Content, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt},
			rank:   i + 1,
		}
	}), nil
}

func (s *MemoryService) fullTextSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]rankedItem, error) {
	tsQuery := strings.Join(strings.Fields(query), " | ")

	rows, err := s.queries.FullTextSearchMemories(ctx, db.FullTextSearchMemoriesParams{
		AgentID:        agentID,
		PlaintoTsquery: tsQuery,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("full text search failed: %w", err)
	}

	return lo.Map(rows, func(r db.FullTextSearchMemoriesRow, i int) rankedItem {
		return rankedItem{
			id:     r.ID,
			memory: &models.MemoryDto{ID: r.ID, AgentID: r.AgentID, Content: r.Content, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt},
			rank:   i + 1,
		}
	}), nil
}

func (s *MemoryService) keywordSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]rankedItem, error) {
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	patterns := lo.Map(words, func(w string, _ int) string {
		return "%" + w + "%"
	})

	var q strings.Builder
	q.WriteString(`SELECT id, agent_id, content, created_at, updated_at FROM memories WHERE deleted_at IS NULL AND agent_id = $1 AND (`)
	args := []any{agentID}
	for i, p := range patterns {
		if i > 0 {
			q.WriteString(" OR ")
		}
		fmt.Fprintf(&q, "content ILIKE $%d", i+2)
		args = append(args, p)
	}
	fmt.Fprintf(&q, ") LIMIT $%d ORDER BY created_at DESC", len(args)+1)
	args = append(args, limit)

	rows, err := s.sqlDB.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("keyword search failed: %w", err)
	}
	defer rows.Close()

	var items []rankedItem
	rank := 1
	for rows.Next() {
		var m models.MemoryDto
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Content, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, rankedItem{id: m.ID, memory: &m, rank: rank})
		rank++
	}
	return items, rows.Err()
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
