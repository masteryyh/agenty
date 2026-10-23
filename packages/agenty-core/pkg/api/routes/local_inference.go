package routes

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/application"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	localruntime "github.com/masteryyh/agenty-core/pkg/infra/localinference"
)

type localInferenceRoutes struct {
	api       *API
	manager   *localruntime.Manager
	installer *localruntime.Installer
	config    *config.Manager
}

func registerLocalInferenceRoutes(v1 *gin.RouterGroup, api *API, deps Dependencies) {
	if deps.LocalInference == nil {
		return
	}
	r := &localInferenceRoutes{api: api, manager: deps.LocalInference, installer: deps.WorkerInstaller, config: deps.Config}
	group := v1.Group("/local-inference")
	group.GET("/config", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(context.Context, struct{}) (any, error) { return r.config.LocalInference(), nil })
	})
	group.PATCH("/config", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(ctx context.Context, patch struct {
			Enabled *bool            `json:"enabled"`
			Workers *[]domain.Worker `json:"workers"`
		}) (any, error) {
			p, err := r.manager.UpdateConfig(r.config, func(value *domain.Config) {
				if patch.Enabled != nil {
					value.Enabled = *patch.Enabled
				}
				if patch.Workers != nil {
					value.Workers = *patch.Workers
				}
			})
			if err != nil {
				if errors.Is(err, localruntime.ErrInvalidConfig) {
					return nil, application.Validation(err.Error())
				}
				return nil, application.Internal(err.Error())
			}
			return p, nil
		})
	})
	group.GET("/runtime", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(context.Context, struct{}) (any, error) { return r.manager.Snapshot(), nil })
	})
	group.GET("/workers", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(context.Context, struct{}) (any, error) { return r.config.LocalInference().Workers, nil })
	})
	group.GET("/worker-bundles", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(ctx context.Context, _ struct{}) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			bundles, err := localruntime.AvailableBundles(ctx)
			if err != nil {
				return nil, application.Internal(err.Error())
			}
			return bundles, nil
		})
	})
	group.GET("/installations", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(context.Context, struct{}) (any, error) { return r.installer.List(), nil })
	})
	group.GET("/installations/:id", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p struct {
			ID string `json:"id"`
		}) (any, error) {
			for _, state := range r.installer.List() {
				if state.ID == p.ID {
					return state, nil
				}
			}
			return nil, application.NotFound("installation not found")
		})
	})
	group.POST("/installations", func(c *gin.Context) {
		execute(r.api, c, http.StatusAccepted, nil, nil, func(ctx context.Context, p localruntime.InstallInput) (any, error) {
			result, err := r.installer.Start(p)
			if err != nil {
				return nil, application.Validation(err.Error())
			}
			return result, nil
		})
	})
	group.DELETE("/installations/:id", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p struct {
			ID string `json:"id"`
		}) (any, error) {
			if err := r.installer.Cancel(p.ID); err != nil {
				return nil, application.NotFound(err.Error())
			}
			return struct{}{}, nil
		})
	})
	group.POST("/model-probes", func(c *gin.Context) {
		execute(r.api, c, http.StatusOK, nil, nil, func(ctx context.Context, p domain.Model) (any, error) {
			probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			resource, err := r.manager.Probe(probeCtx, &p)
			if err != nil {
				return nil, application.Validation(err.Error())
			}
			return map[string]any{"compatible": true, "resource": resource}, nil
		})
	})
}
