package adapter

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

// RegisterProviderHandlers registers provider.* methods on d.
func RegisterProviderHandlers(d *rpc.Dispatcher, svc *application.ProviderService) {
	d.Register("provider.create", providerCreate(svc))
	d.Register("provider.get", providerGet(svc))
	d.Register("provider.list", providerList(svc))
	d.Register("provider.listModels", providerListModels(svc))
	d.Register("provider.update", providerUpdate(svc))
	d.Register("provider.delete", providerDelete(svc))
	d.Register("provider.addModel", providerAddModel(svc))
	d.Register("provider.removeModel", providerRemoveModel(svc))
}

type providerCreateParams struct {
	Code string `json:"code"`
	application.ProviderInput
}

type providerListParams struct {
	ProviderCode string `json:"providerCode,omitempty"`
}

func providerCreate(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerCreateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Create(ctx, p.Code, p.ProviderInput))
	}
}

func providerGet(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p codeParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Get(ctx, p.Code))
	}
}

func providerList(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerListParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.List(ctx, p.ProviderCode))
	}
}

type providerListModelsParams struct {
	ProviderCode string `json:"providerCode"`
}

func providerListModels(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerListModelsParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.ListModels(ctx, p.ProviderCode))
	}
}

type providerUpdateParams struct {
	Code string `json:"code"`
	application.ProviderUpdate
}

func providerUpdate(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerUpdateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Update(ctx, p.Code, p.ProviderUpdate))
	}
}

func providerDelete(svc *application.ProviderService) rpc.Handler {
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

// modelTargetParams identifies a model within a provider.
type modelTargetParams struct {
	ProviderCode string `json:"providerCode"`
	ModelCode    string `json:"modelCode"`
}

type providerAddModelParams struct {
	ProviderCode string `json:"providerCode"`
	ModelCode    string `json:"modelCode"`
	application.ModelInput
}

func providerAddModel(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerAddModelParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.AddModel(ctx, p.ProviderCode, p.ModelCode, p.ModelInput))
	}
}

func providerRemoveModel(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p modelTargetParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.RemoveModel(ctx, p.ProviderCode, p.ModelCode))
	}
}
