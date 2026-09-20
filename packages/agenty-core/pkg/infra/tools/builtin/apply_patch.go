package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

type applyPatchTool struct {
	fileSystem *fileSystem
}

type applyPatchArguments struct {
	Patch     string                            `json:"patch"`
	Operation *conversation.ApplyPatchOperation `json:"operation"`
}

type applyPatchFileResult struct {
	Path         string `json:"path"`
	Diff         string `json:"diff"`
	AddedLines   int    `json:"addedLines"`
	RemovedLines int    `json:"removedLines"`
}

type applyPatchResult struct {
	Success bool                   `json:"success"`
	Cwd     string                 `json:"cwd"`
	Files   []applyPatchFileResult `json:"files"`
}

func (tool *applyPatchTool) Definition() modelcall.ToolDefinition {
	return modelcall.ToolDefinition{
		Type:        modelcall.ToolTypeApplyPatch,
		Destructive: true,
		Name:        "apply_patch",
		Description: "Apply a complete V4A patch atomically and return each file's final diff and line counts.",
		InputSchema: objectSchema(
			map[string]modelcall.JSONSchema{
				"patch": stringSchema("Complete V4A patch envelope."),
			},
			[]string{"patch"},
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
	patch, err := applyPatchEnvelope(arguments)
	if err != nil {
		return nil, fmt.Errorf("apply_patch: %w", err)
	}

	tool.fileSystem.mu.Lock()
	defer tool.fileSystem.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.Command("apply_patch")
	if strings.TrimSpace(callContext.Cwd) != "" {
		command.Dir = callContext.Cwd
	}
	command.Stdin = strings.NewReader(patch)
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
		return nil, fmt.Errorf("apply_patch: %s", message)
	}

	var result applyPatchResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("apply_patch: decode helper result: %w", err)
	}
	if !result.Success {
		return nil, fmt.Errorf("apply_patch: helper returned an unsuccessful result")
	}
	return resultContent(result)
}

func applyPatchEnvelope(arguments applyPatchArguments) (string, error) {
	if strings.TrimSpace(arguments.Patch) != "" {
		if arguments.Operation != nil {
			return "", fmt.Errorf("patch and operation cannot both be set")
		}
		return arguments.Patch, nil
	}
	if arguments.Operation == nil {
		return "", fmt.Errorf("patch must not be empty")
	}

	operation := arguments.Operation
	if strings.TrimSpace(operation.Path) == "" {
		return "", fmt.Errorf("operation path must not be empty")
	}
	var builder strings.Builder
	builder.WriteString(patchBeginMarker)
	builder.WriteByte('\n')
	switch operation.Type {
	case conversation.ApplyPatchCreateFile:
		builder.WriteString(patchAddMarker)
	case conversation.ApplyPatchUpdateFile:
		builder.WriteString(patchUpdateMarker)
	case conversation.ApplyPatchDeleteFile:
		builder.WriteString(patchDeleteMarker)
	default:
		return "", fmt.Errorf("unknown operation type %q", operation.Type)
	}
	builder.WriteByte(' ')
	builder.WriteString(operation.Path)
	builder.WriteByte('\n')
	if operation.MoveTo != "" {
		if operation.Type != conversation.ApplyPatchUpdateFile {
			return "", fmt.Errorf("only update operations can move files")
		}
		builder.WriteString(patchMoveMarker)
		builder.WriteByte(' ')
		builder.WriteString(operation.MoveTo)
		builder.WriteByte('\n')
	}
	if operation.Type == conversation.ApplyPatchDeleteFile && operation.Diff != "" {
		return "", fmt.Errorf("delete operations must not contain a diff")
	}
	if operation.Diff != "" {
		builder.WriteString(operation.Diff)
		if !strings.HasSuffix(operation.Diff, "\n") {
			builder.WriteByte('\n')
		}
	}
	builder.WriteString(patchEndMarker)
	return builder.String(), nil
}
