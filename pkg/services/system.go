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

	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"gorm.io/gorm"
)

type SystemService struct {
	db *gorm.DB
}

var (
	systemService *SystemService
	systemOnce    sync.Once
)

func GetSystemService() *SystemService {
	systemOnce.Do(func() {
		systemService = &SystemService{db: conn.GetDB()}
	})
	return systemService
}

func (s *SystemService) getOrCreate(ctx context.Context) (*models.SystemSettings, error) {
	settings, err := gorm.G[models.SystemSettings](s.db).
		Where("id = ?", consts.DefaultSystemSettingsID).
		First(ctx)
	if err == nil {
		return &settings, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	settings = models.SystemSettings{ID: consts.DefaultSystemSettingsID, Initialized: false}
	if err := gorm.G[models.SystemSettings](s.db).Create(ctx, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (s *SystemService) IsInitialized(ctx context.Context) (bool, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return false, err
	}
	return settings.Initialized, nil
}

func (s *SystemService) SetInitialized(ctx context.Context) error {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return err
	}

	_, err = gorm.G[models.SystemSettings](s.db).
		Where("id = ?", settings.ID).
		Update(ctx, "initialized", true)
	return err
}

func (s *SystemService) GetSettings(ctx context.Context) (*models.SystemSettingsDto, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	return settings.ToDto(), nil
}

func (s *SystemService) UpdateSettings(ctx context.Context, dto *models.UpdateSystemSettingsDto) (*models.SystemSettingsDto, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any)
	next := *settings

	if dto.Initialized != nil {
		updates["initialized"] = *dto.Initialized
		next.Initialized = *dto.Initialized
	}

	if dto.EmbeddingModelID != nil {
		model, err := gorm.G[models.Model](s.db).
			Where("id = ? AND deleted_at IS NULL AND embedding_model IS TRUE", *dto.EmbeddingModelID).
			First(ctx)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, customerrors.ErrModelNotFound
			}
			slog.ErrorContext(ctx, "failed to find embedding model", "error", err, "modelId", *dto.EmbeddingModelID)
			return nil, err
		}
		updates["embedding_model_id"] = model.ID
		next.EmbeddingModelID = &model.ID
	}

	if dto.ContextCompressionModelID != nil {
		model, err := gorm.G[models.Model](s.db).
			Where("id = ? AND deleted_at IS NULL AND context_compression_model IS TRUE", *dto.ContextCompressionModelID).
			First(ctx)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, customerrors.ErrModelNotFound
			}
			slog.ErrorContext(ctx, "failed to find context compression model", "error", err, "modelId", *dto.ContextCompressionModelID)
			return nil, err
		}
		updates["context_compression_model_id"] = model.ID
		next.ContextCompressionModelID = &model.ID
	}

	if dto.WebSearchProvider != nil {
		provider := *dto.WebSearchProvider
		switch provider {
		case models.WebSearchProviderDisabled, models.WebSearchProviderTavily, models.WebSearchProviderBrave, models.WebSearchProviderFirecrawl:
			updates["web_search_provider"] = provider
			next.WebSearchProvider = provider
		default:
			return nil, customerrors.NewBusinessError(400, "invalid web search provider: "+string(provider))
		}
	}

	if dto.BraveAPIKey != nil {
		updates["brave_api_key"] = *dto.BraveAPIKey
		next.BraveAPIKey = *dto.BraveAPIKey
	}
	if dto.TavilyAPIKey != nil {
		updates["tavily_api_key"] = *dto.TavilyAPIKey
		next.TavilyAPIKey = *dto.TavilyAPIKey
	}
	if dto.FirecrawlAPIKey != nil {
		updates["firecrawl_api_key"] = *dto.FirecrawlAPIKey
		next.FirecrawlAPIKey = *dto.FirecrawlAPIKey
	}
	if dto.FirecrawlBaseURL != nil {
		updates["firecrawl_base_url"] = *dto.FirecrawlBaseURL
		next.FirecrawlBaseURL = *dto.FirecrawlBaseURL
	}

	if provider := lastConfiguredWebSearchProviderFromUpdate(dto); provider != "" {
		updates["last_configured_web_search_provider"] = provider
		next.LastConfiguredWebSearchProvider = provider
	}

	if dto.WebSearchProvider != nil && next.WebSearchProvider != models.WebSearchProviderDisabled &&
		!next.IsWebSearchProviderConfigured(next.WebSearchProvider) {
		return nil, customerrors.NewBusinessError(400, "web search provider is not configured: "+string(next.WebSearchProvider))
	}

	if err := s.db.WithContext(ctx).
		Model(&models.SystemSettings{}).
		Where("id = ?", settings.ID).
		Updates(updates).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update system settings", "error", err)
		return nil, err
	}

	updated, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	return updated.ToDto(), nil
}

func lastConfiguredWebSearchProviderFromUpdate(dto *models.UpdateSystemSettingsDto) models.WebSearchProvider {
	if dto == nil {
		return ""
	}

	provider := models.WebSearchProvider("")
	if dto.BraveAPIKey != nil && strings.TrimSpace(*dto.BraveAPIKey) != "" {
		provider = models.WebSearchProviderBrave
	}
	if dto.TavilyAPIKey != nil && strings.TrimSpace(*dto.TavilyAPIKey) != "" {
		provider = models.WebSearchProviderTavily
	}
	if dto.FirecrawlAPIKey != nil && strings.TrimSpace(*dto.FirecrawlAPIKey) != "" {
		provider = models.WebSearchProviderFirecrawl
	}
	return provider
}
