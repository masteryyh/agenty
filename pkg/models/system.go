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

type WebSearchProvider string

const (
	WebSearchProviderDisabled  WebSearchProvider = "disabled"
	WebSearchProviderTavily    WebSearchProvider = "tavily"
	WebSearchProviderBrave     WebSearchProvider = "brave"
	WebSearchProviderFirecrawl WebSearchProvider = "firecrawl"
)

type SystemSettings struct {
	ID                        uuid.UUID         `gorm:"type:uuid;primaryKey"`
	Initialized               bool              `gorm:"not null;default:false"`
	EmbeddingModelID          *uuid.UUID        `gorm:"type:uuid"`
	ContextCompressionModelID *uuid.UUID        `gorm:"type:uuid"`
	EmbeddingMigrating        bool              `gorm:"not null;default:false"`
	WebSearchProvider         WebSearchProvider `gorm:"type:varchar(50);not null;default:'disabled'"`
	BraveAPIKey               string            `gorm:"type:varchar(255);not null;default:''"`
	TavilyAPIKey              string            `gorm:"type:varchar(255);not null;default:''"`
	FirecrawlAPIKey           string            `gorm:"type:varchar(255);not null;default:''"`
	FirecrawlBaseURL          string            `gorm:"type:varchar(255);not null;default:'https://api.firecrawl.dev'"`
	CreatedAt                 time.Time         `gorm:"autoCreateTime:milli"`
	UpdatedAt                 time.Time         `gorm:"autoUpdateTime:milli"`
}

func (SystemSettings) TableName() string {
	return "system_settings"
}

func (s *SystemSettings) ToDto() *SystemSettingsDto {
	return &SystemSettingsDto{
		Initialized:               s.Initialized,
		EmbeddingModelID:          s.EmbeddingModelID,
		ContextCompressionModelID: s.ContextCompressionModelID,
		EmbeddingMigrating:        s.EmbeddingMigrating,
		WebSearchProvider:         s.WebSearchProvider,
		BraveAPIKey:               censorAPIKey(s.BraveAPIKey),
		TavilyAPIKey:              censorAPIKey(s.TavilyAPIKey),
		FirecrawlAPIKey:           censorAPIKey(s.FirecrawlAPIKey),
		FirecrawlBaseURL:          s.FirecrawlBaseURL,
	}
}

func censorAPIKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + "****" + key[len(key)-4:]
}

type SystemSettingsDto struct {
	Initialized               bool              `json:"initialized"`
	EmbeddingModelID          *uuid.UUID        `json:"embeddingModelId,omitempty"`
	ContextCompressionModelID *uuid.UUID        `json:"contextCompressionModelId,omitempty"`
	EmbeddingMigrating        bool              `json:"embeddingMigrating"`
	WebSearchProvider         WebSearchProvider `json:"webSearchProvider"`
	BraveAPIKey               string            `json:"braveApiKey,omitempty"`
	TavilyAPIKey              string            `json:"tavilyApiKey,omitempty"`
	FirecrawlAPIKey           string            `json:"firecrawlApiKey,omitempty"`
	FirecrawlBaseURL          string            `json:"firecrawlBaseUrl,omitempty"`
}

type UpdateSystemSettingsDto struct {
	Initialized               *bool              `json:"initialized" binding:"omitempty"`
	EmbeddingModelID          *uuid.UUID         `json:"embeddingModelId" binding:"omitempty"`
	ContextCompressionModelID *uuid.UUID         `json:"contextCompressionModelId" binding:"omitempty"`
	WebSearchProvider         *WebSearchProvider `json:"webSearchProvider" binding:"omitempty"`
	BraveAPIKey               *string            `json:"braveApiKey" binding:"omitempty"`
	TavilyAPIKey              *string            `json:"tavilyApiKey" binding:"omitempty"`
	FirecrawlAPIKey           *string            `json:"firecrawlApiKey" binding:"omitempty"`
	FirecrawlBaseURL          *string            `json:"firecrawlBaseUrl" binding:"omitempty"`
}
