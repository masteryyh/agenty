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
)

type Memory struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	AgentID   uuid.UUID       `gorm:"type:uuid;not null"`
	Content   string          `gorm:"type:text;not null"`
	Embedding pgvector.Vector `gorm:"type:vector(1536);not null"`
	Migrated  bool            `gorm:"default:true;not null"`
	CreatedAt time.Time       `gorm:"autoCreateTime:milli"`
	UpdatedAt time.Time       `gorm:"autoUpdateTime:milli"`
	DeletedAt *time.Time
}

func (Memory) TableName() string {
	return "memories"
}

func (m *Memory) ToDto() *MemoryDto {
	return &MemoryDto{
		ID:        m.ID,
		AgentID:   m.AgentID,
		Content:   m.Content,
		Migrated:  m.Migrated,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

type MemoryDto struct {
	ID        uuid.UUID `json:"id"`
	AgentID   uuid.UUID `json:"agentId"`
	Content   string    `json:"content"`
	Migrated  bool      `json:"migrated"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type MemorySearchResult struct {
	Memory *MemoryDto `json:"memory"`
	Score  float64    `json:"score"`
}
