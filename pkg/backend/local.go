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
	"github.com/masteryyh/agenty/pkg/utils/signal"
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
	return l.providerSvc.ListProviders(signal.GetBaseContext(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateProvider(dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error) {
	return l.providerSvc.CreateProvider(signal.GetBaseContext(), dto)
}

func (l *LocalBackend) UpdateProvider(providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error) {
	return l.providerSvc.UpdateProvider(signal.GetBaseContext(), providerID, dto)
}

func (l *LocalBackend) DeleteProvider(providerID uuid.UUID, force bool) error {
	return l.providerSvc.DeleteProvider(signal.GetBaseContext(), providerID, force)
}

func (l *LocalBackend) ListModels(page, pageSize int) (*pagination.PagedResponse[models.ModelDto], error) {
	return l.modelSvc.ListModels(signal.GetBaseContext(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateModel(dto *models.CreateModelDto) (*models.ModelDto, error) {
	return l.modelSvc.CreateModel(signal.GetBaseContext(), dto)
}

func (l *LocalBackend) GetDefaultModel() (*models.ModelDto, error) {
	return l.modelSvc.GetDefault(signal.GetBaseContext())
}

func (l *LocalBackend) UpdateModel(modelID uuid.UUID, dto *models.UpdateModelDto) error {
	return l.modelSvc.UpdateModel(signal.GetBaseContext(), modelID, dto)
}

func (l *LocalBackend) DeleteModel(modelID uuid.UUID) error {
	return l.modelSvc.DeleteModel(signal.GetBaseContext(), modelID)
}

func (l *LocalBackend) GetModelThinkingLevels(modelID uuid.UUID) (*[]string, error) {
	result, err := l.modelSvc.GetThinkingLevels(signal.GetBaseContext(), modelID)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (l *LocalBackend) ListSessions(page, pageSize int) (*pagination.PagedResponse[models.ChatSessionDto], error) {
	return l.chatSvc.ListSessions(signal.GetBaseContext(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateSession(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return l.chatSvc.CreateSession(signal.GetBaseContext(), &models.CreateSessionDto{AgentID: agentID})
}

func (l *LocalBackend) GetSession(sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	return l.chatSvc.GetSession(signal.GetBaseContext(), sessionID)
}

func (l *LocalBackend) GetLastSession() (*models.ChatSessionDto, error) {
	return l.chatSvc.GetLastSession(signal.GetBaseContext())
}

func (l *LocalBackend) GetLastSessionByAgent(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return l.chatSvc.GetLastSessionByAgent(signal.GetBaseContext(), agentID)
}

func (l *LocalBackend) Chat(sessionID uuid.UUID, dto *models.ChatDto) (*[]*models.ChatMessageDto, error) {
	result, err := l.chatSvc.Chat(signal.GetBaseContext(), sessionID, dto)
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
	return l.agentSvc.ListAgents(signal.GetBaseContext(), pageReq(page, pageSize))
}

func (l *LocalBackend) GetAgent(agentID uuid.UUID) (*models.AgentDto, error) {
	return l.agentSvc.GetAgent(signal.GetBaseContext(), agentID)
}

func (l *LocalBackend) CreateAgent(dto *models.CreateAgentDto) (*models.AgentDto, error) {
	return l.agentSvc.CreateAgent(signal.GetBaseContext(), dto)
}

func (l *LocalBackend) UpdateAgent(agentID uuid.UUID, dto *models.UpdateAgentDto) error {
	return l.agentSvc.UpdateAgent(signal.GetBaseContext(), agentID, dto)
}

func (l *LocalBackend) DeleteAgent(agentID uuid.UUID) error {
	return l.agentSvc.DeleteAgent(signal.GetBaseContext(), agentID)
}

func (l *LocalBackend) ListMCPServers(page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error) {
	return l.mcpSvc.ListMCPServers(signal.GetBaseContext(), pageReq(page, pageSize))
}

func (l *LocalBackend) CreateMCPServer(dto *models.CreateMCPServerDto) (*models.MCPServerDto, error) {
	return l.mcpSvc.CreateMCPServer(signal.GetBaseContext(), dto)
}

func (l *LocalBackend) UpdateMCPServer(serverID uuid.UUID, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error) {
	return l.mcpSvc.UpdateMCPServer(signal.GetBaseContext(), serverID, dto)
}

func (l *LocalBackend) DeleteMCPServer(serverID uuid.UUID) error {
	return l.mcpSvc.DeleteMCPServer(signal.GetBaseContext(), serverID)
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

func (l *LocalBackend) IsInitialized() (bool, error) {
	return services.GetSystemService().IsInitialized(signal.GetBaseContext())
}

func (l *LocalBackend) SetInitialized() error {
	return services.GetSystemService().SetInitialized(signal.GetBaseContext())
}

func (l *LocalBackend) GetSystemSettings() (*models.SystemSettingsDto, error) {
	return services.GetSystemService().GetSettings(signal.GetBaseContext())
}

func (l *LocalBackend) UpdateSystemSettings(dto *models.UpdateSystemSettingsDto) (*models.SystemSettingsDto, error) {
	return services.GetSystemService().UpdateSettings(signal.GetBaseContext(), dto)
}

func (l *LocalBackend) ListMemories(agentID uuid.UUID) ([]models.KnowledgeItemSummaryDto, error) {
	category := models.KnowledgeCategoryLLMMemory
	return services.GetKnowledgeService().ListItems(signal.GetBaseContext(), agentID, &category)
}
