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
	Reasoning        bool                     `json:"reasoning"`
	ReasoningEfforts []shared.ReasoningEffort `json:"reasoningEfforts"`
	IsDefault        bool                     `json:"isDefault"`
	Cached           bool                     `json:"cached,omitempty"`
	CreatedAt        time.Time                `json:"createdAt"`
	UpdatedAt        time.Time                `json:"updatedAt"`
}

func NormalizeReasoningCapabilities(model *Model) {
	if !model.Reasoning && len(model.ReasoningEfforts) > 0 {
		model.Reasoning = true
	}
	if !model.Reasoning {
		model.ReasoningEfforts = make([]shared.ReasoningEffort, 0)
		return
	}
	if len(model.ReasoningEfforts) == 0 {
		model.ReasoningEfforts = shared.StandardReasoningEfforts()
		return
	}
	model.ReasoningEfforts = shared.NormalizeReasoningEfforts(model.ReasoningEfforts)
}

func (m *Model) SupportsReasoning() bool {
	return m.Reasoning || len(m.ReasoningEfforts) > 0
}

func (m *Model) SupportsReasoningEffort(effort shared.ReasoningEffort) bool {
	for _, supported := range m.ReasoningEfforts {
		if supported == effort {
			return true
		}
	}
	return false
}
