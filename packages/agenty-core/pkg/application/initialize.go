package application

import (
	"context"
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type initializationState interface {
	Initialized() bool
	SetInitialized(initialized bool) error
	DefaultModel() (shared.ModelRef, shared.ReasoningEffort)
	SetDefaultModel(model shared.ModelRef, effort shared.ReasoningEffort) error
}

type InitializeService struct {
	providers *ProviderService
	state     initializationState
}

func NewInitializeService(
	providers *ProviderService,
	state initializationState,
) *InitializeService {
	return &InitializeService{
		providers: providers,
		state:     state,
	}
}

type InitializeAlreadyResult struct {
	Initialized            bool                   `json:"initialized"`
	DefaultModel           *shared.ModelRef       `json:"defaultModel,omitempty"`
	DefaultReasoningEffort shared.ReasoningEffort `json:"defaultReasoningEffort,omitempty"`
}

func (s *InitializeService) Already(context.Context) InitializeAlreadyResult {
	model, effort := s.state.DefaultModel()
	result := InitializeAlreadyResult{Initialized: s.state.Initialized()}
	if !model.IsZero() {
		result.DefaultModel = &model
		result.DefaultReasoningEffort = effort
	}
	return result
}

type InitializeCompleteInput struct {
	ProviderCode    string                 `json:"providerCode"`
	ModelCode       string                 `json:"modelCode"`
	ReasoningEffort shared.ReasoningEffort `json:"reasoningEffort,omitempty"`
}

func (s *InitializeService) Complete(
	ctx context.Context,
	in InitializeCompleteInput,
) (InitializeAlreadyResult, error) {
	p, err := s.providers.Get(ctx, in.ProviderCode)
	if err != nil {
		return InitializeAlreadyResult{}, err
	}
	modelCode, err := shared.NewModelCode(in.ModelCode)
	if err != nil {
		return InitializeAlreadyResult{}, Validation(err.Error())
	}
	m, err := p.Model(modelCode)
	if err != nil {
		return InitializeAlreadyResult{}, NotFound(
			fmt.Sprintf("model %s not found in provider %s", in.ModelCode, in.ProviderCode),
		)
	}
	effort := in.ReasoningEffort
	if effort == "" {
		effort = shared.ReasoningOff
	}
	if !effort.Valid() {
		return InitializeAlreadyResult{}, Validation("invalid default reasoning effort: " + string(effort))
	}
	defaultModel := shared.ModelRef{ProviderCode: p.Code, ModelCode: m.Code}
	if err := s.state.SetDefaultModel(defaultModel, effort); err != nil {
		return InitializeAlreadyResult{}, Internal("failed to persist default model: " + err.Error())
	}
	if err := s.state.SetInitialized(true); err != nil {
		return InitializeAlreadyResult{}, Internal("failed to persist initialization state: " + err.Error())
	}
	return InitializeAlreadyResult{
		Initialized:            true,
		DefaultModel:           &defaultModel,
		DefaultReasoningEffort: effort,
	}, nil
}
