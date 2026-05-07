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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/masteryyh/agenty/pkg/config"
	dbschema "github.com/masteryyh/agenty/pkg/conn/db"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/logger"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	db     *gorm.DB
	dbType string
	dbOnce sync.Once
)

func InitDB(ctx context.Context, cfg *config.DatabaseConfig, debug bool) error {
	var err error
	dbOnce.Do(func() {
		initTimeout := 10 * time.Second
		if cfg.Type == config.DatabaseTypeSQLite {
			initTimeout = 2 * time.Minute
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, initTimeout)
		defer cancel()

		var dbConn *gorm.DB
		var connErr error
		switch cfg.Type {
		case config.DatabaseTypePostgres:
			dbConn, connErr = openPostgres(cfg, debug)
		case config.DatabaseTypeSQLite:
			dbConn, connErr = openSQLite(timeoutCtx, cfg, debug)
		default:
			connErr = fmt.Errorf("unsupported database type: %s", cfg.Type)
		}
		if connErr != nil {
			err = fmt.Errorf("failed to connect to database: %w", connErr)
			return
		}

		schema, schemaErr := dbschema.Schema(cfg.Type)
		if schemaErr != nil {
			err = fmt.Errorf("failed to load database schema: %w", schemaErr)
			return
		}
		if schemaErr := execSQLScript(timeoutCtx, dbConn, schema); schemaErr != nil {
			err = fmt.Errorf("failed to initialize database schema: %w", schemaErr)
			return
		}
		if migrationErr := migrateCoreSchema(timeoutCtx, dbConn, cfg.Type); migrationErr != nil {
			err = fmt.Errorf("failed to migrate database schema: %w", migrationErr)
			return
		}

		if cfg.Type == config.DatabaseTypeSQLite {
			if initErr := initSQLiteVector(timeoutCtx, dbConn); initErr != nil {
				err = initErr
				return
			}
		}

		if seedErr := seedPresets(timeoutCtx, dbConn); seedErr != nil {
			err = fmt.Errorf("failed to seed presets: %w", seedErr)
			return
		}
		db = dbConn
		dbType = cfg.Type
		models.SetVectorStorage(cfg.Type)
	})
	return err
}

func GetDB() *gorm.DB {
	if db == nil {
		panic("database not initialized, call InitDB first")
	}
	return db
}

func GetDBType() string {
	if dbType == "" {
		return config.DatabaseTypePostgres
	}
	return dbType
}

func NowExpr() clause.Expr {
	return gorm.Expr("CURRENT_TIMESTAMP")
}

func openPostgres(cfg *config.DatabaseConfig, debug bool) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger.NewGormLogger(debug),
	})
}

func openSQLite(ctx context.Context, cfg *config.DatabaseConfig, debug bool) (*gorm.DB, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user config dir: %w", err)
	}
	agentyDir := filepath.Join(configDir, "agenty")
	if err = os.MkdirAll(agentyDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create sqlite config dir: %w", err)
	}

	dbPath := filepath.Join(agentyDir, "agenty.db")
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", dbPath)
	dbConn, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger.NewGormLogger(debug),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := dbConn.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	if err = ensureSQLiteFTS5(ctx, dbConn); err != nil {
		return nil, err
	}
	if err = loadSQLiteVector(ctx, sqlDB, cfg.SQLiteVectorExtensionPath, agentyDir); err != nil {
		return nil, err
	}
	if err = verifySQLiteVector(ctx, dbConn); err != nil {
		return nil, err
	}
	return dbConn, nil
}

func ensureSQLiteFTS5(ctx context.Context, dbConn *gorm.DB) error {
	if err := dbConn.WithContext(ctx).Exec("CREATE VIRTUAL TABLE temp.agenty_fts5_check USING fts5(content)").Error; err != nil {
		return fmt.Errorf("sqlite fts5 extension is required but unavailable: %w", err)
	}
	return dbConn.WithContext(ctx).Exec("DROP TABLE temp.agenty_fts5_check").Error
}

func verifySQLiteVector(ctx context.Context, dbConn *gorm.DB) error {
	if err := dbConn.WithContext(ctx).Exec("SELECT vector_version()").Error; err != nil {
		return fmt.Errorf("sqlite-vector extension is required but unavailable: %w", err)
	}
	return nil
}

func initSQLiteVector(ctx context.Context, dbConn *gorm.DB) error {
	if err := dbConn.WithContext(ctx).Exec("SELECT vector_init('kb_data', 'text_embedding', 'type=FLOAT32,dimension=1024,distance=DOT')").Error; err != nil {
		return fmt.Errorf("failed to initialize sqlite-vector column: %w", err)
	}
	return dbConn.WithContext(ctx).Exec("SELECT vector_quantize('kb_data', 'text_embedding')").Error
}

func execSQLScript(ctx context.Context, dbConn *gorm.DB, script string) error {
	for _, stmt := range splitSQLScript(script) {
		if stmt == "" {
			continue
		}
		if err := dbConn.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("failed statement %q: %w", stmt, err)
		}
	}
	return nil
}

func migrateCoreSchema(ctx context.Context, dbConn *gorm.DB, dbType string) error {
	hasRoundID, err := hasColumn(ctx, dbConn, dbType, "chat_messages", "round_id")
	if err != nil {
		return err
	}
	if hasRoundID {
		return nil
	}

	columnType := "UUID"
	if dbType == config.DatabaseTypeSQLite {
		columnType = "TEXT"
	}
	if err := dbConn.WithContext(ctx).Exec("ALTER TABLE chat_messages ADD COLUMN round_id " + columnType).Error; err != nil {
		return fmt.Errorf("failed to add chat_messages.round_id: %w", err)
	}
	return nil
}

func hasColumn(ctx context.Context, dbConn *gorm.DB, dbType, tableName, columnName string) (bool, error) {
	var count int64
	switch dbType {
	case config.DatabaseTypeSQLite:
		if err := dbConn.WithContext(ctx).
			Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", tableName, columnName).
			Scan(&count).Error; err != nil {
			return false, fmt.Errorf("failed to inspect sqlite column %s.%s: %w", tableName, columnName, err)
		}
	case config.DatabaseTypePostgres:
		if err := dbConn.WithContext(ctx).
			Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?", tableName, columnName).
			Scan(&count).Error; err != nil {
			return false, fmt.Errorf("failed to inspect postgres column %s.%s: %w", tableName, columnName, err)
		}
	default:
		return false, fmt.Errorf("unsupported database type: %s", dbType)
	}
	return count > 0, nil
}

func splitSQLScript(script string) []string {
	var stmts []string
	var buf strings.Builder
	inTrigger := false
	for line := range strings.SplitSeq(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "CREATE TRIGGER") {
			inTrigger = true
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if inTrigger {
			if upper == "END;" || strings.HasPrefix(upper, "END;") {
				stmts = append(stmts, strings.TrimSuffix(strings.TrimSpace(buf.String()), ";"))
				buf.Reset()
				inTrigger = false
			}
			continue
		}
		if strings.HasSuffix(trimmed, ";") {
			stmts = append(stmts, strings.TrimSuffix(strings.TrimSpace(buf.String()), ";"))
			buf.Reset()
		}
	}
	if strings.TrimSpace(buf.String()) != "" {
		stmts = append(stmts, strings.TrimSpace(buf.String()))
	}
	return stmts
}
