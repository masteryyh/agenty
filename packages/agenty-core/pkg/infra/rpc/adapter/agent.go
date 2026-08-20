package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

// codeParams identifies a resource by its code.
type codeParams struct {
	Code string `json:"code"`
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
	Code string `json:"code"`
	application.AgentInput
}

func agentCreate(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p agentCreateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Create(ctx, p.Code, p.AgentInput))
	}
}

func agentGet(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p codeParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Get(ctx, p.Code))
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
	Code string `json:"code"`
	application.AgentUpdate
}

func agentUpdate(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p agentUpdateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Update(ctx, p.Code, p.AgentUpdate))
	}
}

func agentDelete(svc *application.AgentService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p codeParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		if err := svc.Delete(ctx, p.Code); err != nil {
			return nil, toRPCError(err)
		}
		return map[string]any{"code": p.Code, "deleted": true}, nil
	}
}
