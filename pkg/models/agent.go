package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/db"
)

type AgentDto struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Soul      string    `json:"soul"`
	IsDefault bool      `json:"isDefault"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func AgentRowToDto(row db.Agent) *AgentDto {
	return &AgentDto{
		ID:        row.ID,
		Name:      row.Name,
		Soul:      row.Soul,
		IsDefault: row.IsDefault,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

type CreateAgentDto struct {
	Name      string  `json:"name" binding:"required"`
	Soul      *string `json:"soul" binding:"omitempty"`
	IsDefault bool    `json:"isDefault"`
}

type UpdateAgentDto struct {
	Name      *string `json:"name" binding:"omitempty"`
	Soul      *string `json:"soul" binding:"omitempty"`
	IsDefault *bool   `json:"isDefault" binding:"omitempty"`
}
