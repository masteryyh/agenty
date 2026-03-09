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
	stdjson "encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/db"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
)

type ModelService struct {
	db      *sql.DB
	queries *db.Queries
}

var (
	modelService *ModelService
	modelOnce    sync.Once
)

func GetModelService() *ModelService {
	modelOnce.Do(func() {
		sqlDB := conn.GetSQLDB()
		modelService = &ModelService{
			db:      sqlDB,
			queries: db.New(sqlDB),
		}
	})
	return modelService
}

func (s *ModelService) GetDefault(ctx context.Context) (*models.ModelDto, error) {
	modelRow, err := s.queries.GetDefaultModel(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find default model", "error", err)
		return nil, err
	}

	providerRow, err := s.queries.GetProviderById(ctx, modelRow.ProviderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider for default model", "error", err, "provider_id", modelRow.ProviderID)
		return nil, err
	}
	return models.ModelRowToDto(modelRow, models.ModelProviderRowToDto(providerRow)), nil
}

func (s *ModelService) CreateModel(ctx context.Context, dto *models.CreateModelDto) (*models.ModelDto, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction", "error", err)
		return nil, err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	providerRow, err := qtx.GetProviderById(ctx, dto.ProviderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		return nil, err
	}

	nameCount, err := qtx.CountModelsByName(ctx, db.CountModelsByNameParams{
		Name:       dto.Name,
		ProviderID: dto.ProviderID,
	})
	if err != nil {
		return nil, err
	}
	if nameCount > 0 {
		return nil, customerrors.ErrModelAlreadyExists
	}

	codeCount, err := qtx.CountModelsByCode(ctx, db.CountModelsByCodeParams{
		Code:       dto.Code,
		ProviderID: dto.ProviderID,
	})
	if err != nil {
		return nil, err
	}
	if codeCount > 0 {
		return nil, customerrors.ErrModelAlreadyExists
	}

	thinkingLevels := stdjson.RawMessage("[]")
	if dto.Thinking && len(dto.ThinkingLevels) > 0 {
		if tl, merr := json.Marshal(dto.ThinkingLevels); merr == nil {
			thinkingLevels = tl
		}
	}

	adaptiveThinking := dto.Thinking && providerRow.Type == string(models.APITypeAnthropic) && dto.AnthropicAdaptiveThinking

	row, err := qtx.CreateModel(ctx, db.CreateModelParams{
		ProviderID:                dto.ProviderID,
		Name:                      dto.Name,
		Code:                      dto.Code,
		DefaultModel:              false,
		Thinking:                  dto.Thinking,
		ThinkingLevels:            thinkingLevels,
		AnthropicAdaptiveThinking: adaptiveThinking,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		slog.ErrorContext(ctx, "failed to create model", "error", err)
		return nil, err
	}
	return models.ModelRowToDto(row, nil), nil
}

func (s *ModelService) UpdateByName(ctx context.Context, name string, dto *models.UpdateModelDto) error {
	if name == "" || !strings.Contains(name, "/") {
		return customerrors.ErrInvalidParams
	}

	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return customerrors.ErrInvalidParams
	}
	providerName, modelName := parts[0], parts[1]

	providerRow, err := s.queries.GetProviderByName(ctx, providerName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "providerName", providerName)
		return err
	}

	modelRow, err := s.queries.GetModelByNameAndProvider(ctx, db.GetModelByNameAndProviderParams{
		Name:       modelName,
		ProviderID: providerRow.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelName", modelName, "providerId", providerRow.ID)
		return err
	}
	return s.UpdateModel(ctx, modelRow.ID, dto)
}

func (s *ModelService) UpdateModel(ctx context.Context, modelID uuid.UUID, dto *models.UpdateModelDto) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	modelRow, err := qtx.GetModelById(ctx, modelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrModelNotFound
		}
		return err
	}

	name := modelRow.Name
	if dto.Name != "" && dto.Name != modelRow.Name {
		count, err := qtx.CountModelsByNameExcluding(ctx, db.CountModelsByNameExcludingParams{
			Name:       dto.Name,
			ProviderID: modelRow.ProviderID,
			ID:         modelID,
		})
		if err != nil {
			return err
		}
		if count > 0 {
			return customerrors.ErrModelAlreadyExists
		}
		name = dto.Name
	}

	if !dto.DefaultModel {
		if err := qtx.SetModelDefaultFalse(ctx, modelID); err != nil {
			return err
		}
	} else {
		currentDefault, err := qtx.GetCurrentDefaultExcluding(ctx, modelID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if currentDefault.ID != uuid.Nil {
			if err := qtx.SetModelDefaultFalse(ctx, currentDefault.ID); err != nil {
				return err
			}
		}
		if err := qtx.SetModelAsDefault(ctx, modelID); err != nil {
			return err
		}
	}

	thinkingLevels := stdjson.RawMessage("[]")
	if dto.Thinking && len(dto.ThinkingLevels) > 0 {
		if tl, merr := json.Marshal(dto.ThinkingLevels); merr == nil {
			thinkingLevels = tl
		}
	}

	adaptiveThinking := false
	if dto.Thinking && dto.AnthropicAdaptiveThinking {
		providerRow, err := qtx.GetProviderById(ctx, modelRow.ProviderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return customerrors.ErrProviderNotFound
			}
			return err
		}
		if providerRow.Type == string(models.APITypeAnthropic) {
			adaptiveThinking = true
		}
	}

	if err := qtx.UpdateModelFields(ctx, db.UpdateModelFieldsParams{
		ID:                        modelID,
		Name:                      name,
		Thinking:                  dto.Thinking,
		ThinkingLevels:            thinkingLevels,
		AnthropicAdaptiveThinking: adaptiveThinking,
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		slog.ErrorContext(ctx, "failed to update model", "error", err, "modelId", modelID)
		return err
	}
	return nil
}

