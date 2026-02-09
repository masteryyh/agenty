package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type ReadFileTool struct{}

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

func (t *ReadFileTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := sonic.Unmarshal(arguments, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cleanPath := filepath.Clean(args.Path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

type WriteFileTool struct{}

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

func (t *WriteFileTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := sonic.Unmarshal(arguments, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cleanPath := filepath.Clean(args.Path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(cleanPath, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("successfully wrote %d bytes to %s", len(args.Content), cleanPath), nil
}

type ListDirectoryTool struct{}

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

func (t *ListDirectoryTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := sonic.Unmarshal(arguments, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cleanPath := filepath.Clean(args.Path)
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
		sb.WriteString(fmt.Sprintf("[%s] %s\n", entryType, entry.Name()))
	}
	return sb.String(), nil
}

func RegisterAll(registry *tools.Registry) {
	registry.Register(&ReadFileTool{})
	registry.Register(&WriteFileTool{})
	registry.Register(&ListDirectoryTool{})
}
