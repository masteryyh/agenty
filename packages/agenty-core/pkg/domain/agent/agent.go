package agent

import (
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type Agent struct {
	Slug                  shared.Slug           `json:"slug"`
	Name                  string                `json:"name"`
	Description           string                `json:"description,omitempty"`
	Soul                  string                `json:"soul"`
	DefaultModel          *shared.ModelRef      `json:"defaultModel,omitempty"`
	DefaultContextWindow  int64                 `json:"defaultContextWindow"`
	DefaultThinkingEffort shared.ThinkingEffort `json:"defaultThinkingEffort,omitempty"`
	IsDefault             bool                  `json:"isDefault"`
	Metadata              shared.Metadata       `json:"metadata,omitempty"`
	CreatedAt             time.Time             `json:"createdAt"`
	UpdatedAt             time.Time             `json:"updatedAt"`
}

func New(slug, name string) (*Agent, error) {
	s, err := shared.NewSlug(slug)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &Agent{
		Slug:      s,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