func (s *ModelService) GetModel(ctx context.Context, modelID uuid.UUID) (*models.ModelDto, error) {
	modelRow, err := s.queries.GetModelById(ctx, modelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", modelID)
		return nil, err
	}

	providerRow, err := s.queries.GetProviderById(ctx, modelRow.ProviderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider for model", "error", err, "provider_id", modelRow.ProviderID)
		return nil, err
	}
	return models.ModelRowToDto(modelRow, models.ModelProviderRowToDto(providerRow)), nil
}

func (s *ModelService) ListModels(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.ModelDto], error) {
	rows, err := s.queries.ListModels(ctx, db.ListModelsParams{
		Limit:  int32(request.PageSize),
		Offset: int32((request.Page - 1) * request.PageSize),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list models", "error", err)
		return nil, err
	}

	total, err := s.queries.CountModels(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count models", "error", err)
		return nil, err
	}

	if len(rows) == 0 {
		return &pagination.PagedResponse[models.ModelDto]{
			Total:    total,
			PageSize: request.PageSize,
			Page:     request.Page,
			Data:     []models.ModelDto{},
		}, nil
	}

	providerIDs := lo.Uniq(lo.Map(rows, func(r db.Model, _ int) uuid.UUID { return r.ProviderID }))
	providerMap := make(map[uuid.UUID]*models.ModelProviderDto, len(providerIDs))
	providerRows, err := s.queries.ListProvidersByIds(ctx, providerIDs)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find model providers", "error", err)
		return nil, err
	}
	for _, prow := range providerRows {
		providerMap[prow.ID] = models.ModelProviderRowToDto(prow)
	}

	dtos := lo.Map(rows, func(r db.Model, _ int) models.ModelDto {
		return *models.ModelRowToDto(r, providerMap[r.ProviderID])
	})

	return &pagination.PagedResponse[models.ModelDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ModelService) ListModelsByProvider(ctx context.Context, providerID uuid.UUID, request *pagination.PageRequest) (*pagination.PagedResponse[models.ModelDto], error) {
	if _, err := s.queries.GetProviderById(ctx, providerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to check provider existence", "error", err)
		return nil, err
	}

	rows, err := s.queries.ListModelsByProvider(ctx, db.ListModelsByProviderParams{
		ProviderID: providerID,
		Limit:      int32(request.PageSize),
		Offset:     int32((request.Page - 1) * request.PageSize),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to find models", "error", err)
		return nil, err
	}

	total, err := s.queries.CountModelsByProvider(ctx, providerID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count models", "error", err)
		return nil, err
	}

	dtos := lo.Map(rows, func(r db.Model, _ int) models.ModelDto {
		return *models.ModelRowToDto(r, nil)
	})

	return &pagination.PagedResponse[models.ModelDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ModelService) GetThinkingLevels(ctx context.Context, modelID uuid.UUID) ([]string, error) {
	modelRow, err := s.queries.GetModelById(ctx, modelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", modelID)
		return nil, err
	}

	if !modelRow.Thinking {
		return []string{}, nil
	}

	if modelRow.AnthropicAdaptiveThinking {
		return []string{"adaptive"}, nil
	}

	var levels []string
	if err := json.Unmarshal(modelRow.ThinkingLevels, &levels); err != nil {
		return []string{}, nil
	}

	if len(levels) == 0 {
		return []string{"on"}, nil
	}
	return levels, nil
}

func (s *ModelService) DeleteModel(ctx context.Context, modelID uuid.UUID) error {
	modelRow, err := s.queries.GetModelById(ctx, modelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", modelID)
		return err
	}

	if modelRow.DefaultModel {
		return customerrors.ErrDeletingDefaultModel
	}

	if err := s.queries.SoftDeleteModel(ctx, modelID); err != nil {
		slog.ErrorContext(ctx, "failed to delete model", "error", err, "modelId", modelID)
		return err
	}
	return nil
}
