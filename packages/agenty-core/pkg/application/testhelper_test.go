package application_test

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

type agentRepositoryFake struct {
	agents    map[shared.Slug]*agent.Agent
	getErr    error
	listErr   error
	saveErr   error
	deleteErr error
}

func newAgentRepositoryFake() *agentRepositoryFake {
	return &agentRepositoryFake{agents: make(map[shared.Slug]*agent.Agent)}
}

func (r *agentRepositoryFake) Get(_ context.Context, slug shared.Slug) (*agent.Agent, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	a, ok := r.agents[slug]
	if !ok {
		return nil, storage.ErrAgentNotFound
	}
	return cloneAgent(a), nil
}

func (r *agentRepositoryFake) List(context.Context) ([]*agent.Agent, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]*agent.Agent, 0, len(r.agents))
	for _, a := range r.agents {
		result = append(result, cloneAgent(a))
	}
	return result, nil
}

func (r *agentRepositoryFake) Save(_ context.Context, a *agent.Agent) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.agents[a.Slug] = cloneAgent(a)
	return nil
}

func (r *agentRepositoryFake) Delete(_ context.Context, slug shared.Slug) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.agents[slug]; !ok {
		return storage.ErrAgentNotFound
	}
	delete(r.agents, slug)
	return nil
}

func cloneAgent(a *agent.Agent) *agent.Agent {
	copy := *a
	if a.DefaultModel != nil {
		model := *a.DefaultModel
		copy.DefaultModel = &model
	}
	copy.Metadata = cloneMetadata(a.Metadata)
	return &copy
}

type providerRepositoryFake struct {
	providers      map[shared.Slug]*catalog.Provider
	getErr         error
	listErr        error
	saveErr        error
	deleteErr      error
	deleteModelErr error
}

func newProviderRepositoryFake() *providerRepositoryFake {
	return &providerRepositoryFake{providers: make(map[shared.Slug]*catalog.Provider)}
}

func (r *providerRepositoryFake) Get(_ context.Context, slug shared.Slug) (*catalog.Provider, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	p, ok := r.providers[slug]
	if !ok {
		return nil, storage.ErrProviderNotFound
	}
	return cloneProvider(p), nil
}

func (r *providerRepositoryFake) List(context.Context) ([]*catalog.Provider, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]*catalog.Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, cloneProvider(p))
	}
	return result, nil
}

func (r *providerRepositoryFake) Save(_ context.Context, p *catalog.Provider) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.providers[p.Slug] = cloneProvider(p)
	return nil
}

func (r *providerRepositoryFake) Delete(_ context.Context, slug shared.Slug) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.providers[slug]; !ok {
		return storage.ErrProviderNotFound
	}
	delete(r.providers, slug)
	return nil
}

func (r *providerRepositoryFake) DeleteModel(_ context.Context, providerSlug, modelSlug shared.Slug) error {
	if r.deleteModelErr != nil {
		return r.deleteModelErr
	}
	p, ok := r.providers[providerSlug]
	if !ok {
		return storage.ErrProviderNotFound
	}
	for i := range p.Models {
		if p.Models[i].Slug == modelSlug {
			p.Models = append(p.Models[:i], p.Models[i+1:]...)
			return nil
		}
	}
	return catalog.ErrModelNotFound
}

func cloneProvider(p *catalog.Provider) *catalog.Provider {
	copy := *p
	copy.Models = slices.Clone(p.Models)
	for i := range copy.Models {
		copy.Models[i].ThinkingEfforts = slices.Clone(copy.Models[i].ThinkingEfforts)
	}
	copy.Metadata = cloneMetadata(p.Metadata)
	return &copy
}

type sessionRepositoryFake struct {
	events    map[uuid.UUID][]shared.Event
	loadErr   error
	saveErr   error
	listErr   error
	deleteErr error
}

func newSessionRepositoryFake() *sessionRepositoryFake {
	return &sessionRepositoryFake{events: make(map[uuid.UUID][]shared.Event)}
}

func (r *sessionRepositoryFake) Load(_ context.Context, id uuid.UUID) (*conversation.Session, error) {
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	events, ok := r.events[id]
	if !ok {
		return nil, storage.ErrConversationNotFound
	}
	return conversation.ReplaySession(events), nil
}

func (r *sessionRepositoryFake) Save(_ context.Context, session *conversation.Session) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.events[session.ID] = append(r.events[session.ID], session.PendingEvents()...)
	return nil
}

func (r *sessionRepositoryFake) List(_ context.Context, query conversation.ListQuery) ([]conversation.SessionSummary, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]conversation.SessionSummary, 0, len(r.events))
	for _, events := range r.events {
		summary := conversation.ReplaySession(events).Summary()
		if query.AgentSlug == nil || summary.AgentSlug == *query.AgentSlug {
			result = append(result, summary)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	start := min(query.Offset, len(result))
	end := len(result)
	if query.Limit > 0 {
		end = min(start+query.Limit, end)
	}
	return result[start:end], nil
}

func (r *sessionRepositoryFake) Delete(_ context.Context, id uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.events[id]; !ok {
		return storage.ErrConversationNotFound
	}
	delete(r.events, id)
	return nil
}

func newServices(t *testing.T) (*application.AgentService, *application.ProviderService, *application.SessionService) {
	t.Helper()
	return application.NewAgentService(newAgentRepositoryFake()),
		application.NewProviderService(newProviderRepositoryFake()),
		application.NewSessionService(newSessionRepositoryFake())
}

func appErrorCode(err error) application.Code {
	if err == nil {
		return application.Code(-1)
	}
	var appErr *application.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return application.Code(-1)
}

func cloneMetadata(metadata shared.Metadata) shared.Metadata {
	if metadata == nil {
		return nil
	}
	copy := make(shared.Metadata, len(metadata))
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func ptr[T any](v T) *T { return &v }
