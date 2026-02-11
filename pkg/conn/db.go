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
	"log/slog"
	"sync"
	"time"

	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db     *gorm.DB
	dbOnce sync.Once
)

func InitDB(ctx context.Context, cfg *config.DatabaseConfig) error {
	var err error
	dbOnce.Do(func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database)
		dbConn, connErr := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if connErr != nil {
			err = connErr
			return
		}

		dbConn.WithContext(timeoutCtx).Exec("CREATE EXTENSION IF NOT EXISTS vector")

		if migrateErr := dbConn.WithContext(timeoutCtx).
			AutoMigrate(
				&models.ChatSession{},
				&models.ChatMessage{},
				&models.ModelProvider{},
				&models.Model{},
				&models.Memory{},
			); migrateErr != nil {
			err = migrateErr
			return
		}

		if result := dbConn.WithContext(timeoutCtx).Exec(`CREATE INDEX IF NOT EXISTS idx_memories_embedding_hnsw ON memories USING hnsw (embedding vector_cosine_ops)`); result.Error != nil {
			slog.WarnContext(timeoutCtx, "failed to create HNSW index on memories", "error", result.Error)
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
