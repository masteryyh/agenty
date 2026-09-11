package adapter

import (
	"context"
	"encoding/json"
	"errors"

	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

type mcpNameParams struct {
	Name string `json:"name"`
}

type mcpEnabledParams struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type mcpConfigParams struct {
	Name   string           `json:"name"`
	Config domainmcp.Config `json:"config"`
}

func RegisterMCPHandlers(d *rpc.Dispatcher, registry *mcp.Registry) {
	d.Register("mcp.list", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return registry.List(ctx), nil
	})
	d.Register("mcp.get", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpNameParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		server, ok := registry.Get(ctx, input.Name)
		if !ok {
			return nil, rpc.NewError(rpc.ErrCodeNotFound, "MCP server not found", nil)
		}
		return server, nil
	})
	d.Register("mcp.logs", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpNameParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		logs, ok := registry.Logs(ctx, input.Name)
		if !ok {
			return nil, rpc.NewError(rpc.ErrCodeNotFound, "MCP server not found", nil)
		}
		return logs, nil
	})
	d.Register("mcp.create", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpConfigParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrapMCP(registry.Create(ctx, input.Name, input.Config))
	})
	d.Register("mcp.update", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpConfigParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrapMCP(registry.Update(ctx, input.Name, input.Config))
	})
	d.Register("mcp.enable", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpEnabledParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrapMCP(registry.SetEnabled(ctx, input.Name, input.Enabled))
	})
	d.Register("mcp.reconnect", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpNameParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrapMCP(registry.Reconnect(ctx, input.Name))
	})
	d.Register("mcp.login", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpNameParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrapMCP(registry.Login(ctx, input.Name))
	})
	d.Register("mcp.logout", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpNameParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		return wrapMCP(registry.Logout(ctx, input.Name))
	})
	d.Register("mcp.remove", func(ctx context.Context, params json.RawMessage) (any, error) {
		var input mcpNameParams
		if err := decodeParams(params, &input); err != nil {
			return nil, rpc.InvalidParams("invalid params: " + err.Error())
		}
		if err := registry.Remove(ctx, input.Name); err != nil {
			return nil, mcpToRPCError(err)
		}
		return map[string]any{"name": input.Name, "removed": true}, nil
	})
}

func wrapMCP(value any, err error) (any, error) {
	if err != nil {
		return nil, mcpToRPCError(err)
	}
	return value, nil
}

func mcpToRPCError(err error) *rpc.Error {
	var registryErr *mcp.RegistryError
	if errors.As(err, &registryErr) {
		switch registryErr.Kind {
		case mcp.RegistryErrorValidation:
			return rpc.NewError(rpc.ErrCodeInvalidParams, registryErr.Error(), nil)
		case mcp.RegistryErrorNotFound:
			return rpc.NewError(rpc.ErrCodeNotFound, registryErr.Error(), nil)
		case mcp.RegistryErrorAlreadyExists:
			return rpc.NewError(rpc.ErrCodeAlreadyExists, registryErr.Error(), nil)
		}
	}
	return toRPCError(err)
}
