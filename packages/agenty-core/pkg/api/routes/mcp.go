package routes

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/mcp"
)

type MCPRoutes struct {
	api      *API
	registry *mcp.Registry
}

func newMCPRoutes(api *API, registry *mcp.Registry) *MCPRoutes {
	return &MCPRoutes{api: api, registry: registry}
}

func (r *MCPRoutes) RegisterRoutes(v1 *gin.RouterGroup) {
	group := v1.Group("/mcp")
	group.GET("", r.List)
	group.POST("", r.Create)
	group.GET("/:name", r.Get)
	group.PUT("/:name", r.Update)
	group.DELETE("/:name", r.Remove)
	group.GET("/:name/logs", r.Logs)
	group.PUT("/:name/enabled", r.Enable)
	group.POST("/:name/reconnect", r.Reconnect)
	group.POST("/:name/login", r.Login)
	group.POST("/:name/logout", r.Logout)
}

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

func (r *MCPRoutes) List(c *gin.Context) {
	execute(r.api, c, http.StatusOK, nil, nil, func(ctx context.Context, _ struct{}) (any, error) {
		return r.registry.List(ctx), nil
	})
}

func (r *MCPRoutes) Get(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpNameParams) (any, error) {
		server, ok := r.registry.Get(ctx, p.Name)
		if !ok {
			return nil, notFound("MCP server not found")
		}
		return server, nil
	})
}

func (r *MCPRoutes) Logs(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpNameParams) (any, error) {
		logs, ok := r.registry.Logs(ctx, p.Name)
		if !ok {
			return nil, notFound("MCP server not found")
		}
		return logs, nil
	})
}

func (r *MCPRoutes) Create(c *gin.Context) {
	execute(r.api, c, http.StatusCreated, nil, nil, func(ctx context.Context, p mcpConfigParams) (any, error) {
		value, err := r.registry.Create(ctx, p.Name, p.Config)
		return value, mcpError(err)
	})
}

func (r *MCPRoutes) Update(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpConfigParams) (any, error) {
		value, err := r.registry.Update(ctx, p.Name, p.Config)
		return value, mcpError(err)
	})
}

func (r *MCPRoutes) Enable(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpEnabledParams) (any, error) {
		value, err := r.registry.SetEnabled(ctx, p.Name, p.Enabled)
		return value, mcpError(err)
	})
}

func (r *MCPRoutes) Reconnect(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpNameParams) (any, error) {
		value, err := r.registry.Reconnect(ctx, p.Name)
		return value, mcpError(err)
	})
}

func (r *MCPRoutes) Login(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpNameParams) (any, error) {
		value, err := r.registry.Login(ctx, p.Name)
		return value, mcpError(err)
	})
}

func (r *MCPRoutes) Logout(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpNameParams) (any, error) {
		value, err := r.registry.Logout(ctx, p.Name)
		return value, mcpError(err)
	})
}

func (r *MCPRoutes) Remove(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"name": "name"}, nil, func(ctx context.Context, p mcpNameParams) (any, error) {
		if err := r.registry.Remove(ctx, p.Name); err != nil {
			return nil, mcpError(err)
		}
		return map[string]any{"name": p.Name, "removed": true}, nil
	})
}

func mcpError(err error) error {
	if err == nil {
		return nil
	}
	var registryErr *mcp.RegistryError
	if errors.As(err, &registryErr) {
		switch registryErr.Kind {
		case mcp.RegistryErrorValidation:
			return invalidParams(registryErr.Error())
		case mcp.RegistryErrorNotFound:
			return notFound(registryErr.Error())
		case mcp.RegistryErrorAlreadyExists:
			return &APIError{Status: http.StatusConflict, Code: "already_exists", Message: registryErr.Error()}
		}
	}
	return toAPIError(err)
}
