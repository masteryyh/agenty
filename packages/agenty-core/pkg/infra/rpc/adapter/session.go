package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func RegisterSessionHandlers(
	d *rpc.Dispatcher,
	svc *application.SessionService,
	execution *agentloop.Engine,
) {
	d.Register("session.create", sessionCreate(svc))
	d.Register("session.get", sessionGet(svc))
	d.Register("session.list", sessionList(svc))
	d.Register("session.delete", sessionDelete(svc))
	d.Register("session.setTitle", sessionSetTitle(svc))
	d.Register("session.setModel", sessionSetModel(execution))
	d.Register("session.setReasoningEffort", sessionSetReasoningEffort(svc))
	d.Register("session.setCwd", sessionSetCwd(svc))
	d.Register("session.start", sessionStart(execution))
	d.Register("session.compact", sessionCompact(execution))
	d.Register("session.stop", sessionStop(execution))
}

type idParams struct {
	ID string `json:"id"`
}

func sessionCreate(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p application.SessionCreateInput
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Create(ctx, p))
	}
}

func sessionGet(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p idParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Get(ctx, p.ID))
	}
}

type sessionListParams struct {
	AgentSlug string `json:"agentSlug,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func sessionList(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p sessionListParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.List(ctx, application.SessionListQuery{
			AgentSlug: p.AgentSlug,
			Limit:     p.Limit,
			Offset:    p.Offset,
		}))
	}
}

func sessionDelete(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p idParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		if err := svc.Delete(ctx, p.ID); err != nil {
			return nil, toRPCError(err)
		}
		return map[string]any{"id": p.ID, "deleted": true}, nil
	}
}

type sessionSetTitleParams struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func sessionSetTitle(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p sessionSetTitleParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.SetTitle(ctx, p.ID, p.Title))
	}
}

type sessionSetModelParams struct {
	ID           string `json:"id"`
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
}

func sessionSetModel(execution *agentloop.Engine) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p sessionSetModelParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(execution.SetModel(ctx, p.ID, p.ProviderSlug, p.ModelSlug))
	}
}

type sessionSetReasoningEffortParams struct {
	ID              string                 `json:"id"`
	ReasoningEffort shared.ReasoningEffort `json:"reasoningEffort"`
}

func sessionSetReasoningEffort(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p sessionSetReasoningEffortParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.SetReasoningEffort(ctx, p.ID, p.ReasoningEffort))
	}
}

type sessionSetCwdParams struct {
	ID  string  `json:"id"`
	Cwd *string `json:"cwd"`
}

func sessionSetCwd(svc *application.SessionService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p sessionSetCwdParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.SetCwd(ctx, p.ID, p.Cwd))
	}
}

type sessionStartParams struct {
	ID      string               `json:"id"`
	Content conversation.Content `json:"content"`
}

func sessionStart(execution *agentloop.Engine) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p sessionStartParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(execution.Start(ctx, p.ID, p.Content))
	}
}

func sessionCompact(execution *agentloop.Engine) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p idParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(execution.Compact(ctx, p.ID))
	}
}

func sessionStop(execution *agentloop.Engine) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p idParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(execution.Stop(ctx, p.ID))
	}
}
