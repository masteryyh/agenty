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

type AgentRoutes struct {
	service *services.AgentService
}

var (
	agentRoutes *AgentRoutes
	agentOnce   sync.Once
)

func GetAgentRoutes() *AgentRoutes {
	agentOnce.Do(func() {
		agentRoutes = &AgentRoutes{
			service: services.GetAgentService(),
		}
	})
	return agentRoutes
}

func (r *AgentRoutes) RegisterRoutes(router *gin.RouterGroup) {
	agentGroup := router.Group("/agents")
	{
		agentGroup.POST("", r.CreateAgent)
		agentGroup.GET("", r.ListAgents)
		agentGroup.GET("/:id", r.GetAgent)
		agentGroup.PUT("/:id", r.UpdateAgent)
		agentGroup.DELETE("/:id", r.DeleteAgent)
	}
}

func (r *AgentRoutes) CreateAgent(c *gin.Context) {
	var dto models.CreateAgentDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	agent, err := r.service.CreateAgent(c, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, agent)
}

func (r *AgentRoutes) GetAgent(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	agentID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	agent, err := r.service.GetAgent(c, agentID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, agent)
}

func (r *AgentRoutes) ListAgents(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	agents, err := r.service.ListAgents(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, agents)
}

func (r *AgentRoutes) UpdateAgent(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	agentID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.UpdateAgentDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.UpdateAgent(c, agentID, &dto); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *AgentRoutes) DeleteAgent(c *gin.Context) {
	idRaw := c.Param("id")
	if idRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	agentID, err := uuid.Parse(idRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.DeleteAgent(c, agentID); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}
