package routes

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/gateway"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type GatewayRoutes struct {
	service *services.GatewayService
}

var (
	gatewayRoutes *GatewayRoutes
	gatewayOnce   sync.Once
)

func GetGatewayRoutes() *GatewayRoutes {
	gatewayOnce.Do(func() {
		gatewayRoutes = &GatewayRoutes{
			service: services.GetGatewayService(),
		}
	})
	return gatewayRoutes
}

func (r *GatewayRoutes) RegisterRoutes(router *gin.RouterGroup) {
	gatewayGroup := router.Group("/gateway")
	{
		gatewayGroup.POST("/channels", r.CreateChannel)
		gatewayGroup.GET("/channels", r.ListChannels)
		gatewayGroup.GET("/channels/:channelId", r.GetChannel)
		gatewayGroup.PUT("/channels/:channelId", r.UpdateChannel)
		gatewayGroup.DELETE("/channels/:channelId", r.DeleteChannel)
		gatewayGroup.GET("/bindings", r.ListBindings)
	}

	agentGroup := router.Group("/agents/:id/gateway-bindings")
	{
		agentGroup.POST("", r.CreateBinding)
		agentGroup.PUT("/:bindingId", r.UpdateBinding)
		agentGroup.DELETE("/:bindingId", r.DeleteBinding)
	}
}

func (r *GatewayRoutes) ListChannels(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()
	result, err := r.service.ListChannels(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}

func (r *GatewayRoutes) GetChannel(c *gin.Context) {
	channelID := c.Param("channelId")
	if channelID == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	result, err := r.service.GetChannel(c, channelID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}

func (r *GatewayRoutes) CreateChannel(c *gin.Context) {
	var dto models.CreateGatewayChannelDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	result, err := r.service.CreateChannelWithReload(c, &dto, gateway.ReloadActiveManagerWithChannels)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}

func (r *GatewayRoutes) UpdateChannel(c *gin.Context) {
	channelID := c.Param("channelId")
	if channelID == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	var dto models.UpdateGatewayChannelDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	result, err := r.service.UpdateChannelWithReload(c, channelID, &dto, gateway.ReloadActiveManagerWithChannels)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}

func (r *GatewayRoutes) DeleteChannel(c *gin.Context) {
	channelID := c.Param("channelId")
	if channelID == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	if err := r.service.DeleteChannelWithReload(c, channelID, gateway.ReloadActiveManagerWithChannels); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *GatewayRoutes) ListBindings(c *gin.Context) {
	var agentID *uuid.UUID
	if raw := c.Query("agentId"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			response.Failed(c, customerrors.ErrInvalidParams)
			return
		}
		agentID = &parsed
	}

	bindings, err := r.service.ListBindings(c, agentID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, bindings)
}

func (r *GatewayRoutes) CreateBinding(c *gin.Context) {
	agentID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var dto models.CreateAgentGatewayBindingDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	binding, err := r.service.CreateBinding(c, agentID, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, binding)
}

func (r *GatewayRoutes) UpdateBinding(c *gin.Context) {
	agentID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	bindingID, ok := parseUUIDParam(c, "bindingId")
	if !ok {
		return
	}
	var dto models.UpdateAgentGatewayBindingDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	binding, err := r.service.UpdateBinding(c, agentID, bindingID, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, binding)
}

func (r *GatewayRoutes) DeleteBinding(c *gin.Context) {
	agentID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	bindingID, ok := parseUUIDParam(c, "bindingId")
	if !ok {
		return
	}
	if err := r.service.DeleteBinding(c, agentID, bindingID); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func parseUUIDParam(c *gin.Context, key string) (uuid.UUID, bool) {
	raw := c.Param(key)
	if raw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return uuid.Nil, false
	}
	return parsed, true
}
