package builtin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/utils"
)

const (
	patchBeginMarker = "*** Begin Patch"
	patchEndMarker   = "*** End Patch"
	patchUpdateFile  = "*** Update File:"
	patchDeleteFile  = "*** Delete File:"
	patchAddFile     = "*** Add File:"
	patchMoveTo      = "*** Move to:"
)

type applyPatchTool struct {
	fileSystem *fileSystem
}

type applyPatchArguments struct {
	Operation *conversation.ApplyPatchOperation `json:"operation,omitempty"`
	Patch     string                            `json:"patch,omitempty"`
}

type applyPatchOperationResult struct {
	Type   conversation.ApplyPatchOperationType `json:"type"`
	Path   string                               `json:"path"`
	MoveTo string                               `json:"moveTo,omitempty"`
}

type applyPatchResult struct {
	Operations []applyPatchOperationResult `json:"operations"`
}

func (tool *applyPatchTool) Definition() agentloop.ToolDefinition {
	operationSchema := objectSchema(
		map[string]agentloop.JSONSchema{
			"type": stringSchema("Operation type: create_file, update_file, or delete_file."),
			"path": stringSchema("Absolute path or path relative to the session working directory."),
			"diff": stringSchema("Headerless V4A diff body for create_file or update_file."),
		},
		[]string{"type", "path"},
	)
	return agentloop.ToolDefinition{
		Type:        agentloop.ToolTypeApplyPatch,
		Name:        "apply_patch",
		Description: "Apply one native file operation or a complete V4A patch envelope.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"operation": operationSchema,
				"patch":     stringSchema("Complete V4A patch envelope."),
			},
			nil,
		),
	}
}

func (tool *applyPatchTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments applyPatchArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("apply_patch: %w", err)
	}

	hasOperation := arguments.Operation != nil
	hasPatch := arguments.Patch != ""
	if hasOperation == hasPatch {
		return nil, fmt.Errorf("apply_patch: exactly one of operation or patch is required")
	}

	operations := make([]conversation.ApplyPatchOperation, 0, 1)
	if hasOperation {
		operations = append(operations, *arguments.Operation)
	} else {
		parsed, err := parsePatchEnvelope(arguments.Patch)
		if err != nil {
			return nil, fmt.Errorf("apply_patch: parse patch: %w", err)
		}
		operations = parsed
	}

	tool.fileSystem.mu.Lock()
	defer tool.fileSystem.mu.Unlock()

	results := make([]applyPatchOperationResult, 0, len(operations))
	for index, operation := range operations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := executeApplyPatchOperation(callContext.Cwd, operation)
		if err != nil {
			return nil, fmt.Errorf(
				"apply_patch: operation %d %s %q: %w",
				index+1,
				operation.Type,
				operation.Path,
				err,
			)
		}
		results = append(results, result)
	}

	return resultContent(applyPatchResult{Operations: results})
}

func parsePatchEnvelope(patch string) ([]conversation.ApplyPatchOperation, error) {
	lines := normalizePatchEnvelopeLines(patch)
	if len(lines) < 3 || lines[0] != patchBeginMarker {
		return nil, fmt.Errorf("patch must start with %q", patchBeginMarker)
	}
	if lines[len(lines)-1] != patchEndMarker {
		return nil, fmt.Errorf("patch must end with %q", patchEndMarker)
	}

	operations := make([]conversation.ApplyPatchOperation, 0)
	for index := 1; index < len(lines)-1; {
		operation, nextIndex, err := parsePatchEnvelopeOperation(lines, index)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
		index = nextIndex
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("patch contains no file operations")
	}
	return operations, nil
}

