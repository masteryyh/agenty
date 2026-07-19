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
	ID          uuid.UUID
	Name        string
	Description string
	SkillMDPath string
	Metadata    datatypes.JSON
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Skill) TableName() string {
	return "skills"
}

type SessionSkill struct {
	ID          uuid.UUID
	Name        string
	Description string
	SkillMDPath string
	Scope       SkillScope
	SourceDir   string
	Metadata    datatypes.JSON
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
		Scope:       SkillScopeGlobal,
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
