package shared

import (
	"encoding/json"

	"github.com/google/uuid"
)

func NewID() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

type Metadata map[string]any

type ModelRef struct {
	ProviderSlug Slug `json:"providerSlug"`
	ModelSlug    Slug `json:"modelSlug"`
}

func NewModelRef(provider, model Slug) ModelRef {
	return ModelRef{
		ProviderSlug: provider,
		ModelSlug:    model,
	}
}

func (r ModelRef) IsZero() bool {
	return r.ProviderSlug.IsZero() && r.ModelSlug.IsZero()
}

func (r ModelRef) String() string {
	return r.ProviderSlug.String() + "/" + r.ModelSlug.String()
}

type RawJSON = json.RawMessage
