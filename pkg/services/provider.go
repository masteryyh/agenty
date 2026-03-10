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
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/db"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
)

type ProviderService struct {
	db      *sql.DB
	queries *db.Queries
}

var (
	providerService *ProviderService
	providerOnce    sync.Once
)

func GetProviderService() *ProviderService {
	providerOnce.Do(func() {
		sqlDB := conn.GetSQLDB()
		providerService = &ProviderService{
			db:      sqlDB,
			queries: db.New(sqlDB),
		}
	})
	return providerService
}

func (s *ProviderService) CreateProvider(ctx context.Context, dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error) {
	count, err := s.queries.CountProvidersByName(ctx, dto.Name)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check provider existence", "error", err)
		return nil, err
	}
	if count > 0 {
		return nil, customerrors.ErrProviderAlreadyExists
	}

	row, err := s.queries.CreateProvider(ctx, db.CreateProviderParams{
		Name:    dto.Name,
		Type:    string(dto.Type),
		BaseUrl: dto.BaseURL,
		ApiKey:  dto.APIKey,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create provider", "error", err)
		return nil, err
	}

	return models.ModelProviderRowToDto(row), nil
}

func (s *ProviderService) GetProvider(ctx context.Context, providerID uuid.UUID) (*models.ModelProviderDto, error) {
	row, err := s.queries.GetProviderById(ctx, providerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "provider_id", providerID)
		return nil, err
	}

	return models.ModelProviderRowToDto(row), nil
}

func (s *ProviderService) ListProviders(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.ModelProviderDto], error) {
	rows, err := s.queries.ListProviders(ctx, db.ListProvidersParams{
		Limit:  int32(request.PageSize),
		Offset: int32((request.Page - 1) * request.PageSize),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list providers", "error", err)
		return nil, err
	}

	total, err := s.queries.CountProviders(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count providers", "error", err)
		return nil, err
	}

	dtos := lo.Map(rows, func(row db.ModelProvider, _ int) models.ModelProviderDto {
		return *models.ModelProviderRowToDto(row)
	})

	return &pagination.PagedResponse[models.ModelProviderDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ProviderService) UpdateProvider(ctx context.Context, providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error) {
	row, err := s.queries.GetProviderById(ctx, providerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "provider_id", providerID)
		return nil, err
	}

	if dto.Name != "" && row.Name != dto.Name {
		count, err := s.queries.CountProvidersByNameExcluding(ctx, db.CountProvidersByNameExcludingParams{
			Name: dto.Name,
			ID:   providerID,
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to check provider name existence", "error", err)
			return nil, err
		}
		if count > 0 {
			return nil, customerrors.ErrProviderAlreadyExists
		}
		row.Name = dto.Name
	}

	if dto.Type != "" {
		row.Type = string(dto.Type)
	}
	if dto.BaseURL != "" {
		row.BaseUrl = dto.BaseURL
	}
	if dto.APIKey != "" {
		row.ApiKey = dto.APIKey
	}

	updated, err := s.queries.UpdateProvider(ctx, db.UpdateProviderParams{
		ID:      providerID,
		Name:    row.Name,
		Type:    row.Type,
		BaseUrl: row.BaseUrl,
		ApiKey:  row.ApiKey,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to update provider", "error", err, "provider_id", providerID)
		return nil, err
	}

	return models.ModelProviderRowToDto(updated), nil
}

func (s *ProviderService) DeleteProvider(ctx context.Context, providerID uuid.UUID, force bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction", "error", err, "provider_id", providerID)
		return err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	if _, err := qtx.GetProviderById(ctx, providerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return customerrors.ErrProviderNotFound
		}
		return fmt.Errorf("failed to find provider: %w", err)
	}

	chatModels, err := qtx.GetModelsByProvider(ctx, providerID)
	if err != nil {
		return fmt.Errorf("failed to check provider usage: %w", err)
	}

	if len(chatModels) > 0 {
		if !force {
			return customerrors.ErrProviderInUse
		}
		if err := qtx.SoftDeleteModelsByProvider(ctx, providerID); err != nil {
			return fmt.Errorf("failed to delete models under provider: %w", err)
		}
	}

	if err := qtx.SoftDeleteProvider(ctx, providerID); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	if err := tx.Commit(); err != nil {
		if customerrors.GetBusinessError(err) == nil {
			slog.ErrorContext(ctx, "failed to delete provider", "error", err, "provider_id", providerID)
		}
		return err
	}
	return nil
}

