package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcatalog"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

type ProviderService struct {
	repo providerRepository
}

type providerRepository interface {
	Get(ctx context.Context, code shared.Code) (*catalog.Provider, error)
	List(ctx context.Context) ([]*catalog.Provider, error)
	Save(ctx context.Context, provider *catalog.Provider) error
	Delete(ctx context.Context, code shared.Code) error
	NeedsModelDiscovery(ctx context.Context, code shared.Code) (bool, error)
	ReplaceModels(ctx context.Context, code shared.Code, models []catalog.Model, expiresAt time.Time) error
}

func NewProviderService(repo providerRepository) *ProviderService {
	return &ProviderService{repo: repo}
}

type ProviderInput struct {
	Name         string          `json:"name"`
	Type         catalog.APIType `json:"type"`
	BaseURL      string          `json:"baseUrl,omitempty"`
	APIKey       string          `json:"apiKey,omitempty"`
	FreeFormTool bool            `json:"freeFormTool,omitempty"`
	Metadata     shared.Metadata `json:"metadata,omitempty"`
}

func (s *ProviderService) Create(ctx context.Context, code string, in ProviderInput) (*catalog.Provider, error) {
	codeVal, err := shared.NewCode(code)
	if err != nil {
		return nil, Validation(err.Error())
	}
	if !in.Type.Valid() {
		return nil, Validation("invalid api type: " + string(in.Type))
	}

	existing, err := s.repo.Get(ctx, codeVal)
	if err == nil && existing != nil {
		return nil, AlreadyExists("provider " + code + " already exists")
	} else if err != nil && !errors.Is(err, storage.ErrProviderNotFound) {
		return nil, Internal("failed to check existing provider: " + err.Error())
	}

	p, err := catalog.NewProvider(code, in.Name, in.Type)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p.BaseURL = in.BaseURL
	p.APIKey = in.APIKey
	p.FreeFormTool = supportsFreeFormTool(in.Type, in.FreeFormTool)
	p.Metadata = in.Metadata

	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) Get(ctx context.Context, code string) (*catalog.Provider, error) {
	codeVal, err := shared.NewCode(code)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p, err := s.repo.Get(ctx, codeVal)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + code + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) List(ctx context.Context, targetCodes ...string) ([]*catalog.Provider, error) {
	providers, err := s.repo.List(ctx)
	if err != nil {
		return nil, Internal("failed to list providers: " + err.Error())
	}
	if providers == nil {
		providers = make([]*catalog.Provider, 0)
	}
	targetCode := ""
	if len(targetCodes) > 0 {
		targetCode = strings.TrimSpace(targetCodes[0])
	}

	toDiscover := make([]shared.Code, 0)
	for _, provider := range providers {
		if provider == nil || (targetCode != "" && provider.Code.String() != targetCode) {
			continue
		}
		if strings.TrimSpace(provider.APIKey) == "" {
			continue
		}
		needsDiscovery, err := s.repo.NeedsModelDiscovery(ctx, provider.Code)
		if err != nil {
			return nil, Internal("failed to inspect model cache for provider " + provider.Code.String() + ": " + err.Error())
		}
		if needsDiscovery {
			toDiscover = append(toDiscover, provider.Code)
		}
	}

	var waitGroup sync.WaitGroup
	for _, code := range toDiscover {
		waitGroup.Add(1)
		go func(providerCode shared.Code) {
			defer waitGroup.Done()
			if _, err := s.ListModels(ctx, providerCode.String()); err != nil {
				slog.WarnContext(ctx, "failed to discover provider models", "providerCode", providerCode, "error", err)
			}
		}(code)
	}
	waitGroup.Wait()
	if len(toDiscover) > 0 {
		providers, err = s.repo.List(ctx)
		if err != nil {
			return nil, Internal("failed to list providers after model discovery: " + err.Error())
		}
	}
	return providers, nil
}

