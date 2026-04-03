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

	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/masteryyh/agenty/pkg/utils/signal"
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

func (s *SystemService) setEmbeddingMigrating(ctx context.Context, dbConn *gorm.DB, migrating bool) error {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return err
	}

	db := s.db.WithContext(ctx)
	if dbConn != nil {
		db = dbConn
	}

	_, err = gorm.G[models.SystemSettings](db).
		Where("id = ?", settings.ID).
		Update(ctx, "embedding_migrating", migrating)
	return err
}

func (s *SystemService) UpdateSettings(ctx context.Context, dto *models.UpdateSystemSettingsDto) (*models.SystemSettingsDto, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any)

	if dto.Initialized != nil {
		updates["initialized"] = *dto.Initialized
	}

	triggerReEmbed := false
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
		if settings.EmbeddingModelID == nil || *settings.EmbeddingModelID != model.ID {
			triggerReEmbed = true
		}
		updates["embedding_model_id"] = model.ID
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
	}

	if dto.WebSearchProvider != nil {
		provider := *dto.WebSearchProvider
		switch provider {
		case models.WebSearchProviderDisabled, models.WebSearchProviderTavily, models.WebSearchProviderBrave, models.WebSearchProviderFirecrawl:
			updates["web_search_provider"] = provider
		default:
			return nil, customerrors.NewBusinessError(400, "invalid web search provider: "+string(provider))
		}
	}

	if dto.BraveAPIKey != nil {
		updates["brave_api_key"] = *dto.BraveAPIKey
	}
	if dto.TavilyAPIKey != nil {
		updates["tavily_api_key"] = *dto.TavilyAPIKey
	}
	if dto.FirecrawlAPIKey != nil {
		updates["firecrawl_api_key"] = *dto.FirecrawlAPIKey
	}
	if dto.FirecrawlBaseURL != nil {
		updates["firecrawl_base_url"] = *dto.FirecrawlBaseURL
	}

	if err := s.db.WithContext(ctx).
		Model(&models.SystemSettings{}).
		Where("id = ?", settings.ID).
		Updates(updates).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update system settings", "error", err)
		return nil, err
	}

	if triggerReEmbed {
		safe.GoOnce("memory-re-embed", func() {
			if err := GetMemoryService().ReEmbedAll(signal.GetBaseContext()); err != nil {
				slog.Error("background re-embedding failed", "error", err)
			}
		})
	}

	updated, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	return updated.ToDto(), nil
}
