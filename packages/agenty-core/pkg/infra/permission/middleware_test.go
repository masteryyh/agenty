package permission_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
)

type recordingRuntime struct {
	calls   []string
	batches int
}

func (*recordingRuntime) Definitions() []modelcall.ToolDefinition { return nil }

func (runtime *recordingRuntime) ExecuteBatch(_ context.Context, _ agentloop.CallContext, calls []conversation.ToolUseBlock) []conversation.ToolResultBlock {
	runtime.batches++
	results := make([]conversation.ToolResultBlock, len(calls))
	for i, call := range calls {
		runtime.calls = append(runtime.calls, call.ID)
		results[i] = conversation.ToolResultBlock{ToolUseID: call.ID, Content: conversation.Text("executed")}
	}
	return results
}

func TestMiddlewareToolDecisions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		decisions []permission.Decision
		wantCalls []string
	}{
		{name: "allow all", decisions: []permission.Decision{permission.Allow, permission.Allow, permission.Allow}, wantCalls: []string{"read", "shell", "patch"}},
		{name: "deny all", decisions: []permission.Decision{permission.Deny, permission.Deny, permission.Deny}},
		{name: "mixed", decisions: []permission.Decision{permission.Deny, permission.Allow, permission.Deny}, wantCalls: []string{"shell"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := permission.NewPermissionManager()
			middlewares := middleware.NewManager()
			if err := middlewares.Register(manager.Middleware()); err != nil {
				t.Fatal(err)
			}
			chain, err := middlewares.Compile()
			if err != nil {
				t.Fatal(err)
			}
			runtime := &recordingRuntime{}
			session := &conversation.Session{ID: uuid.New()}
			round := &conversation.Round{ID: uuid.New()}
			calls := conversation.Content{
				conversation.ToolUseBlock{ID: "read", Name: "read_file", Input: []byte(`{"path":"notes.txt"}`)},
				conversation.ShellCallBlock{CallID: "shell", Commands: []string{"echo hello"}},
				conversation.ApplyPatchCallBlock{CallID: "patch", Source: conversation.ApplyPatchSourceCustom, Patch: "test patch"},
			}
			var results conversation.Content
			requested, resolved, after, invocations := 0, 0, 0, 0
			hooks := chain.AgentLoopHooks()
			hooks.AfterToolCall = func(_ context.Context, state *agentloop.ToolCallState) error {
				after++
				if state.Result == nil || state.Result.ToolUseID != state.Call.ID {
					t.Fatal("missing matching after-hook result")
				}
				return nil
			}
			loop, err := agentloop.NewAgentLoop(agentloop.AgentLoopOptions{
				Session: session, Round: func() *conversation.Round { return round }, Hooks: hooks, ToolRuntime: runtime,
				BuildRequest: func(_ context.Context, iteration int) (modelcall.ModelCallRequest, conversation.TokenUsage, error) {
					if iteration == 2 && len(results) != 3 {
						t.Fatal("continuation is missing tool results")
					}
					return modelcall.ModelCallRequest{}, conversation.TokenUsage{}, nil
				},
				InvokeModel: func(context.Context, modelcall.ModelCallConfig, modelcall.ModelCallRequest, modelcall.ModelCallStreamHandler) (*modelcall.ModelCallResponse, error) {
					invocations++
					if invocations == 1 {
						return &modelcall.ModelCallResponse{Content: calls}, nil
					}
					return &modelcall.ModelCallResponse{Content: conversation.Text("done")}, nil
				},
				Emit: func(ctx context.Context, event agentloop.Event) error {
					switch event.Type {
					case permission.EventRequested:
						if runtime.batches != 0 {
							t.Fatal("tools executed before all approvals")
						}
						request := event.Payload.(permission.Request)
						resolution := permission.Resolution{SessionID: session.ID, RoundID: round.ID, ApprovalID: request.ApprovalID, Decision: tt.decisions[requested]}
						wrongScope := resolution
						wrongScope.SessionID = uuid.New()
						if manager.Resolve(ctx, wrongScope) == nil {
							t.Fatal("accepted another session's decision")
						}
						wrongScope = resolution
						wrongScope.RoundID = uuid.New()
						if manager.Resolve(ctx, wrongScope) == nil {
							t.Fatal("accepted another round's decision")
						}
						if err := manager.Resolve(ctx, resolution); err != nil {
							return err
						}
						if manager.Resolve(ctx, resolution) == nil {
							t.Fatal("accepted duplicate decision")
						}
						requested++
					case permission.EventResolved:
						resolved++
					case agentloop.EventToolResults:
						results = event.ToolResults
					}
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := loop.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(runtime.calls, tt.wantCalls) {
				t.Fatalf("executed %v, want %v", runtime.calls, tt.wantCalls)
			}
			if len(tt.wantCalls) == 0 && runtime.batches != 0 {
				t.Fatal("executed empty batch")
			}
			if requested != 3 || resolved != 3 || after != 3 || invocations != 2 {
				t.Fatalf("counts: %d %d %d %d", requested, resolved, after, invocations)
			}
			for i, id := range []string{"read", "shell", "patch"} {
				result := results[i].(conversation.ToolResultBlock)
				if result.ToolUseID != id || result.IsError != (tt.decisions[i] == permission.Deny) {
					t.Fatalf("result: %+v", result)
				}
				if result.IsError && result.Content[0].(conversation.TextBlock).Text != permission.DeniedMessage {
					t.Fatal("missing denial message")
				}
			}
		})
	}
}

func TestMiddlewareYoloSkipsApproval(t *testing.T) {
	manager := permission.NewPermissionManager()
	session := &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionYolo}
	state := &middleware.ToolCallContext{
		Session: session,
		Round:   &conversation.Round{ID: uuid.New()},
		Call:    &conversation.ToolUseBlock{ID: "call", Name: "read_file", Input: []byte(`{"path":"notes.txt"}`)},
	}
	emitted := 0
	state.Emit = func(context.Context, agentloop.Event) error {
		emitted++
		return nil
	}
	if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if emitted != 0 {
		t.Fatalf("yolo emitted %d approval events", emitted)
	}
	if state.Result != nil {
		t.Fatal("yolo replaced the tool call result")
	}
}

func TestPermissionModeChangedReleasesPendingApproval(t *testing.T) {
	manager := permission.NewPermissionManager()
	session := &conversation.Session{ID: uuid.New()}
	round := &conversation.Round{ID: uuid.New()}
	state := &middleware.ToolCallContext{
		Session: session,
		Round:   round,
		Call:    &conversation.ToolUseBlock{ID: "call", Name: "read_file", Input: []byte(`{"path":"notes.txt"}`)},
	}
	requested := make(chan permission.Request, 1)
	state.Emit = func(_ context.Context, event agentloop.Event) error {
		if request, ok := event.Payload.(permission.Request); ok {
			requested <- request
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- manager.Middleware().BeforeToolCall(t.Context(), state) }()
	request := <-requested
	if err := manager.PermissionModeChanged(t.Context(), session.ID, conversation.PermissionYolo); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state.Result != nil {
		t.Fatal("switching to yolo denied the pending call")
	}
	if manager.Resolve(t.Context(), permission.Resolution{
		SessionID:  session.ID,
		RoundID:    round.ID,
		ApprovalID: request.ApprovalID,
		Decision:   permission.Allow,
	}) == nil {
		t.Fatal("released approval remained resolvable")
	}
}

func TestMiddlewareCancellationAndPublicationFailure(t *testing.T) {
	for _, tt := range []struct {
		name         string
		publishError bool
	}{
		{name: "cancel while waiting"}, {name: "publication failed", publishError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				manager := permission.NewPermissionManager()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				state := &middleware.ToolCallContext{
					Session: &conversation.Session{ID: uuid.New()}, Round: &conversation.Round{ID: uuid.New()},
					Call: &conversation.ToolUseBlock{ID: "call", Name: "mcp_lookup", Input: []byte(`{}`)},
				}
				var request permission.Request
				publishErr := errors.New("connection closed")
				state.Emit = func(_ context.Context, event agentloop.Event) error {
					request = event.Payload.(permission.Request)
					if tt.publishError {
						return publishErr
					}
					return nil
				}
				done := make(chan error, 1)
				go func() { done <- manager.Middleware().BeforeToolCall(ctx, state) }()
				synctest.Wait()
				if request.ApprovalID == uuid.Nil {
					t.Fatal("request not published")
				}
				cancel()
				err := <-done
				want := context.Canceled
				if tt.publishError {
					want = publishErr
				}
				if !errors.Is(err, want) {
					t.Fatalf("error = %v", err)
				}
				if state.Result != nil {
					t.Fatal("cancellation became a denial")
				}
				if manager.Resolve(t.Context(), permission.Resolution{SessionID: state.Session.ID, RoundID: state.Round.ID, ApprovalID: request.ApprovalID, Decision: permission.Allow}) == nil {
					t.Fatal("resolved stale request")
				}
			})
		})
	}
}

