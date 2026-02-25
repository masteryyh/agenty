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
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type ProviderService struct {
	db *gorm.DB
}

var (
	providerService *ProviderService
	providerOnce    sync.Once
)

func GetProviderService() *ProviderService {
	providerOnce.Do(func() {
		providerService = &ProviderService{
			db: conn.GetDB(),
		}
	})
	return providerService
}

func (s *ProviderService) CreateProvider(ctx context.Context, dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error) {
	nameExists, err := gorm.G[models.ModelProvider](s.db).
		Where("name = ? AND deleted_at IS NULL", dto.Name).
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to check provider existence", "error", err)
		return nil, err
	}
	if nameExists > 0 {
		return nil, customerrors.ErrProviderAlreadyExists
	}

	provider := &models.ModelProvider{
		Name:    dto.Name,
		Type:    dto.Type,
		BaseURL: dto.BaseURL,
		APIKey:  dto.APIKey,
	}

	if err := gorm.G[models.ModelProvider](s.db).Create(ctx, provider); err != nil {
		slog.ErrorContext(ctx, "failed to create provider", "error", err)
		return nil, err
	}

	return provider.ToDto(), nil
}

func (s *ProviderService) GetProvider(ctx context.Context, providerID uuid.UUID) (*models.ModelProviderDto, error) {
	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", providerID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "provider_id", providerID)
		return nil, err
	}

	return provider.ToDto(), nil
}

func (s *ProviderService) ListProviders(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.ModelProviderDto], error) {
	var dtos []models.ModelProviderDto
	var total int64

	providers, err := gorm.G[models.ModelProvider](s.db).
		Where("deleted_at IS NULL").
		Offset((request.Page - 1) * request.PageSize).
		Limit(request.PageSize).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list providers", "error", err)
		return nil, err
	}

	countResult, err := gorm.G[models.ModelProvider](s.db).
		Where("deleted_at IS NULL").
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to count providers", "error", err)
		return nil, err
	}
	total = countResult

	dtos = lo.Map(providers, func(p models.ModelProvider, _ int) models.ModelProviderDto {
		return *p.ToDto()
	})

	return &pagination.PagedResponse[models.ModelProviderDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ProviderService) UpdateProvider(ctx context.Context, providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error) {
	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", providerID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "provider_id", providerID)
		return nil, err
	}

	if dto.Name != "" && provider.Name != dto.Name {
		exists, err := gorm.G[models.ModelProvider](s.db).
			Where("name = ? AND id != ? AND deleted_at IS NULL", dto.Name, providerID).
			Count(ctx, "id")
		if err != nil {
			slog.ErrorContext(ctx, "failed to check provider name existence", "error", err)
			return nil, err
		}
		if exists > 0 {
			return nil, customerrors.ErrProviderAlreadyExists
		}
		provider.Name = dto.Name
	}

	if dto.Type != "" {
		provider.Type = dto.Type
	}

	if dto.BaseURL != "" {
		provider.BaseURL = dto.BaseURL
	}

	if dto.APIKey != "" {
		provider.APIKey = dto.APIKey
	}

	if _, err := gorm.G[models.ModelProvider](s.db).Updates(ctx, provider); err != nil {
		slog.ErrorContext(ctx, "failed to update provider", "error", err, "provider_id", providerID)
		return nil, err
	}

	return provider.ToDto(), nil
}

func (s *ProviderService) DeleteProvider(ctx context.Context, providerID uuid.UUID, force bool) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := gorm.G[models.ModelProvider](tx).
			Where("id = ? AND deleted_at IS NULL", providerID).
			First(ctx)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return customerrors.ErrProviderNotFound
			}
			return fmt.Errorf("failed to find provider: %w", err)
		}

		chatModels, err := gorm.G[models.Model](tx).
			Where("provider_id = ? AND deleted_at IS NULL", providerID).
			Find(ctx)
		if err != nil {
			return fmt.Errorf("failed to check provider usage: %w", err)
		}

		if len(chatModels) > 0 {
			if !force {
				return customerrors.ErrProviderInUse
			}
			if _, err := gorm.G[models.Model](tx).
				Where("provider_id = ? AND deleted_at IS NULL", providerID).
				Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
				return fmt.Errorf("failed to delete models under provider: %w", err)
			}
		}

		if _, err := gorm.G[models.ModelProvider](tx).
			Where("id = ? AND deleted_at IS NULL", providerID).
			Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
			return fmt.Errorf("failed to delete provider: %w", err)
		}
		return nil
	}); err != nil {
		if customerrors.GetBusinessError(err) == nil {
			slog.ErrorContext(ctx, "failed to delete provider", "error", err, "provider_id", providerID)
		}
		return err
	}

	return nil
}
