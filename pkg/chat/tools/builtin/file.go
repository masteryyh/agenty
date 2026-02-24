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
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/utils"
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
				"startLine": {
					Type:        "integer",
					Description: "The line number to start reading from (1-based index). Optional, defaults to 1.",
				},
				"endLine": {
					Type:        "integer",
					Description: "The line number to stop reading at (1-based index). Optional, defaults to the end of the file.",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, arguments string) (string, error) {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"startLine,omitempty"`
		EndLine   int    `json:"endLine,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	abs, err := utils.GetCleanPath(args.Path, true)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file does not exist: %s", args.Path)
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if args.StartLine <= 0 && args.EndLine <= 0 {
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
		return string(data), nil
	}

	start := 1
	if args.StartLine > 0 {
		start = args.StartLine
	}

	if args.StartLine > 0 && args.EndLine > 0 && args.StartLine > args.EndLine {
		return "", fmt.Errorf("startLine %d is greater than endLine %d", args.StartLine, args.EndLine)
	}

	f, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if args.EndLine > 0 && lineNum > args.EndLine {
			break
		}
		if lineNum < start {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if lineNum < start {
		return "", fmt.Errorf("startLine %d exceeds file length %d", start, lineNum)
	}

	return sb.String(), nil
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

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(cleanPath, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("successfully wrote %d bytes to %s", len(args.Content), cleanPath), nil
}

type ReplaceInFileTool struct{}

func (t *ReplaceInFileTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "replace_in_file",
		Description: "Replace a range of lines in a file with new content. Lines from startLine to endLine (inclusive, 1-based) are replaced.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"path": {
					Type:        "string",
					Description: "The absolute or relative path to the file",
				},
				"startLine": {
					Type:        "integer",
					Description: "The first line number to replace (1-based index)",
				},
				"endLine": {
					Type:        "integer",
					Description: "The last line number to replace (1-based index, inclusive)",
				},
				"newContent": {
					Type:        "string",
					Description: "The new content to replace the specified lines with",
				},
			},
			Required: []string{"path", "startLine", "endLine", "newContent"},
		},
	}
}

func (t *ReplaceInFileTool) Execute(_ context.Context, arguments string) (string, error) {
	var args struct {
		Path       string `json:"path"`
		StartLine  int    `json:"startLine"`
		EndLine    int    `json:"endLine"`
		NewContent string `json:"newContent"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cleanPath, err := utils.GetCleanPath(args.Path, true)
	if err != nil {
		return "", fmt.Errorf("failed to replace in file: %w", err)
	}

	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file does not exist: %s", args.Path)
		}
		return "", fmt.Errorf("failed to replace in file: %w", err)
	}
	if fileInfo.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", args.Path)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file does not exist: %s", args.Path)
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	total := int64(len(lines))

	if args.StartLine < 1 || args.StartLine > int(total) {
		return "", fmt.Errorf("startLine %d out of range [1, %d]", args.StartLine, total)
	}
	if args.EndLine < args.StartLine || args.EndLine > int(total) {
		return "", fmt.Errorf("endLine %d out of range [%d, %d]", args.EndLine, args.StartLine, total)
	}

	newLines := strings.Split(args.NewContent, "\n")
	result := make([]string, 0, total-int64(args.EndLine-args.StartLine+1)+int64(len(newLines)))
	result = append(result, lines[:args.StartLine-1]...)
	result = append(result, newLines...)
	result = append(result, lines[args.EndLine:]...)

	if err := os.WriteFile(cleanPath, []byte(strings.Join(result, "\n")), fileInfo.Mode()); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("successfully replaced lines %d-%d in %s", args.StartLine, args.EndLine, cleanPath), nil
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
