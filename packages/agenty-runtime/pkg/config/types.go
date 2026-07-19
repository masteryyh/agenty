package config

import (
	"fmt"
	"runtime"
)

const (
	DatabaseTypePostgres = "postgres"
	DatabaseTypeSQLite   = "sqlite"
	DefaultServerPort    = 8080
	DefaultSQLitePath    = "~/.agenty/agenty.db"
)

type AppConfig struct {
	Debug bool
	Port  int
	DB    *DatabaseConfig
}

// DatabaseConfig retains PostgreSQL fields while the server CLI currently
// constructs only SQLite configurations.
type DatabaseConfig struct {
	Type       string
	SQLitePath string
	Host       string
	Port       int
	Username   string
	Password   string
	Database   string
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
		if c.SQLitePath == "" {
			return fmt.Errorf("sqlite database path is required")
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

func (c *AppConfig) Validate() error {
	if c.Port <= 0 {
		c.Port = DefaultServerPort
	}
	if c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}
	if c.DB == nil {
		return fmt.Errorf("database configuration is required")
	}
	if err := c.DB.Validate(); err != nil {
		return fmt.Errorf("invalid db config: %w", err)
	}
	return nil
}
