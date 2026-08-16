package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func RegisterInitializeHandlers(d *rpc.Dispatcher, svc *application.InitializeService) {
	d.Register("initialize.already", initializeAlready(svc))
	d.Register("initialize.complete", initializeComplete(svc))
}

func initializeAlready(svc *application.InitializeService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct{}
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return svc.Already(ctx), nil
	}
}

func initializeComplete(svc *application.InitializeService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p application.InitializeCompleteInput
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Complete(ctx, p))
	}
}
