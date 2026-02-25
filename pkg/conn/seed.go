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

package conn

import (
	"context"
	"log/slog"

	"github.com/masteryyh/agenty/pkg/models"
	"gorm.io/gorm"
)

type presetProvider struct {
	Name    string
	Type    models.APIType
	BaseURL string
	Models  []presetModel
}

type presetModel struct {
	Name         string
	Code         string
	DefaultModel bool
}

var presetProviders = []presetProvider{
	{
		Name:    "OpenAI",
		Type:    models.APITypeOpenAI,
		BaseURL: "https://api.openai.com/v1",
		Models: []presetModel{
			{Name: "GPT-5.3 Codex", Code: "gpt-5.3-codex"},
			{Name: "GPT-5.2", Code: "gpt-5.2"},
			{Name: "GPT-4o", Code: "gpt-4o-2024-11-20"},
		},
	},
	{
		Name:    "Google",
		Type:    models.APITypeGemini,
		BaseURL: "https://generativelanguage.googleapis.com",
		Models: []presetModel{
			{Name: "Gemini 3.1 Pro Preview", Code: "gemini-3.1-pro-preview"},
			{Name: "Gemini 3 Pro Preview", Code: "gemini-3-pro-preview"},
			{Name: "Gemini 3 Flash Preview", Code: "gemini-3-flash-preview"},
		},
	},
	{
		Name:    "Anthropic",
		Type:    models.APITypeAnthropic,
		BaseURL: "https://api.anthropic.com",
		Models: []presetModel{
			{Name: "Claude Opus 4.6", Code: "claude-opus-4-6"},
			{Name: "Claude Sonnet 4.6", Code: "claude-sonnet-4-6"},
			{Name: "Claude Haiku 4.5", Code: "claude-haiku-4-5-20251001"},
		},
	},
	{
		Name:    "Kimi",
		Type:    models.APITypeKimi,
		BaseURL: "https://api.moonshot.cn/v1",
		Models: []presetModel{
			{Name: "Kimi k2.5", Code: "kimi-k2.5", DefaultModel: true},
		},
	},
}

func seedPresets(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, pp := range presetProviders {
			var count int64
			if err := tx.Model(&models.ModelProvider{}).
				Where("name = ? AND type = ? AND deleted_at IS NULL", pp.Name, pp.Type).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}

			provider := &models.ModelProvider{
				Name:    pp.Name,
				Type:    pp.Type,
				BaseURL: pp.BaseURL,
				APIKey:  "",
			}
			if err := tx.Create(provider).Error; err != nil {
				return err
			}
			slog.InfoContext(ctx, "created preset provider", "name", pp.Name)

			for _, pm := range pp.Models {
				model := &models.Model{
					ProviderID:   provider.ID,
					Name:         pm.Name,
					Code:         pm.Code,
					DefaultModel: pm.DefaultModel,
				}
				if err := tx.Create(model).Error; err != nil {
					return err
				}
				slog.InfoContext(ctx, "created preset model", "provider", pp.Name, "model", pm.Name, "code", pm.Code)
			}
		}
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to seed preset providers and models", "error", err)
		return err
	}
	return nil
}
