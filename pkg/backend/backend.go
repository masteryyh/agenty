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

package backend

import (
	"context"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
)

type Backend interface {
	ListProviders(page, pageSize int) (*pagination.PagedResponse[models.ModelProviderDto], error)
	CreateProvider(dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error)
	UpdateProvider(providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error)
	DeleteProvider(providerID uuid.UUID, force bool) error

	ListModels(page, pageSize int) (*pagination.PagedResponse[models.ModelDto], error)
	CreateModel(dto *models.CreateModelDto) (*models.ModelDto, error)
	GetDefaultModel() (*models.ModelDto, error)
	UpdateModel(modelID uuid.UUID, dto *models.UpdateModelDto) error
	DeleteModel(modelID uuid.UUID) error
	GetModelThinkingLevels(modelID uuid.UUID) (*[]string, error)

	ListSessions(page, pageSize int) (*pagination.PagedResponse[models.ChatSessionDto], error)
	CreateSession(agentID uuid.UUID) (*models.ChatSessionDto, error)
	GetSession(sessionID uuid.UUID) (*models.ChatSessionDto, error)
	GetLastSession() (*models.ChatSessionDto, error)
	GetLastSessionByAgent(agentID uuid.UUID) (*models.ChatSessionDto, error)

	Chat(sessionID uuid.UUID, dto *models.ChatDto) (*[]*models.ChatMessageDto, error)
	StreamChat(ctx context.Context, sessionID uuid.UUID, dto *models.ChatDto, handler func(event provider.StreamEvent) error) error

	ListAgents(page, pageSize int) (*pagination.PagedResponse[models.AgentDto], error)
	GetAgent(agentID uuid.UUID) (*models.AgentDto, error)
	CreateAgent(dto *models.CreateAgentDto) (*models.AgentDto, error)
	UpdateAgent(agentID uuid.UUID, dto *models.UpdateAgentDto) error
	DeleteAgent(agentID uuid.UUID) error

	ListMCPServers(page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error)
	CreateMCPServer(dto *models.CreateMCPServerDto) (*models.MCPServerDto, error)
	UpdateMCPServer(serverID uuid.UUID, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error)
	DeleteMCPServer(serverID uuid.UUID) error
	ConnectMCPServer(serverID uuid.UUID) error
	DisconnectMCPServer(serverID uuid.UUID) error

	GetSystemSettings() (*models.SystemSettingsDto, error)
	UpdateSystemSettings(dto *models.UpdateSystemSettingsDto) (*models.SystemSettingsDto, error)

	IsInitialized() (bool, error)
	SetInitialized() error
}
