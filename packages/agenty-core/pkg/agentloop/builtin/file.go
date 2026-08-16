package builtin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type readFileTool struct {
	fileSystem *fileSystem
}

type readFileArguments struct {
	Path      string `json:"path"`
	StartLine *int   `json:"startLine,omitempty"`
	EndLine   *int   `json:"endLine,omitempty"`
}

type readFileResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Truncated bool   `json:"truncated"`
}

func (tool *readFileTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name: "read_file",
		Description: "Read a text file with optional inclusive 1-based line bounds. " +
			"Relative paths resolve from the session working directory. The result contains numbered lines.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"path":      stringSchema("Absolute path or path relative to the session working directory."),
				"startLine": integerSchema("Optional inclusive first line, using a 1-based index.", 1),
				"endLine":   integerSchema("Optional inclusive last line, using a 1-based index.", 1),
			},
			[]string{"path"},
		),
	}
}

func (tool *readFileTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments readFileArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("read_file: %w", err)
	}

	startLine := 0
	if arguments.StartLine != nil {
		startLine = *arguments.StartLine
		if startLine < 1 {
			return nil, fmt.Errorf("read_file: startLine must be positive")
		}
	}
	endLine := 0
	if arguments.EndLine != nil {
		endLine = *arguments.EndLine
		if endLine < 1 {
			return nil, fmt.Errorf("read_file: endLine must be positive")
		}
	}
	if startLine > 0 && endLine > 0 && startLine > endLine {
		return nil, fmt.Errorf("read_file: startLine must not exceed endLine")
	}

	path, err := resolvePath(arguments.Path, callContext.Cwd, false)
	if err != nil {
		return nil, fmt.Errorf("read_file: %w", err)
	}

	tool.fileSystem.mu.RLock()
	defer tool.fileSystem.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err := readTextFile(ctx, path, startLine, endLine)
	if err != nil {
		return nil, fmt.Errorf("read_file: %w", err)
	}
	return resultContent(result)
}

func readTextFile(
	ctx context.Context,
	path string,
	startLine int,
	endLine int,
) (readFileResult, error) {
	info, err := regularFileInfo(path)
	if err != nil {
		return readFileResult{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return readFileResult{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()

	start := max(1, startLine)
	result := readFileResult{
		Path:      path,
		StartLine: start,
		EndLine:   start - 1,
	}
	if info.Size() == 0 {
		return result, nil
	}

	var builder strings.Builder
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxScannerTokenSize)
	lineNumber := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return readFileResult{}, err
		}

		lineNumber++
		if lineNumber < start {
			continue
		}
		if endLine > 0 && lineNumber > endLine {
			break
		}
		previousLength := builder.Len()
		if !appendNumberedLine(&builder, lineNumber, scanner.Text()) {
			if builder.Len() > previousLength {
				result.EndLine = lineNumber
			}
			result.Truncated = true
			break
		}
		result.EndLine = lineNumber
	}
	if err := scanner.Err(); err != nil {
		return readFileResult{}, fmt.Errorf("scan %q: %w", path, err)
	}
	if lineNumber < start {
		return readFileResult{}, fmt.Errorf("startLine %d exceeds file length %d", start, lineNumber)
	}

	result.Content = builder.String()
	return result, nil
}

func appendNumberedLine(builder *strings.Builder, lineNumber int, line string) bool {
	prefix := strconv.Itoa(lineNumber) + ": "
	separatorLength := 0
	if builder.Len() > 0 {
		separatorLength = 1
	}
	remaining := maxReadOutputBytes - builder.Len() - separatorLength - len(prefix)
	if remaining < 0 {
		return false
	}

	truncated := false
	if len(line) > remaining {
		line = truncateUTF8(line, remaining)
		truncated = true
	}
	if separatorLength > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(prefix)
	builder.WriteString(line)
	return !truncated
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}

	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

type writeFileTool struct {
	fileSystem *fileSystem
}

type writeFileArguments struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

type writeFileResult struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytesWritten"`
	Created      bool   `json:"created"`
}

func (tool *writeFileTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name: "write_file",
		Description: "Create or overwrite a file. Missing parent directories are created. " +
			"Relative paths resolve from the session working directory.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"path":    stringSchema("Absolute path or path relative to the session working directory."),
				"content": stringSchema("Complete file content to write."),
			},
			[]string{"path", "content"},
		),
	}
}

