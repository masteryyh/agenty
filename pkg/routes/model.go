/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
		modelGroup.GET("/:id", r.GetModel)
		modelGroup.DELETE("/:id", r.DeleteModel)
	}
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
	response.OK(c, nil)
}
