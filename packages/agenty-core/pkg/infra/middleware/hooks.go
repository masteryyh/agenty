// Package middleware defines the infrastructure-level hook contract used to
// assemble optional session capabilities around the agent loop.
package middleware

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

type Phase string

const (
	PhaseBeforeSessionStart Phase = "BeforeSessionStart"
	PhaseBeforeRound        Phase = "BeforeRound"
	PhaseAfterRound         Phase = "AfterRound"
	PhaseBeforeModelCall    Phase = "BeforeModelCall"
	PhaseAfterModelCall     Phase = "AfterModelCall"
	PhaseBeforeToolCall     Phase = "BeforeToolCall"
	PhaseAfterToolCall      Phase = "AfterToolCall"
	PhaseAfterSessionStop   Phase = "AfterSessionStop"
	PhaseOnEvent            Phase = "OnEvent"
)

type SessionStartContext struct {
	// Context is the context used for this lifecycle hook. Hooks may replace it
	// with a derived context for later hooks and the session execution.
	Context      context.Context
	Session      *conversation.Session
	Provider     *catalog.Provider
	Model        *catalog.Model
	SystemPrompt *string
	FreeFormTool *bool
	Tools        *agentloop.ToolRuntime
	// AppendHiddenMessage queues a hidden message until the first round is
	// allocated. The role is chosen by the middleware according to the provider
	// contract.
	AppendHiddenMessage func(conversation.Role, conversation.Content, map[string]any) error
	Emit                agentloop.EventEmitter
}

type RoundContext struct {
	// Context is the context used for this lifecycle hook. Hooks may replace it
	// with a derived context for later hooks and the round execution.
	Context             context.Context
	Session             *conversation.Session
	Round               *conversation.Round
	RoundID             uuid.UUID
	Provider            *catalog.Provider
	Model               *catalog.Model
	Content             *conversation.Content
	Tools               *agentloop.ToolRuntime
	SystemPrompt        *string
	AppendHiddenMessage func(conversation.Role, conversation.Content, map[string]any) error
	Status              conversation.RoundStatus
	Usage               conversation.TokenUsage
	Err                 error
	Emit                agentloop.EventEmitter
}

type ModelCallContext struct {
	// Context is the context used for this hook and model invocation. Hooks may
	// replace it with a derived context for later hooks and iterations.
	Context   context.Context
	Session   *conversation.Session
	Round     *conversation.Round
	Iteration int
	Request   *modelcall.ModelCallRequest
	Response  *modelcall.ModelCallResponse
	Err       error
	// RequestUsage accounts for preparation work performed by a hook.
	RequestUsage conversation.TokenUsage
	Emit         agentloop.EventEmitter
}

type ToolCallContext struct {
	// Context is the context used for this hook and tool batch. Hooks may replace
	// it with a derived context for later hooks and iterations.
	Context   context.Context
	Session   *conversation.Session
	Round     *conversation.Round
	Iteration int
	Call      *conversation.ToolUseBlock
	Tools     agentloop.ToolRuntime
	Model     modelcall.ModelCallConfig
	// A result supplied by BeforeToolCall replaces execution of this call.
	Result *conversation.ToolResultBlock
	Err    error
	Emit   agentloop.EventEmitter
}

type SessionStopContext struct {
	// Context is the context used for this lifecycle hook. Hooks may replace it
	// with a derived context for later hooks in the stop phase.
	Context context.Context
	Session *conversation.Session
	Err     error
	Emit    agentloop.EventEmitter
}

// EventContext gives event consumers the runtime state associated with one
// emitted event. Consumers may use Emit to publish a follow-up event.
type EventContext struct {
	Context  context.Context
	Session  *conversation.Session
	Round    *conversation.Round
	RoundID  uuid.UUID
	Provider *catalog.Provider
	Model    *catalog.Model
	Event    *agentloop.Event
	Emit     agentloop.EventEmitter
}

type BeforeSessionStartHook func(context.Context, *SessionStartContext) error
type BeforeRoundHook func(context.Context, *RoundContext) error
type AfterRoundHook func(context.Context, *RoundContext) error
type BeforeModelCallHook func(context.Context, *ModelCallContext) error
type AfterModelCallHook func(context.Context, *ModelCallContext) error
type BeforeToolCallHook func(context.Context, *ToolCallContext) error
type AfterToolCallHook func(context.Context, *ToolCallContext) error
type AfterSessionStopHook func(context.Context, *SessionStopContext) error
type OnEventHook func(context.Context, *EventContext) error

// Middleware defines a named capability with any subset of the nine hooks.
// The manager skips nil hooks while compiling its immutable execution chains.
type Middleware struct {
	Name               string
	BeforeSessionStart BeforeSessionStartHook
	BeforeRound        BeforeRoundHook
	AfterRound         AfterRoundHook
	BeforeModelCall    BeforeModelCallHook
	AfterModelCall     AfterModelCallHook
	BeforeToolCall     BeforeToolCallHook
	AfterToolCall      AfterToolCallHook
	AfterSessionStop   AfterSessionStopHook
	OnEvent            OnEventHook
}

// LifecycleHooks is the compiled session-level hook bridge. It stays in the
// middleware package so session orchestration does not add lifecycle concepts
// to the atomic agent loop.
type LifecycleHooks struct {
	BeforeSessionStart func(context.Context, *SessionStartContext) error
	BeforeRound        func(context.Context, *RoundContext) error
	AfterRound         func(context.Context, *RoundContext) error
	AfterSessionStop   func(context.Context, *SessionStopContext) error
	OnEvent            func(context.Context, *EventContext) error
}

type HookError struct {
	Middleware string
	Phase      Phase
	Err        error
}

func (err *HookError) Error() string {
	if err == nil {
		return "middleware hook failed"
	}
	return fmt.Sprintf("middleware %q %s: %v", err.Middleware, err.Phase, err.Err)
}

func (err *HookError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}
