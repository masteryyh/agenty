package agentloop_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

type agentRepositoryFake struct {
	agents map[shared.Code]*agent.Agent
}

func newAgentRepositoryFake() *agentRepositoryFake {
	return &agentRepositoryFake{agents: make(map[shared.Code]*agent.Agent)}
}

func (repository *agentRepositoryFake) Get(
	_ context.Context,
	code shared.Code,
) (*agent.Agent, error) {
	definition, ok := repository.agents[code]
	if !ok {
		return nil, storage.ErrAgentNotFound
	}

	copy := *definition
	copy.Metadata = maps.Clone(definition.Metadata)
	return &copy, nil
}

func (repository *agentRepositoryFake) Save(_ context.Context, definition *agent.Agent) error {
	copy := *definition
	copy.Metadata = maps.Clone(definition.Metadata)
	repository.agents[definition.Code] = &copy
	return nil
}

type providerRepositoryFake struct {
	providers map[shared.Code]*catalog.Provider
}

func newProviderRepositoryFake() *providerRepositoryFake {
	return &providerRepositoryFake{providers: make(map[shared.Code]*catalog.Provider)}
}

func (repository *providerRepositoryFake) Get(
	_ context.Context,
	code shared.Code,
) (*catalog.Provider, error) {
	provider, ok := repository.providers[code]
	if !ok {
		return nil, storage.ErrProviderNotFound
	}

	return cloneProvider(provider), nil
}

func (repository *providerRepositoryFake) Save(_ context.Context, provider *catalog.Provider) error {
	repository.providers[provider.Code] = cloneProvider(provider)
	return nil
}

func cloneProvider(provider *catalog.Provider) *catalog.Provider {
	copy := *provider
	copy.Models = slices.Clone(provider.Models)
	for index := range copy.Models {
		copy.Models[index].ReasoningEffortMapping = maps.Clone(
			copy.Models[index].ReasoningEffortMapping,
		)
	}
	copy.Metadata = maps.Clone(provider.Metadata)
	return &copy
}

type sessionRepositoryFake struct {
	mu            sync.RWMutex
	events        map[uuid.UUID][]shared.Event
	loadAttempted chan struct{}
	deleteStarted chan struct{}
	deleteRelease chan struct{}
}

func newSessionRepositoryFake() *sessionRepositoryFake {
	return &sessionRepositoryFake{events: make(map[uuid.UUID][]shared.Event)}
}

func (repository *sessionRepositoryFake) Load(
	_ context.Context,
	id uuid.UUID,
) (*conversation.Session, error) {
	if repository.loadAttempted != nil {
		close(repository.loadAttempted)
	}

	repository.mu.RLock()
	defer repository.mu.RUnlock()

	events, ok := repository.events[id]
	if !ok {
		return nil, storage.ErrConversationNotFound
	}

	return conversation.ReplaySession(events), nil
}

func (repository *sessionRepositoryFake) Save(
	_ context.Context,
	session *conversation.Session,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.events[session.ID] = append(
		repository.events[session.ID],
		session.PendingEvents()...,
	)
	return nil
}

func (repository *sessionRepositoryFake) List(
	_ context.Context,
	_ conversation.ListQuery,
) ([]conversation.SessionSummary, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	result := make([]conversation.SessionSummary, 0, len(repository.events))
	for _, events := range repository.events {
		result = append(result, conversation.ReplaySession(events).Summary())
	}
	return result, nil
}

func (repository *sessionRepositoryFake) Delete(_ context.Context, id uuid.UUID) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	if repository.deleteStarted != nil {
		close(repository.deleteStarted)
	}
	if repository.deleteRelease != nil {
		<-repository.deleteRelease
	}
	if _, ok := repository.events[id]; !ok {
		return storage.ErrConversationNotFound
	}

	delete(repository.events, id)
	return nil
}

func appErrorCode(err error) application.Code {
	if err == nil {
		return application.Code(-1)
	}

	var applicationError *application.Error
	if errors.As(err, &applicationError) {
		return applicationError.Code
	}
	return application.Code(-1)
}

func ptr[T any](value T) *T {
	return &value
}
