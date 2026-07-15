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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func NewServerConfig(port int, databasePath string, debug bool) (*AppConfig, error) {
	resolvedPath, err := ResolveSQLitePath(databasePath)
	if err != nil {
		return nil, err
	}
	cfg := &AppConfig{
		Debug: debug,
		Port:  port,
		DB: &DatabaseConfig{
			Type:       DatabaseTypeSQLite,
			SQLitePath: resolvedPath,
		},
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func ResolveSQLitePath(value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		path = DefaultSQLitePath
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine user home directory: %w", err)
		}
		if path == "~" {
			path = homeDir
		} else {
			path = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
		}
	} else if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("unsupported home directory path: %s", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sqlite database path %q: %w", value, err)
	}
	return filepath.Clean(absPath), nil
}
