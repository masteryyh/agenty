package builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

const (
	maxReadOutputBytes  = 1 << 20
	maxScannerTokenSize = 4 << 20
	defaultMaxResults   = 200
	maxResults          = 1_000
)

type fileSystem struct {
	mu sync.RWMutex
}

func decodeArguments(input []byte, target any) error {
	if err := json.Unmarshal(input, target); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	return nil
}

func resultContent(value any) (conversation.Content, error) {
	encoded, err := json.MarshalString(value)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return conversation.Text(encoded), nil
}

func resolvePath(path, cwd string, allowEmpty bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		if !allowEmpty {
			return "", fmt.Errorf("path must not be empty")
		}
		path = "."
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	base := cwd
	if strings.TrimSpace(base) == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve process working directory: %w", err)
		}
	}

	resolved, err := filepath.Abs(filepath.Join(base, path))
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	return filepath.Clean(resolved), nil
}

func regularFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path %q is a directory", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	return info, nil
}

func objectSchema(
	properties map[string]agentloop.JSONSchema,
	required []string,
) agentloop.JSONSchema {
	return agentloop.JSONSchema{
		Type:                 agentloop.JSONSchemaTypeObject,
		Properties:           properties,
		Required:             required,
		AdditionalProperties: agentloop.AllowAdditionalProperties(false),
	}
}

func stringSchema(description string) agentloop.JSONSchema {
	return agentloop.JSONSchema{
		Type:        agentloop.JSONSchemaTypeString,
		Description: description,
	}
}

func integerSchema(description string, minimum float64) agentloop.JSONSchema {
	return agentloop.JSONSchema{
		Type:        agentloop.JSONSchemaTypeInteger,
		Description: description,
		Minimum:     &minimum,
	}
}

func booleanSchema(description string) agentloop.JSONSchema {
	return agentloop.JSONSchema{
		Type:        agentloop.JSONSchemaTypeBoolean,
		Description: description,
	}
}

func normalizeMaxResults(value *int) (int, error) {
	if value == nil {
		return defaultMaxResults, nil
	}
	if *value < 1 || *value > maxResults {
		return 0, fmt.Errorf("maxResults must be between 1 and %d", maxResults)
	}
	return *value, nil
}
