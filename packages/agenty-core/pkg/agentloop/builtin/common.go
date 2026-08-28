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

	path, err := expandEnvironmentVariables(path)
	if err != nil {
		return "", err
	}

	absolutePath, isAbsolute, err := normalizeAbsolutePath(path)
	if err != nil {
		return "", err
	}
	if isAbsolute {
		return filepath.Clean(absolutePath), nil
	}

	base := cwd
	if strings.TrimSpace(base) == "" {
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

func normalizeAbsolutePath(path string) (string, bool, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, fmt.Errorf("resolve user home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimLeft(path[1:], `/\\`)), true, nil
	}

	return path, filepath.IsAbs(path) || isWindowsAbsolutePath(path), nil
}

func expandEnvironmentVariables(path string) (string, error) {
	path, err := expandWindowsStyleEnvironmentVariables(path)
	if err != nil {
		return "", err
	}

	path, err = expandPowerShellEnvironmentVariables(path)
	if err != nil {
		return "", err
	}

	return expandPOSIXEnvironmentVariables(path)
}

func expandWindowsStyleEnvironmentVariables(path string) (string, error) {
	var builder strings.Builder
	for offset := 0; offset < len(path); {
		opening := strings.IndexByte(path[offset:], '%')
		if opening < 0 {
			builder.WriteString(path[offset:])
			break
		}

		start := offset + opening
		builder.WriteString(path[offset:start])
		closing := strings.IndexByte(path[start+1:], '%')
		if closing < 0 {
			builder.WriteString(path[start:])
			break
		}

		end := start + closing + 1
		variable := path[start+1 : end]
		if variable == "" {
			builder.WriteString("%%")
		} else {
			value, err := environmentVariable(variable)
			if err != nil {
				return "", err
			}
			builder.WriteString(value)
		}
		offset = end + 1
	}
	return builder.String(), nil
}

func expandPowerShellEnvironmentVariables(path string) (string, error) {
	const prefix = "$env:"

	var builder strings.Builder
	for offset := 0; offset < len(path); {
		start := powerShellEnvironmentVariableStart(path, offset, prefix)
		if start < 0 {
			builder.WriteString(path[offset:])
			break
		}

		builder.WriteString(path[offset:start])
		variableStart := start + len(prefix)
		variableEnd := variableStart
		for variableEnd < len(path) && path[variableEnd] != '/' && path[variableEnd] != '\\' {
			variableEnd++
		}
		if variableEnd == variableStart {
			return "", fmt.Errorf("resolve PowerShell environment variable: name must not be empty")
		}

		value, err := environmentVariable(path[variableStart:variableEnd])
		if err != nil {
			return "", err
		}
		builder.WriteString(value)
		offset = variableEnd
	}
	return builder.String(), nil
}

func powerShellEnvironmentVariableStart(path string, offset int, prefix string) int {
	for index := offset; index+len(prefix) <= len(path); index++ {
		if strings.EqualFold(path[index:index+len(prefix)], prefix) {
			return index
		}
	}
	return -1
}

func expandPOSIXEnvironmentVariables(path string) (string, error) {
	var expandErr error
	expanded := os.Expand(path, func(name string) string {
		if expandErr != nil {
			return ""
		}
		if name == "" {
			expandErr = fmt.Errorf("resolve environment variable: name must not be empty")
			return ""
		}

		value, err := environmentVariable(name)
		if err != nil {
			expandErr = err
			return ""
		}
		return value
	})
	if expandErr != nil {
		return "", expandErr
	}
	return expanded, nil
}

func environmentVariable(name string) (string, error) {
	value, found := os.LookupEnv(name)
	if !found {
		return "", fmt.Errorf("resolve environment variable %q: not set", name)
	}
	return value, nil
}

func isWindowsAbsolutePath(path string) bool {
	if strings.HasPrefix(path, `\\`) {
		return true
	}

	return len(path) >= 2 &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) &&
		path[1] == ':'
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
		return 0, fmt.Errorf("max_results must be between 1 and %d", maxResults)
	}
	return *value, nil
}
