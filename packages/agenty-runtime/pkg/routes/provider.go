package routes

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/response"
	"github.com/masteryyh/agenty/pkg/utils/typeutil"
)

type ProviderRoutes struct {
	service *services.ProviderService
}

var (
	providerRoutes *ProviderRoutes
	providerOnce   sync.Once
)

func GetProviderRoutes() *ProviderRoutes {
	providerOnce.Do(func() {
		providerRoutes = &ProviderRoutes{
			service: services.GetProviderService(),
		}
	})
	return providerRoutes
}

func (r *ProviderRoutes) RegisterRoutes(router *gin.RouterGroup) {
	providerGroup := router.Group("/providers")
	{
		providerGroup.POST("", r.CreateProvider)
		providerGroup.GET("", r.ListProviders)
		providerGroup.GET("/:id", r.GetProvider)
		providerGroup.PUT("/:id", r.UpdateProvider)
		providerGroup.DELETE("/:id", r.DeleteProvider)
	}
}

func (r *ProviderRoutes) CreateProvider(c *gin.Context) {
	var dto models.CreateModelProviderDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	provider, err := r.service.CreateProvider(c, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, provider)
}

func (r *ProviderRoutes) ListProviders(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	providers, err := r.service.ListProviders(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, providers)
}

func (r *ProviderRoutes) GetProvider(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	providerID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	provider, err := r.service.GetProvider(c, providerID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, provider)
}

func (r *ProviderRoutes) UpdateProvider(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	providerID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.UpdateModelProviderDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	provider, err := r.service.UpdateProvider(c, providerID, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, provider)
}

func (r *ProviderRoutes) DeleteProvider(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	providerID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	forceRaw := c.Query("force")
	force := typeutil.ParseBoolQueryParam(forceRaw)

	if err := r.service.DeleteProvider(c, providerID, force); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}
