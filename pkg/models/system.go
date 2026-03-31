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

type SystemSettings struct {
	ID                        uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Initialized               bool       `gorm:"not null;default:false"`
	EmbeddingModelID          *uuid.UUID `gorm:"type:uuid"`
	ContextCompressionModelID *uuid.UUID `gorm:"type:uuid"`
	EmbeddingMigrating        bool       `gorm:"not null;default:false"`
	CreatedAt                 time.Time  `gorm:"autoCreateTime:milli"`
	UpdatedAt                 time.Time  `gorm:"autoUpdateTime:milli"`
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
	}
}

type SystemSettingsDto struct {
	Initialized               bool       `json:"initialized"`
	EmbeddingModelID          *uuid.UUID `json:"embeddingModelId,omitempty"`
	ContextCompressionModelID *uuid.UUID `json:"contextCompressionModelId,omitempty"`
	EmbeddingMigrating        bool       `json:"embeddingMigrating"`
}

type UpdateSystemSettingsDto struct {
	Initialized               *bool      `json:"initialized" binding:"omitempty"`
	EmbeddingModelID          *uuid.UUID `json:"embeddingModelId" binding:"omitempty"`
	ContextCompressionModelID *uuid.UUID `json:"contextCompressionModelId" binding:"omitempty"`
}
