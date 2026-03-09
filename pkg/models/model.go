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

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/db"
)

type ModelDto struct {
	ID                        uuid.UUID         `json:"id"`
	Provider                  *ModelProviderDto `json:"provider,omitempty"`
	Name                      string            `json:"name"`
	Code                      string            `json:"code"`
	DefaultModel              bool              `json:"defaultModel"`
	Thinking                  bool              `json:"thinking"`
	ThinkingLevels            []string          `json:"thinkingLevels"`
	AnthropicAdaptiveThinking bool              `json:"anthropicAdaptiveThinking"`
	CreatedAt                 time.Time         `json:"createdAt"`
	UpdatedAt                 time.Time         `json:"updatedAt"`
}

func ModelRowToDto(row db.Model, provider *ModelProviderDto) *ModelDto {
	var thinkingLevels []string
	if err := json.Unmarshal(row.ThinkingLevels, &thinkingLevels); err != nil {
		thinkingLevels = []string{}
	}

	dto := &ModelDto{
		ID:                        row.ID,
		Name:                      row.Name,
		Code:                      row.Code,
		DefaultModel:              row.DefaultModel,
		Thinking:                  row.Thinking,
		ThinkingLevels:            thinkingLevels,
		AnthropicAdaptiveThinking: row.AnthropicAdaptiveThinking,
		CreatedAt:                 row.CreatedAt,
		UpdatedAt:                 row.UpdatedAt,
	}

	if provider != nil {
		dto.Provider = provider
	}
	return dto
}

type CreateModelDto struct {
	ProviderID                uuid.UUID `json:"providerId" binding:"required"`
	Name                      string    `json:"name" binding:"required"`
	Code                      string    `json:"code" binding:"required,code"`
	Thinking                  bool      `json:"thinking" binding:"required"`
	ThinkingLevels            []string  `json:"thinkingLevels" binding:"omitempty"`
	AnthropicAdaptiveThinking bool      `json:"anthropicAdaptiveThinking" binding:"omitempty"`
}

type UpdateModelDto struct {
	Name                      string   `json:"name" binding:"omitempty"`
	DefaultModel              bool     `json:"defaultModel" binding:"omitempty"`
	Thinking                  bool     `json:"thinking" binding:"omitempty"`
	ThinkingLevels            []string `json:"thinkingLevels" binding:"omitempty"`
	AnthropicAdaptiveThinking bool     `json:"anthropicAdaptiveThinking" binding:"omitempty"`
}
