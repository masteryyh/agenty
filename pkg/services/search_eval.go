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
	"fmt"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"gorm.io/gorm"
)

type SearchQuality string

const (
	SearchQualityHigh      SearchQuality = "high"
	SearchQualityMedium    SearchQuality = "medium"
	SearchQualityLow       SearchQuality = "low"
	SearchQualityNoResults SearchQuality = "no_results"
	SearchQualityError     SearchQuality = "error"
)

type SearchEvaluation struct {
	Quality   SearchQuality `json:"quality"`
	Relevance float64       `json:"relevance"`
	Summary   string        `json:"summary"`
	Reasoning string        `json:"reasoning"`
}

type SearchEvaluator struct {
	db        *gorm.DB
	providers map[models.APIType]provider.ChatProvider
}

var (
	searchEvaluator *SearchEvaluator
	evaluatorOnce   sync.Once
)

func GetSearchEvaluator() *SearchEvaluator {
	evaluatorOnce.Do(func() {
		searchEvaluator = &SearchEvaluator{
			db: conn.GetDB(),
			providers: map[models.APIType]provider.ChatProvider{
				models.APITypeOpenAI:       provider.NewOpenAIProvider(),
				models.APITypeOpenAILegacy: provider.NewOpenAILegacyProvider(),
				models.APITypeAnthropic:    provider.NewAnthropicProvider(),
				models.APITypeKimi:         provider.NewKimiProvider(),
				models.APITypeGemini:       provider.NewGeminiProvider(),
			},
		}
	})
	return searchEvaluator
}

func (e *SearchEvaluator) Evaluate(ctx context.Context, modelID, query, searchResults string) (*SearchEvaluation, error) {
	model, apiProvider, err := e.getModelConfig(ctx, modelID)
	if err != nil {
		return &SearchEvaluation{
			Quality:   SearchQualityError,
			Reasoning: fmt.Sprintf("failed to get model config: %v", err),
		}, nil
	}

	prompt := fmt.Sprintf(consts.SearchEvaluationPrompt, query, searchResults)
	messages := []provider.Message{
		{Role: models.RoleUser, Content: prompt},
	}

	p, ok := e.providers[apiProvider.Type]
	if !ok {
		p = e.providers[models.APITypeOpenAI]
	}

	resp, err := p.Chat(ctx, &provider.ChatRequest{
		Model:    model.Code,
		Messages: messages,
		BaseURL:  apiProvider.BaseURL,
		APIKey:   apiProvider.APIKey,
		APIType:  apiProvider.Type,
		ResponseFormat: &provider.ResponseFormat{
			Type: "json_object",
		},
	})
	if err != nil {
		return &SearchEvaluation{
			Quality:   SearchQualityError,
			Reasoning: fmt.Sprintf("LLM evaluation failed: %v", err),
		}, nil
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var eval SearchEvaluation
	if err := json.UnmarshalString(content, &eval); err != nil {
		return &SearchEvaluation{
			Quality:   SearchQualityMedium,
			Summary:   content,
			Reasoning: "failed to parse evaluation JSON, returning raw response",
		}, nil
	}

	return &eval, nil
}

func (e *SearchEvaluator) getModelConfig(ctx context.Context, modelID string) (*models.Model, *models.ModelProvider, error) {
	var model models.Model
	if err := e.db.WithContext(ctx).
		Where("(id::text = ? OR code = ?) AND deleted_at IS NULL", modelID, modelID).
		First(&model).Error; err != nil {
		return nil, nil, fmt.Errorf("model not found: %w", err)
	}

	var prov models.ModelProvider
	if err := e.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(&prov).Error; err != nil {
		return nil, nil, fmt.Errorf("provider not found: %w", err)
	}

	return &model, &prov, nil
}
