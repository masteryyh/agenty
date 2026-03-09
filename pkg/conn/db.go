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
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/masteryyh/agenty/pkg/config"
	sqlschema "github.com/masteryyh/agenty/sql/schema"
)

var (
	sqlDB  *sql.DB
	dbOnce sync.Once
)

func InitDB(ctx context.Context, cfg *config.DatabaseConfig) error {
	var err error
	dbOnce.Do(func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database)
		dbConn, connErr := sql.Open("postgres", dsn)
		if connErr != nil {
			err = connErr
			return
		}

		if pingErr := dbConn.PingContext(timeoutCtx); pingErr != nil {
			err = pingErr
			return
		}

		entries, readErr := sqlschema.FS.ReadDir(".")
		if readErr != nil {
			err = fmt.Errorf("failed to read schema directory: %w", readErr)
			return
		}

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ddl, readFileErr := sqlschema.FS.ReadFile(entry.Name())
			if readFileErr != nil {
				err = fmt.Errorf("failed to read schema file %s: %w", entry.Name(), readFileErr)
				return
			}
			if _, execErr := dbConn.ExecContext(timeoutCtx, string(ddl)); execErr != nil {
				err = fmt.Errorf("failed to execute schema file %s: %w", entry.Name(), execErr)
				return
			}
		}

		if seedErr := seedPresets(timeoutCtx, dbConn); seedErr != nil {
			err = seedErr
			return
		}

		sqlDB = dbConn
	})
	return err
}

func GetSQLDB() *sql.DB {
	if sqlDB == nil {
		panic("database not initialized, call InitDB first")
	}
	return sqlDB
}
