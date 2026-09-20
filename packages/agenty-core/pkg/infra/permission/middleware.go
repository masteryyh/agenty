package permission

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/prompt"
	"github.com/masteryyh/agenty-core/pkg/infra/tools"
)

const (
	EventRequested      agentloop.EventType = "tool_approval_requested"
	EventResolved       agentloop.EventType = "tool_approval_resolved"
	EventReviewStarted  agentloop.EventType = "tool_review_started"
	EventReviewResolved agentloop.EventType = "tool_review_resolved"
	DeniedMessage                           = "The user denied this tool call. The tool was not executed."
	AutoDeniedMessage                       = "Auto reviewer rejected this tool execution."
)

var previewJSON = json.Config{UseNumber: true, SortMapKeys: true}.Froze()

type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"

	recheck Decision = "recheck"
)

var errInvalidReviewResponse = errors.New("automatic review returned invalid output")

func autoReview(
	ctx context.Context,
	model modelcall.ModelCallConfig,
	content string,
) (ReviewResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	effort, err := reviewReasoningEffort(model)
	if err != nil {
		return ReviewResult{}, err
	}
	request := modelcall.ModelCallRequest{
		SystemPrompt:    prompt.AutoReviewSystemPrompt,
		Messages:        []modelcall.ModelCallMessage{{Role: conversation.RoleUser, Content: conversation.Text(content)}},
		MaxOutputTokens: 512,
		ReasoningEffort: effort,
		OutputFormat:    autoReviewFormat(),
	}
	// SDKs retry transient transport errors. Only invalid completed outputs
	// receive a second model invocation, so HTTP attempts are bounded at six.
	for attempt := range 2 {
		response, err := modelcall.Call(ctx, model, request, modelcall.WithMaxRetries(2))
		if err != nil {
			return ReviewResult{}, fmt.Errorf("invoke automatic reviewer: %w", err)
		}

		result, err := reviewDecision(response)
		if err == nil {
			return result, nil
		}
		if response.StopReason == modelcall.ModelCallStopReasonContentFilter {
			return ReviewResult{}, fmt.Errorf("%w: %v", errInvalidReviewResponse, err)
		}
		if attempt == 1 {
			break
		}
		request.Messages = append(request.Messages, modelcall.ModelCallMessage{
			Role: conversation.RoleUser,
			Content: conversation.Text("Your previous response failed validation: " + err.Error() +
				". Return a complete JSON object matching the schema. Reassess the same action; do not change a valid risk judgment just to obtain allow."),
		})
	}
	return ReviewResult{
		Decision: Deny,
		Message:  "Auto review failed: invalid output after retry.",
	}, nil
}

func reviewReasoningEffort(model modelcall.ModelCallConfig) (shared.ReasoningEffort, error) {
	if !model.SupportsReasoning {
		return "", nil
	}
	for _, effort := range shared.StandardReasoningEfforts() {
		if slices.Contains(model.ReasoningEfforts, effort) {
			return effort, nil
		}
	}
	return "", fmt.Errorf("automatic reviewer has no supported reasoning level")
}

func reviewFailureMessage(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Auto review failed: request timed out; please review this action."
	case errors.Is(err, errInvalidReviewResponse):
		return "Auto review failed: invalid output; please review this action."
	default:
		// Provider errors may contain request bodies or credentials.
		return "Auto review failed: request could not be completed; please review this action."
	}
}

