package application

import (
	"context"
	"errors"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

// AgentService implements agent CRUD use-cases on top of an AgentRepository.
type AgentService struct {
	repo agentRepository
}

type agentRepository interface {
	Get(ctx context.Context, slug shared.Slug) (*agent.Agent, error)
	List(ctx context.Context) ([]*agent.Agent, error)
	Save(ctx context.Context, agent *agent.Agent) error
	Delete(ctx context.Context, slug shared.Slug) error
}

func NewAgentService(repo agentRepository) *AgentService {
	return &AgentService{repo: repo}
}

// AgentInput carries the mutable fields for creating an agent. Slug is the
// identity and is passed separately to Create.
type AgentInput struct {
	Name                  string                `json:"name"`
	Description           string                `json:"description,omitempty"`
	Soul                  string                `json:"soul,omitempty"`
	DefaultModel          *shared.ModelRef      `json:"defaultModel,omitempty"`
	DefaultContextWindow  int64                 `json:"defaultContextWindow,omitempty"`
	DefaultThinkingEffort shared.ThinkingEffort `json:"defaultThinkingEffort,omitempty"`
	IsDefault             bool                  `json:"isDefault,omitempty"`
	Metadata              shared.Metadata       `json:"metadata,omitempty"`
}

func (s *AgentService) Create(ctx context.Context, slug string, in AgentInput) (*agent.Agent, error) {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	existing, err := s.repo.Get(ctx, slugVal)
	if err == nil && existing != nil {
		return nil, AlreadyExists("agent " + slug + " already exists")
	} else if err != nil && !errors.Is(err, storage.ErrAgentNotFound) {
		return nil, Internal("failed to check existing agent: " + err.Error())
	}

	a, err := agent.New(slug, in.Name)
	if err != nil {
		return nil, Validation(err.Error())
	}
	a.Description = in.Description
	a.Soul = in.Soul
	a.DefaultModel = in.DefaultModel
	a.DefaultContextWindow = in.DefaultContextWindow
	a.DefaultThinkingEffort = in.DefaultThinkingEffort
	a.IsDefault = in.IsDefault
	a.Metadata = in.Metadata

	if err := s.repo.Save(ctx, a); err != nil {
		return nil, Internal("failed to save agent: " + err.Error())
	}
	return a, nil
}

func (s *AgentService) Get(ctx context.Context, slug string) (*agent.Agent, error) {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	a, err := s.repo.Get(ctx, slugVal)
	if err != nil {
		if errors.Is(err, storage.ErrAgentNotFound) {
			return nil, NotFound("agent " + slug + " not found")
		}
		return nil, Internal("failed to get agent: " + err.Error())
	}
	return a, nil
}

func (s *AgentService) List(ctx context.Context) ([]*agent.Agent, error) {
	agents, err := s.repo.List(ctx)
	if err != nil {
		return nil, Internal("failed to list agents: " + err.Error())
	}
	return agents, nil
}

// AgentUpdate is a partial update: only non-nil fields are applied.
type AgentUpdate struct {
	Name                  *string                `json:"name,omitempty"`
	Description           *string                `json:"description,omitempty"`
	Soul                  *string                `json:"soul,omitempty"`
	DefaultModel          *shared.ModelRef       `json:"defaultModel,omitempty"`
	DefaultContextWindow  *int64                 `json:"defaultContextWindow,omitempty"`
	DefaultThinkingEffort *shared.ThinkingEffort `json:"defaultThinkingEffort,omitempty"`
	IsDefault             *bool                  `json:"isDefault,omitempty"`
	Metadata              *shared.Metadata       `json:"metadata,omitempty"`
}

func (s *AgentService) Update(ctx context.Context, slug string, upd AgentUpdate) (*agent.Agent, error) {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	a, err := s.repo.Get(ctx, slugVal)
	if err != nil {
		if errors.Is(err, storage.ErrAgentNotFound) {
			return nil, NotFound("agent " + slug + " not found")
		}
		return nil, Internal("failed to get agent: " + err.Error())
	}

	if upd.Name != nil {
		a.Name = *upd.Name
	}
	if upd.Description != nil {
		a.Description = *upd.Description
	}
	if upd.Soul != nil {
		a.Soul = *upd.Soul
	}
	if upd.DefaultModel != nil {
		a.DefaultModel = upd.DefaultModel
	}
	if upd.DefaultContextWindow != nil {
		a.DefaultContextWindow = *upd.DefaultContextWindow
	}
	if upd.DefaultThinkingEffort != nil {
		a.DefaultThinkingEffort = *upd.DefaultThinkingEffort
	}
	if upd.IsDefault != nil {
		a.IsDefault = *upd.IsDefault
	}
	if upd.Metadata != nil {
		a.Metadata = *upd.Metadata
	}
	a.UpdatedAt = time.Now().UTC()

	if err := s.repo.Save(ctx, a); err != nil {
		return nil, Internal("failed to save agent: " + err.Error())
	}
	return a, nil
}

func (s *AgentService) Delete(ctx context.Context, slug string) error {
	slugVal, err := shared.NewSlug(slug)
	if err != nil {
		return Validation(err.Error())
	}
	if err := s.repo.Delete(ctx, slugVal); err != nil {
		if errors.Is(err, storage.ErrAgentNotFound) {
			return NotFound("agent " + slug + " not found")
		}
		return Internal("failed to delete agent: " + err.Error())
	}
	return nil
}
