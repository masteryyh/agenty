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
	"fmt"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	mcppkg "github.com/masteryyh/agenty/pkg/mcp"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
)

type LocalBackend struct {
	chatSvc     *services.ChatService
	agentSvc    *services.AgentService
	modelSvc    *services.ModelService
	providerSvc *services.ProviderService
	mcpSvc      *services.MCPServerService
}

func NewLocalBackend() *LocalBackend {
	return &LocalBackend{
		chatSvc:     services.GetChatService(),
		agentSvc:    services.GetAgentService(),
		modelSvc:    services.GetModelService(),
		providerSvc: services.GetProviderService(),
		mcpSvc:      services.GetMCPServerService(),
	}
}

func pageReq(page, pageSize int) *pagination.PageRequest {
	return &pagination.PageRequest{Page: page, PageSize: pageSize}
}

func (l *LocalBackend) ListProviders(page, pageSize int) (*pagination.PagedResponse[models.ModelProviderDto], error) {
	return l.providerSvc.ListProviders(context.Background(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateProvider(dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error) {
	return l.providerSvc.CreateProvider(context.Background(), dto)
}

func (l *LocalBackend) UpdateProvider(providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error) {
	return l.providerSvc.UpdateProvider(context.Background(), providerID, dto)
}

func (l *LocalBackend) DeleteProvider(providerID uuid.UUID, force bool) error {
	return l.providerSvc.DeleteProvider(context.Background(), providerID, force)
}

func (l *LocalBackend) ListModels(page, pageSize int) (*pagination.PagedResponse[models.ModelDto], error) {
	return l.modelSvc.ListModels(context.Background(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateModel(dto *models.CreateModelDto) (*models.ModelDto, error) {
	return l.modelSvc.CreateModel(context.Background(), dto)
}

func (l *LocalBackend) GetDefaultModel() (*models.ModelDto, error) {
	return l.modelSvc.GetDefault(context.Background())
}

func (l *LocalBackend) UpdateModel(modelID uuid.UUID, dto *models.UpdateModelDto) error {
	return l.modelSvc.UpdateModel(context.Background(), modelID, dto)
}

func (l *LocalBackend) DeleteModel(modelID uuid.UUID) error {
	return l.modelSvc.DeleteModel(context.Background(), modelID)
}

func (l *LocalBackend) GetModelThinkingLevels(modelID uuid.UUID) (*[]string, error) {
	result, err := l.modelSvc.GetThinkingLevels(context.Background(), modelID)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (l *LocalBackend) ListSessions(page, pageSize int) (*pagination.PagedResponse[models.ChatSessionDto], error) {
	return l.chatSvc.ListSessions(context.Background(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateSession(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return l.chatSvc.CreateSession(context.Background(), &models.CreateSessionDto{AgentID: agentID})
}

func (l *LocalBackend) GetSession(sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	return l.chatSvc.GetSession(context.Background(), sessionID)
}

func (l *LocalBackend) GetLastSession() (*models.ChatSessionDto, error) {
	return l.chatSvc.GetLastSession(context.Background())
}

func (l *LocalBackend) GetLastSessionByAgent(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return l.chatSvc.GetLastSessionByAgent(context.Background(), agentID)
}

func (l *LocalBackend) Chat(sessionID uuid.UUID, dto *models.ChatDto) (*[]*models.ChatMessageDto, error) {
	result, err := l.chatSvc.Chat(context.Background(), sessionID, dto)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (l *LocalBackend) StreamChat(ctx context.Context, sessionID uuid.UUID, dto *models.ChatDto, handler func(event provider.StreamEvent) error) error {
	ch, err := l.chatSvc.StreamChat(ctx, sessionID, dto)
	if err != nil {
		return err
	}
	for evt := range ch {
		if err := handler(evt); err != nil {
			return err
		}
		if evt.Type == provider.EventDone {
			return nil
		}
	}
	return nil
}

func (l *LocalBackend) ListAgents(page, pageSize int) (*pagination.PagedResponse[models.AgentDto], error) {
	return l.agentSvc.ListAgents(context.Background(), pageReq(page, pageSize))
}

func (l *LocalBackend) GetAgent(agentID uuid.UUID) (*models.AgentDto, error) {
	return l.agentSvc.GetAgent(context.Background(), agentID)
}

func (l *LocalBackend) CreateAgent(dto *models.CreateAgentDto) (*models.AgentDto, error) {
	return l.agentSvc.CreateAgent(context.Background(), dto)
}

func (l *LocalBackend) UpdateAgent(agentID uuid.UUID, dto *models.UpdateAgentDto) error {
	return l.agentSvc.UpdateAgent(context.Background(), agentID, dto)
}

func (l *LocalBackend) DeleteAgent(agentID uuid.UUID) error {
	return l.agentSvc.DeleteAgent(context.Background(), agentID)
}

func (l *LocalBackend) ListMCPServers(page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error) {
	return l.mcpSvc.ListMCPServers(context.Background(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateMCPServer(dto *models.CreateMCPServerDto) (*models.MCPServerDto, error) {
	return l.mcpSvc.CreateMCPServer(context.Background(), dto)
}

func (l *LocalBackend) UpdateMCPServer(serverID uuid.UUID, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error) {
	return l.mcpSvc.UpdateMCPServer(context.Background(), serverID, dto)
}

func (l *LocalBackend) DeleteMCPServer(serverID uuid.UUID) error {
	return l.mcpSvc.DeleteMCPServer(context.Background(), serverID)
}

func (l *LocalBackend) ConnectMCPServer(serverID uuid.UUID) error {
	mgr := mcppkg.GetManager()
	if mgr == nil {
		return fmt.Errorf("MCP manager not initialized")
	}
	return mgr.Connect(serverID)
}

func (l *LocalBackend) DisconnectMCPServer(serverID uuid.UUID) error {
	mgr := mcppkg.GetManager()
	if mgr == nil {
		return fmt.Errorf("MCP manager not initialized")
	}
	return mgr.Disconnect(serverID)
}
