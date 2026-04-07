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
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type KnowledgeRoutes struct {
	service *services.KnowledgeService
}

var (
	knowledgeRoutes *KnowledgeRoutes
	knowledgeOnce   sync.Once
)

func GetKnowledgeRoutes() *KnowledgeRoutes {
	knowledgeOnce.Do(func() {
		knowledgeRoutes = &KnowledgeRoutes{
			service: services.GetKnowledgeService(),
		}
	})
	return knowledgeRoutes
}

func (r *KnowledgeRoutes) RegisterRoutes(router *gin.RouterGroup) {
	kbGroup := router.Group("/agents/:agentId/knowledge")
	{
		kbGroup.POST("", r.CreateItem)
		kbGroup.GET("", r.ListItems)
		kbGroup.GET("/:itemId", r.GetItem)
		kbGroup.DELETE("/:itemId", r.DeleteItem)
		kbGroup.POST("/search", r.Search)
	}
}

func (r *KnowledgeRoutes) CreateItem(c *gin.Context) {
	agentID, err := uuid.Parse(c.Param("agentId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.CreateKnowledgeItemDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	item, err := r.service.CreateItem(c, agentID, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, item)
}

func (r *KnowledgeRoutes) GetItem(c *gin.Context) {
	agentID, err := uuid.Parse(c.Param("agentId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	item, err := r.service.GetItem(c, agentID, itemID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, item)
}

func (r *KnowledgeRoutes) ListItems(c *gin.Context) {
	agentID, err := uuid.Parse(c.Param("agentId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var category *models.KnowledgeCategory
	if cat := c.Query("category"); cat != "" {
		kc := models.KnowledgeCategory(cat)
		category = &kc
	}

	items, err := r.service.ListItems(c, agentID, category)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, items)
}

func (r *KnowledgeRoutes) DeleteItem(c *gin.Context) {
	agentID, err := uuid.Parse(c.Param("agentId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.DeleteItem(c, agentID, itemID); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *KnowledgeRoutes) Search(c *gin.Context) {
	agentID, err := uuid.Parse(c.Param("agentId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var req models.KBSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	results, err := r.service.HybridSearch(c, agentID, req.Query, req.Limit)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, results)
}
