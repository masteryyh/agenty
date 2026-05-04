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

package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type AgentService struct {
	db *gorm.DB
}

var (
	agentService *AgentService
	agentOnce    sync.Once
)

func GetAgentService() *AgentService {
	agentOnce.Do(func() {
		agentService = &AgentService{
			db: conn.GetDB(),
		}
	})
	return agentService
}

func (s *AgentService) CreateAgent(ctx context.Context, dto *models.CreateAgentDto) (*models.AgentDto, error) {
	nameExists, err := gorm.G[models.Agent](s.db).
		Where("name = ? AND deleted_at IS NULL", dto.Name).
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to check agent name existence", "error", err)
		return nil, err
	}
	if nameExists > 0 {
		return nil, customerrors.ErrAgentAlreadyExists
	}

	soul := consts.DefaultAgentSoul
	if dto.Soul != nil && *dto.Soul != "" {
		soul = *dto.Soul
	}

	agent := &models.Agent{
		Name:      dto.Name,
		Soul:      soul,
		IsDefault: dto.IsDefault,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if agent.IsDefault {
			if _, err := gorm.G[models.Agent](tx).
				Where("deleted_at IS NULL").
				Update(ctx, "is_default", false); err != nil {
				return err
			}
		}
		if err := gorm.G[models.Agent](tx).Create(ctx, agent); err != nil {
			return err
		}
		if len(dto.ModelIDs) > 0 {
			entries := make([]models.AgentModel, len(dto.ModelIDs))
			for i, modelID := range dto.ModelIDs {
				entries[i] = models.AgentModel{AgentID: agent.ID, ModelID: modelID, SortOrder: i}
			}
			if err := tx.WithContext(ctx).Create(&entries).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to create agent", "error", err)
		return nil, err
	}
	return s.loadAgentModels(ctx, agent.ToDto())
}

func (s *AgentService) GetAgent(ctx context.Context, agentID uuid.UUID) (*models.AgentDto, error) {
	agent, err := gorm.G[models.Agent](s.db).
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", agentID)
		return nil, err
	}
	return s.loadAgentModels(ctx, agent.ToDto())
}

func (s *AgentService) loadAgentModels(ctx context.Context, dto *models.AgentDto) (*models.AgentDto, error) {
	agentModelEntries, err := gorm.G[models.AgentModel](s.db).
		Where("agent_id = ?", dto.ID).
		Order("sort_order ASC").
		Find(ctx)
	if err != nil || len(agentModelEntries) == 0 {
		return dto, nil
	}

	modelIDs := make([]uuid.UUID, len(agentModelEntries))
	for i, entry := range agentModelEntries {
		modelIDs[i] = entry.ModelID
	}

	mdls, err := gorm.G[models.Model](s.db).
		Where("id IN ? AND deleted_at IS NULL", modelIDs).
		Find(ctx)
	if err != nil {
		return dto, nil
	}

	providerIDs := lo.Uniq(lo.Map(mdls, func(m models.Model, _ int) uuid.UUID { return m.ProviderID }))
	providers, _ := gorm.G[models.ModelProvider](s.db).
		Where("id IN ? AND deleted_at IS NULL", providerIDs).
		Find(ctx)

	providerMap := lo.KeyBy(providers, func(p models.ModelProvider) uuid.UUID { return p.ID })
	modelMap := lo.KeyBy(mdls, func(m models.Model) uuid.UUID { return m.ID })

	dto.Models = make([]models.ModelDto, 0, len(agentModelEntries))
	for _, entry := range agentModelEntries {
		mdl, ok := modelMap[entry.ModelID]
		if !ok {
			continue
		}
		var provDto *models.ModelProviderDto
		if p, ok := providerMap[mdl.ProviderID]; ok {
			provDto = p.ToDto()
		}
		dto.Models = append(dto.Models, *mdl.ToDto(provDto))
	}
	return dto, nil
}

