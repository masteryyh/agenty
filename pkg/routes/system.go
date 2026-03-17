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
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type SystemRoutes struct {
	service *services.SystemService
}

var (
	systemRoutes *SystemRoutes
	systemOnce   sync.Once
)

func GetSystemRoutes() *SystemRoutes {
	systemOnce.Do(func() {
		systemRoutes = &SystemRoutes{
			service: services.GetSystemService(),
		}
	})
	return systemRoutes
}

func (r *SystemRoutes) RegisterRoutes(router *gin.RouterGroup) {
	systemGroup := router.Group("/system")
	{
		systemGroup.GET("/settings", r.GetSettings)
		systemGroup.PUT("/settings", r.UpdateSettings)
	}
}

func (r *SystemRoutes) GetSettings(c *gin.Context) {
	dto, err := r.service.GetSettings(c.Request.Context())
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, dto)
}

func (r *SystemRoutes) UpdateSettings(c *gin.Context) {
	var dto models.UpdateSystemSettingsDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, err)
		return
	}
	result, err := r.service.UpdateSettings(c.Request.Context(), &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}
