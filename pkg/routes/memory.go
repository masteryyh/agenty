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

type MemoryRoutes struct {
	service *services.KnowledgeService
}

var (
	memoryRoutes *MemoryRoutes
	memoryOnce   sync.Once
)

func GetMemoryRoutes() *MemoryRoutes {
	memoryOnce.Do(func() {
		memoryRoutes = &MemoryRoutes{
			service: services.GetKnowledgeService(),
		}
	})
	return memoryRoutes
}

func (r *MemoryRoutes) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/agents/:agentId/memories", r.ListMemories)
}

func (r *MemoryRoutes) ListMemories(c *gin.Context) {
	agentID, err := uuid.Parse(c.Param("agentId"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	category := models.KnowledgeCategoryLLMMemory
	items, err := r.service.ListItems(c, agentID, &category)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, items)
}
