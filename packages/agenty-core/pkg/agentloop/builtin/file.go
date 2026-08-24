package builtin

import (
	"bufio"
	"context"
	"fmt"
	"os"
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
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
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
				"path":       stringSchema("Absolute path or path relative to the session working directory."),
				"start_line": integerSchema("Optional inclusive first line, using a 1-based index.", 1),
				"end_line":   integerSchema("Optional inclusive last line, using a 1-based index.", 1),
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
			return nil, fmt.Errorf("read_file: start_line must be positive")
		}
	}
	endLine := 0
	if arguments.EndLine != nil {
		endLine = *arguments.EndLine
		if endLine < 1 {
			return nil, fmt.Errorf("read_file: end_line must be positive")
		}
	}
	if startLine > 0 && endLine > 0 && startLine > endLine {
		return nil, fmt.Errorf("read_file: start_line must not exceed end_line")
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
		return readFileResult{}, fmt.Errorf("start_line %d exceeds file length %d", start, lineNumber)
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
