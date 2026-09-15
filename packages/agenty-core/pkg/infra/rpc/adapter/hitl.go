package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/infra/hitl"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func RegisterHITLHandlers(dispatcher *rpc.Dispatcher, manager *hitl.Manager) {
	dispatcher.Register("session.resolveToolApproval", resolveToolApproval(manager))
}

func resolveToolApproval(manager *hitl.Manager) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var resolution hitl.Resolution
		if err := decodeParams(params, &resolution); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		if err := manager.Resolve(ctx, resolution); err != nil {
			return nil, toRPCError(err)
		}
		return resolution, nil
	}
}
