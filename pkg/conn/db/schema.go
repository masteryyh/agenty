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

package db

import (
	"embed"
	"fmt"
)

//go:embed *.sql
var schemaFS embed.FS

func Schema(dbType string) (string, error) {
	switch dbType {
	case "postgres":
		return readSchema("postgres.sql")
	case "sqlite":
		return readSchema("sqlite.sql")
	default:
		return "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}

func readSchema(name string) (string, error) {
	data, err := schemaFS.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
