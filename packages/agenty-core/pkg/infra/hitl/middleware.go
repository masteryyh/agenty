// Package hitl pauses tool calls until the connected user makes a decision.
package hitl

import (
	"context"
	"fmt"
	"os"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/tools"
)

const (
	EventRequested agentloop.EventType = "tool_approval_requested"
	EventResolved  agentloop.EventType = "tool_approval_resolved"
	DeniedMessage                      = "The user denied this tool call. The tool was not executed."
)

var previewJSON = json.Config{UseNumber: true, SortMapKeys: true}.Froze()

type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
)

type Request struct {
	ApprovalID uuid.UUID                 `json:"approvalId"`
	ToolCall   conversation.ToolUseBlock `json:"toolCall"`
	Cwd        string                    `json:"cwd"`
	Preview    tools.CallPreview         `json:"preview"`
}

type Resolution struct {
	SessionID  uuid.UUID `json:"sessionId"`
	RoundID    uuid.UUID `json:"roundId"`
	ApprovalID uuid.UUID `json:"approvalId"`
	Decision   Decision  `json:"decision"`
}

type pendingRequest struct {
	sessionID uuid.UUID
	roundID   uuid.UUID
	ctx       context.Context
	decision  chan Decision
}

type Manager struct {
	mu      sync.Mutex
	pending map[uuid.UUID]*pendingRequest
}

func NewManager() *Manager {
	return &Manager{pending: make(map[uuid.UUID]*pendingRequest)}
}

func (manager *Manager) Middleware() middleware.Middleware {
	return middleware.Middleware{Name: "hitl", BeforeToolCall: manager.beforeToolCall}
}

func (manager *Manager) beforeToolCall(ctx context.Context, state *middleware.ToolCallContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if state == nil || state.Session == nil || state.Round == nil || state.Call == nil || state.Emit == nil {
		return fmt.Errorf("HITL requires a session, round, tool call and event emitter")
	}
	cwd := ""
	if state.Round.Cwd != nil {
		cwd = *state.Round.Cwd
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve approval working directory: %w", err)
		}
	}
	call := *state.Call
	call.Input = append([]byte(nil), call.Input...)
	preview := tools.CallPreview{Title: "Agenty wants to call this tool: " + call.Name, Detail: string(call.Input)}
	var input any
	if previewJSON.Unmarshal(call.Input, &input) == nil {
		if formatted, err := previewJSON.MarshalIndent(input, "", "  "); err == nil {
			preview.Detail = string(formatted)
		}
	}
	if previewer, ok := state.Tools.(tools.CallPreviewer); ok {
		callContext := agentloop.CallContext{SessionID: state.Session.ID, RoundID: state.Round.ID, Cwd: cwd}
		if custom := previewer.PreviewToolCall(callContext, call); custom != nil {
			preview = *custom
		}
	}
	request := Request{ApprovalID: uuid.New(), ToolCall: call, Cwd: cwd, Preview: preview}
	pending := &pendingRequest{
		sessionID: state.Session.ID, roundID: state.Round.ID, ctx: ctx, decision: make(chan Decision, 1),
	}
	manager.mu.Lock()
	manager.pending[request.ApprovalID] = pending
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.pending, request.ApprovalID)
		manager.mu.Unlock()
	}()

	// Register before publishing: a client can answer before Emit returns.
	if err := state.Emit(ctx, agentloop.Event{Type: EventRequested, Iteration: state.Iteration, Payload: request}); err != nil {
		return fmt.Errorf("publish tool approval: %w", err)
	}
	var decision Decision
	select {
	case <-ctx.Done():
		return ctx.Err()
	case decision = <-pending.decision:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if decision == Deny {
		state.Result = &conversation.ToolResultBlock{
			ToolUseID: call.ID, IsError: true, Content: conversation.Text(DeniedMessage),
		}
	}
	return state.Emit(ctx, agentloop.Event{
		Type: EventResolved, Iteration: state.Iteration,
		Payload: Resolution{SessionID: pending.sessionID, RoundID: pending.roundID, ApprovalID: request.ApprovalID, Decision: decision},
	})
}

// Resolve consumes one approval without waiting for tool execution.
func (manager *Manager) Resolve(ctx context.Context, resolution Resolution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if resolution.SessionID == uuid.Nil || resolution.RoundID == uuid.Nil || resolution.ApprovalID == uuid.Nil {
		return application.Validation("sessionId, roundId and approvalId are required")
	}
	if resolution.Decision != Allow && resolution.Decision != Deny {
		return application.Validation("decision must be allow or deny")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	pending, ok := manager.pending[resolution.ApprovalID]
	if !ok || pending.sessionID != resolution.SessionID || pending.roundID != resolution.RoundID || pending.ctx.Err() != nil {
		return application.NotFound("tool approval is no longer pending")
	}
	delete(manager.pending, resolution.ApprovalID)
	pending.decision <- resolution.Decision
	return nil
}
