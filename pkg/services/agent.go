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
	"database/sql"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/db"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
)

type AgentService struct {
	db      *sql.DB
	queries *db.Queries
}

var (
	agentService *AgentService
	agentOnce    sync.Once
)

func GetAgentService() *AgentService {
	agentOnce.Do(func() {
		sqlDB := conn.GetSQLDB()
		agentService = &AgentService{
			db:      sqlDB,
			queries: db.New(sqlDB),
		}
	})
	return agentService
}

func (s *AgentService) CreateAgent(ctx context.Context, dto *models.CreateAgentDto) (*models.AgentDto, error) {
	nameExists, err := s.queries.CountAgentsByName(ctx, dto.Name)
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	if dto.IsDefault {
		if err := qtx.ClearAllDefaultAgents(ctx); err != nil {
			return nil, err
		}
	}

	row, err := qtx.CreateAgent(ctx, db.CreateAgentParams{
		Name:      dto.Name,
		Soul:      soul,
		IsDefault: dto.IsDefault,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create agent", "error", err)
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		slog.ErrorContext(ctx, "failed to commit create agent", "error", err)
		return nil, err
	}
	return models.AgentRowToDto(row), nil
}

func (s *AgentService) GetAgent(ctx context.Context, agentID uuid.UUID) (*models.AgentDto, error) {
	row, err := s.queries.GetAgentById(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", agentID)
		return nil, err
	}
	return models.AgentRowToDto(row), nil
}

func (s *AgentService) ListAgents(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.AgentDto], error) {
	rows, err := s.queries.ListAgents(ctx, db.ListAgentsParams{
		Limit:  int32(request.PageSize),
		Offset: int32((request.Page - 1) * request.PageSize),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list agents", "error", err)
		return nil, err
	}

	total, err := s.queries.CountAgents(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count agents", "error", err)
		return nil, err
	}

	dtos := lo.Map(rows, func(a db.Agent, _ int) models.AgentDto {
		return *models.AgentRowToDto(a)
	})

	return &pagination.PagedResponse[models.AgentDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *AgentService) UpdateAgent(ctx context.Context, agentID uuid.UUID, dto *models.UpdateAgentDto) error {
	row, err := s.queries.GetAgentById(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", agentID)
		return err
	}

	name := row.Name
	soul := row.Soul

	if dto.Name != nil && *dto.Name != row.Name {
		nameExists, err := s.queries.CountAgentsByNameExcluding(ctx, db.CountAgentsByNameExcludingParams{
			Name: *dto.Name,
			ID:   agentID,
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to check agent name existence", "error", err)
			return err
		}
		if nameExists > 0 {
			return customerrors.ErrAgentAlreadyExists
		}
		name = *dto.Name
	}

	if dto.Soul != nil {
		soul = *dto.Soul
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	if err := qtx.UpdateAgentFields(ctx, db.UpdateAgentFieldsParams{
		ID:   agentID,
		Name: name,
		Soul: soul,
	}); err != nil {
		slog.ErrorContext(ctx, "failed to update agent", "error", err, "agentId", agentID)
		return err
	}

	if dto.IsDefault != nil {
		if *dto.IsDefault {
			if err := qtx.ClearAllDefaultAgents(ctx); err != nil {
				return err
			}
			if err := qtx.SetAgentDefault(ctx, agentID); err != nil {
				return err
			}
		} else {
			if err := qtx.SetAgentNotDefault(ctx, agentID); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		slog.ErrorContext(ctx, "failed to commit update agent", "error", err, "agentId", agentID)
		return err
	}
	return nil
}

func (s *AgentService) DeleteAgent(ctx context.Context, agentID uuid.UUID) error {
	row, err := s.queries.GetAgentById(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", agentID)
		return err
	}

	if row.IsDefault {
		return customerrors.ErrDeletingDefaultAgent
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	if err := qtx.SoftDeleteMessagesByAgent(ctx, agentID); err != nil {
		return err
	}
	if err := qtx.SoftDeleteSessionsByAgent(ctx, agentID); err != nil {
		return err
	}
	if err := qtx.SoftDeleteMemoriesByAgent(ctx, agentID); err != nil {
		return err
	}
	if err := qtx.SoftDeleteAgent(ctx, agentID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		slog.ErrorContext(ctx, "failed to delete agent", "error", err, "agentId", agentID)
		return err
	}
	return nil
}
