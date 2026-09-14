package agentloop

// This file contains the loop-owned callback bridge. The public middleware
// abstraction and typed middleware contexts live in pkg/infra/middleware so
// infrastructure features do not become agent-loop dependencies.

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

type ModelCallState struct {
	// Context is the context used for this hook and model invocation. A hook
	// may replace it with a derived context; the loop carries that value into
	// subsequent hooks and iterations.
	Context   context.Context
	Session   *conversation.Session
	Round     *conversation.Round
	Iteration int
	Request   *modelcall.ModelCallRequest
	Response  *modelcall.ModelCallResponse
	Err       error
	Emit      EventEmitter
	// RequestUsage accounts for work performed while preparing this call.
	RequestUsage conversation.TokenUsage
}

type ToolCallState struct {
	// Context is the context used for this hook and tool batch. A hook may
	// replace it with a derived context for the remainder of the iteration.
	Context   context.Context
	Session   *conversation.Session
	Round     *conversation.Round
	Iteration int
	Call      *conversation.ToolUseBlock
	Result    *conversation.ToolResultBlock
	Err       error
	Emit      EventEmitter
}

// LoopHooks are the low-level callback ports used by AgentLoop. The
// middleware package adapts its typed hook contexts to these callbacks; the
// loop package itself has no dependency on infrastructure middleware.
type LoopHooks struct {
	BeforeModelCall func(context.Context, *ModelCallState) error
	AfterModelCall  func(context.Context, *ModelCallState) error
	BeforeToolCall  func(context.Context, *ToolCallState) error
	AfterToolCall   func(context.Context, *ToolCallState) error
}

func contextOr(current, candidate context.Context) context.Context {
	if candidate != nil {
		return candidate
	}
	return current
}