func TestConcurrentApprovalsStayIndependent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := permission.NewPermissionManager()
		requests := make(chan permission.Resolution, 2)
		results := make(chan *conversation.ToolResultBlock, 2)
		for range 2 {
			go func() {
				state := &middleware.ToolCallContext{
					Session: &conversation.Session{ID: uuid.New()}, Round: &conversation.Round{ID: uuid.New()},
					Call: &conversation.ToolUseBlock{ID: "same-provider-call-id", Name: "mcp_lookup", Input: []byte(`{}`)},
				}
				state.Emit = func(_ context.Context, event agentloop.Event) error {
					if request, ok := event.Payload.(permission.Request); ok {
						requests <- permission.Resolution{SessionID: state.Session.ID, RoundID: state.Round.ID, ApprovalID: request.ApprovalID}
					}
					return nil
				}
				if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
					t.Error(err)
				}
				results <- state.Result
			}()
		}
		synctest.Wait()
		first, second := <-requests, <-requests
		first.Decision = permission.Deny
		if err := manager.Resolve(t.Context(), first); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if len(results) != 1 {
			t.Fatal("decision resolved more than one waiting call")
		}
		if result := <-results; result == nil || !result.IsError {
			t.Fatal("first call was not denied")
		}
		second.Decision = permission.Allow
		if err := manager.Resolve(t.Context(), second); err != nil {
			t.Fatal(err)
		}
		if result := <-results; result != nil {
			t.Fatal("second call did not retain normal execution")
		}
	})
}

func TestGenericPreviewPreservesLargeNumbers(t *testing.T) {
	manager := permission.NewPermissionManager()
	state := &middleware.ToolCallContext{
		Session: &conversation.Session{ID: uuid.New()}, Round: &conversation.Round{ID: uuid.New()},
		Call: &conversation.ToolUseBlock{ID: "call", Name: "mcp_lookup", Input: []byte(`{"id":9007199254740993}`)},
	}
	state.Emit = func(ctx context.Context, event agentloop.Event) error {
		if request, ok := event.Payload.(permission.Request); ok {
			if !strings.Contains(request.Preview.Detail, "9007199254740993") {
				t.Fatalf("preview rounded tool arguments: %s", request.Preview.Detail)
			}
			return manager.Resolve(ctx, permission.Resolution{
				SessionID: state.Session.ID, RoundID: state.Round.ID, ApprovalID: request.ApprovalID, Decision: permission.Deny,
			})
		}
		return nil
	}
	if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
		t.Fatal(err)
	}
}
