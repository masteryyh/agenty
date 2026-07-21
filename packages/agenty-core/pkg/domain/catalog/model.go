package catalog

import (
	"slices"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type Model struct {
	Slug            shared.Slug             `json:"slug"`
	Name            string                  `json:"name"`
	ContextWindow   int                     `json:"contextWindow"`
	MultiModal      bool                    `json:"multiModal"`
	Embedding       bool                    `json:"embedding"`
	Light           bool                    `json:"light"`
	ThinkingEfforts []shared.ThinkingEffort `json:"thinkingEfforts,omitempty"`
	IsDefault       bool                    `json:"isDefault"`
	CreatedAt       time.Time               `json:"createdAt"`
	UpdatedAt       time.Time               `json:"updatedAt"`
}

func (m *Model) SupportsThinking() bool {
	return len(m.ThinkingEfforts) > 0
}

func (m *Model) SupportsEffort(effort shared.ThinkingEffort) bool {
	return slices.Contains(m.ThinkingEfforts, effort)
}
