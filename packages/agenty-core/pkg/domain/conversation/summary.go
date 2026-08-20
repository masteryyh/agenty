package conversation

import (
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type SessionSummary struct {
	ID                  uuid.UUID              `json:"id"`
	Title               string                 `json:"title"`
	AgentCode           shared.Code            `json:"agentCode"`
	LastProviderCode    shared.Code            `json:"lastProviderCode"`
	LastModelCode       shared.ModelCode       `json:"lastModelCode"`
	ContextWindow       int64                  `json:"contextWindow"`
	LastReasoningEffort shared.ReasoningEffort `json:"lastReasoningEffort"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
}

func (s *Session) Summary() SessionSummary {
	sum := SessionSummary{
		ID:                  s.ID,
		AgentCode:           s.AgentCode,
		ContextWindow:       s.ContextWindow,
		LastReasoningEffort: s.CurrentReasoningEffort,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
	}

	if s.Title != nil {
		sum.Title = *s.Title
	}
	if s.CurrentModel != nil {
		sum.LastProviderCode = s.CurrentModel.ProviderCode
		sum.LastModelCode = s.CurrentModel.ModelCode
	}

	return sum
}
