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
	"github.com/masteryyh/agenty/pkg/mcp"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type MCPServerRoutes struct {
	service *services.MCPServerService
}

var (
	mcpServerRoutes     *MCPServerRoutes
	mcpServerRoutesOnce sync.Once
)

func GetMCPServerRoutes() *MCPServerRoutes {
	mcpServerRoutesOnce.Do(func() {
		mcpServerRoutes = &MCPServerRoutes{
			service: services.GetMCPServerService(),
		}
	})
	return mcpServerRoutes
}

func (r *MCPServerRoutes) RegisterRoutes(router *gin.RouterGroup) {
	mcpGroup := router.Group("/mcp/servers")
	{
		mcpGroup.POST("", r.CreateMCPServer)
		mcpGroup.GET("", r.ListMCPServers)
		mcpGroup.GET("/:id", r.GetMCPServer)
		mcpGroup.PUT("/:id", r.UpdateMCPServer)
		mcpGroup.DELETE("/:id", r.DeleteMCPServer)
		mcpGroup.POST("/:id/connect", r.ConnectMCPServer)
		mcpGroup.POST("/:id/disconnect", r.DisconnectMCPServer)
	}
}

func (r *MCPServerRoutes) CreateMCPServer(c *gin.Context) {
	var dto models.CreateMCPServerDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	server, err := r.service.CreateMCPServer(c, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, server)
}

func (r *MCPServerRoutes) ListMCPServers(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	result, err := r.service.ListMCPServers(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}

	mgr := mcp.GetManager()
	if mgr != nil {
		statuses := mgr.GetAllStatuses()
		for i := range result.Data {
			if statusDto, ok := statuses[result.Data[i].ID]; ok {
				result.Data[i].Status = statusDto.Status
				result.Data[i].Tools = statusDto.Tools
				result.Data[i].Error = statusDto.Error
			}
		}
	}

	response.OK(c, result)
}

func (r *MCPServerRoutes) GetMCPServer(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	server, err := r.service.GetMCPServer(c, serverID)
	if err != nil {
		response.Failed(c, err)
		return
	}

	mgr := mcp.GetManager()
	if mgr != nil {
		status, errMsg, tools := mgr.GetStatus(serverID)
		server.Status = string(status)
		server.Error = errMsg
		server.Tools = tools
	}

	response.OK(c, server)
}

func (r *MCPServerRoutes) UpdateMCPServer(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.UpdateMCPServerDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	server, err := r.service.UpdateMCPServer(c, serverID, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, server)
}

func (r *MCPServerRoutes) DeleteMCPServer(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	mgr := mcp.GetManager()
	if mgr != nil {
		_ = mgr.Disconnect(serverID)
	}

	if err := r.service.DeleteMCPServer(c, serverID); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *MCPServerRoutes) ConnectMCPServer(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	mgr := mcp.GetManager()
	if mgr == nil {
		response.Failed(c, customerrors.ErrMCPServerConnectionFailed)
		return
	}

	if err := mgr.Connect(serverID); err != nil {
		response.Failed(c, customerrors.ErrMCPServerConnectionFailed)
		return
	}

	response.OK[any](c, nil)
}

func (r *MCPServerRoutes) DisconnectMCPServer(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	mgr := mcp.GetManager()
	if mgr == nil {
		response.Failed(c, customerrors.ErrMCPServerNotConnected)
		return
	}

	if err := mgr.Disconnect(serverID); err != nil {
		response.Failed(c, err)
		return
	}

	response.OK[any](c, nil)
}
