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

type SkillRoutes struct {
	service *services.SkillService
}

var (
	skillRoutes *SkillRoutes
	skillOnce   sync.Once
)

func GetSkillRoutes() *SkillRoutes {
	skillOnce.Do(func() {
		skillRoutes = &SkillRoutes{
			service: services.GetSkillService(),
		}
	})
	return skillRoutes
}

func (r *SkillRoutes) RegisterRoutes(router *gin.RouterGroup) {
	skillGroup := router.Group("/skills")
	{
		skillGroup.GET("", r.ListSkills)
		skillGroup.POST("/content", r.GetSkillContent)
		skillGroup.POST("/rescan", r.RescanGlobalSkills)
	}
}

func (r *SkillRoutes) ListSkills(c *gin.Context) {
	var sessionID *uuid.UUID
	if sessionIDStr := c.Query("sessionId"); sessionIDStr != "" {
		parsed, err := uuid.Parse(sessionIDStr)
		if err != nil {
			response.Failed(c, customerrors.ErrInvalidParams)
			return
		}
		sessionID = &parsed
	}

	skills, err := r.service.ListSkills(c, sessionID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, skills)
}

type getSkillContentRequest struct {
	Name      string  `json:"name" binding:"required"`
	SessionID *string `json:"sessionId"`
}

func (r *SkillRoutes) GetSkillContent(c *gin.Context) {
	var req getSkillContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var sessionID *uuid.UUID
	if req.SessionID != nil {
		parsed, err := uuid.Parse(*req.SessionID)
		if err != nil {
			response.Failed(c, customerrors.ErrInvalidParams)
			return
		}
		sessionID = &parsed
	}

	content, err := r.service.GetSkillContent(c, req.Name, sessionID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, &models.SkillContentResult{Content: content})
}

func (r *SkillRoutes) RescanGlobalSkills(c *gin.Context) {
	if err := r.service.RescanGlobalSkills(c.Request.Context()); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}
