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
	"github.com/masteryyh/agenty/pkg/cli/api"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
)

type RemoteBackend struct {
	client *api.Client
}

func NewRemoteBackend(url, username, password string) *RemoteBackend {
	var c *api.Client
	if username != "" && password != "" {
		c = api.NewClientWithAuth(url, username, password)
	} else {
		c = api.NewClient(url)
	}
	return &RemoteBackend{client: c}
}

func (r *RemoteBackend) ListProviders(page, pageSize int) (*pagination.PagedResponse[models.ModelProviderDto], error) {
	return r.client.ListProviders(page, pageSize)
}

func (r *RemoteBackend) CreateProvider(dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error) {
	return r.client.CreateProvider(dto)
}

func (r *RemoteBackend) UpdateProvider(providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error) {
	return r.client.UpdateProvider(providerID, dto)
}

func (r *RemoteBackend) DeleteProvider(providerID uuid.UUID, force bool) error {
	return r.client.DeleteProvider(providerID, force)
}

func (r *RemoteBackend) ListModels(page, pageSize int) (*pagination.PagedResponse[models.ModelDto], error) {
	return r.client.ListModels(page, pageSize)
}

func (r *RemoteBackend) CreateModel(dto *models.CreateModelDto) (*models.ModelDto, error) {
	return r.client.CreateModel(dto)
}

func (r *RemoteBackend) GetDefaultModel() (*models.ModelDto, error) {
	return r.client.GetDefaultModel()
}

func (r *RemoteBackend) UpdateModel(modelID uuid.UUID, dto *models.UpdateModelDto) error {
	return r.client.UpdateModel(modelID, dto)
}

func (r *RemoteBackend) DeleteModel(modelID uuid.UUID) error {
	return r.client.DeleteModel(modelID)
}

func (r *RemoteBackend) GetModelThinkingLevels(modelID uuid.UUID) (*[]string, error) {
	return r.client.GetModelThinkingLevels(modelID)
}

func (r *RemoteBackend) ListSessions(page, pageSize int) (*pagination.PagedResponse[models.ChatSessionDto], error) {
	return r.client.ListSessions(page, pageSize)
}

func (r *RemoteBackend) CreateSession(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return r.client.CreateSession(agentID)
}

func (r *RemoteBackend) GetSession(sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	return r.client.GetSession(sessionID)
}

func (r *RemoteBackend) GetLastSession() (*models.ChatSessionDto, error) {
	return r.client.GetLastSession()
}

func (r *RemoteBackend) GetLastSessionByAgent(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return r.client.GetLastSessionByAgent(agentID)
}

func (r *RemoteBackend) SetSessionCwd(sessionID uuid.UUID, cwd *string, agentsMD *string) error {
	return r.client.SetSessionCwd(sessionID, cwd, agentsMD)
}

func (r *RemoteBackend) Chat(sessionID uuid.UUID, dto *models.ChatDto) (*[]*models.ChatMessageDto, error) {
	return r.client.Chat(sessionID, dto)
}

func (r *RemoteBackend) StreamChat(ctx context.Context, sessionID uuid.UUID, dto *models.ChatDto, handler func(event providers.StreamEvent) error) error {
	return r.client.StreamChat(ctx, sessionID, dto, handler)
}

func (r *RemoteBackend) ListAgents(page, pageSize int) (*pagination.PagedResponse[models.AgentDto], error) {
	return r.client.ListAgents(page, pageSize)
}

func (r *RemoteBackend) GetAgent(agentID uuid.UUID) (*models.AgentDto, error) {
	return r.client.GetAgent(agentID)
}

func (r *RemoteBackend) CreateAgent(dto *models.CreateAgentDto) (*models.AgentDto, error) {
	return r.client.CreateAgent(dto)
}

func (r *RemoteBackend) UpdateAgent(agentID uuid.UUID, dto *models.UpdateAgentDto) error {
	return r.client.UpdateAgent(agentID, dto)
}

func (r *RemoteBackend) DeleteAgent(agentID uuid.UUID) error {
	return r.client.DeleteAgent(agentID)
}

func (r *RemoteBackend) ListMCPServers(page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error) {
	return r.client.ListMCPServers(page, pageSize)
}

func (r *RemoteBackend) CreateMCPServer(dto *models.CreateMCPServerDto) (*models.MCPServerDto, error) {
	return r.client.CreateMCPServer(dto)
}

func (r *RemoteBackend) UpdateMCPServer(serverID uuid.UUID, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error) {
	return r.client.UpdateMCPServer(serverID, dto)
}

func (r *RemoteBackend) DeleteMCPServer(serverID uuid.UUID) error {
	return r.client.DeleteMCPServer(serverID)
}

func (r *RemoteBackend) ConnectMCPServer(serverID uuid.UUID) error {
	return r.client.ConnectMCPServer(serverID)
}

func (r *RemoteBackend) DisconnectMCPServer(serverID uuid.UUID) error {
	return r.client.DisconnectMCPServer(serverID)
}

func (r *RemoteBackend) IsInitialized() (bool, error) {
	dto, err := r.client.GetSystemSettings()
	if err != nil {
		return false, err
	}
	return dto.Initialized, nil
}

func (r *RemoteBackend) SetInitialized() error {
	return r.client.SetSystemInitialized()
}

func (r *RemoteBackend) GetSystemSettings() (*models.SystemSettingsDto, error) {
	return r.client.GetSystemSettings()
}

func (r *RemoteBackend) UpdateSystemSettings(dto *models.UpdateSystemSettingsDto) (*models.SystemSettingsDto, error) {
	return r.client.UpdateSystemSettings(dto)
}

func (r *RemoteBackend) ListMemories(agentID uuid.UUID) ([]models.KnowledgeItemSummaryDto, error) {
	result, err := r.client.ListMemories(agentID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return *result, nil
}

func (r *RemoteBackend) ListSkills(sessionID uuid.UUID) ([]models.SkillDto, error) {
	return r.client.ListSkills(sessionID)
}

func (r *RemoteBackend) GetSkillContent(name string, sessionID *uuid.UUID) (string, error) {
	return r.client.GetSkillContent(name, sessionID)
}
