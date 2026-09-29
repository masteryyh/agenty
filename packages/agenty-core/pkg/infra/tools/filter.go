package tools

import (
	"context"
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

// Filter returns an immutable tool view. Definitions, execution, approval
// previews, and auto-approval checks all use the same selected set.
func Filter(runtime agentloop.ToolRuntime, include func(modelcall.ToolDefinition) bool) agentloop.ToolRuntime {
	if runtime == nil || include == nil {
		return runtime
	}

	definitions := runtime.Definitions()
	allowed := make(map[string]struct{}, len(definitions))
	filtered := make([]modelcall.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if !include(definition) {
			continue
		}
		allowed[definition.Name] = struct{}{}
		filtered = append(filtered, definition)
	}
	return &filteredRuntime{runtime: runtime, definitions: filtered, allowed: allowed}
}

type filteredRuntime struct {
	runtime     agentloop.ToolRuntime
	definitions []modelcall.ToolDefinition
	allowed     map[string]struct{}
}

func (runtime *filteredRuntime) Definitions() []modelcall.ToolDefinition {
	return append([]modelcall.ToolDefinition(nil), runtime.definitions...)
}

func (runtime *filteredRuntime) ExecuteBatch(
	ctx context.Context,
	callContext agentloop.CallContext,
	calls []conversation.ToolUseBlock,
) []conversation.ToolResultBlock {
	results := make([]conversation.ToolResultBlock, len(calls))
	allowedCalls := make([]conversation.ToolUseBlock, 0, len(calls))
	allowedIndexes := make([]int, 0, len(calls))
	for index, call := range calls {
		if _, ok := runtime.allowed[call.Name]; !ok {
			results[index] = conversation.ToolResultBlock{
				ToolUseID: call.ID,
				Content:   conversation.Text(fmt.Sprintf("tool %q is not available in this session", call.Name)),
				IsError:   true,
			}
			continue
		}
		allowedCalls = append(allowedCalls, call)
		allowedIndexes = append(allowedIndexes, index)
	}

	delegated := runtime.runtime.ExecuteBatch(ctx, callContext, allowedCalls)
	for delegatedIndex, resultIndex := range allowedIndexes {
		if delegatedIndex < len(delegated) {
			results[resultIndex] = delegated[delegatedIndex]
			continue
		}
		results[resultIndex] = conversation.ToolResultBlock{
			ToolUseID: calls[resultIndex].ID,
			Content:   conversation.Text(fmt.Sprintf("tool %q returned no result", calls[resultIndex].Name)),
			IsError:   true,
		}
	}
	return results
}

func (runtime *filteredRuntime) PreviewToolCall(
	ctx agentloop.CallContext,
	call conversation.ToolUseBlock,
) *CallPreview {
	if _, ok := runtime.allowed[call.Name]; !ok {
		return nil
	}
	previewer, ok := runtime.runtime.(CallPreviewer)
	if !ok {
		return nil
	}
	return previewer.PreviewToolCall(ctx, call)
}

func (runtime *filteredRuntime) CanAutoApproveToolCall(
	ctx agentloop.CallContext,
	call conversation.ToolUseBlock,
) bool {
	if _, ok := runtime.allowed[call.Name]; !ok {
		return false
	}
	checker, ok := runtime.runtime.(AutoApprovalChecker)
	return ok && checker.CanAutoApproveToolCall(ctx, call)
}
