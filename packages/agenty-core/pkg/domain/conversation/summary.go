package conversation

import (
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type SessionSummary struct {
	ID                  uuid.UUID              `json:"id"`
	Title               string                 `json:"title"`
	AgentSlug           shared.Slug            `json:"agentSlug"`
	LastProviderSlug    shared.Slug            `json:"lastProviderSlug"`
	LastModelSlug       shared.ModelID         `json:"lastModelSlug"`
	ContextWindow       int64                  `json:"contextWindow"`
	LastReasoningEffort shared.ReasoningEffort `json:"lastReasoningEffort"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
}

func (s *Session) Summary() SessionSummary {
	sum := SessionSummary{
		ID:                  s.ID,
		AgentSlug:           s.AgentSlug,
		ContextWindow:       s.ContextWindow,
		LastReasoningEffort: s.CurrentReasoningEffort,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
	}

	if s.Title != nil {
		sum.Title = *s.Title
	}
	if s.CurrentModel != nil {
		sum.LastProviderSlug = s.CurrentModel.ProviderSlug
		sum.LastModelSlug = s.CurrentModel.ModelSlug
	}

	return sum
}
