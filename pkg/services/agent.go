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
		return gorm.G[models.Agent](tx).Create(ctx, agent)
	}); err != nil {
		slog.ErrorContext(ctx, "failed to create agent", "error", err)
		return nil, err
	}
	return agent.ToDto(), nil
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
	return agent.ToDto(), nil
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

	updates := make(map[string]any)

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
		updates["name"] = *dto.Name
	}

	if dto.Soul != nil {
		updates["soul"] = *dto.Soul
	}

	if err := s.db.WithContext(ctx).
		Model(&models.Agent{}).
		Where("id = ? AND deleted_at IS NULL", agentID).
		Updates(updates).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update agent", "error", err, "agentId", agentID)
		return err
	}

	if dto.IsDefault != nil {
		if *dto.IsDefault {
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
				return nil
			}); err != nil {
				slog.ErrorContext(ctx, "failed to update agent default flag", "error", err, "agentId", agentID)
				return err
			}
		} else {
			if _, err := gorm.G[models.Agent](s.db).
				Where("id = ? AND deleted_at IS NULL", agentID).
				Update(ctx, "is_default", false); err != nil {
				slog.ErrorContext(ctx, "failed to clear agent default flag", "error", err, "agentId", agentID)
				return err
			}
		}
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
			Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
			return err
		}

		if _, err := gorm.G[models.ChatSession](tx).
			Where("agent_id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
			return err
		}

		if _, err := gorm.G[models.Memory](tx).
			Where("agent_id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
			return err
		}

		if _, err := gorm.G[models.Agent](tx).
			Where("id = ? AND deleted_at IS NULL", agentID).
			Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
			return err
		}
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to delete agent", "error", err, "agentId", agentID)
		return err
	}
	return nil
}
