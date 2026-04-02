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
	"sort"
	"strings"
	"sync"

	"github.com/allisson/go-pglock/v3"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/signal"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	rrfK = 60
)

type MemoryService struct {
	db *gorm.DB
}

var (
	memoryService *MemoryService
	memoryOnce    sync.Once
)

func GetMemoryService() *MemoryService {
	memoryOnce.Do(func() {
		memoryService = &MemoryService{
			db: conn.GetDB(),
		}
	})
	return memoryService
}

func (s *MemoryService) IsEnabled(ctx context.Context) bool {
	return GetEmbeddingService().IsEnabled(ctx)
}

func (s *MemoryService) embed(ctx context.Context, text string) ([]float32, error) {
	return GetEmbeddingService().Embed(ctx, text)
}

const reEmbedBatchSize = 100

func (s *MemoryService) ReEmbedAll(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql DB: %w", err)
	}
	locker, err := pglock.NewLock(ctx, 2026, sqlDB)
	if err != nil {
		return fmt.Errorf("failed to create lock: %w", err)
	}
	locked, err := locker.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return customerrors.ErrEmbeddingMigrating
	}
	defer func() {
		if err := locker.Unlock(context.Background()); err != nil {
			slog.ErrorContext(ctx, "failed to release lock", "error", err)
		}
	}()

	settings, err := GetSystemService().getOrCreate(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system settings: %w", err)
	}
	if settings.EmbeddingModelID == nil {
		return nil
	}
	if settings.EmbeddingMigrating {
		slog.WarnContext(ctx, "EmbeddingMigrating was true on lock acquisition, treating as stale flag from previous failed migration")
	}

	var memoryCount int64
	if err := s.db.WithContext(ctx).Model(&models.Memory{}).Where("deleted_at IS NULL").Count(&memoryCount).Error; err != nil {
		return fmt.Errorf("failed to count memories: %w", err)
	}
	if memoryCount == 0 {
		return nil
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := GetSystemService().setEmbeddingMigrating(ctx, tx, true); err != nil {
			return fmt.Errorf("failed to set migrating flag: %w", err)
		}

		_, err = gorm.G[models.Memory](tx).
			Where("deleted_at IS NULL").
			Update(ctx, "migrated", false)
		if err != nil {
			return fmt.Errorf("failed to reset migrated flag: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to initialize migration: %w", err)
	}

	defer func() {
		if err := GetSystemService().setEmbeddingMigrating(signal.GetBaseContext(), nil, false); err != nil {
			slog.ErrorContext(ctx, "failed to clear migrating flag", "error", err)
		}
	}()

	client, modelCode, err := GetEmbeddingService().GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get embedding client: %w", err)
	}

	for {
		batch, err := gorm.G[models.Memory](s.db).
			Where("deleted_at IS NULL AND migrated IS FALSE").
			Order("id").
			Limit(reEmbedBatchSize).
			Find(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch memories for re-embedding: %w", err)
		}
		if len(batch) == 0 {
			break
		}

		texts := lo.Map(batch, func(m models.Memory, _ int) string { return m.Content })
		embeddings, batchErr := GetEmbeddingService().embedBatchWithClient(ctx, client, modelCode, texts)
		if batchErr != nil {
			return fmt.Errorf("failed to embed batch: %w", batchErr)
		}

		for i := range batch {
			vec := make([]float32, consts.DefaultVectorDimension)
			if i < len(embeddings) && embeddings[i] != nil {
				copy(vec, embeddings[i])
			}
			batch[i].Embedding = pgvector.NewVector(vec)
			batch[i].Migrated = true
		}

		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Save(&batch).Error
		}); err != nil {
			return fmt.Errorf("failed to re-embed batch: %w", err)
		}
	}
	return nil
}

func (s *MemoryService) SaveMemory(ctx context.Context, agentID uuid.UUID, content string) (*models.MemoryDto, error) {
	embedding, err := s.embed(ctx, content)
	if err != nil {
		slog.ErrorContext(ctx, "failed to embed memory content", "error", err)
		return nil, fmt.Errorf("failed to embed content: %w", err)
	}

	memory := &models.Memory{
		AgentID:   agentID,
		Content:   content,
		Embedding: pgvector.NewVector(embedding),
	}

	if err := gorm.G[models.Memory](s.db).Create(ctx, memory); err != nil {
		slog.ErrorContext(ctx, "failed to save memory", "error", err)
		return nil, err
	}
	return memory.ToDto(), nil
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

	var memories []models.Memory
	err = s.db.WithContext(ctx).
		Where("deleted_at IS NULL AND agent_id = ? AND migrated = TRUE", agentID).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{SQL: "embedding <=> ?", Vars: []any{pgvector.NewVector(embedding)}},
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

func (s *MemoryService) fullTextSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]rankedItem, error) {
	tsQuery := strings.Join(strings.Fields(query), " | ")

	var memories []models.Memory
	err := s.db.WithContext(ctx).
		Where("deleted_at IS NULL AND agent_id = ? AND migrated = TRUE AND to_tsvector('simple', content) @@ plainto_tsquery('simple', ?)", agentID, tsQuery).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "ts_rank(to_tsvector('simple', content), plainto_tsquery('simple', ?)) DESC",
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

func (s *MemoryService) keywordSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]rankedItem, error) {
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	patterns := lo.Map(words, func(w string, _ int) string {
		return "%" + w + "%"
	})

	tx := s.db.WithContext(ctx).Where("deleted_at IS NULL AND agent_id = ? AND migrated = TRUE", agentID)
	orConditions := s.db.Where("content ILIKE ?", patterns[0])
	for _, p := range patterns[1:] {
		orConditions = orConditions.Or("content ILIKE ?", p)
	}
	tx = tx.Where(orConditions).Order("created_at DESC")

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
