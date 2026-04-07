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
	"math"
	"sync"

	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type EmbeddingService struct {
	db *gorm.DB
}

var (
	embeddingService *EmbeddingService
	embeddingOnce    sync.Once
)

func GetEmbeddingService() *EmbeddingService {
	embeddingOnce.Do(func() {
		embeddingService = &EmbeddingService{db: conn.GetDB()}
	})
	return embeddingService
}

func (s *EmbeddingService) IsEnabled(ctx context.Context) bool {
	settings, err := GetSystemService().getOrCreate(ctx)
	if err != nil {
		return false
	}
	return settings.EmbeddingModelID != nil
}

func (s *EmbeddingService) getEmbeddingConfig(ctx context.Context) (*models.Model, *models.ModelProvider, error) {
	settings, err := GetSystemService().getOrCreate(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get system settings: %w", err)
	}
	if settings.EmbeddingModelID == nil {
		return nil, nil, fmt.Errorf("embedding model is not configured")
	}

	model, err := gorm.G[models.Model](s.db).
		Where("id = ? AND deleted_at IS NULL AND embedding_model IS TRUE", *settings.EmbeddingModelID).
		First(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find embedding model: %w", err)
	}

	provider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find embedding model provider: %w", err)
	}

	return &model, &provider, nil
}

func (s *EmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	model, provider, err := s.getEmbeddingConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := conn.GetOpenAIClient(provider.BaseURL, provider.APIKey)
	return s.embedWithClient(ctx, client, model.Code, text)
}

func (s *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	model, provider, err := s.getEmbeddingConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := conn.GetOpenAIClient(provider.BaseURL, provider.APIKey)
	return s.embedBatchWithClient(ctx, client, model.Code, texts)
}

func (s *EmbeddingService) GetClient(ctx context.Context) (*openai.Client, string, error) {
	model, provider, err := s.getEmbeddingConfig(ctx)
	if err != nil {
		return nil, "", err
	}
	return conn.GetOpenAIClient(provider.BaseURL, provider.APIKey), model.Code, nil
}

func (s *EmbeddingService) embedWithClient(ctx context.Context, client *openai.Client, modelCode string, text string) ([]float32, error) {
	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: modelCode,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		Dimensions: param.NewOpt(int64(consts.DefaultVectorDimension)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	vec := lo.Map(resp.Data[0].Embedding, func(v float64, _ int) float32 {
		return float32(v)
	})
	return NormalizeVector(vec), nil
}

func (s *EmbeddingService) embedBatchWithClient(ctx context.Context, client *openai.Client, modelCode string, texts []string) ([][]float32, error) {
	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: modelCode,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		Dimensions: param.NewOpt(int64(consts.DefaultVectorDimension)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create batch embeddings: %w", err)
	}

	result := make([][]float32, len(resp.Data))
	for _, item := range resp.Data {
		if int(item.Index) >= len(result) {
			continue
		}
		vec := lo.Map(item.Embedding, func(v float64, _ int) float32 {
			return float32(v)
		})
		result[item.Index] = NormalizeVector(vec)
	}
	return result, nil
}

func NormalizeVector(vec []float32) []float32 {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	return lo.Map(vec, func(v float32, _ int) float32 {
		return float32(float64(v) / norm)
	})
}
