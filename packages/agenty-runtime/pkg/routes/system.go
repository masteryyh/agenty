package routes

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type SystemRoutes struct {
	service *services.ConfigService
}

var (
	systemRoutes *SystemRoutes
	systemOnce   sync.Once
)

func GetSystemRoutes() *SystemRoutes {
	systemOnce.Do(func() {
		systemRoutes = &SystemRoutes{
			service: services.GetConfigService(),
		}
	})
	return systemRoutes
}

func (r *SystemRoutes) RegisterRoutes(router *gin.RouterGroup) {
	systemGroup := router.Group("/system")
	{
		systemGroup.GET("/version", r.GetVersion)
		systemGroup.GET("/config", r.GetConfig)
		systemGroup.PUT("/config", r.UpdateConfig)
	}
}

func (r *SystemRoutes) GetVersion(c *gin.Context) {
	dto, err := r.service.GetVersion(c.Request.Context())
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, dto)
}

func (r *SystemRoutes) GetConfig(c *gin.Context) {
	dto, err := r.service.GetConfig(c.Request.Context())
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, dto)
}

func (r *SystemRoutes) UpdateConfig(c *gin.Context) {
	var dto models.UpdateSystemConfigDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, err)
		return
	}
	result, err := r.service.UpdateConfig(c.Request.Context(), &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}
