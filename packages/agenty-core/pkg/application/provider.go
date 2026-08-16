package application

import (
	"context"
	"errors"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

type ProviderService struct {
	repo providerRepository
}

type providerRepository interface {
	Get(ctx context.Context, slug shared.Slug) (*catalog.Provider, error)
	List(ctx context.Context) ([]*catalog.Provider, error)
	Save(ctx context.Context, provider *catalog.Provider) error
	Delete(ctx context.Context, slug shared.Slug) error
	DeleteModel(ctx context.Context, providerSlug shared.Slug, modelSlug shared.ModelID) error
}

func NewProviderService(repo providerRepository) *ProviderService {
	return &ProviderService{repo: repo}
}

type ProviderInput struct {
	Name     string          `json:"name"`
	Type     catalog.APIType `json:"type"`
	BaseURL  string          `json:"baseUrl,omitempty"`
	APIKey   string          `json:"apiKey,omitempty"`
	Metadata shared.Metadata `json:"metadata,omitempty"`
}

func (s *ProviderService) Create(ctx context.Context, slug string, in ProviderInput) (*catalog.Provider, error) {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	if !in.Type.Valid() {
		return nil, Validation("invalid api type: " + string(in.Type))
	}

	existing, err := s.repo.Get(ctx, slugVal)
	if err == nil && existing != nil {
		return nil, AlreadyExists("provider " + slug + " already exists")
	} else if err != nil && !errors.Is(err, storage.ErrProviderNotFound) {
		return nil, Internal("failed to check existing provider: " + err.Error())
	}

	p, err := catalog.NewProvider(slug, in.Name, in.Type)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p.BaseURL = in.BaseURL
	p.APIKey = in.APIKey
	p.Metadata = in.Metadata

	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) Get(ctx context.Context, slug string) (*catalog.Provider, error) {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p, err := s.repo.Get(ctx, slugVal)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + slug + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) List(ctx context.Context) ([]*catalog.Provider, error) {
	providers, err := s.repo.List(ctx)
	if err != nil {
		return nil, Internal("failed to list providers: " + err.Error())
	}
	if providers == nil {
		providers = make([]*catalog.Provider, 0)
	}
	return providers, nil
}

type ProviderUpdate struct {
	Name     *string          `json:"name,omitempty"`
	Type     *catalog.APIType `json:"type,omitempty"`
	BaseURL  *string          `json:"baseUrl,omitempty"`
	APIKey   *string          `json:"apiKey,omitempty"`
	Metadata *shared.Metadata `json:"metadata,omitempty"`
}

func (s *ProviderService) Update(ctx context.Context, slug string, upd ProviderUpdate) (*catalog.Provider, error) {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p, err := s.repo.Get(ctx, slugVal)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + slug + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}

	if upd.Name != nil {
		p.Name = *upd.Name
	}
	if upd.Type != nil {
		if !(*upd.Type).Valid() {
			return nil, Validation("invalid api type: " + string(*upd.Type))
		}
		p.Type = *upd.Type
	}
	if upd.BaseURL != nil {
		p.BaseURL = *upd.BaseURL
	}
	if upd.APIKey != nil {
		p.APIKey = *upd.APIKey
	}
	if upd.Metadata != nil {
		p.Metadata = *upd.Metadata
	}
	p.UpdatedAt = time.Now().UTC()

	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) Delete(ctx context.Context, slug string) error {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return Validation(err.Error())
	}

	if err := s.repo.Delete(ctx, slugVal); err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return NotFound("provider " + slug + " not found")
		}
		return Internal("failed to delete provider: " + err.Error())
	}
	return nil
}

type ModelInput struct {
	Name                   string                            `json:"name"`
	ContextWindow          int                               `json:"contextWindow,omitempty"`
	MaxOutputTokens        int64                             `json:"maxOutputTokens"`
	MultiModal             bool                              `json:"multiModal,omitempty"`
	Light                  bool                              `json:"light,omitempty"`
	ReasoningEffortMapping map[string]shared.ReasoningEffort `json:"reasoningEffortMapping,omitempty"`
	IsDefault              bool                              `json:"isDefault,omitempty"`
}

func (s *ProviderService) AddModel(ctx context.Context, providerSlug, modelSlug string, in ModelInput) (*catalog.Provider, error) {
	ps, err := shared.NewSlug(providerSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	ms, err := shared.NewModelID(modelSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	if in.MaxOutputTokens <= 0 {
		return nil, Validation("max output tokens must be greater than zero")
	}

	for nativeEffort, agentyEffort := range in.ReasoningEffortMapping {
		if nativeEffort == "" {
			return nil, Validation("reasoning effort mapping contains an empty native effort")
		}
		if !agentyEffort.Valid() {
			return nil, Validation("invalid agenty reasoning effort: " + string(agentyEffort))
		}
	}

	p, err := s.repo.Get(ctx, ps)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + providerSlug + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}

	now := time.Now().UTC()
	p.AddModel(catalog.Model{
		Slug:                   ms,
		Name:                   in.Name,
		ContextWindow:          in.ContextWindow,
		MaxOutputTokens:        in.MaxOutputTokens,
		MultiModal:             in.MultiModal,
		Light:                  in.Light,
		ReasoningEffortMapping: in.ReasoningEffortMapping,
		IsDefault:              in.IsDefault,
		CreatedAt:              now,
		UpdatedAt:              now,
	})
	p.UpdatedAt = now

	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) RemoveModel(ctx context.Context, providerSlug, modelSlug string) (*catalog.Provider, error) {
	ps, err := shared.NewSlug(providerSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	ms, err := shared.NewModelID(modelSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	if err := s.repo.DeleteModel(ctx, ps, ms); err != nil {
		switch {
		case errors.Is(err, storage.ErrProviderNotFound):
			return nil, NotFound("provider " + providerSlug + " not found")
		case errors.Is(err, catalog.ErrModelNotFound):
			return nil, NotFound("model " + modelSlug + " not found in provider " + providerSlug)
		default:
			return nil, Internal("failed to remove model: " + err.Error())
		}
	}

	p, err := s.repo.Get(ctx, ps)
	if err != nil {
		return nil, Internal("failed to reload provider: " + err.Error())
	}
	return p, nil
}
