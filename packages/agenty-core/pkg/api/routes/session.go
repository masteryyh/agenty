package routes

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
)

type SessionRoutes struct {
	api       *API
	service   *application.SessionService
	execution *infrasession.Engine
}

func newSessionRoutes(api *API, service *application.SessionService, execution *infrasession.Engine) *SessionRoutes {
	return &SessionRoutes{api: api, service: service, execution: execution}
}

func (r *SessionRoutes) RegisterRoutes(v1 *gin.RouterGroup) {
	sessions := v1.Group("/sessions")
	sessions.POST("", r.Create)
	sessions.GET("", r.List)
	sessions.GET("/:id", r.Get)
	sessions.DELETE("/:id", r.Delete)
	sessions.PUT("/:id/title", r.SetTitle)
	sessions.PUT("/:id/model", r.SetModel)
	sessions.PUT("/:id/reasoning-effort", r.SetReasoningEffort)
	sessions.PUT("/:id/cwd", r.SetCwd)
	sessions.PUT("/:id/permission-mode", r.SetPermissionMode)
	sessions.PUT("/:id/tool-dialect", r.SetToolDialect)
	sessions.POST("/:id/codex-mode", r.EnableCodexMode)
	sessions.POST("/:id/rounds", r.Start)
	sessions.POST("/:id/rounds/:roundId/cancel", r.Stop)
	sessions.POST("/:id/compact", r.Compact)
}

func sessionMutationUsesRepository(method, route string) bool {
	switch method + " " + route {
	case "DELETE /v1/sessions/:id", "PUT /v1/sessions/:id/title", "PUT /v1/sessions/:id/reasoning-effort", "PUT /v1/sessions/:id/cwd":
		return true
	default:
		return false
	}
}

type idParams struct {
	ID string `json:"id"`
}

type sessionListParams struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

type sessionSetTitleParams struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type sessionSetModelParams struct {
	ID           string `json:"id"`
	ProviderCode string `json:"providerCode"`
	ModelCode    string `json:"modelCode"`
}

type sessionSetReasoningEffortParams struct {
	ID              string                 `json:"id"`
	ReasoningEffort shared.ReasoningEffort `json:"reasoningEffort"`
}

type sessionSetCwdParams struct {
	ID  string  `json:"id"`
	Cwd *string `json:"cwd"`
}

type sessionSetPermissionModeParams struct {
	ID             string                      `json:"id"`
	PermissionMode conversation.PermissionMode `json:"permissionMode"`
}

type sessionSetToolDialectParams struct {
	ID          string                   `json:"id"`
	ToolDialect conversation.ToolDialect `json:"toolDialect"`
}

type sessionStartParams struct {
	ID      string               `json:"id"`
	Content conversation.Content `json:"content"`
}

type sessionStopParams struct {
	ID      string `json:"id"`
	RoundID string `json:"roundId"`
}

func (r *SessionRoutes) Create(c *gin.Context) {
	execute(r.api, c, http.StatusCreated, nil, nil, func(ctx context.Context, p application.SessionCreateInput) (any, error) {
		return r.service.Create(ctx, p)
	})
}

func (r *SessionRoutes) Get(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p idParams) (any, error) {
		return r.service.Get(ctx, p.ID)
	})
}

func (r *SessionRoutes) List(c *gin.Context) {
	execute(r.api, c, http.StatusOK, nil, []string{"limit", "offset"}, func(ctx context.Context, p sessionListParams) (any, error) {
		return r.service.List(ctx, application.SessionListQuery{Limit: p.Limit, Offset: p.Offset})
	})
}

func (r *SessionRoutes) Delete(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p idParams) (any, error) {
		if err := r.service.Delete(ctx, p.ID); err != nil {
			return nil, err
		}
		return map[string]any{"id": p.ID, "deleted": true}, nil
	})
}

func (r *SessionRoutes) SetTitle(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionSetTitleParams) (any, error) {
		return r.service.SetTitle(ctx, p.ID, p.Title)
	})
}

func (r *SessionRoutes) SetModel(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionSetModelParams) (any, error) {
		return r.execution.SetModel(ctx, p.ID, p.ProviderCode, p.ModelCode)
	})
}

func (r *SessionRoutes) SetReasoningEffort(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionSetReasoningEffortParams) (any, error) {
		return r.service.SetReasoningEffort(ctx, p.ID, p.ReasoningEffort)
	})
}

func (r *SessionRoutes) SetCwd(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionSetCwdParams) (any, error) {
		return r.service.SetCwd(ctx, p.ID, p.Cwd)
	})
}

func (r *SessionRoutes) SetPermissionMode(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionSetPermissionModeParams) (any, error) {
		return r.execution.SetPermissionMode(ctx, p.ID, p.PermissionMode)
	})
}

func (r *SessionRoutes) EnableCodexMode(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p idParams) (any, error) {
		return r.execution.EnableCodexMode(ctx, p.ID)
	})
}

func (r *SessionRoutes) SetToolDialect(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionSetToolDialectParams) (any, error) {
		return r.execution.SetToolDialect(ctx, p.ID, p.ToolDialect)
	})
}

func (r *SessionRoutes) Start(c *gin.Context) {
	execute(r.api, c, http.StatusAccepted, map[string]string{"id": "id"}, nil, func(ctx context.Context, p sessionStartParams) (any, error) {
		return r.execution.Start(ctx, p.ID, p.Content)
	})
}

func (r *SessionRoutes) Compact(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p idParams) (any, error) {
		return r.execution.Compact(ctx, p.ID)
	})
}

func (r *SessionRoutes) Stop(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"id": "id", "roundId": "roundId"}, nil, func(_ context.Context, p sessionStopParams) (any, error) {
		if p.RoundID == "" {
			return nil, invalidParams("roundId is required")
		}
		return r.execution.StopRound(p.ID, p.RoundID)
	})
}
