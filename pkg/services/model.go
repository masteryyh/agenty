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
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type ModelService struct {
	db *gorm.DB
}

var (
	modelService *ModelService
	modelOnce    sync.Once
)

func GetModelService() *ModelService {
	modelOnce.Do(func() {
		modelService = &ModelService{
			db: conn.GetDB(),
		}
	})
	return modelService
}

func (s *ModelService) GetDefault(ctx context.Context) (*models.ModelDto, error) {
	model, err := gorm.G[models.Model](s.db).
		Where("default_model IS TRUE AND deleted_at IS NULL").
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find default model", "error", err)
		return nil, err
	}

	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider for default model", "error", err, "provider_id", model.ProviderID)
		return nil, err
	}
	return model.ToDto(provider.ToDto()), nil
}

func (s *ModelService) CreateModel(ctx context.Context, dto *models.CreateModelDto) (*models.ModelDto, error) {
	var result *models.ModelDto
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		providerExists, err := gorm.G[models.ModelProvider](tx).
			Where("id = ? AND deleted_at IS NULL", dto.ProviderID).
			Count(ctx, "id")
		if err != nil {
			return err
		}
		if providerExists == 0 {
			return customerrors.ErrProviderNotFound
		}

		nameExists, err := gorm.G[models.Model](tx).
			Where("name = ? AND provider_id = ? AND deleted_at IS NULL", dto.Name, dto.ProviderID).
			Count(ctx, "id")
		if err != nil {
			return err
		}
		if nameExists > 0 {
			return customerrors.ErrModelAlreadyExists
		}

		codeExists, err := gorm.G[models.Model](tx).
			Where("code = ? AND provider_id = ? AND deleted_at IS NULL", dto.Code, dto.ProviderID).
			Count(ctx, "id")
		if err != nil {
			return err
		}
		if codeExists > 0 {
			return customerrors.ErrModelAlreadyExists
		}

		model := &models.Model{
			ProviderID:   dto.ProviderID,
			Name:         dto.Name,
			Code:         dto.Code,
			DefaultModel: dto.DefaultModel,
		}

		if err := gorm.G[models.Model](tx).Create(ctx, model); err != nil {
			return err
		}
		result = model.ToDto(nil)
		return nil
	}); err != nil {
		if customerrors.GetBusinessError(err) == nil {
			slog.ErrorContext(ctx, "failed to create model", "error", err)
		}
		return nil, err
	}
	return result, nil
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

	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("name = ? AND deleted_at IS NULL", providerName).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "providerName", providerName)
		return err
	}

	model, err := gorm.G[models.Model](s.db).
		Where("name = ? AND provider_id = ? AND deleted_at IS NULL", modelName, provider.ID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelName", modelName, "providerId", provider.ID)
		return err
	}
	return s.UpdateModel(ctx, model.ID, dto)
}

func (s *ModelService) UpdateModel(ctx context.Context, modelID uuid.UUID, dto *models.UpdateModelDto) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model, err := gorm.G[models.Model](tx).
			Where("id = ? AND deleted_at IS NULL", modelID).
			First(ctx)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return customerrors.ErrModelNotFound
			}
			return err
		}

		if dto.Name != "" && dto.Name != model.Name {
			exists, err := gorm.G[models.Model](tx).
				Where("name = ? AND provider_id = ? AND id != ? AND deleted_at IS NULL", dto.Name, model.ProviderID, modelID).
				Count(ctx, "id")
			if err != nil {
				return err
			}

			if exists > 0 {
				return customerrors.ErrModelAlreadyExists
			}
		}

		if !dto.DefaultModel {
			_, err := gorm.G[models.Model](tx).
				Where("id = ? AND deleted_at IS NULL", modelID).
				Update(ctx, "default_model", false)
			if err != nil {
				return err
			}
			return nil
		}

		currentDefaultModel, err := gorm.G[models.Model](tx).
			Where("default_model IS TRUE AND deleted_at IS NULL").
			First(ctx)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		if currentDefaultModel.ID != uuid.Nil {
			_, err := gorm.G[models.Model](tx).
				Where("id = ? AND deleted_at IS NULL", currentDefaultModel.ID).
				Update(ctx, "default_model", false)
			if err != nil {
				return err
			}
		}

		if _, err := gorm.G[models.Model](tx).
			Where("id = ? AND deleted_at IS NULL", modelID).
			Update(ctx, "default_model", true); err != nil {
			return err
		}
		return nil
	}); err != nil {
		if customerrors.GetBusinessError(err) == nil {
			slog.ErrorContext(ctx, "failed to update model", "error", err, "modelId", modelID)
		}
		return err
	}
	return nil
}

