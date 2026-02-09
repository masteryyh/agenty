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
	"path/filepath"
)

// AppConfig is the config definition for this app
type AppConfig struct {
	// Debug mode enabled or not
	Debug bool `mapstructure:"debug"`

	// Port of the HTTP server
	Port int `mapstructure:"port"`

	// AllowedPaths is a list of allowed filesystem paths for tools to operate, if empty, all paths are allowed
	AllowedPaths []string `mapstructure:"allowedPaths"`

	// DB configuration
	DB *DatabaseConfig `mapstructure:"db"`
}

// DatabaseConfig is the config definition for database connection, only postgresql is supported for now
type DatabaseConfig struct {
	// Host of the database server
	Host string `mapstructure:"host"`

	// Port of the database server
	Port int `mapstructure:"port"`

	// Username for database authentication
	Username string `mapstructure:"username"`

	// Password for database authentication
	Password string `mapstructure:"password"`

	// Database name
	Database string `mapstructure:"database"`
}

func (c *DatabaseConfig) Validate() error {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = 5432
	}
	if c.Username == "" {
		c.Username = "postgres"
	}
	if c.Password == "" {
		return fmt.Errorf("database password is required")
	}
	if c.Database == "" {
		c.Database = "agenty"
	}
	return nil
}

func (c *AppConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	cleanedPaths := make([]string, 0, len(c.AllowedPaths))
	if len(c.AllowedPaths) > 0 {
		for _, path := range c.AllowedPaths {
			if path == "" {
				return fmt.Errorf("allowed paths cannot contain empty string")
			}

			abs, err := filepath.Abs(filepath.Clean(path))
			if err != nil {
				return fmt.Errorf("invalid allowed path '%s': %w", path, err)
			}
			cleanedPaths = append(cleanedPaths, abs)
		}
		c.AllowedPaths = cleanedPaths
	}

	return c.DB.Validate()
}