func (s *AgentService) ListAgents(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.AgentDto], error) {
	agentsResult, err := gorm.G[models.Agent](s.db).
		Where("deleted_at IS NULL").
		Offset((request.Page - 1) * request.PageSize).
		Limit(request.PageSize).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list agents", "error", err)
		return nil, err
	}

	countResult, err := gorm.G[models.Agent](s.db).
		Where("deleted_at IS NULL").
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to count agents", "error", err)
		return nil, err
	}

	dtos := lo.Map(agentsResult, func(a models.Agent, _ int) models.AgentDto {
		return *a.ToDto()
	})

	if len(dtos) > 0 {
		agentIDs := lo.Map(agentsResult, func(a models.Agent, _ int) uuid.UUID { return a.ID })
		agentModelEntries, err := gorm.G[models.AgentModel](s.db).
			Where("agent_id IN ?", agentIDs).
			Order("sort_order ASC").
			Find(ctx)
		if err == nil && len(agentModelEntries) > 0 {
			modelIDs := lo.Uniq(lo.Map(agentModelEntries, func(am models.AgentModel, _ int) uuid.UUID { return am.ModelID }))
			mdls, err := gorm.G[models.Model](s.db).
				Where("id IN ? AND deleted_at IS NULL", modelIDs).
				Find(ctx)
			if err == nil {
				providerIDs := lo.Uniq(lo.Map(mdls, func(m models.Model, _ int) uuid.UUID { return m.ProviderID }))
				providers, _ := gorm.G[models.ModelProvider](s.db).
					Where("id IN ? AND deleted_at IS NULL", providerIDs).
					Find(ctx)
				providerMap := lo.KeyBy(providers, func(p models.ModelProvider) uuid.UUID { return p.ID })
				modelMap := lo.KeyBy(mdls, func(m models.Model) uuid.UUID { return m.ID })
				dtoMap := make(map[uuid.UUID]*models.AgentDto, len(dtos))
				for i := range dtos {
					dtoMap[dtos[i].ID] = &dtos[i]
				}
				for _, entry := range agentModelEntries {
					agentDto, ok := dtoMap[entry.AgentID]
					if !ok {
						continue
					}
					mdl, ok := modelMap[entry.ModelID]
					if !ok {
						continue
					}
					var provDto *models.ModelProviderDto
					if p, ok := providerMap[mdl.ProviderID]; ok {
						provDto = p.ToDto()
					}
					agentDto.Models = append(agentDto.Models, *mdl.ToDto(provDto))
				}
			}
		}
	}

	return &pagination.PagedResponse[models.AgentDto]{
		Total:    countResult,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *AgentService) UpdateAgent(ctx context.Context, agentID uuid.UUID, dto *models.UpdateAgentDto) error {
	agent, err := gorm.G[models.Agent](s.db).
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", agentID)
		return err
	}

	if dto.Name != nil && *dto.Name != agent.Name {
		nameExists, err := gorm.G[models.Agent](s.db).
			Where("name = ? AND id != ? AND deleted_at IS NULL", *dto.Name, agentID).
			Count(ctx, "id")
		if err != nil {
			slog.ErrorContext(ctx, "failed to check agent name existence", "error", err)
			return err
		}
		if nameExists > 0 {
			return customerrors.ErrAgentAlreadyExists
		}
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := make(map[string]any)
		if dto.Name != nil && *dto.Name != agent.Name {
			updates["name"] = *dto.Name
		}
		if dto.Soul != nil {
			updates["soul"] = *dto.Soul
		}
		if len(updates) > 0 {
			if err := tx.Model(&models.Agent{}).
				Where("id = ? AND deleted_at IS NULL", agentID).
				Updates(updates).Error; err != nil {
				return err
			}
		}

		if dto.IsDefault != nil {
			if *dto.IsDefault {
				if _, err := gorm.G[models.Agent](tx).
					Where("id != ? AND deleted_at IS NULL", agentID).
					Update(ctx, "is_default", false); err != nil {
					return err
				}
				if _, err := gorm.G[models.Agent](tx).
					Where("id = ? AND deleted_at IS NULL", agentID).
					Update(ctx, "is_default", true); err != nil {
					return err
				}
			} else {
				if _, err := gorm.G[models.Agent](tx).
					Where("id = ? AND deleted_at IS NULL", agentID).
					Update(ctx, "is_default", false); err != nil {
					return err
				}
			}
		}

		if dto.ModelIDs != nil {
			if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentModel{}).Error; err != nil {
				return err
			}
			if len(*dto.ModelIDs) > 0 {
				entries := make([]models.AgentModel, len(*dto.ModelIDs))
				for i, modelID := range *dto.ModelIDs {
					entries[i] = models.AgentModel{AgentID: agentID, ModelID: modelID, SortOrder: i}
				}
				return tx.Create(&entries).Error
			}
		}

		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to update agent", "error", err, "agentId", agentID)
		return err
	}

	return nil
}

func (s *AgentService) DeleteAgent(ctx context.Context, agentID uuid.UUID) error {
	currentAgent, err := gorm.G[models.Agent](s.db).
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", agentID)
		return err
	}

	if currentAgent.IsDefault {
		return customerrors.ErrDeletingDefaultAgent
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := gorm.G[models.ChatMessage](tx).
			Where("agent_id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", conn.NowExpr()); err != nil {
			return err
		}

		if _, err := gorm.G[models.ChatSession](tx).
			Where("agent_id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", conn.NowExpr()); err != nil {
			return err
		}

		if _, err := gorm.G[models.KnowledgeItem](tx).
			Where("agent_id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", conn.NowExpr()); err != nil {
			return err
		}

		if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentModel{}).Error; err != nil {
			return err
		}

		if _, err := gorm.G[models.Agent](tx).
			Where("id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", conn.NowExpr()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to delete agent", "error", err, "agentId", agentID)
		return err
	}
	return nil
}
