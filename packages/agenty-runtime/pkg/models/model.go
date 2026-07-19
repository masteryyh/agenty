package models

import (
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Model struct {
	ID                        uuid.UUID
	ProviderID                uuid.UUID
	Name                      string
	Code                      string
	DefaultModel              bool
	EmbeddingModel            bool
	ContextCompressionModel   bool
	MultiModal                bool
	Light                     bool
	Thinking                  bool
	ThinkingLevels            datatypes.JSON
	AnthropicAdaptiveThinking bool
	IsPreset                  bool
	ContextWindow             int
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	DeletedAt                 *time.Time
}

func (Model) TableName() string {
	return "models"
}

func (m *Model) ToDto(provider *ModelProviderDto) *ModelDto {
	var thinkingLevels []string
	if err := json.Unmarshal(m.ThinkingLevels, &thinkingLevels); err != nil {
		thinkingLevels = []string{}
	}

	dto := &ModelDto{
		ID:                        m.ID,
		Name:                      m.Name,
		Code:                      m.Code,
		DefaultModel:              m.DefaultModel,
		EmbeddingModel:            m.EmbeddingModel,
		ContextCompressionModel:   m.ContextCompressionModel,
		MultiModal:                m.MultiModal,
		Light:                     m.Light,
		Thinking:                  m.Thinking,
		ThinkingLevels:            thinkingLevels,
		AnthropicAdaptiveThinking: m.AnthropicAdaptiveThinking,
		IsPreset:                  m.IsPreset,
		ContextWindow:             m.ContextWindow,
		CreatedAt:                 m.CreatedAt,
		UpdatedAt:                 m.UpdatedAt,
	}

	if provider != nil {
		dto.Provider = provider
	}
	return dto
}

type ModelDto struct {
	ID                        uuid.UUID         `json:"id"`
	Provider                  *ModelProviderDto `json:"provider,omitempty"`
	Name                      string            `json:"name"`
	Code                      string            `json:"code"`
	DefaultModel              bool              `json:"defaultModel"`
	EmbeddingModel            bool              `json:"embeddingModel"`
	ContextCompressionModel   bool              `json:"contextCompressionModel"`
	MultiModal                bool              `json:"multiModal"`
	Light                     bool              `json:"light"`
	Thinking                  bool              `json:"thinking"`
	ThinkingLevels            []string          `json:"thinkingLevels"`
	AnthropicAdaptiveThinking bool              `json:"anthropicAdaptiveThinking"`
	IsPreset                  bool              `json:"isPreset"`
	ContextWindow             int               `json:"contextWindow"`
	CreatedAt                 time.Time         `json:"createdAt"`
	UpdatedAt                 time.Time         `json:"updatedAt"`
}

type CreateModelDto struct {
	ProviderID                uuid.UUID `json:"providerId" binding:"required"`
	Name                      string    `json:"name" binding:"required"`
	Code                      string    `json:"code" binding:"required,code"`
	EmbeddingModel            bool      `json:"embeddingModel" binding:"omitempty"`
	ContextCompressionModel   bool      `json:"contextCompressionModel" binding:"omitempty"`
	MultiModal                bool      `json:"multiModal" binding:"omitempty"`
	Light                     bool      `json:"light" binding:"omitempty"`
	Thinking                  bool      `json:"thinking" binding:"omitempty"`
	ThinkingLevels            []string  `json:"thinkingLevels" binding:"omitempty"`
	AnthropicAdaptiveThinking bool      `json:"anthropicAdaptiveThinking" binding:"omitempty"`
}

type UpdateModelDto struct {
	Name                      *string  `json:"name" binding:"omitempty"`
	DefaultModel              *bool    `json:"defaultModel" binding:"omitempty"`
	EmbeddingModel            *bool    `json:"embeddingModel" binding:"omitempty"`
	ContextCompressionModel   *bool    `json:"contextCompressionModel" binding:"omitempty"`
	MultiModal                *bool    `json:"multiModal" binding:"omitempty"`
	Light                     *bool    `json:"light" binding:"omitempty"`
	Thinking                  *bool    `json:"thinking" binding:"omitempty"`
	ThinkingLevels            []string `json:"thinkingLevels" binding:"omitempty"`
	AnthropicAdaptiveThinking *bool    `json:"anthropicAdaptiveThinking" binding:"omitempty"`
}
