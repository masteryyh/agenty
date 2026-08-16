package application

import (
	"context"
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type initializationState interface {
	Initialized() bool
	SetInitialized(initialized bool) error
}

type InitializeService struct {
	agents    *AgentService
	providers *ProviderService
	state     initializationState
}

func NewInitializeService(
	agents *AgentService,
	providers *ProviderService,
	state initializationState,
) *InitializeService {
	return &InitializeService{
		agents:    agents,
		providers: providers,
		state:     state,
	}
}

type InitializeAlreadyResult struct {
	Initialized bool `json:"initialized"`
}

func (s *InitializeService) Already(context.Context) InitializeAlreadyResult {
	return InitializeAlreadyResult{Initialized: s.state.Initialized()}
}

type InitializeCompleteInput struct {
	AgentSlug    string `json:"agentSlug"`
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
}

func (s *InitializeService) Complete(
	ctx context.Context,
	in InitializeCompleteInput,
) (InitializeAlreadyResult, error) {
	a, err := s.agents.Get(ctx, in.AgentSlug)
	if err != nil {
		return InitializeAlreadyResult{}, err
	}
	p, err := s.providers.Get(ctx, in.ProviderSlug)
	if err != nil {
		return InitializeAlreadyResult{}, err
	}
	modelSlug, err := shared.NewModelID(in.ModelSlug)
	if err != nil {
		return InitializeAlreadyResult{}, Validation(err.Error())
	}
	m, err := p.Model(modelSlug)
	if err != nil {
		return InitializeAlreadyResult{}, NotFound(
			fmt.Sprintf("model %s not found in provider %s", in.ModelSlug, in.ProviderSlug),
		)
	}
	wantModel := shared.ModelRef{ProviderSlug: p.Slug, ModelSlug: m.Slug}
	if a.DefaultModel == nil || *a.DefaultModel != wantModel {
		return InitializeAlreadyResult{}, Validation("agent default model does not match the initialized provider and model")
	}
	if err := s.state.SetInitialized(true); err != nil {
		return InitializeAlreadyResult{}, Internal("failed to persist initialization state: " + err.Error())
	}
	return InitializeAlreadyResult{Initialized: true}, nil
}