func (tool *writeFileTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments writeFileArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("write_file: %w", err)
	}
	if arguments.Content == nil {
		return nil, fmt.Errorf("write_file: content is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := resolvePath(arguments.Path, callContext.Cwd, false)
	if err != nil {
		return nil, fmt.Errorf("write_file: %w", err)
	}

	tool.fileSystem.mu.Lock()
	defer tool.fileSystem.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	created, err := writeTextFile(path, *arguments.Content, 0o644)
	if err != nil {
		return nil, fmt.Errorf("write_file: %w", err)
	}
	return resultContent(writeFileResult{
		Path:         path,
		BytesWritten: len(*arguments.Content),
		Created:      created,
	})
}

func writeTextFile(path, content string, defaultMode os.FileMode) (bool, error) {
	created := false
	mode := defaultMode
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		created = true
	case err != nil:
		return false, fmt.Errorf("inspect %q: %w", path, err)
	default:
		if info.IsDir() {
			return false, fmt.Errorf("path %q is a directory", path)
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("path %q is not a regular file", path)
		}
		mode = info.Mode().Perm()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create parent directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return false, fmt.Errorf("write %q: %w", path, err)
	}
	return created, nil
}

type patchFileTool struct {
	fileSystem *fileSystem
}

type patchFileArguments struct {
	Path       string  `json:"path"`
	OldText    string  `json:"oldText"`
	NewText    *string `json:"newText"`
	ReplaceAll bool    `json:"replaceAll,omitempty"`
}

type patchFileResult struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
	BytesWritten int    `json:"bytesWritten"`
}

func (tool *patchFileTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name: "patch_file",
		Description: "Replace exact text in an existing file. By default oldText must occur exactly once; " +
			"set replaceAll to replace every occurrence.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"path":       stringSchema("Absolute path or path relative to the session working directory."),
				"oldText":    stringSchema("Exact text that must already exist in the file."),
				"newText":    stringSchema("Replacement text. May be empty to remove oldText."),
				"replaceAll": booleanSchema("Replace all occurrences instead of requiring one unique occurrence."),
			},
			[]string{"path", "oldText", "newText"},
		),
	}
}

func (tool *patchFileTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments patchFileArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("patch_file: %w", err)
	}

	if arguments.OldText == "" {
		return nil, fmt.Errorf("patch_file: oldText must not be empty")
	}
	if arguments.NewText == nil {
		return nil, fmt.Errorf("patch_file: newText is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := resolvePath(arguments.Path, callContext.Cwd, false)
	if err != nil {
		return nil, fmt.Errorf("patch_file: %w", err)
	}

	tool.fileSystem.mu.Lock()
	defer tool.fileSystem.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info, err := regularFileInfo(path)
	if err != nil {
		return nil, fmt.Errorf("patch_file: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("patch_file: read %q: %w", path, err)
	}

	content := string(data)
	occurrences := strings.Count(content, arguments.OldText)
	if occurrences == 0 {
		return nil, fmt.Errorf("patch_file: oldText was not found in %q", path)
	}
	if !arguments.ReplaceAll && occurrences != 1 {
		return nil, fmt.Errorf(
			"patch_file: oldText occurs %d times in %q; set replaceAll to replace every occurrence",
			occurrences,
			path,
		)
	}

	replacements := 1
	if arguments.ReplaceAll {
		replacements = occurrences
	}
	updated := strings.Replace(content, arguments.OldText, *arguments.NewText, replacements)
	if _, err := writeTextFile(path, updated, info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("patch_file: %w", err)
	}
	return resultContent(patchFileResult{
		Path:         path,
		Replacements: replacements,
		BytesWritten: len(updated),
	})
}

type deleteFileTool struct {
	fileSystem *fileSystem
}

type deleteFileArguments struct {
	Path string `json:"path"`
}

type deleteFileResult struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

func (tool *deleteFileTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name:        "delete_file",
		Description: "Delete one file or symbolic link. Directories are rejected and are never removed recursively.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"path": stringSchema("Absolute path or path relative to the session working directory."),
			},
			[]string{"path"},
		),
	}
}

func (tool *deleteFileTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments deleteFileArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("delete_file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := resolvePath(arguments.Path, callContext.Cwd, false)
	if err != nil {
		return nil, fmt.Errorf("delete_file: %w", err)
	}

	tool.fileSystem.mu.Lock()
	defer tool.fileSystem.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("delete_file: inspect %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("delete_file: path %q is a directory", path)
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return nil, fmt.Errorf("delete_file: path %q is not a file or symbolic link", path)
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("delete_file: remove %q: %w", path, err)
	}
	return resultContent(deleteFileResult{Path: path, Deleted: true})
}
