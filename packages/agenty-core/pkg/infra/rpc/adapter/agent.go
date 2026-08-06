package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

// slugParams identifies a resource by its slug.
type slugParams struct {
	Slug string `json:"slug"`
}

// RegisterAgentHandlers registers agent.* methods on d.
func RegisterAgentHandlers(d *rpc.Dispatcher, svc *application.AgentService) {
	d.Register("agent.create", agentCreate(svc))
	d.Register("agent.get", agentGet(svc))
	d.Register("agent.list", agentList(svc))
	d.Register("agent.update", agentUpdate(svc))
	d.Register("agent.delete", agentDelete(svc))
}

type agentCreateParams struct {
	Slug string `json:"slug"`
	application.AgentInput
}

func agentCreate(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p agentCreateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Create(ctx, p.Slug, p.AgentInput))
	}
}

func agentGet(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p slugParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Get(ctx, p.Slug))
	}
}

func agentList(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct{}
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.List(ctx))
	}
}

type agentUpdateParams struct {
	Slug string `json:"slug"`
	application.AgentUpdate
}

func agentUpdate(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p agentUpdateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Update(ctx, p.Slug, p.AgentUpdate))
	}
}

func agentDelete(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p slugParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		if err := svc.Delete(ctx, p.Slug); err != nil {
			return nil, toRPCError(err)
		}
		return map[string]any{"slug": p.Slug, "deleted": true}, nil
	}
}
