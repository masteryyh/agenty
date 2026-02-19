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

	"github.com/masteryyh/agenty/pkg/utils"
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

	// Embedding configuration for memory features
	Embedding *EmbeddingConfig `mapstructure:"embedding"`

	// Auth configuration for HTTP Basic Auth
	Auth *AuthConfig `mapstructure:"auth"`
}

// AuthConfig is the config definition for HTTP Basic Auth
type AuthConfig struct {
	// Enabled indicates whether HTTP Basic Auth is enabled
	Enabled bool `mapstructure:"enabled"`

	// Username for HTTP Basic Auth
	Username string `mapstructure:"username"`

	// Password for HTTP Basic Auth
	Password string `mapstructure:"password"`
}

// EmbeddingConfig is the config definition for embedding model used by memory features
type EmbeddingConfig struct {
	// BaseURL is the base URL of the embedding API (OpenAI-compatible)
	BaseURL string `mapstructure:"baseUrl"`

	// APIKey is the API key for the embedding API
	APIKey string `mapstructure:"apiKey"`

	// Model is the embedding model name
	Model string `mapstructure:"model"`
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

func (c *EmbeddingConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.Model == "" {
		c.Model = "text-embedding-3-small"
	}
	return nil
}

func (c *AuthConfig) Validate() error {
	if c == nil || !c.Enabled {
		return nil
	}
	if c.Username == "" {
		return fmt.Errorf("auth username is required when auth is enabled")
	}
	if c.Password == "" {
		return fmt.Errorf("auth password is required when auth is enabled")
	}
	return nil
}

func (c *AppConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	cleanedPaths := make([]string, 0, len(c.AllowedPaths))
	for _, p := range c.AllowedPaths {
		cleaned, err := utils.GetCleanPath(p, true)
		if err != nil {
			return fmt.Errorf("invalid allowed path '%s': %w", p, err)
		}
		cleanedPaths = append(cleanedPaths, cleaned)
	}
	c.AllowedPaths = cleanedPaths

	if err := c.Embedding.Validate(); err != nil {
		return fmt.Errorf("invalid embedding config: %w", err)
	}

	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("invalid auth config: %w", err)
	}

	return c.DB.Validate()
}
