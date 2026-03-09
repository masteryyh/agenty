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
	"github.com/masteryyh/agenty/pkg/db"
)

type APIType string

const (
	APITypeOpenAI       APIType = "openai"
	APITypeOpenAILegacy APIType = "openai-legacy"
	APITypeAnthropic    APIType = "anthropic"
	APITypeKimi         APIType = "kimi"
	APITypeGemini       APIType = "gemini"
)

type ModelProviderDto struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Type           APIType   `json:"type"`
	BaseURL        string    `json:"baseUrl"`
	APIKeyCensored string    `json:"apiKeyCensored"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func ModelProviderRowToDto(row db.ModelProvider) *ModelProviderDto {
	censored := "****"
	if len(row.ApiKey) > 10 {
		censored = row.ApiKey[:4] + "****" + row.ApiKey[len(row.ApiKey)-2:]
	} else if row.ApiKey == "" {
		censored = "<not set>"
	}

	return &ModelProviderDto{
		ID:             row.ID,
		Name:           row.Name,
		Type:           APIType(row.Type),
		BaseURL:        row.BaseUrl,
		APIKeyCensored: censored,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

type CreateModelProviderDto struct {
	Name    string  `json:"name" binding:"required"`
	Type    APIType `json:"type" binding:"required,oneof=openai openai-legacy anthropic kimi gemini"`
	BaseURL string  `json:"baseUrl" binding:"required,url"`
	APIKey  string  `json:"apiKey" binding:"required"`
}

type UpdateModelProviderDto struct {
	Name    string  `json:"name" binding:"omitempty"`
	Type    APIType `json:"type" binding:"omitempty,oneof=openai openai-legacy anthropic kimi gemini"`
	BaseURL string  `json:"baseUrl" binding:"omitempty,url"`
	APIKey  string  `json:"apiKey" binding:"omitempty"`
}
