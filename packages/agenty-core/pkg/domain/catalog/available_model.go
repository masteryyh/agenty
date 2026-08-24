package catalog

import (
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const (
	DefaultAvailableModelContextWindow         = 256_000
	DefaultAvailableModelMaxOutputTokens int64 = 65_536
	ModelDiscoveryCacheTTL                     = 8 * time.Hour
)

type AvailableModel struct {
	Code             shared.ModelCode         `json:"code"`
	Name             string                   `json:"name"`
	ContextWindow    int                      `json:"contextWindow"`
	MaxOutputTokens  int64                    `json:"maxOutputTokens"`
	MultiModal       bool                     `json:"multiModal"`
	ReasoningEfforts []shared.ReasoningEffort `json:"reasoningEfforts"`
}
