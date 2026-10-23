package routes

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/infra/modelhub"
)

func modelHubError(err error) error {
	if err == nil {
		return nil
	}
	var space *modelhub.SpaceError
	if errors.As(err, &space) {
		return &APIError{Status: http.StatusConflict, Code: "insufficient_disk_space", Message: err.Error()}
	}
	return invalidParams(err.Error())
}
func registerModelHubRoutes(v1 *gin.RouterGroup, api *API, manager *modelhub.Manager) {
	if manager == nil {
		return
	}
	group := v1.Group("/local-inference")
	group.GET("/model-storage", func(c *gin.Context) {
		execute(api, c, http.StatusOK, nil, nil, func(ctx context.Context, _ struct{}) (any, error) {
			value, err := manager.Storage()
			return value, modelHubError(err)
		})
	})
	group.GET("/hub-models", func(c *gin.Context) {
		execute(api, c, http.StatusOK, nil, []string{"source", "search", "cursor"}, func(ctx context.Context, p struct {
			Source modelhub.Source `json:"source"`
			Search string          `json:"search"`
			Cursor string          `json:"cursor"`
		}) (any, error) { value, err := manager.Catalog.Search(ctx, p.Source, p.Search, p.Cursor); return value, modelHubError(err) })
	})
	group.GET("/hub-model", func(c *gin.Context) {
		execute(api, c, http.StatusOK, nil, []string{"source", "repository"}, func(ctx context.Context, p struct {
			Source     modelhub.Source `json:"source"`
			Repository string          `json:"repository"`
		}) (any, error) { value, err := manager.Catalog.Inspect(ctx, p.Source, p.Repository); return value, modelHubError(err) })
	})
	group.POST("/model-download-plans", func(c *gin.Context) {
		execute(api, c, http.StatusOK, nil, nil, func(ctx context.Context, p modelhub.PlanInput) (any, error) {
			value, err := manager.Prepare(ctx, p)
			return value, modelHubError(err)
		})
	})
	group.GET("/model-downloads", func(c *gin.Context) {
		execute(api, c, http.StatusOK, nil, nil, func(ctx context.Context, _ struct{}) (any, error) { return manager.List(), nil })
	})
	group.POST("/model-downloads", func(c *gin.Context) {
		execute(api, c, http.StatusAccepted, nil, nil, func(ctx context.Context, p struct {
			PlanID       string `json:"planId"`
			DownloadOnly bool   `json:"downloadOnly"`
		}) (any, error) { value, err := manager.Start(ctx, p.PlanID, p.DownloadOnly); return value, modelHubError(err) })
	})
	group.DELETE("/model-downloads/:id", func(c *gin.Context) {
		execute(api, c, http.StatusOK, map[string]string{"id": "id"}, nil, func(ctx context.Context, p struct {
			ID string `json:"id"`
		}) (any, error) { if err := manager.Cancel(p.ID); err != nil {
			return nil, notFound(err.Error())
		}; return struct{}{}, nil })
	})
}
