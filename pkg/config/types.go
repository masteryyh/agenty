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
	"runtime"
)

const (
	DatabaseTypePostgres = "postgres"
	DatabaseTypeSQLite   = "sqlite"
	DefaultDaemonPort    = 8080
)

// MCPConfig holds runtime parameters for MCP client connections
type MCPConfig struct {
	HealthCheckInterval int `mapstructure:"healthCheckInterval"`
	ConnectTimeout      int `mapstructure:"connectTimeout"`
}

func (c *MCPConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.HealthCheckInterval <= 0 {
		c.HealthCheckInterval = 30
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 15
	}
	return nil
}

// ServerConfig holds configuration for connecting to a remote agenty server in non-daemon mode
type ServerConfig struct {
	// URL of the remote server (e.g. http://localhost:8080)
	URL string `mapstructure:"url"`

	// Username for authentication with the remote server
	Username string `mapstructure:"username"`

	// Password for authentication with the remote server
	Password string `mapstructure:"password"`
}

// AppConfig is the config definition for this app
type AppConfig struct {
	// Daemon indicates whether to run in daemon mode
	Daemon bool `mapstructure:"-"`

	// Debug mode enables more verbose logging and other debug features
	Debug bool `mapstructure:"debug"`

	// Port of the HTTP server
	Port int `mapstructure:"port"`

	// DB configuration
	DB *DatabaseConfig `mapstructure:"db"`

	// Auth configuration for HTTP Basic Auth
	Auth *AuthConfig `mapstructure:"auth"`

	// MCP client runtime configuration
	MCP *MCPConfig `mapstructure:"mcp"`

	// Server configuration for remote mode
	Server *ServerConfig `mapstructure:"server"`
}

func (c *AppConfig) IsRemoteMode() bool {
	return !c.Daemon && c.Server != nil && c.Server.URL != ""
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

// DatabaseConfig is the config definition for database connection.
type DatabaseConfig struct {
	Type string `mapstructure:"type"`

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

	SQLiteVectorExtensionPath string `mapstructure:"sqliteVectorExtensionPath"`
}

func (c *DatabaseConfig) Validate() error {
	if c.Type == "" {
		c.Type = DatabaseTypeSQLite
	}
	switch c.Type {
	case DatabaseTypeSQLite:
		if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
			return fmt.Errorf("external database is required on Windows ARM64 due to lack of SQLite vector support")
		}
		return nil
	case DatabaseTypePostgres:
	default:
		return fmt.Errorf("unsupported database type: %s", c.Type)
	}
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
	if c.Daemon {
		if c.Port <= 0 {
			c.Port = DefaultDaemonPort
		}
		if c.Port > 65535 {
			return fmt.Errorf("invalid port number: %d", c.Port)
		}
		if c.DB == nil {
			return fmt.Errorf("database configuration is required in daemon mode")
		}
		if err := c.DB.Validate(); err != nil {
			return fmt.Errorf("invalid db config: %w", err)
		}
		if err := c.Auth.Validate(); err != nil {
			return fmt.Errorf("invalid auth config: %w", err)
		}
		if err := c.MCP.Validate(); err != nil {
			return fmt.Errorf("invalid mcp config: %w", err)
		}
		return nil
	}

	if c.IsRemoteMode() {
		return nil
	}

	if c.DB == nil {
		return fmt.Errorf("database configuration is required (or configure server.url for remote mode)")
	}
	if err := c.DB.Validate(); err != nil {
		return fmt.Errorf("invalid db config: %w", err)
	}
	if err := c.MCP.Validate(); err != nil {
		return fmt.Errorf("invalid mcp config: %w", err)
	}
	return nil
}
