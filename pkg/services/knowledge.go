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

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/chunk"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/openai/openai-go/v3"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	kbEmbedBatchSize = 50
	kbSearchLimit    = 20
	kbRRFK           = 60
)

type KnowledgeService struct {
	db *gorm.DB
}

var (
	knowledgeService *KnowledgeService
	knowledgeOnce    sync.Once
)

func GetKnowledgeService() *KnowledgeService {
	knowledgeOnce.Do(func() {
		knowledgeService = &KnowledgeService{db: conn.GetDB()}
	})
	return knowledgeService
}

func (s *KnowledgeService) CreateItem(ctx context.Context, agentID uuid.UUID, dto *models.CreateKnowledgeItemDto) (*models.KnowledgeItemDto, error) {
	if dto.Content == "" {
		return nil, customerrors.ErrKnowledgeContentEmpty
	}
	if dto.Category == "" {
		dto.Category = models.KnowledgeCategoryUserDocument
	}

	contentType := models.KnowledgeContentTypeText
	if dto.ContentType != "" {
		contentType = models.KnowledgeContentType(dto.ContentType)
	}

	item := &models.KnowledgeItem{
		AgentID:     agentID,
		Category:    dto.Category,
		ContentType: contentType,
		Title:       dto.Title,
		Content:     dto.Content,
		Language:    dto.Language,
	}

	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("failed to create knowledge item: %w", err)
	}

	safe.GoSafe("chunk-and-embed", func(ctx context.Context) {
		if err := s.chunkAndEmbed(ctx, item); err != nil {
			slog.Error("failed to chunk and embed knowledge item", "itemId", item.ID, "error", err)
		}
	})

	return item.ToDto(), nil
}

func (s *KnowledgeService) CreateItemSync(ctx context.Context, agentID uuid.UUID, dto *models.CreateKnowledgeItemDto) (*models.KnowledgeItemDto, error) {
	if dto.Content == "" {
		return nil, customerrors.ErrKnowledgeContentEmpty
	}
	if dto.Category == "" {
		dto.Category = models.KnowledgeCategoryUserDocument
	}

	contentType := models.KnowledgeContentTypeText
	if dto.ContentType != "" {
		contentType = models.KnowledgeContentType(dto.ContentType)
	}

	item := &models.KnowledgeItem{
		AgentID:     agentID,
		Category:    dto.Category,
		ContentType: contentType,
		Title:       dto.Title,
		Content:     dto.Content,
		Language:    dto.Language,
	}

	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("failed to create knowledge item: %w", err)
	}

	if err := s.chunkAndEmbed(ctx, item); err != nil {
		return nil, fmt.Errorf("failed to chunk and embed knowledge item: %w", err)
	}

	return item.ToDto(), nil
}

func (s *KnowledgeService) CreateSessionMemory(ctx context.Context, agentID uuid.UUID, sessionID uuid.UUID, title, content string) (*models.KnowledgeItemDto, error) {
	item := &models.KnowledgeItem{
		AgentID:         agentID,
		Category:        models.KnowledgeCategorySessionMemory,
		ContentType:     models.KnowledgeContentTypeText,
		Title:           title,
		Content:         content,
		SourceSessionID: &sessionID,
	}

	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("failed to create session memory: %w", err)
	}

	if err := s.chunkAndEmbed(ctx, item); err != nil {
		slog.ErrorContext(ctx, "failed to chunk and embed session memory", "itemId", item.ID, "error", err)
	}

	return item.ToDto(), nil
}

func (s *KnowledgeService) GetItem(ctx context.Context, agentID, itemID uuid.UUID) (*models.KnowledgeItemDto, error) {
	item, err := gorm.G[models.KnowledgeItem](s.db).
		Where("id = ? AND agent_id = ? AND deleted_at IS NULL", itemID, agentID).
		First(ctx)
	if err != nil {
		return nil, customerrors.ErrKnowledgeItemNotFound
	}
	return item.ToDto(), nil
}

func (s *KnowledgeService) ListItems(ctx context.Context, agentID uuid.UUID, category *models.KnowledgeCategory) ([]models.KnowledgeItemSummaryDto, error) {
	tx := s.db.WithContext(ctx).Model(&models.KnowledgeItem{}).Where("agent_id = ? AND deleted_at IS NULL", agentID)
	if category != nil {
		tx = tx.Where("category = ?", *category)
	}
	tx = tx.Order("created_at DESC")

	var items []models.KnowledgeItem
	if err := tx.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to list knowledge items: %w", err)
	}

	return lo.Map(items, func(item models.KnowledgeItem, _ int) models.KnowledgeItemSummaryDto {
		return *item.ToSummaryDto()
	}), nil
}

