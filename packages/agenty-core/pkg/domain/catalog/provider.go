package catalog

import (
	"errors"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var (
	ErrModelNotFound           = errors.New("catalog: model not found")
	ErrBuiltinProviderReadOnly = errors.New("catalog: built-in provider is read-only")
)

type Provider struct {
	Code          shared.Code     `json:"code"`
	Name          string          `json:"name"`
	Type          APIType         `json:"type"`
	BaseURL       string          `json:"baseUrl"`
	APIKey        string          `json:"apiKey"`
	Builtin       bool            `json:"builtin"`
	Official      bool            `json:"official"`
	FreeFormTool  bool            `json:"freeFormTool"`
	ModelsURL     string          `json:"modelsUrl,omitempty"`
	TokenCountURL string          `json:"tokenCountUrl,omitempty"`
	Models        []Model         `json:"models"`
	ModelsCached  bool            `json:"modelsCached,omitempty"`
	Metadata      shared.Metadata `json:"metadata,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

func NewProvider(code, name string, apiType APIType) (*Provider, error) {
	s, err := shared.NewCode(code)
	if err != nil {
		return nil, err
	}
	if !apiType.Valid() {
		return nil, errors.New("catalog: invalid API type")
	}

	now := time.Now().UTC()
	return &Provider{
		Code:      s,
		Name:      name,
		Type:      apiType,
		Models:    make([]Model, 0),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (p *Provider) Model(code shared.ModelCode) (*Model, error) {
	for i := range p.Models {
		if p.Models[i].Code == code {
			return &p.Models[i], nil
		}
	}
	return nil, ErrModelNotFound
}

func (p *Provider) AddModel(m Model) {
	NormalizeReasoningCapabilities(&m)
	if m.MaxOutputTokens <= 0 {
		m.MaxOutputTokens = DefaultMaxOutputTokens
	}
	for i := range p.Models {
		if p.Models[i].Code == m.Code {
			p.Models[i] = m
			return
		}
	}
	p.Models = append(p.Models, m)
}

func (p *Provider) RemoveModel(code shared.ModelCode) {
	for i := range p.Models {
		if p.Models[i].Code == code {
			p.Models = append(p.Models[:i], p.Models[i+1:]...)
			return
		}
	}
}

func (p *Provider) DefaultModel() (*Model, bool) {
	for i := range p.Models {
		if p.Models[i].IsDefault {
			return &p.Models[i], true
		}
	}
	return nil, false
}
