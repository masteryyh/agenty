package catalog

import (
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type Model struct {
	Slug                   shared.Slug                       `json:"slug"`
	Name                   string                            `json:"name"`
	ContextWindow          int                               `json:"contextWindow"`
	MultiModal             bool                              `json:"multiModal"`
	Embedding              bool                              `json:"embedding"`
	Light                  bool                              `json:"light"`
	ReasoningEffortMapping map[string]shared.ReasoningEffort `json:"reasoningEffortMapping,omitempty"`
	IsDefault              bool                              `json:"isDefault"`
	CreatedAt              time.Time                         `json:"createdAt"`
	UpdatedAt              time.Time                         `json:"updatedAt"`
}

func (m *Model) SupportsReasoning() bool {
	for _, effort := range m.ReasoningEffortMapping {
		if effort.Enabled() {
			return true
		}
	}
	return false
}

func (m *Model) SupportsReasoningEffort(effort shared.ReasoningEffort) bool {
	for _, mappedEffort := range m.ReasoningEffortMapping {
		if mappedEffort == effort {
			return true
		}
	}
	return false
}

func (m *Model) MapReasoningEffort(nativeEffort string) (shared.ReasoningEffort, bool) {
	effort, ok := m.ReasoningEffortMapping[nativeEffort]
	return effort, ok
}