func (s *KnowledgeService) DeleteItem(ctx context.Context, agentID, itemID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND agent_id = ? AND deleted_at IS NULL", itemID, agentID).
			Delete(&models.KnowledgeItem{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete knowledge item: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return customerrors.ErrKnowledgeItemNotFound
		}

		if err := tx.Where("item_id = ?", itemID).Delete(&models.KnowledgeBaseData{}).Error; err != nil {
			return fmt.Errorf("failed to delete knowledge chunks: %w", err)
		}
		return nil
	})
}

func (s *KnowledgeService) HybridSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]models.KBSearchResult, error) {
	if limit <= 0 {
		limit = kbSearchLimit
	}

	candidateLimit := limit * 3

	var (
		vectorResults   []kbRankedItem
		fullTextResults []kbRankedItem
		keywordResults  []kbRankedItem
		vectorErr       error
		fullTextErr     error
		keywordErr      error
		wg              sync.WaitGroup
	)

	agentIDStr := agentID.String()
	embeddingEnabled := GetEmbeddingService().IsEnabled(ctx)

	goroutineCount := 2
	if embeddingEnabled {
		goroutineCount = 3
	}
	wg.Add(goroutineCount)

	if embeddingEnabled {
		safe.GoOnce("vector-search-"+agentIDStr, func() {
			defer wg.Done()
			vectorResults, vectorErr = s.vectorSearch(ctx, agentID, query, candidateLimit)
		})
	}
	safe.GoOnce("full-text-search-"+agentIDStr, func() {
		defer wg.Done()
		fullTextResults, fullTextErr = s.fullTextSearch(ctx, agentID, query, candidateLimit)
	})
	safe.GoOnce("keyword-search-"+agentIDStr, func() {
		defer wg.Done()
		keywordResults, keywordErr = s.keywordSearch(ctx, agentID, query, candidateLimit)
	})
	wg.Wait()

	if vectorErr != nil {
		slog.ErrorContext(ctx, "kb vector search failed", "error", vectorErr)
	}
	if fullTextErr != nil {
		slog.ErrorContext(ctx, "kb full text search failed", "error", fullTextErr)
	}
	if keywordErr != nil {
		slog.ErrorContext(ctx, "kb keyword search failed", "error", keywordErr)
	}

	merged := kbRRFMerge(limit, vectorResults, fullTextResults, keywordResults)
	return merged, nil
}

func (s *KnowledgeService) chunkAndEmbed(ctx context.Context, item *models.KnowledgeItem) error {
	chunks := chunk.SplitText(item.Content, consts.KnowledgeChunkSize, consts.KnowledgeChunkOverlap)
	if len(chunks) == 0 {
		return nil
	}

	embeddingSvc := GetEmbeddingService()
	embeddingEnabled := embeddingSvc.IsEnabled(ctx)

	var client *openai.Client
	var modelCode string
	if embeddingEnabled {
		var err error
		client, modelCode, err = embeddingSvc.GetClient(ctx)
		if err != nil {
			slog.WarnContext(ctx, "failed to get embedding client, chunks will be saved without embeddings", "error", err)
			embeddingEnabled = false
		}
	}

	var allData []models.KnowledgeBaseData
	for i := 0; i < len(chunks); i += kbEmbedBatchSize {
		end := min(i+kbEmbedBatchSize, len(chunks))
		batch := chunks[i:end]

		var embeddings [][]float32
		if embeddingEnabled {
			var err error
			embeddings, err = embeddingSvc.embedBatchWithClient(ctx, client, modelCode, batch)
			if err != nil {
				return fmt.Errorf("failed to embed chunk batch: %w", err)
			}
		}

		for j, chunkText := range batch {
			vec := make([]float32, consts.DefaultVectorDimension)
			if embeddingEnabled && j < len(embeddings) && embeddings[j] != nil {
				copy(vec, embeddings[j])
			}
			allData = append(allData, models.KnowledgeBaseData{
				ItemID:        item.ID,
				AgentID:       item.AgentID,
				ChunkIndex:    i + j,
				ChunkContent:  chunkText,
				TextEmbedding: pgvector.NewVector(vec),
			})
		}
	}

	if len(allData) > 0 {
		if err := s.db.WithContext(ctx).CreateInBatches(&allData, 100).Error; err != nil {
			return fmt.Errorf("failed to save knowledge chunks: %w", err)
		}
	}
	return nil
}

type kbRankedItem struct {
	chunkID    uuid.UUID
	itemID     uuid.UUID
	itemTitle  string
	category   models.KnowledgeCategory
	chunkIndex int
	content    string
	rank       int
}

