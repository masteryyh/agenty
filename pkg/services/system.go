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
	"github.com/masteryyh/agenty/pkg/version"
	"gorm.io/gorm"
)

type ConfigService struct {
	db *gorm.DB
}

var (
	systemService *ConfigService
	systemOnce    sync.Once
)

func GetConfigService() *ConfigService {
	systemOnce.Do(func() {
		systemService = &ConfigService{db: conn.GetDB()}
	})
	return systemService
}

func (s *ConfigService) getOrCreate(ctx context.Context) (*models.SystemConfig, error) {
	config, err := gorm.G[models.SystemConfig](s.db).
		Where("id = ?", consts.DefaultSystemConfigID).
		First(ctx)
	if err == nil {
		return &config, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	config = models.SystemConfig{ID: consts.DefaultSystemConfigID, Initialized: false}
	if err := gorm.G[models.SystemConfig](s.db).Create(ctx, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *ConfigService) IsInitialized(ctx context.Context) (bool, error) {
	config, err := s.getOrCreate(ctx)
	if err != nil {
		return false, err
	}
	return config.Initialized, nil
}

func (s *ConfigService) SetInitialized(ctx context.Context) error {
	config, err := s.getOrCreate(ctx)
	if err != nil {
		return err
	}

	_, err = gorm.G[models.SystemConfig](s.db).
		Where("id = ?", config.ID).
		Update(ctx, "initialized", true)
	return err
}

func (s *ConfigService) GetConfig(ctx context.Context) (*models.SystemConfigDto, error) {
	config, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	return config.ToDto(), nil
}

func (s *ConfigService) GetVersion(ctx context.Context) (*models.VersionDto, error) {
	return &models.VersionDto{Version: version.Current()}, nil
}

func (s *ConfigService) UpdateConfig(ctx context.Context, dto *models.UpdateSystemConfigDto) (*models.SystemConfigDto, error) {
	config, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any)
	next := *config

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
		Model(&models.SystemConfig{}).
		Where("id = ?", config.ID).
		Updates(updates).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update system config", "error", err)
		return nil, err
	}

	updated, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	return updated.ToDto(), nil
}

func lastConfiguredWebSearchProviderFromUpdate(dto *models.UpdateSystemConfigDto) models.WebSearchProvider {
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
