package models

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name      string    `gorm:"type:varchar(255);not null"`
	Soul      string    `gorm:"type:text;not null"`
	IsDefault bool      `gorm:"type:boolean;default:false"`
	CreatedAt time.Time `gorm:"autoCreateTime:milli"`
	UpdatedAt time.Time `gorm:"autoUpdateTime:milli"`
	DeletedAt *time.Time
}

func (Agent) TableName() string {
	return "agents"
}

func (a *Agent) ToDto() *AgentDto {
	return &AgentDto{
		ID:        a.ID,
		Name:      a.Name,
		Soul:      a.Soul,
		IsDefault: a.IsDefault,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

type AgentModel struct {
	AgentID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	ModelID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	SortOrder int       `gorm:"not null;default:0"`
}

func (AgentModel) TableName() string {
	return "agent_models"
}

type AgentDto struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Soul      string     `json:"soul"`
	IsDefault bool       `json:"isDefault"`
	Models    []ModelDto `json:"models,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type CreateAgentDto struct {
	Name      string      `json:"name" binding:"required"`
	Soul      *string     `json:"soul" binding:"omitempty"`
	IsDefault bool        `json:"isDefault"`
	ModelIDs  []uuid.UUID `json:"modelIds" binding:"omitempty"`
}

type UpdateAgentDto struct {
	Name      *string      `json:"name" binding:"omitempty"`
	Soul      *string      `json:"soul" binding:"omitempty"`
	IsDefault *bool        `json:"isDefault" binding:"omitempty"`
	ModelIDs  *[]uuid.UUID `json:"modelIds" binding:"omitempty"`
}
