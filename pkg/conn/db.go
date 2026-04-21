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

package conn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db     *gorm.DB
	dbOnce sync.Once
)

func InitDB(ctx context.Context, cfg *config.DatabaseConfig, debug bool) error {
	var err error
	dbOnce.Do(func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database)
		dbConn, connErr := gorm.Open(postgres.Open(dsn), &gorm.Config{
			TranslateError: true,
			Logger:         logger.NewGormLogger(debug),
		})
		if connErr != nil {
			err = fmt.Errorf("failed to connect to database: %w", connErr)
			return
		}

		if extErr := dbConn.WithContext(timeoutCtx).Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; extErr != nil {
			err = fmt.Errorf("pgvector extension is required but could not be created: %w", extErr)
			return
		}

		if extErr := dbConn.WithContext(timeoutCtx).Exec("CREATE EXTENSION IF NOT EXISTS pg_search").Error; extErr != nil {
			err = fmt.Errorf("pg_search extension is required but could not be created: %w", extErr)
			return
		}

		if migrateErr := dbConn.WithContext(timeoutCtx).
			AutoMigrate(
				&models.SystemSettings{},
				&models.ChatSession{},
				&models.ChatMessage{},
				&models.ModelProvider{},
				&models.Model{},
				&models.Agent{},
				&models.AgentModel{},
				&models.MCPServer{},
				&models.KnowledgeItem{},
				&models.KnowledgeBaseData{},
				&models.Skill{},
			); migrateErr != nil {
			err = fmt.Errorf("failed to migrate database: %w", migrateErr)
			return
		}

		if idxErr := dbConn.WithContext(timeoutCtx).Exec(`CREATE INDEX IF NOT EXISTS idx_kb_data_text_embedding_hnsw ON kb_data USING hnsw (text_embedding vector_ip_ops)`).Error; idxErr != nil {
			err = fmt.Errorf("failed to create index: %w", idxErr)
			return
		}

		if idxErr := dbConn.WithContext(timeoutCtx).Exec(`CREATE INDEX IF NOT EXISTS idx_skills_bm25 ON skills USING bm25 (id, name, description) WITH (key_field = 'id')`).Error; idxErr != nil {
			err = fmt.Errorf("failed to create skills BM25 index: %w", idxErr)
			return
		}

		if seedErr := seedPresets(timeoutCtx, dbConn); seedErr != nil {
			err = fmt.Errorf("failed to seed presets: %w", seedErr)
			return
		}
		db = dbConn
	})
	return err
}

func GetDB() *gorm.DB {
	if db == nil {
		panic("database not initialized, call InitDB first")
	}
	return db
}
