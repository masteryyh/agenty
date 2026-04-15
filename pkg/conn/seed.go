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
	_ "embed"
	"log/slog"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"gorm.io/gorm"
)

//go:embed presets.json
var presetsJSON []byte

type presetModelJSON struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name"`
	Code                      string   `json:"code"`
	ContextWindow             int      `json:"contextWindow"`
	EmbeddingModel            bool     `json:"embeddingModel"`
	MultiModal                bool     `json:"multiModal"`
	Light                     bool     `json:"light"`
	Thinking                  bool     `json:"thinking"`
	ThinkingLevels            []string `json:"thinkingLevels"`
	AnthropicAdaptiveThinking bool     `json:"anthropicAdaptiveThinking"`
	DefaultModel              bool     `json:"defaultModel"`
}

type presetProviderJSON struct {
	ID                                string            `json:"id"`
	Name                              string            `json:"name"`
	Type                              models.APIType    `json:"type"`
	BaseURL                           string            `json:"baseUrl"`
	BailianMultiModalEmbeddingBaseURL *string           `json:"bailianMultiModalEmbeddingBaseUrl,omitempty"`
	Models                            []presetModelJSON `json:"models"`
}

type presetsFile struct {
	Providers []presetProviderJSON `json:"providers"`
}

func seedPresets(ctx context.Context, db *gorm.DB) error {
	var file presetsFile
	if err := json.Unmarshal(presetsJSON, &file); err != nil {
		return err
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jsonProviderIDs := make([]uuid.UUID, 0, len(file.Providers))
		jsonModelIDs := make([]uuid.UUID, 0)

		for _, pp := range file.Providers {
			providerID, err := uuid.Parse(pp.ID)
			if err != nil {
				return err
			}
			jsonProviderIDs = append(jsonProviderIDs, providerID)

			var existing models.ModelProvider
			err = tx.Where("id = ?", providerID).First(&existing).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}

			if err == gorm.ErrRecordNotFound {
				provider := &models.ModelProvider{
					ID:                                providerID,
					Name:                              pp.Name,
					Type:                              pp.Type,
					BaseURL:                           pp.BaseURL,
					BailianMultiModalEmbeddingBaseURL: pp.BailianMultiModalEmbeddingBaseURL,
					APIKey:                            "",
					IsPreset:                          true,
				}
				if createErr := tx.Create(provider).Error; createErr != nil {
					return createErr
				}
				slog.InfoContext(ctx, "created preset provider", "name", pp.Name)
			} else {
				if updateErr := tx.Model(&existing).Updates(map[string]any{
					"name":       pp.Name,
					"type":       pp.Type,
					"base_url":   pp.BaseURL,
					"is_preset":  true,
					"deleted_at": nil,
				}).Error; updateErr != nil {
					return updateErr
				}
			}

			for _, pm := range pp.Models {
				modelID, err := uuid.Parse(pm.ID)
				if err != nil {
					return err
				}
				jsonModelIDs = append(jsonModelIDs, modelID)

				thinkingLevels := []byte("[]")
				if len(pm.ThinkingLevels) > 0 {
					thinkingLevels, _ = json.Marshal(pm.ThinkingLevels)
				}

				var existingModel models.Model
				err = tx.Where("id = ?", modelID).First(&existingModel).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				}

				if err == gorm.ErrRecordNotFound {
					model := &models.Model{
						ID:                        modelID,
						ProviderID:                providerID,
						Name:                      pm.Name,
						Code:                      pm.Code,
						IsPreset:                  true,
						ContextWindow:             pm.ContextWindow,
						EmbeddingModel:            pm.EmbeddingModel,
						MultiModal:                pm.MultiModal,
						Light:                     pm.Light,
						Thinking:                  pm.Thinking,
						ThinkingLevels:            thinkingLevels,
						AnthropicAdaptiveThinking: pm.AnthropicAdaptiveThinking,
					}
					if createErr := tx.Create(model).Error; createErr != nil {
						return createErr
					}
					slog.InfoContext(ctx, "created preset model", "provider", pp.Name, "model", pm.Name, "code", pm.Code)
				} else {
					if updateErr := tx.Model(&existingModel).Updates(map[string]any{
						"provider_id":                 providerID,
						"name":                        pm.Name,
						"code":                        pm.Code,
						"is_preset":                   true,
						"context_window":              pm.ContextWindow,
						"embedding_model":             pm.EmbeddingModel,
						"multi_modal":                 pm.MultiModal,
						"light":                       pm.Light,
						"thinking":                    pm.Thinking,
						"thinking_levels":             thinkingLevels,
						"anthropic_adaptive_thinking": pm.AnthropicAdaptiveThinking,
						"deleted_at":                  nil,
					}).Error; updateErr != nil {
						return updateErr
					}
				}
			}
		}

		if len(jsonProviderIDs) > 0 {
			if err := tx.Model(&models.ModelProvider{}).
				Where("is_preset = TRUE AND id NOT IN ? AND deleted_at IS NULL", jsonProviderIDs).
				Update("deleted_at", gorm.Expr("NOW()")).Error; err != nil {
				return err
			}
		}

		if len(jsonModelIDs) > 0 {
			if err := tx.Model(&models.Model{}).
				Where("is_preset = TRUE AND id NOT IN ? AND deleted_at IS NULL", jsonModelIDs).
				Update("deleted_at", gorm.Expr("NOW()")).Error; err != nil {
				return err
			}
		}

		var defaultCount int64
		if err := tx.Model(&models.Model{}).
			Where("default_model = TRUE AND deleted_at IS NULL").
			Count(&defaultCount).Error; err != nil {
			return err
		}

		if defaultCount == 0 {
			for _, pp := range file.Providers {
				found := false
				for _, pm := range pp.Models {
					if pm.DefaultModel {
						modelID, _ := uuid.Parse(pm.ID)
						if err := tx.Model(&models.Model{}).
							Where("id = ?", modelID).
							Update("default_model", true).Error; err != nil {
							return err
						}
						slog.InfoContext(ctx, "set default model", "model", pm.Name, "code", pm.Code)
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}

		var agentCount int64
		if err := tx.Model(&models.Agent{}).Where("deleted_at IS NULL").Count(&agentCount).Error; err != nil {
			return err
		}
		if agentCount == 0 {
			agent := &models.Agent{
				Name:      "default",
				Soul:      consts.DefaultAgentSoul,
				IsDefault: true,
			}
			if err := tx.Create(agent).Error; err != nil {
				return err
			}
			slog.InfoContext(ctx, "created default agent", "name", agent.Name)
		}

		return nil
	})
}
