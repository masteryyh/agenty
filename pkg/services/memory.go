package services

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const rrfK = 60

type MemoryService struct {
	db               *gorm.DB
	embeddingService *EmbeddingService
}

var (
	memoryService *MemoryService
	memoryOnce    sync.Once
)

func GetMemoryService() *MemoryService {
	memoryOnce.Do(func() {
		memoryService = &MemoryService{
			db:               conn.GetDB(),
			embeddingService: GetEmbeddingService(),
		}
	})
	return memoryService
}

func (s *MemoryService) SaveMemory(ctx context.Context, content string) (*models.MemoryDto, error) {
	embedding, err := s.embeddingService.Embed(ctx, content)
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
	embedding, err := s.embeddingService.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	var memories []models.Memory
	queryVec := pgvector.NewVector(embedding)
	err = s.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Clauses(clause.OrderBy{
			Expression: clause.Expr{SQL: "embedding <-> ?", Vars: []interface{}{queryVec}},
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
		Where("deleted_at IS NULL AND to_tsvector('english', content) @@ to_tsquery('english', ?)", tsQuery).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "ts_rank(to_tsvector('english', content), to_tsquery('english', ?)) DESC",
				Vars: []interface{}{tsQuery},
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
		return "%" + strings.ToLower(w) + "%"
	})

	tx := s.db.WithContext(ctx).Where("deleted_at IS NULL")
	orConditions := s.db.Where("LOWER(content) LIKE ?", patterns[0])
	for _, p := range patterns[1:] {
		orConditions = orConditions.Or("LOWER(content) LIKE ?", p)
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
