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

package services

import (
	"context"
	"sync"

	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"gorm.io/gorm"
)

type SystemService struct {
	db *gorm.DB
}

var (
	systemService *SystemService
	systemOnce    sync.Once
)

func GetSystemService() *SystemService {
	systemOnce.Do(func() {
		systemService = &SystemService{db: conn.GetDB()}
	})
	return systemService
}

func (s *SystemService) getOrCreate(ctx context.Context) (*models.SystemSettings, error) {
	var settings models.SystemSettings
	result := s.db.WithContext(ctx).First(&settings)
	if result.Error == nil {
		return &settings, nil
	}
	if result.Error != gorm.ErrRecordNotFound {
		return nil, result.Error
	}
	settings = models.SystemSettings{Initialized: false}
	if err := s.db.WithContext(ctx).Create(&settings).Error; err != nil {
		return nil, err
	}
	return &settings, nil
}

func (s *SystemService) IsInitialized(ctx context.Context) (bool, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return false, err
	}
	return settings.Initialized, nil
}

func (s *SystemService) SetInitialized(ctx context.Context) error {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(settings).Update("initialized", true).Error
}

func (s *SystemService) GetSettings(ctx context.Context) (*models.SystemSettingsDto, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	return settings.ToDto(), nil
}

func (s *SystemService) UpdateSettings(ctx context.Context, dto *models.UpdateSystemSettingsDto) (*models.SystemSettingsDto, error) {
	settings, err := s.getOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	if dto.Initialized != nil {
		if err := s.db.WithContext(ctx).Model(settings).Update("initialized", *dto.Initialized).Error; err != nil {
			return nil, err
		}
		settings.Initialized = *dto.Initialized
	}
	return settings.ToDto(), nil
}
