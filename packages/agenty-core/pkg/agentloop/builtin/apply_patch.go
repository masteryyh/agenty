package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type applyPatchTool struct {
	fileSystem *fileSystem
}

type applyPatchArguments struct {
	Patch string `json:"patch"`
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

func (tool *applyPatchTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Type:        agentloop.ToolTypeApplyPatch,
		Name:        "apply_patch",
		Description: "Apply a complete V4A patch atomically and return each file's final diff and line counts.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
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
	if strings.TrimSpace(arguments.Patch) == "" {
		return nil, fmt.Errorf("apply_patch: patch must not be empty")
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
	command.Stdin = strings.NewReader(arguments.Patch)
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
