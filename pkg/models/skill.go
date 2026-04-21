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
	"gorm.io/datatypes"
)

type SkillScope string

const (
	SkillScopeGlobal  SkillScope = "global"
	SkillScopeProject SkillScope = "project"
)

type Skill struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey"`
	Name        string         `gorm:"type:varchar(255);not null"`
	Description string         `gorm:"type:text;not null"`
	SkillMDPath string         `gorm:"column:skill_md_path;type:text;not null"`
	Metadata    datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt   time.Time      `gorm:"autoCreateTime:milli"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime:milli"`
}

func (Skill) TableName() string {
	return "skills"
}

type SessionSkill struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey"`
	Name        string         `gorm:"type:varchar(255);not null"`
	Description string         `gorm:"type:text;not null"`
	SkillMDPath string         `gorm:"column:skill_md_path;type:text;not null"`
	Scope       SkillScope     `gorm:"type:varchar(20);not null;default:'global'"`
	SourceDir   string         `gorm:"type:text;not null"`
	Metadata    datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt   time.Time      `gorm:"autoCreateTime:milli"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime:milli"`
}

type SkillDto struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	SkillMDPath string     `json:"skillMdPath"`
	Scope       SkillScope `json:"scope,omitempty"`
	SourceDir   string     `json:"sourceDir,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (s *Skill) ToDto() *SkillDto {
	return &SkillDto{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		SkillMDPath: s.SkillMDPath,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

func (s *SessionSkill) ToDto() *SkillDto {
	return &SkillDto{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		SkillMDPath: s.SkillMDPath,
		Scope:       s.Scope,
		SourceDir:   s.SourceDir,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

type SkillSearchResult struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	SkillMDPath string     `json:"skillMdPath"`
	Scope       SkillScope `json:"scope,omitempty"`
	Score       float64    `json:"score,omitempty"`
}

type SkillContentResult struct {
	Content string `json:"content"`
}
