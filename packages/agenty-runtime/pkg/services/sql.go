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
	"fmt"
	"strings"

	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"gorm.io/gorm"
)

func usingSQLite() bool {
	return conn.GetDBType() == config.DatabaseTypeSQLite
}

func sqliteFTSQuery(query string) string {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		quoted = append(quoted, `"`+term+`"`)
	}
	return strings.Join(quoted, " OR ")
}

func execStatements(ctx context.Context, db *gorm.DB, script string) error {
	for _, stmt := range splitSQLScript(script) {
		if stmt == "" {
			continue
		}
		if err := db.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("failed statement %q: %w", stmt, err)
		}
	}
	return nil
}

func splitSQLScript(script string) []string {
	var stmts []string
	var buf strings.Builder
	inTrigger := false
	for _, line := range strings.Split(script, "\n") {
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
