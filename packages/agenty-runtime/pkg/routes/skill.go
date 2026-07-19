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
