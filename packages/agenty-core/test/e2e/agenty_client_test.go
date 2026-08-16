//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"time"
)

type agentyClient struct {
	rpc *rpcClient
}

func newAgentyClient(process *coreProcess) *agentyClient {
	return &agentyClient{rpc: newRPCClient(process)}
}

func (c *agentyClient) InitializeAlready(ctx context.Context) (InitializeResult, error) {
	return callResult[InitializeResult](ctx, c.rpc, "initialize.already", struct{}{})
}

func (c *agentyClient) CompleteInitialization(
	ctx context.Context,
	agentSlug, providerSlug, modelSlug string,
) (InitializeResult, error) {
	return callResult[InitializeResult](ctx, c.rpc, "initialize.complete", map[string]any{
		"agentSlug":    agentSlug,
		"providerSlug": providerSlug,
		"modelSlug":    modelSlug,
	})
}

func (c *agentyClient) CreateAgent(ctx context.Context, input AgentCreateInput) (Agent, error) {
	return callResult[Agent](
		ctx,
		c.rpc,
		"agent.create",
		input,
	)
}

func (c *agentyClient) GetAgent(ctx context.Context, slug string) (Agent, error) {
	return callResult[Agent](
		ctx,
		c.rpc,
		"agent.get",
		map[string]any{"slug": slug},
	)
}

func (c *agentyClient) ListAgents(ctx context.Context) ([]Agent, error) {
	return callResult[[]Agent](
		ctx,
		c.rpc,
		"agent.list",
		struct{}{},
	)
}

func (c *agentyClient) UpdateAgent(ctx context.Context, input AgentUpdateInput) (Agent, error) {
	return callResult[Agent](
		ctx,
		c.rpc,
		"agent.update",
		input,
	)
}

func (c *agentyClient) DeleteAgent(ctx context.Context, slug string) (DeleteResult, error) {
	return callResult[DeleteResult](
		ctx,
		c.rpc,
		"agent.delete",
		map[string]any{"slug": slug},
	)
}

func (c *agentyClient) CreateProvider(ctx context.Context, input ProviderCreateInput) (Provider, error) {
	return callResult[Provider](
		ctx,
		c.rpc,
		"provider.create",
		input,
	)
}

func (c *agentyClient) GetProvider(ctx context.Context, slug string) (Provider, error) {
	return callResult[Provider](
		ctx,
		c.rpc,
		"provider.get",
		map[string]any{"slug": slug},
	)
}

func (c *agentyClient) ListProviders(ctx context.Context) ([]Provider, error) {
	return callResult[[]Provider](
		ctx,
		c.rpc,
		"provider.list",
		struct{}{},
	)
}

func (c *agentyClient) UpdateProvider(ctx context.Context, input ProviderUpdateInput) (Provider, error) {
	return callResult[Provider](
		ctx,
		c.rpc,
		"provider.update",
		input,
	)
}

func (c *agentyClient) DeleteProvider(ctx context.Context, slug string) (DeleteResult, error) {
	return callResult[DeleteResult](
		ctx,
		c.rpc,
		"provider.delete",
		map[string]any{"slug": slug},
	)
}

func (c *agentyClient) AddModel(ctx context.Context, input ModelInput) (Provider, error) {
	return callResult[Provider](
		ctx,
		c.rpc,
		"provider.addModel",
		input,
	)
}

func (c *agentyClient) RemoveModel(ctx context.Context, providerSlug, modelSlug string) (Provider, error) {
	return callResult[Provider](
		ctx,
		c.rpc,
		"provider.removeModel",
		map[string]any{
			"providerSlug": providerSlug,
			"modelSlug":    modelSlug,
		},
	)
}

func (c *agentyClient) CreateSession(ctx context.Context, input SessionCreateInput) (Session, error) {
	return callResult[Session](
		ctx,
		c.rpc,
		"session.create",
		input,
	)
}

func (c *agentyClient) GetSession(ctx context.Context, id string) (Session, error) {
	return callResult[Session](
		ctx,
		c.rpc,
		"session.get",
		map[string]any{"id": id},
	)
}

func (c *agentyClient) ListSessions(ctx context.Context, input SessionListInput) ([]SessionSummary, error) {
	return callResult[[]SessionSummary](
		ctx,
		c.rpc,
		"session.list",
		input,
	)
}