func normalizePatchEnvelopeLines(patch string) []string {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func parsePatchEnvelopeOperation(
	lines []string,
	index int,
) (conversation.ApplyPatchOperation, int, error) {
	header := lines[index]
	operation := conversation.ApplyPatchOperation{}
	switch {
	case strings.HasPrefix(header, patchUpdateFile):
		operation.Type = conversation.ApplyPatchUpdateFile
		operation.Path = strings.TrimSpace(strings.TrimPrefix(header, patchUpdateFile))
	case strings.HasPrefix(header, patchDeleteFile):
		operation.Type = conversation.ApplyPatchDeleteFile
		operation.Path = strings.TrimSpace(strings.TrimPrefix(header, patchDeleteFile))
	case strings.HasPrefix(header, patchAddFile):
		operation.Type = conversation.ApplyPatchCreateFile
		operation.Path = strings.TrimSpace(strings.TrimPrefix(header, patchAddFile))
	default:
		return conversation.ApplyPatchOperation{}, 0, fmt.Errorf(
			"invalid patch header at line %d: %s",
			index+1,
			header,
		)
	}
	if operation.Path == "" {
		return conversation.ApplyPatchOperation{}, 0, fmt.Errorf("operation at line %d has an empty path", index+1)
	}

	index++
	if operation.Type == conversation.ApplyPatchUpdateFile && index < len(lines)-1 &&
		strings.HasPrefix(lines[index], patchMoveTo) {
		operation.MoveTo = strings.TrimSpace(strings.TrimPrefix(lines[index], patchMoveTo))
		if operation.MoveTo == "" {
			return conversation.ApplyPatchOperation{}, 0, fmt.Errorf("move at line %d has an empty path", index+1)
		}
		index++
	}

	bodyStart := index
	for index < len(lines)-1 && !isPatchOperationHeader(lines[index]) {
		index++
	}
	body := lines[bodyStart:index]
	if operation.Type == conversation.ApplyPatchDeleteFile && len(body) > 0 {
		return conversation.ApplyPatchOperation{}, 0, fmt.Errorf(
			"delete operation for %q must not contain a diff body",
			operation.Path,
		)
	}
	operation.Diff = strings.Join(body, "\n")
	return operation, index, nil
}

func isPatchOperationHeader(line string) bool {
	return strings.HasPrefix(line, patchUpdateFile) ||
		strings.HasPrefix(line, patchDeleteFile) ||
		strings.HasPrefix(line, patchAddFile)
}

func executeApplyPatchOperation(
	cwd string,
	operation conversation.ApplyPatchOperation,
) (applyPatchOperationResult, error) {
	path, err := resolvePath(operation.Path, cwd, false)
	if err != nil {
		return applyPatchOperationResult{}, err
	}
	result := applyPatchOperationResult{Type: operation.Type, Path: path}

	switch operation.Type {
	case conversation.ApplyPatchCreateFile:
		content, err := utils.ApplyDiff("", operation.Diff, utils.ApplyDiffCreate)
		if err != nil {
			return applyPatchOperationResult{}, fmt.Errorf("apply create diff: %w", err)
		}
		if _, err := writeTextFile(path, content, 0o644); err != nil {
			return applyPatchOperationResult{}, err
		}
	case conversation.ApplyPatchUpdateFile:
		if err := updateFileWithDiff(path, operation.Diff); err != nil {
			return applyPatchOperationResult{}, err
		}
		if operation.MoveTo != "" {
			moveTo, err := resolvePath(operation.MoveTo, cwd, false)
			if err != nil {
				return applyPatchOperationResult{}, fmt.Errorf("resolve move destination: %w", err)
			}
			if err := movePatchedFile(path, moveTo); err != nil {
				return applyPatchOperationResult{}, err
			}
			result.MoveTo = moveTo
		}
	case conversation.ApplyPatchDeleteFile:
		if err := removeApplyPatchFile(path); err != nil {
			return applyPatchOperationResult{}, err
		}
	default:
		return applyPatchOperationResult{}, fmt.Errorf("unsupported operation type %q", operation.Type)
	}

	return result, nil
}

func updateFileWithDiff(path, diff string) error {
	info, err := regularFileInfo(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	updated, err := utils.ApplyDiff(string(data), diff, utils.ApplyDiffDefault)
	if err != nil {
		return fmt.Errorf("apply update diff: %w", err)
	}
	if _, err := writeTextFile(path, updated, info.Mode().Perm()); err != nil {
		return err
	}
	return nil
}

func movePatchedFile(source, destination string) error {
	if source == destination {
		return nil
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("move destination %q already exists", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect move destination %q: %w", destination, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create move destination parent for %q: %w", destination, err)
	}
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("move %q to %q: %w", source, destination, err)
	}
	return nil
}

func removeApplyPatchFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("path %q is a directory", path)
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("path %q is not a file or symbolic link", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}
