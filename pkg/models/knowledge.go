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

package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/datatypes"
)

type KnowledgeCategory string

const (
	KnowledgeCategoryUserDocument  KnowledgeCategory = "user_document"
	KnowledgeCategorySessionMemory KnowledgeCategory = "session_memory"
	KnowledgeCategoryLLMMemory     KnowledgeCategory = "llm_memory"
)

type KnowledgeContentType string

const (
	KnowledgeContentTypeText KnowledgeContentType = "text"
)

type KnowledgeItem struct {
	ID              uuid.UUID            `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	AgentID         uuid.UUID            `gorm:"type:uuid;not null;index"`
	Category        KnowledgeCategory    `gorm:"type:varchar(50);not null;index"`
	ContentType     KnowledgeContentType `gorm:"type:varchar(50);not null;default:'text'"`
	Title           string               `gorm:"type:varchar(500)"`
	Content         string               `gorm:"type:text;not null"`
	Language        string               `gorm:"type:varchar(20)"`
	Metadata        datatypes.JSON       `gorm:"type:jsonb"`
	SourceSessionID *uuid.UUID           `gorm:"type:uuid;uniqueIndex"`
	CreatedAt       time.Time            `gorm:"autoCreateTime:milli"`
	UpdatedAt       time.Time            `gorm:"autoUpdateTime:milli"`
	DeletedAt       *time.Time
}

func (KnowledgeItem) TableName() string {
	return "knowledge_items"
}

func (k *KnowledgeItem) ToDto() *KnowledgeItemDto {
	return &KnowledgeItemDto{
		ID:              k.ID,
		AgentID:         k.AgentID,
		Category:        k.Category,
		ContentType:     k.ContentType,
		Title:           k.Title,
		Content:         k.Content,
		Language:        k.Language,
		Metadata:        k.Metadata,
		SourceSessionID: k.SourceSessionID,
		CreatedAt:       k.CreatedAt,
		UpdatedAt:       k.UpdatedAt,
	}
}

func (k *KnowledgeItem) ToSummaryDto() *KnowledgeItemSummaryDto {
	contentPreview := k.Content
	if len(contentPreview) > 200 {
		contentPreview = contentPreview[:200] + "..."
	}
	return &KnowledgeItemSummaryDto{
		ID:          k.ID,
		AgentID:     k.AgentID,
		Category:    k.Category,
		ContentType: k.ContentType,
		Title:       k.Title,
		Preview:     contentPreview,
		CreatedAt:   k.CreatedAt,
	}
}

type KnowledgeItemDto struct {
	ID              uuid.UUID            `json:"id"`
	AgentID         uuid.UUID            `json:"agentId"`
	Category        KnowledgeCategory    `json:"category"`
	ContentType     KnowledgeContentType `json:"contentType"`
	Title           string               `json:"title,omitempty"`
	Content         string               `json:"content"`
	Language        string               `json:"language,omitempty"`
	Metadata        datatypes.JSON       `json:"metadata,omitempty"`
	SourceSessionID *uuid.UUID           `json:"sourceSessionId,omitempty"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

type KnowledgeItemSummaryDto struct {
	ID          uuid.UUID            `json:"id"`
	AgentID     uuid.UUID            `json:"agentId"`
	Category    KnowledgeCategory    `json:"category"`
	ContentType KnowledgeContentType `json:"contentType"`
	Title       string               `json:"title,omitempty"`
	Preview     string               `json:"preview"`
	CreatedAt   time.Time            `json:"createdAt"`
}

type CreateKnowledgeItemDto struct {
	Category    KnowledgeCategory `json:"category" binding:"required"`
	ContentType string            `json:"contentType"`
	Title       string            `json:"title"`
	Content     string            `json:"content" binding:"required"`
	Language    string            `json:"language"`
}

type KnowledgeBaseData struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	ItemID        uuid.UUID       `gorm:"type:uuid;not null;index"`
	AgentID       uuid.UUID       `gorm:"type:uuid;not null;index"`
	ChunkIndex    int             `gorm:"not null;default:0"`
	ChunkContent  string          `gorm:"type:text;not null"`
	TextEmbedding pgvector.Vector `gorm:"type:vector(1024)"`
	CreatedAt     time.Time       `gorm:"autoCreateTime:milli"`
	UpdatedAt     time.Time       `gorm:"autoUpdateTime:milli"`
}

func (KnowledgeBaseData) TableName() string {
	return "kb_data"
}

type KBSearchResult struct {
	ItemID     uuid.UUID         `json:"itemId"`
	ItemTitle  string            `json:"itemTitle,omitempty"`
	Category   KnowledgeCategory `json:"category"`
	ChunkIndex int               `json:"chunkIndex"`
	Content    string            `json:"content"`
	Score      float64           `json:"score"`
}

type KBSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}
