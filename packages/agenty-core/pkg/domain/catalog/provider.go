package catalog

import (
	"errors"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var (
	ErrModelNotFound = errors.New("catalog: model not found")
)

type Provider struct {
	Slug      shared.Slug     `json:"slug"`
	Name      string          `json:"name"`
	Type      APIType         `json:"type"`
	BaseURL   string          `json:"baseUrl"`
	APIKey    string          `json:"apiKey"`
	Models    []Model         `json:"models"`
	Metadata  shared.Metadata `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func NewProvider(slug, name string, apiType APIType) (*Provider, error) {
	s, err := shared.NewSlug(slug)
	if err != nil {
		return nil, err
	}
	if !apiType.Valid() {
		return nil, errors.New("catalog: invalid API type")
	}

	now := time.Now().UTC()
	return &Provider{
		Slug:      s,
		Name:      name,
		Type:      apiType,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (p *Provider) Model(slug shared.Slug) (*Model, error) {
	for i := range p.Models {
		if p.Models[i].Slug == slug {
			return &p.Models[i], nil
		}
	}
	return nil, ErrModelNotFound
}

func (p *Provider) AddModel(m Model) {
	for i := range p.Models {
		if p.Models[i].Slug == m.Slug {
			p.Models[i] = m
			return
		}
	}
	p.Models = append(p.Models, m)
}

func (p *Provider) RemoveModel(slug shared.Slug) {
	for i := range p.Models {
		if p.Models[i].Slug == slug {
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
