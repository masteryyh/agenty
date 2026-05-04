package models

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID        uuid.UUID
	Name      string
	Soul      string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
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
	AgentID   uuid.UUID
	ModelID   uuid.UUID
	SortOrder int
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
