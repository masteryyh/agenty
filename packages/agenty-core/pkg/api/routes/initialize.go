package routes

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/application"
)

type InitializationRoutes struct {
	api     *API
	service *application.InitializeService
}

func newInitializationRoutes(api *API, service *application.InitializeService) *InitializationRoutes {
	return &InitializationRoutes{api: api, service: service}
}

func (r *InitializationRoutes) RegisterRoutes(v1 *gin.RouterGroup) {
	v1.GET("/initialization", r.Already)
	v1.POST("/initialization", r.Complete)
}

func (r *InitializationRoutes) Already(c *gin.Context) {
	execute(r.api, c, http.StatusOK, nil, nil, func(ctx context.Context, _ struct{}) (any, error) {
		return r.service.Already(ctx), nil
	})
}

func (r *InitializationRoutes) Complete(c *gin.Context) {
	execute(r.api, c, http.StatusOK, nil, nil, func(ctx context.Context, input application.InitializeCompleteInput) (any, error) {
		return r.service.Complete(ctx, input)
	})
}
