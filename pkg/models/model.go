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

type Model struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	ProviderID uuid.UUID `gorm:"type:uuid;not null"`
	Name       string    `gorm:"type:varchar(255);not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
	DeletedAt  *time.Time
}

func (Model) TableName() string {
	return "models"
}

func (m *Model) ToDto(provider *ModelProviderDto) *ModelDto {
	dto := &ModelDto{
		ID:        m.ID,
		Name:      m.Name,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}

	if provider != nil {
		dto.Provider = provider
	}
	return dto
}

type CreateModelDto struct {
	ProviderID uuid.UUID `json:"providerId" binding:"required"`
	Name       string    `json:"name" binding:"required"`
}

type ModelDto struct {
	ID        uuid.UUID         `json:"id"`
	Provider  *ModelProviderDto `json:"provider,omitempty"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}
