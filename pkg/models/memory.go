package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

type Memory struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Content   string          `gorm:"type:text;not null"`
	Embedding pgvector.Vector `gorm:"type:vector(1536)"`
	CreatedAt time.Time       `gorm:"autoCreateTime:milli"`
	UpdatedAt time.Time       `gorm:"autoUpdateTime:milli"`
	DeletedAt *time.Time
}

func (Memory) TableName() string {
	return "memories"
}

type MemoryDto struct {
	ID        uuid.UUID `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (m *Memory) ToDto() *MemoryDto {
	return &MemoryDto{
		ID:        m.ID,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

type MemorySearchResult struct {
	Memory *MemoryDto `json:"memory"`
	Score  float64    `json:"score"`
}
