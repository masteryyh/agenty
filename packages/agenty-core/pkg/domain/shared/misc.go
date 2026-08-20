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
	ProviderCode Code      `json:"providerCode"`
	ModelCode    ModelCode `json:"modelCode"`
}

func NewModelRef(provider Code, model ModelCode) ModelRef {
	return ModelRef{
		ProviderCode: provider,
		ModelCode:    model,
	}
}

func (r ModelRef) IsZero() bool {
	return r.ProviderCode.IsZero() && r.ModelCode.IsZero()
}

func (r ModelRef) String() string {
	return r.ProviderCode.String() + "/" + r.ModelCode.String()
}

type RawJSON = json.RawMessage
