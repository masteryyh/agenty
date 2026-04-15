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

package models

import (
	"time"

	"github.com/google/uuid"
)

type APIType string

const (
	APITypeOpenAI       APIType = "openai"
	APITypeOpenAILegacy APIType = "openai-legacy"
	APITypeAnthropic    APIType = "anthropic"
	APITypeKimi         APIType = "kimi"
	APITypeGemini       APIType = "gemini"
	APITypeBigModel     APIType = "bigmodel"
	APITypeQwen         APIType = "qwen"
)

type ModelProvider struct {
	ID                                uuid.UUID `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name                              string    `gorm:"type:varchar(255);not null"`
	Type                              APIType   `gorm:"type:varchar(50);not null"`
	BaseURL                           string    `gorm:"type:varchar(255);not null"`
	BailianMultiModalEmbeddingBaseURL *string   `gorm:"type:varchar(255)"`
	APIKey                            string    `gorm:"type:varchar(255);not null;default:''" json:"-"`
	IsPreset                          bool      `gorm:"default:false"`
	CreatedAt                         time.Time `gorm:"autoCreateTime"`
	UpdatedAt                         time.Time `gorm:"autoUpdateTime"`
	DeletedAt                         *time.Time
}

func (ModelProvider) TableName() string {
	return "model_providers"
}

func (p *ModelProvider) ToDto() *ModelProviderDto {
	censored := "****"
	if len(p.APIKey) > 10 {
		censored = p.APIKey[:4] + "****" + p.APIKey[len(p.APIKey)-2:]
	} else if p.APIKey == "" {
		censored = "<not set>"
	}

	return &ModelProviderDto{
		ID:                                p.ID,
		Name:                              p.Name,
		Type:                              p.Type,
		BaseURL:                           p.BaseURL,
		BailianMultiModalEmbeddingBaseURL: p.BailianMultiModalEmbeddingBaseURL,
		APIKeyCensored:                    censored,
		IsPreset:                          p.IsPreset,
		CreatedAt:                         p.CreatedAt,
		UpdatedAt:                         p.UpdatedAt,
	}
}

type ModelProviderDto struct {
	ID                                uuid.UUID `json:"id"`
	Name                              string    `json:"name"`
	Type                              APIType   `json:"type"`
	BaseURL                           string    `json:"baseUrl"`
	BailianMultiModalEmbeddingBaseURL *string   `json:"bailianMultiModalEmbeddingBaseUrl,omitempty"`
	APIKeyCensored                    string    `json:"apiKeyCensored"`
	IsPreset                          bool      `json:"isPreset"`
	CreatedAt                         time.Time `json:"createdAt"`
	UpdatedAt                         time.Time `json:"updatedAt"`
}

type CreateModelProviderDto struct {
	Name                              string  `json:"name" binding:"required"`
	Type                              APIType `json:"type" binding:"required,oneof=openai openai-legacy anthropic kimi gemini bigmodel qwen"`
	BaseURL                           string  `json:"baseUrl" binding:"required,url"`
	BailianMultiModalEmbeddingBaseURL *string `json:"bailianMultiModalEmbeddingBaseUrl,omitempty" binding:"omitempty,url"`
	APIKey                            string  `json:"apiKey" binding:"required"`
}

type UpdateModelProviderDto struct {
	Name                              string  `json:"name" binding:"omitempty"`
	Type                              APIType `json:"type" binding:"omitempty,oneof=openai openai-legacy anthropic kimi gemini bigmodel qwen"`
	BaseURL                           string  `json:"baseUrl" binding:"omitempty,url"`
	BailianMultiModalEmbeddingBaseURL *string `json:"bailianMultiModalEmbeddingBaseUrl,omitempty" binding:"omitempty,url"`
	APIKey                            string  `json:"apiKey" binding:"omitempty"`
}
