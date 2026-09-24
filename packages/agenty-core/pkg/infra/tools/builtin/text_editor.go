package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"unicode/utf8"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

const (
	textEditorToolName      = "str_replace_based_edit_tool"
	textEditorMaxCharacters = 10_000
)

type textEditorTool struct {
	fileSystem *fileSystem
}

type textEditorArguments struct {
	Command    string  `json:"command"`
	Path       string  `json:"path"`
	ViewRange  []int   `json:"view_range,omitempty"`
	OldStr     *string `json:"old_str,omitempty"`
	NewStr     *string `json:"new_str,omitempty"`
	FileText   *string `json:"file_text,omitempty"`
	InsertLine *int    `json:"insert_line,omitempty"`
	InsertText *string `json:"insert_text,omitempty"`
}

func (tool *textEditorTool) Definition() modelcall.ToolDefinition {
	return modelcall.ToolDefinition{
		Type:        modelcall.ToolTypeTextEditor,
		Destructive: true,
		Name:        textEditorToolName,
		Description: "View files or directories, then make one precise text edit. Commands: view, str_replace, create, insert.",
		InputSchema: objectSchema(map[string]modelcall.JSONSchema{
			"command": stringEnumSchema("Editor command.", "view", "str_replace", "create", "insert"),
			"path":    stringSchema("Path relative to the session working directory or an absolute path."),
			"view_range": {
				Type:        modelcall.JSONSchemaTypeArray,
				Description: "Optional [start, end] 1-based line range for view; use -1 for the final line.",
				Items:       &modelcall.JSONSchema{Type: modelcall.JSONSchemaTypeInteger},
				MinItems:    new(uint64(2)),
				MaxItems:    new(uint64(2)),
			},
			"old_str":     stringSchema("Exact unique text to replace."),
			"new_str":     stringSchema("Replacement text; may be empty."),
			"file_text":   stringSchema("Complete content for a new file."),
			"insert_line": integerSchema("Line after which to insert text; 0 means before the first line.", 0),
			"insert_text": stringSchema("Text to insert."),
		}, []string{"command", "path"}),
	}
}

func (tool *textEditorTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments textEditorArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("%s: %w", textEditorToolName, err)
	}
	if err := validateTextEditorArguments(arguments); err != nil {
		return nil, fmt.Errorf("%s: %w", textEditorToolName, err)
	}
	if arguments.Command == "view" {
		return tool.view(ctx, callContext, arguments)
	}

	tool.fileSystem.mu.Lock()
	defer tool.fileSystem.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	command := exec.CommandContext(ctx, "fileedit", "text_editor")
	if strings.TrimSpace(callContext.Cwd) != "" {
		command.Dir = callContext.Cwd
	}
	command.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", textEditorToolName, message)
	}

	var result applyPatchResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("%s: decode fileedit result: %w", textEditorToolName, err)
	}
	if !result.Success {
		return nil, fmt.Errorf("%s: fileedit returned an unsuccessful result", textEditorToolName)
	}
	return resultContent(result)
}

func (tool *textEditorTool) view(
	ctx context.Context,
	callContext agentloop.CallContext,
	arguments textEditorArguments,
) (conversation.Content, error) {
	path, err := resolvePath(arguments.Path, callContext.Cwd, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", textEditorToolName, err)
	}

	tool.fileSystem.mu.RLock()
	defer tool.fileSystem.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%s: inspect %q: %w", textEditorToolName, path, err)
	}
	if info.IsDir() {
		if len(arguments.ViewRange) != 0 {
			return nil, fmt.Errorf("%s: view_range only applies to files", textEditorToolName)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("%s: list %q: %w", textEditorToolName, path, err)
		}
		content := formatDirectoryEntries(entries)
		if len(content) > textEditorMaxCharacters {
			content = truncateText(content, textEditorMaxCharacters) + "\n[output truncated]"
		}
		return conversation.Text(content), nil
	}

	startLine, endLine := 0, 0
	if len(arguments.ViewRange) == 2 {
		startLine, endLine = arguments.ViewRange[0], arguments.ViewRange[1]
		if endLine == -1 {
			endLine = 0
		}
	}
	result, err := readTextFile(ctx, path, startLine, endLine)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", textEditorToolName, err)
	}
	content := truncateText(result.Content, textEditorMaxCharacters)
	if result.Truncated || len(content) < len(result.Content) {
		content += "\n[output truncated; use view_range to continue]"
	}
	return conversation.Text(content), nil
}

func validateTextEditorArguments(arguments textEditorArguments) error {
	if strings.TrimSpace(arguments.Path) == "" {
		return fmt.Errorf("path must not be empty")
	}
	switch arguments.Command {
	case "view":
		if len(arguments.ViewRange) != 0 {
			if len(arguments.ViewRange) != 2 || arguments.ViewRange[0] < 1 ||
				(arguments.ViewRange[1] != -1 && arguments.ViewRange[1] < arguments.ViewRange[0]) {
				return fmt.Errorf("view_range must be [start, end] with 1-based lines and -1 only as the end")
			}
		}
	case "str_replace":
		if arguments.OldStr == nil || *arguments.OldStr == "" || arguments.NewStr == nil {
			return fmt.Errorf("str_replace requires non-empty old_str and new_str")
		}
	case "create":
		if arguments.FileText == nil {
			return fmt.Errorf("create requires file_text")
		}
	case "insert":
		if arguments.InsertLine == nil || *arguments.InsertLine < 0 || arguments.InsertText == nil {
			return fmt.Errorf("insert requires non-negative insert_line and insert_text")
		}
	default:
		return fmt.Errorf("command must be view, str_replace, create, or insert")
	}
	return nil
}

func formatDirectoryEntries(entries []os.DirEntry) string {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		values = append(values, name)
	}
	sort.Strings(values)
	return strings.Join(values, "\n")
}

func truncateText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	return text[:cut]
}
