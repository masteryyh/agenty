package routes

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/application"
)

type ProviderRoutes struct {
	api     *API
	service *application.ProviderService
}

func newProviderRoutes(api *API, service *application.ProviderService) *ProviderRoutes {
	return &ProviderRoutes{api: api, service: service}
}

func (r *ProviderRoutes) RegisterRoutes(v1 *gin.RouterGroup) {
	providers := v1.Group("/providers")
	providers.GET("", r.List)
	providers.POST("", r.Create)
	providers.GET("/:code", r.Get)
	providers.PATCH("/:code", r.Update)
	providers.DELETE("/:code", r.Delete)
	providers.GET("/:code/models", r.ListModels)
	providers.POST("/:code/models", r.AddModel)
	providers.DELETE("/:code/models/:modelCode", r.RemoveModel)
}

type codeParams struct {
	Code string `json:"code"`
}

type providerCreateParams struct {
	Code string `json:"code"`
	application.ProviderInput
}

type providerListParams struct {
	ProviderCode string `json:"providerCode,omitempty"`
}

type providerListModelsParams struct {
	ProviderCode string `json:"providerCode"`
}

type providerUpdateParams struct {
	Code string `json:"code"`
	application.ProviderUpdate
}

type modelTargetParams struct {
	ProviderCode string `json:"providerCode"`
	ModelCode    string `json:"modelCode"`
}

type providerAddModelParams struct {
	ProviderCode string `json:"providerCode"`
	ModelCode    string `json:"modelCode"`
	application.ModelInput
}

func (r *ProviderRoutes) Create(c *gin.Context) {
	execute(r.api, c, http.StatusCreated, nil, nil, func(ctx context.Context, p providerCreateParams) (any, error) {
		return r.service.Create(ctx, p.Code, p.ProviderInput)
	})
}

func (r *ProviderRoutes) Get(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"code": "code"}, nil, func(ctx context.Context, p codeParams) (any, error) {
		return r.service.Get(ctx, p.Code)
	})
}

func (r *ProviderRoutes) List(c *gin.Context) {
	execute(r.api, c, http.StatusOK, nil, []string{"providerCode"}, func(ctx context.Context, p providerListParams) (any, error) {
		return r.service.List(ctx, p.ProviderCode)
	})
}

func (r *ProviderRoutes) ListModels(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"providerCode": "code"}, nil, func(ctx context.Context, p providerListModelsParams) (any, error) {
		return r.service.ListModels(ctx, p.ProviderCode)
	})
}

func (r *ProviderRoutes) Update(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"code": "code"}, nil, func(ctx context.Context, p providerUpdateParams) (any, error) {
		return r.service.Update(ctx, p.Code, p.ProviderUpdate)
	})
}

func (r *ProviderRoutes) Delete(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"code": "code"}, nil, func(ctx context.Context, p codeParams) (any, error) {
		if err := r.service.Delete(ctx, p.Code); err != nil {
			return nil, err
		}
		return map[string]any{"code": p.Code, "deleted": true}, nil
	})
}

func (r *ProviderRoutes) AddModel(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"providerCode": "code"}, nil, func(ctx context.Context, p providerAddModelParams) (any, error) {
		return r.service.AddModel(ctx, p.ProviderCode, p.ModelCode, p.ModelInput)
	})
}

func (r *ProviderRoutes) RemoveModel(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"providerCode": "code", "modelCode": "modelCode"}, nil, func(ctx context.Context, p modelTargetParams) (any, error) {
		return r.service.RemoveModel(ctx, p.ProviderCode, p.ModelCode)
	})
}
