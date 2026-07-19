package routes

import (
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type ModelRoutes struct {
	service *services.ModelService
}

var (
	modelRoutes *ModelRoutes
	modelOnce   sync.Once
)

func GetModelRoutes() *ModelRoutes {
	modelOnce.Do(func() {
		modelRoutes = &ModelRoutes{
			service: services.GetModelService(),
		}
	})
	return modelRoutes
}

func (r *ModelRoutes) RegisterRoutes(router *gin.RouterGroup) {
	modelGroup := router.Group("/models")
	{
		modelGroup.POST("", r.CreateModel)
		modelGroup.GET("", r.ListModels)
		modelGroup.GET("/default", r.GetDefaultModel)
		modelGroup.GET("/embedding", r.ListEmbeddingModels)
		modelGroup.GET("/context-compression", r.ListContextCompressionModels)
		modelGroup.GET("/:id", r.GetModel)
		modelGroup.GET("/:id/thinking-levels", r.GetThinkingLevels)
		modelGroup.PUT("/by-name", r.UpdateByName)
		modelGroup.PUT("/:id", r.UpdateModel)
		modelGroup.DELETE("/:id", r.DeleteModel)
	}
}

func (r *ModelRoutes) GetDefaultModel(c *gin.Context) {
	model, err := r.service.GetDefault(c)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, model)
}

func (r *ModelRoutes) CreateModel(c *gin.Context) {
	var dto models.CreateModelDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	model, err := r.service.CreateModel(c, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, model)
}

func (r *ModelRoutes) UpdateByName(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	name, err := url.QueryUnescape(name)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.UpdateModelDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.UpdateByName(c, name, &dto); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *ModelRoutes) UpdateModel(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	modelID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.UpdateModelDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.UpdateModel(c, modelID, &dto); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *ModelRoutes) ListModels(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	var providerID *uuid.UUID
	providerIDRaw := c.Query("providerId")
	if providerIDRaw != "" {
		parsedID, err := uuid.Parse(providerIDRaw)
		if err != nil {
			response.Failed(c, customerrors.ErrInvalidParams)
			return
		}
		providerID = &parsedID
	}

	var modelsList *pagination.PagedResponse[models.ModelDto]
	var err error
	if providerID != nil {
		modelsList, err = r.service.ListModelsByProvider(c, *providerID, &pageRequest)
	} else {
		modelsList, err = r.service.ListModels(c, &pageRequest)
	}
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, modelsList)
}

func (r *ModelRoutes) GetModel(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	modelID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	model, err := r.service.GetModel(c, modelID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, model)
}

func (r *ModelRoutes) GetThinkingLevels(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	modelID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	levels, err := r.service.GetThinkingLevels(c, modelID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, levels)
}

func (r *ModelRoutes) DeleteModel(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	modelID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.DeleteModel(c, modelID); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *ModelRoutes) ListEmbeddingModels(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	result, err := r.service.ListEmbeddingModels(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}

func (r *ModelRoutes) ListContextCompressionModels(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	result, err := r.service.ListContextCompressionModels(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}
