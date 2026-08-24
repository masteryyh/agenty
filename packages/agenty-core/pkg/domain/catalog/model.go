package catalog

import (
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const DefaultMaxOutputTokens int64 = 8_192

type Model struct {
	Code             shared.ModelCode         `json:"code"`
	Name             string                   `json:"name"`
	ContextWindow    int                      `json:"contextWindow"`
	MaxOutputTokens  int64                    `json:"maxOutputTokens"`
	MultiModal       bool                     `json:"multiModal"`
	Light            bool                     `json:"light"`
	ReasoningEfforts []shared.ReasoningEffort `json:"reasoningEfforts"`
	IsDefault        bool                     `json:"isDefault"`
	CreatedAt        time.Time                `json:"createdAt"`
	UpdatedAt        time.Time                `json:"updatedAt"`
}

func (m *Model) SupportsReasoning() bool {
	for _, effort := range m.ReasoningEfforts {
		if effort.Enabled() {
			return true
		}
	}
	return false
}

func (m *Model) SupportsReasoningEffort(effort shared.ReasoningEffort) bool {
	for _, supported := range m.ReasoningEfforts {
		if supported == effort {
			return true
		}
	}
	return false
}