func (s *KnowledgeService) vectorSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]kbRankedItem, error) {
	embedding, err := GetEmbeddingService().Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	type chunkWithItem struct {
		models.KnowledgeBaseData
		ItemTitle string                   `gorm:"column:item_title"`
		Category  models.KnowledgeCategory `gorm:"column:category"`
	}

	var results []chunkWithItem
	err = s.db.WithContext(ctx).
		Table("kb_data").
		Select("kb_data.*, knowledge_items.title AS item_title, knowledge_items.category AS category").
		Joins("JOIN knowledge_items ON knowledge_items.id = kb_data.item_id AND knowledge_items.deleted_at IS NULL").
		Where("kb_data.agent_id = ?", agentID).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{SQL: "kb_data.text_embedding <#> ?", Vars: []any{pgvector.NewVector(embedding)}},
		}).
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("kb vector search failed: %w", err)
	}

	return lo.Map(results, func(r chunkWithItem, i int) kbRankedItem {
		return kbRankedItem{
			chunkID:    r.ID,
			itemID:     r.ItemID,
			itemTitle:  r.ItemTitle,
			category:   r.Category,
			chunkIndex: r.ChunkIndex,
			content:    r.ChunkContent,
			rank:       i + 1,
		}
	}), nil
}

func (s *KnowledgeService) fullTextSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]kbRankedItem, error) {
	tsQuery := strings.Join(strings.Fields(query), " | ")

	type chunkWithItem struct {
		models.KnowledgeBaseData
		ItemTitle string                   `gorm:"column:item_title"`
		Category  models.KnowledgeCategory `gorm:"column:category"`
	}

	var results []chunkWithItem
	err := s.db.WithContext(ctx).
		Table("kb_data").
		Select("kb_data.*, knowledge_items.title AS item_title, knowledge_items.category AS category").
		Joins("JOIN knowledge_items ON knowledge_items.id = kb_data.item_id AND knowledge_items.deleted_at IS NULL").
		Where("kb_data.agent_id = ? AND to_tsvector('simple', kb_data.chunk_content) @@ plainto_tsquery('simple', ?)", agentID, tsQuery).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "ts_rank(to_tsvector('simple', kb_data.chunk_content), plainto_tsquery('simple', ?)) DESC",
				Vars: []any{tsQuery},
			},
		}).
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("kb full text search failed: %w", err)
	}

	return lo.Map(results, func(r chunkWithItem, i int) kbRankedItem {
		return kbRankedItem{
			chunkID:    r.ID,
			itemID:     r.ItemID,
			itemTitle:  r.ItemTitle,
			category:   r.Category,
			chunkIndex: r.ChunkIndex,
			content:    r.ChunkContent,
			rank:       i + 1,
		}
	}), nil
}

func (s *KnowledgeService) keywordSearch(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]kbRankedItem, error) {
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	patterns := lo.Map(words, func(w string, _ int) string {
		return "%" + w + "%"
	})

	type chunkWithItem struct {
		models.KnowledgeBaseData
		ItemTitle string                   `gorm:"column:item_title"`
		Category  models.KnowledgeCategory `gorm:"column:category"`
	}

	tx := s.db.WithContext(ctx).
		Table("kb_data").
		Select("kb_data.*, knowledge_items.title AS item_title, knowledge_items.category AS category").
		Joins("JOIN knowledge_items ON knowledge_items.id = kb_data.item_id AND knowledge_items.deleted_at IS NULL").
		Where("kb_data.agent_id = ?", agentID)

	orConditions := s.db.Where("kb_data.chunk_content ILIKE ?", patterns[0])
	for _, p := range patterns[1:] {
		orConditions = orConditions.Or("kb_data.chunk_content ILIKE ?", p)
	}
	tx = tx.Where(orConditions).Order("kb_data.created_at DESC")

	var results []chunkWithItem
	if err := tx.Limit(limit).Find(&results).Error; err != nil {
		return nil, fmt.Errorf("kb keyword search failed: %w", err)
	}

	return lo.Map(results, func(r chunkWithItem, i int) kbRankedItem {
		return kbRankedItem{
			chunkID:    r.ID,
			itemID:     r.ItemID,
			itemTitle:  r.ItemTitle,
			category:   r.Category,
			chunkIndex: r.ChunkIndex,
			content:    r.ChunkContent,
			rank:       i + 1,
		}
	}), nil
}

func kbRRFMerge(limit int, resultSets ...[]kbRankedItem) []models.KBSearchResult {
	scores := make(map[uuid.UUID]float64)
	items := make(map[uuid.UUID]*kbRankedItem)

	for _, results := range resultSets {
		for _, item := range results {
			scores[item.chunkID] += 1.0 / float64(kbRRFK+item.rank)
			if _, exists := items[item.chunkID]; !exists {
				copied := item
				items[item.chunkID] = &copied
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

	return lo.Map(sorted, func(sc scored, _ int) models.KBSearchResult {
		item := items[sc.id]
		return models.KBSearchResult{
			ItemID:     item.itemID,
			ItemTitle:  item.itemTitle,
			Category:   item.category,
			ChunkIndex: item.chunkIndex,
			Content:    item.content,
			Score:      sc.score,
		}
	})
}