type Request struct {
	Message    string                    `json:"message,omitempty"`
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

type ReviewEvent struct {
	ToolUseID string `json:"toolUseId"`
}

type pendingRequest struct {
	sessionID uuid.UUID
	roundID   uuid.UUID
	ctx       context.Context
	decision  chan Decision
}

type PermissionManager struct {
	mu      sync.Mutex
	pending map[uuid.UUID]*pendingRequest
}

func NewPermissionManager() *PermissionManager {
	return &PermissionManager{pending: make(map[uuid.UUID]*pendingRequest)}
}

func (manager *PermissionManager) Middleware() middleware.Middleware {
	return middleware.Middleware{Name: "permissions", BeforeToolCall: manager.beforeToolCall}
}

// PermissionModeChanged wakes a manual approval when a later auto-mode change
// needs to re-evaluate it. Switching to yolo keeps the existing immediate
// release behavior.
func (manager *PermissionManager) PermissionModeChanged(
	ctx context.Context,
	sessionID uuid.UUID,
	mode conversation.PermissionMode,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	for approvalID, pending := range manager.pending {
		if pending.sessionID != sessionID || pending.ctx.Err() != nil {
			continue
		}

		switch mode {
		case conversation.PermissionYolo:
			delete(manager.pending, approvalID)
			select {
			case pending.decision <- Allow:
			default:
			}
		case conversation.PermissionAuto:
			select {
			case pending.decision <- recheck:
			default:
			}
		}
	}
	return nil
}

func (manager *PermissionManager) beforeToolCall(ctx context.Context, state *middleware.ToolCallContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if state == nil || state.Session == nil || state.Round == nil || state.Call == nil || state.Emit == nil {
		return fmt.Errorf("HITL requires a session, round, tool call and event emitter")
	}

	cwd, err := approvalCwd(state.Round)
	if err != nil {
		return err
	}
	call := cloneCall(*state.Call)
	var message string

	switch state.Session.CurrentPermissionMode() {
	case conversation.PermissionYolo:
		return nil
	case conversation.PermissionAuto:
		review, reviewErr := manager.autoDecision(ctx, state, call, cwd)
		if reviewErr == nil && review.Decision == Allow {
			return nil
		}
		if reviewErr == nil && review.Decision == Deny {
			state.Result = deniedResult(call.ID, AutoDeniedMessage+" "+review.Message)
			return nil
		}
		message = review.Message
		if reviewErr != nil {
			message = reviewFailureMessage(reviewErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return manager.awaitManualDecision(ctx, state, call, cwd, message)
}

func approvalCwd(round *conversation.Round) (string, error) {
	if round != nil && round.Cwd != nil && *round.Cwd != "" {
		return *round.Cwd, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve approval working directory: %w", err)
	}
	return cwd, nil
}

func cloneCall(call conversation.ToolUseBlock) conversation.ToolUseBlock {
	call.Input = append([]byte(nil), call.Input...)
	return call
}

func (manager *PermissionManager) autoDecision(
	ctx context.Context,
	state *middleware.ToolCallContext,
	call conversation.ToolUseBlock,
	cwd string,
) (ReviewResult, error) {
	if automaticallyAllowed(state, call, cwd) {
		return ReviewResult{Decision: Allow}, nil
	}
	if state.Model.ModelCode == "" || !state.Model.APIType.Valid() {
		return ReviewResult{Decision: Ask}, fmt.Errorf("automatic reviewer has no model configuration")
	}

	content := autoReviewContent(state.Session, state.Round, call, cwd)
	review := ReviewEvent{ToolUseID: call.ID}
	if err := state.Emit(ctx, agentloop.Event{
		Type:      EventReviewStarted,
		Iteration: state.Iteration,
		Payload:   review,
	}); err != nil {
		return ReviewResult{Decision: Ask}, fmt.Errorf("publish automatic review start: %w", err)
	}

	decision, reviewErr := autoReview(ctx, state.Model, content)
	resolveErr := state.Emit(ctx, agentloop.Event{
		Type:      EventReviewResolved,
		Iteration: state.Iteration,
		Payload:   review,
	})
	if resolveErr != nil {
		resolveErr = fmt.Errorf("publish automatic review resolution: %w", resolveErr)
	}
	if reviewErr != nil || resolveErr != nil {
		return ReviewResult{Decision: Ask}, errors.Join(reviewErr, resolveErr)
	}

	switch state.Session.CurrentPermissionMode() {
	case conversation.PermissionYolo:
		return ReviewResult{Decision: Allow}, nil
	case conversation.PermissionAuto:
		return decision, nil
	default:
		return ReviewResult{Decision: Ask}, nil
	}
}

func automaticallyAllowed(state *middleware.ToolCallContext, call conversation.ToolUseBlock, cwd string) bool {
	callContext := agentloop.CallContext{SessionID: state.Session.ID, RoundID: state.Round.ID, Cwd: cwd}
	if checker, ok := state.Tools.(tools.AutoApprovalChecker); ok && checker.CanAutoApproveToolCall(callContext, call) {
		return true
	}

	if !strings.HasPrefix(call.Name, "mcp__") || state.Tools == nil {
		return false
	}
	for _, definition := range state.Tools.Definitions() {
		if definition.Name == call.Name {
			return !definition.Destructive
		}
	}
	return false
}

func (manager *PermissionManager) awaitManualDecision(
	ctx context.Context,
	state *middleware.ToolCallContext,
	call conversation.ToolUseBlock,
	cwd string,
	message string,
) error {
	request := Request{
		Message: message, ApprovalID: uuid.New(),
		ToolCall: call,
		Cwd:      cwd,
		Preview:  approvalPreview(state, call, cwd),
	}
	pending := &pendingRequest{
		sessionID: state.Session.ID,
		roundID:   state.Round.ID,
		ctx:       ctx,
		decision:  make(chan Decision, 1),
	}

	manager.mu.Lock()
	manager.pending[request.ApprovalID] = pending
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.pending, request.ApprovalID)
		manager.mu.Unlock()
	}()

	if state.Session.CurrentPermissionMode() == conversation.PermissionYolo {
		return nil
	}
	if err := state.Emit(ctx, agentloop.Event{Type: EventRequested, Iteration: state.Iteration, Payload: request}); err != nil {
		return fmt.Errorf("publish tool approval: %w", err)
	}

	for {
		var decision Decision
		select {
		case <-ctx.Done():
			return ctx.Err()
		case decision = <-pending.decision:
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		if decision == recheck {
			switch state.Session.CurrentPermissionMode() {
			case conversation.PermissionYolo:
				decision = Allow
			case conversation.PermissionAuto:
				autoDecision, err := manager.autoDecision(ctx, state, call, cwd)
				if err := ctx.Err(); err != nil {
					return err
				}
				if err != nil || autoDecision.Decision == Ask {
					request.Message = autoDecision.Message
					if err != nil {
						request.Message = reviewFailureMessage(err)
					}
					if err := state.Emit(ctx, agentloop.Event{Type: EventRequested, Iteration: state.Iteration, Payload: request}); err != nil {
						return fmt.Errorf("publish updated tool approval: %w", err)
					}
					continue
				}
				if autoDecision.Decision == Deny {
					state.Result = deniedResult(call.ID, AutoDeniedMessage+" "+autoDecision.Message)
				}
				decision = autoDecision.Decision
			default:
				continue
			}
		}

		if decision == Deny && state.Result == nil {
			state.Result = deniedResult(call.ID, DeniedMessage)
		}
		return state.Emit(ctx, agentloop.Event{
			Type:      EventResolved,
			Iteration: state.Iteration,
			Payload: Resolution{
				SessionID:  pending.sessionID,
				RoundID:    pending.roundID,
				ApprovalID: request.ApprovalID,
				Decision:   decision,
			},
		})
	}
}

func approvalPreview(
	state *middleware.ToolCallContext,
	call conversation.ToolUseBlock,
	cwd string,
) tools.CallPreview {
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
	return preview
}

func deniedResult(toolUseID, message string) *conversation.ToolResultBlock {
	return &conversation.ToolResultBlock{ToolUseID: toolUseID, IsError: true, Content: conversation.Text(message)}
}

// Resolve consumes one approval without waiting for tool execution.
func (manager *PermissionManager) Resolve(ctx context.Context, resolution Resolution) error {
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
