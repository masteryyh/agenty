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
	APITypeOpenAI    APIType = "openai"
	APITypeAnthropic APIType = "anthropic"
	APITypeKimi      APIType = "kimi"
	APITypeGemini    APIType = "gemini"
)

type ModelProvider struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name      string    `gorm:"type:varchar(255);not null"`
	Type      APIType   `gorm:"type:varchar(50);not null"`
	BaseURL   string    `gorm:"type:varchar(255);not null"`
	APIKey    string    `gorm:"type:varchar(255);not null;default:''" json:"-"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	DeletedAt *time.Time
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
		ID:             p.ID,
		Name:           p.Name,
		Type:           p.Type,
		BaseURL:        p.BaseURL,
		APIKeyCensored: censored,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

type CreateModelProviderDto struct {
	Name    string  `json:"name" binding:"required"`
	Type    APIType `json:"type" binding:"required,oneof=openai anthropic kimi gemini"`
	BaseURL string  `json:"baseUrl" binding:"required,url"`
	APIKey  string  `json:"apiKey" binding:"required"`
}

type UpdateModelProviderDto struct {
	Name    string  `json:"name" binding:"omitempty"`
	Type    APIType `json:"type" binding:"omitempty,oneof=openai anthropic kimi gemini"`
	BaseURL string  `json:"baseUrl" binding:"omitempty,url"`
	APIKey  string  `json:"apiKey" binding:"omitempty"`
}

type ModelProviderDto struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Type           APIType   `json:"type"`
	BaseURL        string    `json:"baseUrl"`
	APIKeyCensored string    `json:"apiKeyCensored"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