func (s *ProviderService) ListModels(ctx context.Context, code string) ([]catalog.AvailableModel, error) {
	codeVal, err := shared.NewCode(code)
	if err != nil {
		return nil, Validation(err.Error())
	}

	provider, err := s.repo.Get(ctx, codeVal)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + code + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}
	if provider == nil {
		return nil, Internal("provider repository returned an empty provider")
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		return nil, Validation("provider " + code + " has no API key")
	}
	needsDiscovery, err := s.repo.NeedsModelDiscovery(ctx, codeVal)
	if err != nil {
		return nil, Internal("failed to inspect model cache for provider " + code + ": " + err.Error())
	}
	if !needsDiscovery {
		return availableModelsFromCatalog(provider.Models), nil
	}
	models, err := modelcatalog.List(ctx, *provider)
	if err != nil {
		return nil, Internal("failed to list models for provider " + code + ": " + err.Error())
	}
	if models == nil {
		models = make([]catalog.AvailableModel, 0)
	}
	if err := s.repo.ReplaceModels(
		ctx,
		codeVal,
		catalogModelsFromAvailable(models),
		time.Now().UTC().Add(catalog.ModelDiscoveryCacheTTL),
	); err != nil {
		return nil, Internal("failed to cache models for provider " + code + ": " + err.Error())
	}
	return models, nil
}

func catalogModelsFromAvailable(models []catalog.AvailableModel) []catalog.Model {
	now := time.Now().UTC()
	result := make([]catalog.Model, 0, len(models))
	for _, available := range models {
		result = append(result, catalog.Model{
			Code:             available.Code,
			Name:             available.Name,
			ContextWindow:    available.ContextWindow,
			MaxOutputTokens:  available.MaxOutputTokens,
			MultiModal:       available.MultiModal,
			Reasoning:        available.Reasoning,
			ReasoningEfforts: available.ReasoningEfforts,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	return result
}

func availableModelsFromCatalog(models []catalog.Model) []catalog.AvailableModel {
	result := make([]catalog.AvailableModel, 0, len(models))
	for _, model := range models {
		result = append(result, catalog.AvailableModel{
			Code:             model.Code,
			Name:             model.Name,
			ContextWindow:    model.ContextWindow,
			MaxOutputTokens:  model.MaxOutputTokens,
			MultiModal:       model.MultiModal,
			Reasoning:        model.Reasoning,
			ReasoningEfforts: model.ReasoningEfforts,
		})
	}
	return result
}

type ProviderUpdate struct {
	Name         *string          `json:"name,omitempty"`
	Type         *catalog.APIType `json:"type,omitempty"`
	BaseURL      *string          `json:"baseUrl,omitempty"`
	APIKey       *string          `json:"apiKey,omitempty"`
	FreeFormTool *bool            `json:"freeFormTool,omitempty"`
	Metadata     *shared.Metadata `json:"metadata,omitempty"`
}

func (s *ProviderService) Update(ctx context.Context, code string, upd ProviderUpdate) (*catalog.Provider, error) {
	codeVal, err := shared.NewCode(code)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p, err := s.repo.Get(ctx, codeVal)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + code + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}
	if p.Builtin {
		if upd.Name != nil || upd.Type != nil || upd.BaseURL != nil || upd.FreeFormTool != nil || upd.Metadata != nil {
			return nil, Validation("built-in provider metadata is read-only; only the API key can be changed")
		}
		if upd.APIKey == nil {
			return p, nil
		}
		p.APIKey = *upd.APIKey
		if err := s.repo.Save(ctx, p); err != nil {
			return nil, Internal("failed to save provider API key: " + err.Error())
		}
		return p, nil
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
	if upd.FreeFormTool != nil {
		p.FreeFormTool = *upd.FreeFormTool
	}
	p.FreeFormTool = supportsFreeFormTool(p.Type, p.FreeFormTool)
	if upd.Metadata != nil {
		p.Metadata = *upd.Metadata
	}
	p.UpdatedAt = time.Now().UTC()

	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}

func supportsFreeFormTool(apiType catalog.APIType, enabled bool) bool {
	return apiType == catalog.APIOpenAI && enabled
}

func (s *ProviderService) Delete(ctx context.Context, code string) error {
	codeVal, err := shared.NewCode(code)
	if err != nil {
		return Validation(err.Error())
	}

	p, err := s.repo.Get(ctx, codeVal)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return NotFound("provider " + code + " not found")
		}
		return Internal("failed to get provider: " + err.Error())
	}
	if p.Builtin {
		return Validation("built-in provider is read-only; only the API key can be changed")
	}

	if err := s.repo.Delete(ctx, codeVal); err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return NotFound("provider " + code + " not found")
		}
		return Internal("failed to delete provider: " + err.Error())
	}
	return nil
}

