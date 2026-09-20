package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/infra/permission"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func RegisterHITLHandlers(dispatcher *rpc.Dispatcher, manager *permission.PermissionManager) {
	dispatcher.Register("session.resolveToolApproval", resolveToolApproval(manager))
}

func resolveToolApproval(manager *permission.PermissionManager) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var resolution permission.Resolution
		if err := decodeParams(params, &resolution); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		if err := manager.Resolve(ctx, resolution); err != nil {
			return nil, toRPCError(err)
		}
		return resolution, nil
	}
}
