package tools

import (
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
)

// CallPreview is display-only metadata; producing it must not execute the tool.
type CallPreview struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// ApprovalPreviewer is implemented by tools with a dedicated approval display.
type ApprovalPreviewer interface {
	ApprovalPreview(agentloop.CallContext, []byte) *CallPreview
}

// CallPreviewer resolves previews from the same snapshot used for execution.
type CallPreviewer interface {
	PreviewToolCall(agentloop.CallContext, conversation.ToolUseBlock) *CallPreview
}

func (registry *Registry) PreviewToolCall(ctx agentloop.CallContext, call conversation.ToolUseBlock) *CallPreview {
	tool, ok := registry.Get(call.Name)
	if !ok {
		return nil
	}
	if previewer, ok := tool.(ApprovalPreviewer); ok {
		return previewer.ApprovalPreview(ctx, call.Input)
	}
	return nil
}

func (runtime *combinedRuntime) PreviewToolCall(ctx agentloop.CallContext, call conversation.ToolUseBlock) *CallPreview {
	owner, ok := runtime.owners[call.Name]
	if !ok {
		return nil
	}
	if previewer, ok := runtime.components[owner].(CallPreviewer); ok {
		return previewer.PreviewToolCall(ctx, call)
	}
	return nil
}