type ModelInput struct {
	Name             string                   `json:"name"`
	ContextWindow    int                      `json:"contextWindow,omitempty"`
	MaxOutputTokens  int64                    `json:"maxOutputTokens"`
	MultiModal       bool                     `json:"multiModal,omitempty"`
	Light            bool                     `json:"light,omitempty"`
	Reasoning        *bool                    `json:"reasoning,omitempty"`
	ReasoningEfforts []shared.ReasoningEffort `json:"reasoningEfforts,omitempty"`
	IsDefault        bool                     `json:"isDefault,omitempty"`
}

func (s *ProviderService) AddModel(ctx context.Context, providerCode, modelCode string, in ModelInput) (*catalog.Provider, error) {
	ps, err := shared.NewCode(providerCode)
	if err != nil {
		return nil, Validation(err.Error())
	}

	ms, err := shared.NewModelCode(modelCode)
	if err != nil {
		return nil, Validation(err.Error())
	}
	p, err := s.repo.Get(ctx, ps)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + providerCode + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}
	if p.Builtin {
		return nil, Validation("built-in provider models are read-only")
	}

	now := time.Now().UTC()
	maxOutputTokens := in.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = catalog.DefaultMaxOutputTokens
	}
	reasoning := true
	if in.Reasoning != nil {
		reasoning = *in.Reasoning
	}
	if err := shared.ValidateReasoningEfforts(in.ReasoningEfforts); err != nil {
		return nil, Validation(err.Error())
	}
	if !reasoning && len(in.ReasoningEfforts) > 0 {
		return nil, Validation("reasoning efforts require reasoning to be enabled")
	}
	reasoningEfforts := make([]shared.ReasoningEffort, 0)
	if reasoning {
		reasoningEfforts = append(reasoningEfforts, in.ReasoningEfforts...)
		if len(reasoningEfforts) == 0 {
			reasoningEfforts = shared.StandardReasoningEfforts()
		}
	}
	p.AddModel(catalog.Model{
		Code:             ms,
		Name:             in.Name,
		ContextWindow:    in.ContextWindow,
		MaxOutputTokens:  maxOutputTokens,
		MultiModal:       in.MultiModal,
		Light:            in.Light,
		Reasoning:        reasoning,
		ReasoningEfforts: reasoningEfforts,
		IsDefault:        in.IsDefault,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	p.UpdatedAt = now

	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}

func (s *ProviderService) RemoveModel(ctx context.Context, providerCode, modelCode string) (*catalog.Provider, error) {
	ps, err := shared.NewCode(providerCode)
	if err != nil {
		return nil, Validation(err.Error())
	}

	ms, err := shared.NewModelCode(modelCode)
	if err != nil {
		return nil, Validation(err.Error())
	}

	p, err := s.repo.Get(ctx, ps)
	if err != nil {
		if errors.Is(err, storage.ErrProviderNotFound) {
			return nil, NotFound("provider " + providerCode + " not found")
		}
		return nil, Internal("failed to get provider: " + err.Error())
	}
	if p.Builtin {
		return nil, Validation("built-in provider models are read-only")
	}

	if _, err := p.Model(ms); err != nil {
		if errors.Is(err, catalog.ErrModelNotFound) {
			return nil, NotFound("model " + modelCode + " not found in provider " + providerCode)
		}
		return nil, Internal("failed to find model: " + err.Error())
	}

	p.RemoveModel(ms)
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, p); err != nil {
		return nil, Internal("failed to save provider: " + err.Error())
	}
	return p, nil
}
