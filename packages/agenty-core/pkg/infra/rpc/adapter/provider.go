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
	d.Register("provider.update", providerUpdate(svc))
	d.Register("provider.delete", providerDelete(svc))
	d.Register("provider.addModel", providerAddModel(svc))
	d.Register("provider.removeModel", providerRemoveModel(svc))
}

type providerCreateParams struct {
	Slug string `json:"slug"`
	application.ProviderInput
}

func providerCreate(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerCreateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Create(ctx, p.Slug, p.ProviderInput))
	}
}

func providerGet(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p slugParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Get(ctx, p.Slug))
	}
}

func providerList(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct{}
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.List(ctx))
	}
}

type providerUpdateParams struct {
	Slug string `json:"slug"`
	application.ProviderUpdate
}

func providerUpdate(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerUpdateParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.Update(ctx, p.Slug, p.ProviderUpdate))
	}
}

func providerDelete(svc *application.ProviderService) rpc.Handler {
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

// modelTargetParams identifies a model within a provider.
type modelTargetParams struct {
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
}

type providerAddModelParams struct {
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
	application.ModelInput
}

func providerAddModel(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p providerAddModelParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.AddModel(ctx, p.ProviderSlug, p.ModelSlug, p.ModelInput))
	}
}

func providerRemoveModel(svc *application.ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		var p modelTargetParams
		if err := decodeParams(params, &p); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrap(svc.RemoveModel(ctx, p.ProviderSlug, p.ModelSlug))
	}
}