func (c *agentyClient) DeleteSession(ctx context.Context, id string) (DeleteResult, error) {
	return callResult[DeleteResult](
		ctx,
		c.rpc,
		"session.delete",
		map[string]any{"id": id},
	)
}

func (c *agentyClient) SetSessionTitle(ctx context.Context, id, title string) (Session, error) {
	return callResult[Session](
		ctx,
		c.rpc,
		"session.setTitle",
		map[string]any{
			"id":    id,
			"title": title,
		},
	)
}

func (c *agentyClient) SetSessionModel(
	ctx context.Context,
	id string,
	model ModelRef,
	contextWindow int64,
) (Session, error) {
	return callResult[Session](
		ctx,
		c.rpc,
		"session.setModel",
		map[string]any{
			"id":            id,
			"providerSlug":  model.ProviderSlug,
			"modelSlug":     model.ModelSlug,
			"contextWindow": contextWindow,
		},
	)
}

func (c *agentyClient) SetSessionReasoningEffort(
	ctx context.Context,
	id, reasoningEffort string,
) (Session, error) {
	return callResult[Session](
		ctx,
		c.rpc,
		"session.setReasoningEffort",
		map[string]any{
			"id":              id,
			"reasoningEffort": reasoningEffort,
		},
	)
}

func (c *agentyClient) SetSessionCwd(ctx context.Context, id string, cwd *string) (Session, error) {
	return callResult[Session](
		ctx,
		c.rpc,
		"session.setCwd",
		map[string]any{
			"id":  id,
			"cwd": cwd,
		},
	)
}

func (c *agentyClient) StartSession(
	ctx context.Context,
	id string,
	content []ContentInput,
) (ExecutionStart, error) {
	return callResult[ExecutionStart](
		ctx,
		c.rpc,
		"session.start",
		map[string]any{
			"id":      id,
			"content": content,
		},
	)
}

func (c *agentyClient) StopSession(ctx context.Context, id string) (ExecutionStop, error) {
	return callResult[ExecutionStop](
		ctx,
		c.rpc,
		"session.stop",
		map[string]any{"id": id},
	)
}

func (c *agentyClient) WaitForRoundStatus(
	ctx context.Context,
	sessionID string,
	roundID string,
	want string,
) (Session, error) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		session, err := c.GetSession(ctx, sessionID)
		if err != nil {
			return Session{}, err
		}
		for _, round := range session.Rounds {
			if round.ID != roundID {
				continue
			}
			if round.Status == want {
				return session, nil
			}
			if round.Status == "completed" || round.Status == "failed" || round.Status == "cancelled" {
				errorText := ""
				if round.Error != nil {
					errorText = *round.Error
				}
				return Session{}, fmt.Errorf(
					"session %s round %s reached %q while waiting for %q: %s",
					sessionID,
					roundID,
					round.Status,
					want,
					errorText,
				)
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return Session{}, fmt.Errorf(
				"session %s round %s did not reach %q: %w",
				sessionID,
				roundID,
				want,
				ctx.Err(),
			)
		}
	}
}

func (c *agentyClient) WaitForRoundEvent(
	ctx context.Context,
	sessionID string,
	roundID string,
	want string,
) (SessionEvent, error) {
	var lastSequence uint64
	for {
		select {
		case event := <-c.rpc.SessionEvents():
			if event.SessionID != sessionID || event.RoundID != roundID {
				continue
			}
			if event.Sequence != lastSequence+1 {
				return SessionEvent{}, fmt.Errorf(
					"session %s round %s event sequence = %d after %d",
					sessionID,
					roundID,
					event.Sequence,
					lastSequence,
				)
			}
			lastSequence = event.Sequence
			if event.Type == want {
				return event, nil
			}
		case <-ctx.Done():
			return SessionEvent{}, fmt.Errorf(
				"session %s round %s did not emit %q: %w",
				sessionID,
				roundID,
				want,
				ctx.Err(),
			)
		}
	}
}

func callResult[T any](ctx context.Context, client *rpcClient, method string, params any) (T, error) {
	var result T
	if err := client.Call(
		ctx,
		method,
		params,
		&result,
	); err != nil {
		return result, err
	}
	return result, nil
}
