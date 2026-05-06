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
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type BatchEmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)

type EmbeddingService struct {
	db *gorm.DB
}

var (
	embeddingService *EmbeddingService
	embeddingOnce    sync.Once
)

func GetEmbeddingService() *EmbeddingService {
	embeddingOnce.Do(func() {
		embeddingService = &EmbeddingService{
			db: conn.GetDB(),
		}
	})
	return embeddingService
}

func (s *EmbeddingService) IsEnabled(ctx context.Context) bool {
	_, _, err := s.getEmbeddingConfig(ctx)
	return err == nil
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

	embeddingProvider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find embedding model provider: %w", err)
	}

	if embeddingProvider.APIKey == "" {
		return nil, nil, customerrors.ErrProviderNotConfigured
	}
	return &model, &embeddingProvider, nil
}

func (s *EmbeddingService) NewBatchEmbedder(ctx context.Context) (BatchEmbedFunc, error) {
	model, prov, err := s.getEmbeddingConfig(ctx)
	if err != nil {
		return nil, err
	}

	p, ok := providers.ModelProviders[prov.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported embedding provider type: %s", prov.Type)
	}

	baseUrl := prov.BaseURL
	if prov.BailianMultiModalEmbeddingBaseURL != nil && *prov.BailianMultiModalEmbeddingBaseURL != "" {
		baseUrl = *prov.BailianMultiModalEmbeddingBaseURL
	}

	baseReq := providers.EmbeddingRequest{
		Model:      model.Code,
		BaseURL:    baseUrl,
		APIKey:     prov.APIKey,
		Dimensions: consts.DefaultVectorDimension,
	}

	return func(ctx context.Context, texts []string) ([][]float32, error) {
		req := baseReq
		req.Texts = texts
		resp, err := p.Embed(ctx, &req)
		if err != nil {
			return nil, err
		}
		normalized := lo.Map(resp.Embeddings, func(v []float32, _ int) []float32 {
			if v == nil {
				return nil
			}

			if !p.VectorNormalized() {
				return normalizeVector(v)
			}
			return v
		})
		return normalized, nil
	}, nil
}

func (s *EmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	batchFn, err := s.NewBatchEmbedder(ctx)
	if err != nil {
		return nil, err
	}

	results, err := batchFn(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(results) == 0 || results[0] == nil {
		return nil, fmt.Errorf("empty embedding response")
	}
	return results[0], nil
}

func (s *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	batchFn, err := s.NewBatchEmbedder(ctx)
	if err != nil {
		return nil, err
	}
	return batchFn(ctx, texts)
}

func normalizeVector(vec []float32) []float32 {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	if sumSq < consts.Float64Epsilon {
		return vec
	}
	norm := math.Sqrt(sumSq)
	return lo.Map(vec, func(v float32, _ int) float32 {
		return float32(float64(v) / norm)
	})
}
