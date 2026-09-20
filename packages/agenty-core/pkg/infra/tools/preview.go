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

// AutoApprovalTool is implemented by tools whose inputs can be checked without
// executing the tool. A false result always falls through to the permission
// reviewer; it never blocks normal execution by itself.
type AutoApprovalTool interface {
	CanAutoApprove(agentloop.CallContext, []byte) bool
}

// AutoApprovalChecker resolves a check from the same immutable tool snapshot
// that will later execute the call.
type AutoApprovalChecker interface {
	CanAutoApproveToolCall(agentloop.CallContext, conversation.ToolUseBlock) bool
}

func (registry *Registry) CanAutoApproveToolCall(ctx agentloop.CallContext, call conversation.ToolUseBlock) bool {
	tool, ok := registry.Get(call.Name)
	if !ok {
		return false
	}
	checker, ok := tool.(AutoApprovalTool)
	return ok && checker.CanAutoApprove(ctx, call.Input)
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

func (runtime *combinedRuntime) CanAutoApproveToolCall(ctx agentloop.CallContext, call conversation.ToolUseBlock) bool {
	owner, ok := runtime.owners[call.Name]
	if !ok {
		return false
	}
	checker, ok := runtime.components[owner].(AutoApprovalChecker)
	return ok && checker.CanAutoApproveToolCall(ctx, call)
}
