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

package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/utils"
)

type ReadFileTool struct {
	cfg *config.AppConfig
}

func (t *ReadFileTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path. Returns the file content as text.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"path": {
					Type:        "string",
					Description: "The absolute or relative path to the file to read",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, arguments string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	abs, err := utils.GetCleanPath(args.Path, true)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	contains, err := utils.PathContained(t.cfg.AllowedPaths, abs)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if !contains {
		return "", fmt.Errorf("access to path '%s' is not allowed", args.Path)
	}

	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file does not exist: %s", args.Path)
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

type WriteFileTool struct {
	cfg *config.AppConfig
}

func (t *WriteFileTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "write_file",
		Description: "Write content to a file at the given path. Creates the file if it does not exist, or overwrites it if it does.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"path": {
					Type:        "string",
					Description: "The absolute or relative path to the file to write",
				},
				"content": {
					Type:        "string",
					Description: "The content to write to the file",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, arguments string) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cleanPath, err := utils.GetCleanPath(args.Path, false)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	contains, err := utils.PathContained(t.cfg.AllowedPaths, cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	if !contains {
		return "", fmt.Errorf("access to path '%s' is not allowed", args.Path)
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(cleanPath, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("successfully wrote %d bytes to %s", len(args.Content), cleanPath), nil
}

type ListDirectoryTool struct {
	cfg *config.AppConfig
}

func (t *ListDirectoryTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "list_directory",
		Description: "List the contents of a directory. Returns file and directory names with their types.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"path": {
					Type:        "string",
					Description: "The absolute or relative path to the directory to list",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t *ListDirectoryTool) Execute(_ context.Context, arguments string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cleanPath, err := utils.GetCleanPath(args.Path, true)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}

	contains, err := utils.PathContained(t.cfg.AllowedPaths, cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}
	if !contains {
		return "", fmt.Errorf("access to path '%s' is not allowed", args.Path)
	}

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var sb strings.Builder
	for _, entry := range entries {
		entryType := "file"
		if entry.IsDir() {
			entryType = "dir"
		}
		fmt.Fprintf(&sb, "[%s] %s\n", entryType, entry.Name())
	}
	return sb.String(), nil
}