func (s *ModelService) GetModel(ctx context.Context, modelID uuid.UUID) (*models.ModelDto, error) {
	model, err := gorm.G[models.Model](s.db).
		Where("id = ? AND deleted_at IS NULL", modelID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", modelID)
		return nil, err
	}

	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider for model", "error", err, "provider_id", model.ProviderID)
		return nil, err
	}
	return model.ToDto(provider.ToDto()), nil
}

func (s *ModelService) ListModels(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.ModelDto], error) {
	modelsResult, err := gorm.G[models.Model](s.db).
		Where("deleted_at IS NULL").
		Offset((request.Page - 1) * request.PageSize).
		Limit(request.PageSize).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list models", "error", err)
		return nil, err
	}

	countResult, err := gorm.G[models.Model](s.db).
		Where("deleted_at IS NULL").
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to count models", "error", err)
		return nil, err
	}

	if len(modelsResult) == 0 {
		return &pagination.PagedResponse[models.ModelDto]{
			Total:    countResult,
			PageSize: request.PageSize,
			Page:     request.Page,
			Data:     []models.ModelDto{},
		}, nil
	}

	providerIds := lo.Uniq(lo.Map(modelsResult, func(m models.Model, _ int) uuid.UUID {
		return m.ProviderID
	}))
	providers, err := gorm.G[models.ModelProvider](s.db).
		Where("id IN ? AND deleted_at IS NULL", providerIds).
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find model providers", "error", err)
		return nil, err
	}
	providerMap := lo.Associate(providers, func(p models.ModelProvider) (uuid.UUID, *models.ModelProviderDto) {
		return p.ID, p.ToDto()
	})

	dtos := lo.Map(modelsResult, func(m models.Model, _ int) models.ModelDto {
		return *m.ToDto(providerMap[m.ProviderID])
	})

	return &pagination.PagedResponse[models.ModelDto]{
		Total:    countResult,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ModelService) ListModelsByProvider(ctx context.Context, providerID uuid.UUID, request *pagination.PageRequest) (*pagination.PagedResponse[models.ModelDto], error) {
	providerExists, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", providerID).
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to check provider existence", "error", err)
		return nil, err
	}
	if providerExists == 0 {
		return nil, customerrors.ErrProviderNotFound
	}

	var dtos []models.ModelDto
	var total int64

	modelsResult, err := gorm.G[models.Model](s.db).
		Where("provider_id = ? AND deleted_at IS NULL", providerID).
		Offset((request.Page - 1) * request.PageSize).
		Limit(request.PageSize).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find models", "error", err)
		return nil, err
	}

	countResult, err := gorm.G[models.Model](s.db).
		Where("provider_id = ? AND deleted_at IS NULL", providerID).
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to count models", "error", err)
		return nil, err
	}
	total = countResult

	dtos = lo.Map(modelsResult, func(m models.Model, _ int) models.ModelDto {
		return *m.ToDto(nil)
	})

	return &pagination.PagedResponse[models.ModelDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ModelService) DeleteModel(ctx context.Context, modelID uuid.UUID) error {
	model, err := gorm.G[models.Model](s.db).
		Where("id = ? AND deleted_at IS NULL", modelID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrModelNotFound
		}

		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", modelID)
		return err
	}

	if model.DefaultModel {
		return customerrors.ErrDeletingDefaultModel
	}

	if _, err := gorm.G[models.Model](s.db).
		Where("id = ? AND deleted_at IS NULL", modelID).
		Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
		slog.ErrorContext(ctx, "failed to delete model", "error", err, "modelId", modelID)
		return err
	}

	return nil
}
