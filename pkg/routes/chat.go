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
		chatGroup.GET("/session/:sessionId", r.GetSession)
		chatGroup.POST("/chat", r.Chat)
	}
}

func (r *ChatRoutes) CreateSession(c *gin.Context) {
	session, err := r.service.CreateSession(c)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, session)
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

	result, err := r.service.Chat(c, sessionID, data.Message)
	if err != nil {
		response.Failed(c, err)
		return
	}
	response.OK(c, result)
}
