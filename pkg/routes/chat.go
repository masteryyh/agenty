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
	"io"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type ChatRoutes struct {
	service *services.ChatService
}

var (
	chatRoutes *ChatRoutes
	chatOnce   sync.Once
)

func GetChatRoutes() *ChatRoutes {
	chatOnce.Do(func() {
		chatRoutes = &ChatRoutes{
			service: services.GetChatService(),
		}
	})
	return chatRoutes
}

func (r *ChatRoutes) RegisterRoutes(router *gin.RouterGroup) {
	chatGroup := router.Group("/chats")
	{
		chatGroup.POST("/session", r.CreateSession)
		chatGroup.GET("/sessions", r.ListSessions)
		chatGroup.GET("/session/last", r.GetLastSession)
		chatGroup.GET("/session/last/:agentId", r.GetLastSessionByAgent)
		chatGroup.GET("/session/:sessionId", r.GetSession)
		chatGroup.PATCH("/session/:sessionId/cwd", r.SetSessionCwd)
		chatGroup.POST("/chat", r.Chat)
		chatGroup.POST("/stream", r.StreamChat)
	}
}

func (r *ChatRoutes) CreateSession(c *gin.Context) {
	var dto models.CreateSessionDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	session, err := r.service.CreateSession(c, &dto)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, session)
}

func (r *ChatRoutes) ListSessions(c *gin.Context) {
	var pageRequest pagination.PageRequest
	if err := c.ShouldBindQuery(&pageRequest); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}
	pageRequest.ApplyDefaults()

	sessions, err := r.service.ListSessions(c, &pageRequest)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, sessions)
}

func (r *ChatRoutes) GetSession(c *gin.Context) {
	sessionIDRaw := c.Param("sessionId")
	if sessionIDRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	sessionID, err := uuid.Parse(sessionIDRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	session, err := r.service.GetSession(c, sessionID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, session)
}

func (r *ChatRoutes) GetLastSession(c *gin.Context) {
	session, err := r.service.GetLastSession(c)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, session)
}

func (r *ChatRoutes) GetLastSessionByAgent(c *gin.Context) {
	agentIDRaw := c.Param("agentId")
	if agentIDRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	agentID, err := uuid.Parse(agentIDRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	session, err := r.service.GetLastSessionByAgent(c, agentID)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, session)
}

func (r *ChatRoutes) SetSessionCwd(c *gin.Context) {
	sessionIDRaw := c.Param("sessionId")
	if sessionIDRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	sessionID, err := uuid.Parse(sessionIDRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var dto models.SetSessionCwdDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	if err := r.service.SetSessionCwd(c, sessionID, dto.Cwd, dto.AgentsMD); err != nil {
		response.Failed(c, err)
		return
	}
	response.OK[any](c, nil)
}

func (r *ChatRoutes) Chat(c *gin.Context) {
	sessionIDRaw := c.Query("sessionId")
	if sessionIDRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	sessionID, err := uuid.Parse(sessionIDRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var data *models.ChatDto
	if err := c.ShouldBindJSON(&data); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	result, err := r.service.Chat(c, sessionID, data)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}

func (r *ChatRoutes) StreamChat(c *gin.Context) {
	sessionIDRaw := c.Query("sessionId")
	if sessionIDRaw == "" {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	sessionID, err := uuid.Parse(sessionIDRaw)
	if err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	var data *models.ChatDto
	if err := c.ShouldBindJSON(&data); err != nil {
		response.Failed(c, customerrors.ErrInvalidParams)
		return
	}

	events, err := r.service.StreamChat(c.Request.Context(), sessionID, data)
	if err != nil {
		response.Failed(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		select {
		case evt, ok := <-events:
			if !ok {
				return false
			}
			data, _ := json.Marshal(evt)
			c.SSEvent(string(evt.Type), string(data))
			return evt.Type != providers.EventDone
		case <-c.Request.Context().Done():
			return false
		}
	})
}
