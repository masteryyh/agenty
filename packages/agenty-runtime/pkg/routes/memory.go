package routes

import (
	"sync"

	"github.com/gin-gonic/gin"
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
	router.GET("/agents/:id/memories", r.ListMemories)
}

func (r *MemoryRoutes) ListMemories(c *gin.Context) {
	agentID, ok := parseUUIDParam(c, "id")
	if !ok {
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
