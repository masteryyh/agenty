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
