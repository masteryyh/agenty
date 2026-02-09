package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/samber/lo"
)

type EmbeddingService struct {
	cfg *config.EmbeddingConfig
}

var (
	embeddingService *EmbeddingService
	embeddingOnce    sync.Once
)

func GetEmbeddingService() *EmbeddingService {
	embeddingOnce.Do(func() {
		embeddingService = &EmbeddingService{
			cfg: config.GetConfigManager().GetConfig().Embedding,
		}
	})
	return embeddingService
}

func (s *EmbeddingService) IsEnabled() bool {
	return s.cfg != nil && s.cfg.APIKey != ""
}

func (s *EmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("embedding service is not configured")
	}

	client := conn.GetOpenAIClient(s.cfg.BaseURL, s.cfg.APIKey)

	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: s.cfg.Model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		Dimensions: param.NewOpt(int64(s.cfg.Dimensions)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	return lo.Map(resp.Data[0].Embedding, func(v float64, _ int) float32 {
		return float32(v)
	}), nil
}
