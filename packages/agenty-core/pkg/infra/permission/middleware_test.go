package permission_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

type autoApprovalRuntime struct {
	automaticallyAllowed bool
	definitions          []modelcall.ToolDefinition
}

func (runtime *autoApprovalRuntime) Definitions() []modelcall.ToolDefinition {
	return runtime.definitions
}

func (*autoApprovalRuntime) ExecuteBatch(context.Context, agentloop.CallContext, []conversation.ToolUseBlock) []conversation.ToolResultBlock {
	return nil
}

func (runtime *autoApprovalRuntime) CanAutoApproveToolCall(agentloop.CallContext, conversation.ToolUseBlock) bool {
	return runtime.automaticallyAllowed
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

func TestMiddlewareAutoSkipsLowRiskToolCalls(t *testing.T) {
	for _, test := range []struct {
		name    string
		call    conversation.ToolUseBlock
		runtime *autoApprovalRuntime
	}{
		{
			name:    "cwd checked builtin",
			call:    conversation.ToolUseBlock{ID: "read", Name: "read_file", Input: []byte(`{"path":"notes.txt"}`)},
			runtime: &autoApprovalRuntime{automaticallyAllowed: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := permission.NewPermissionManager()
			cwd := t.TempDir()
			state := &middleware.ToolCallContext{
				Session: &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionAuto},
				Round:   &conversation.Round{ID: uuid.New(), Cwd: &cwd},
				Call:    &test.call,
				Tools:   test.runtime,
			}
			state.Emit = func(context.Context, agentloop.Event) error {
				t.Fatal("low-risk tool call emitted a manual approval event")
				return nil
			}

			if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			if state.Result != nil {
				t.Fatal("low-risk tool call was denied")
			}
		})
	}
}

func TestMiddlewareAutoReviewsEveryMCPTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"review","choices":[{"message":{"role":"assistant","content":"{\"decision\":\"allow\",\"message\":null}"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	manager := permission.NewPermissionManager()
	cwd := t.TempDir()
	state := &middleware.ToolCallContext{
		Session: &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionAuto},
		Round:   &conversation.Round{ID: uuid.New(), Cwd: &cwd},
		Call: &conversation.ToolUseBlock{
			ID: "mcp-call", Name: "mcp__docs__lookup", Input: []byte(`{"path":".ssh/id_ed25519"}`),
		},
		Tools: &autoApprovalRuntime{automaticallyAllowed: true, definitions: []modelcall.ToolDefinition{{
			Name: "mcp__docs__lookup",
		}}},
		Model: modelcall.ModelCallConfig{
			APIType: modelcall.APIOpenAICompletions, BaseURL: server.URL, APIKey: "test", ModelCode: "test",
		},
	}
	reviews := 0
	state.Emit = func(_ context.Context, event agentloop.Event) error {
		if event.Type == permission.EventReviewStarted {
			reviews++
		}
		if event.Type == permission.EventRequested {
			t.Fatal("MCP call unexpectedly bypassed automatic review")
		}
		return nil
	}

	if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 || state.Result != nil {
		t.Fatalf("review count = %d, result = %#v", reviews, state.Result)
	}
}

func TestMiddlewareAutoReviewLifecycleAndRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
			t.Errorf("review request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{
			"id":"chat-review","object":"chat.completion","created":1,"model":"review-model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"{\"decision\":\"deny\",\"message\":\"The command is not authorized.\"}"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`)
	}))
	t.Cleanup(server.Close)

	manager := permission.NewPermissionManager()
	cwd := t.TempDir()
	state := &middleware.ToolCallContext{
		Session: &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionAuto},
		Round:   &conversation.Round{ID: uuid.New(), Cwd: &cwd},
		Call:    &conversation.ToolUseBlock{ID: "call-review", Name: "shell", Input: []byte(`{"commands":["touch outside"]}`)},
		Model: modelcall.ModelCallConfig{
			BaseURL:   server.URL + "/v1",
			APIType:   modelcall.APIOpenAICompletions,
			APIKey:    "test-key",
			ModelCode: "review-model",
		},
	}
	var eventTypes []agentloop.EventType
	state.Emit = func(_ context.Context, event agentloop.Event) error {
		switch event.Type {
		case permission.EventReviewStarted, permission.EventReviewResolved:
			review, ok := event.Payload.(permission.ReviewEvent)
			if !ok || review.ToolUseID != state.Call.ID {
				t.Fatalf("review payload = %#v", event.Payload)
			}
			eventTypes = append(eventTypes, event.Type)
		case permission.EventRequested:
			t.Fatal("review rejection fell back to manual approval")
		}
		return nil
	}

	if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(eventTypes, []agentloop.EventType{
		permission.EventReviewStarted,
		permission.EventReviewResolved,
	}) {
		t.Fatalf("review events = %v", eventTypes)
	}
	if state.Result == nil || !state.Result.IsError {
		t.Fatalf("review result = %#v", state.Result)
	}
	message, ok := state.Result.Content[0].(conversation.TextBlock)
	if !ok || message.Text != permission.AutoDeniedMessage+" The command is not authorized." {
		t.Fatalf("review rejection message = %#v", state.Result.Content)
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

func TestPermissionModeChangedRechecksPendingApprovalInAutoMode(t *testing.T) {
	manager := permission.NewPermissionManager()
	cwd := t.TempDir()
	session := &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionAsk}
	round := &conversation.Round{ID: uuid.New(), Cwd: &cwd}
	state := &middleware.ToolCallContext{
		Session: session,
		Round:   round,
		Call:    &conversation.ToolUseBlock{ID: "call", Name: "read_file", Input: []byte(`{"path":"notes.txt"}`)},
		Tools:   &autoApprovalRuntime{automaticallyAllowed: true},
	}
	requested := make(chan permission.Request, 1)
	resolved := make(chan permission.Resolution, 1)
	state.Emit = func(_ context.Context, event agentloop.Event) error {
		switch event.Type {
		case permission.EventRequested:
			requested <- event.Payload.(permission.Request)
		case permission.EventResolved:
			resolved <- event.Payload.(permission.Resolution)
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- manager.Middleware().BeforeToolCall(t.Context(), state) }()
	request := <-requested
	if !session.SetPermissionMode(conversation.PermissionAuto, round.ID) {
		t.Fatal("failed to switch session to auto mode")
	}
	if err := manager.PermissionModeChanged(t.Context(), session.ID, conversation.PermissionAuto); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state.Result != nil {
		t.Fatalf("result = %#v", state.Result)
	}
	resolution := <-resolved
	if resolution.ApprovalID != request.ApprovalID || resolution.Decision != permission.Allow {
		t.Fatalf("resolution = %#v", resolution)
	}
}

func TestManualDenialWinsWhileAutomaticRecheckIsRunning(t *testing.T) {
	reviewStarted := make(chan struct{})
	releaseReview := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		select {
		case <-reviewStarted:
		default:
			close(reviewStarted)
		}
		<-releaseReview
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"review","choices":[{"message":{"role":"assistant","content":"{\"decision\":\"allow\",\"message\":null}"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	manager := permission.NewPermissionManager()
	cwd := t.TempDir()
	session := &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionAsk}
	round := &conversation.Round{ID: uuid.New(), Cwd: &cwd}
	state := &middleware.ToolCallContext{
		Session: session,
		Round:   round,
		Call:    &conversation.ToolUseBlock{ID: "call", Name: "shell", Input: []byte(`{"commands":["printf hello"]}`)},
		Model:   modelcall.ModelCallConfig{APIType: modelcall.APIOpenAICompletions, BaseURL: server.URL, APIKey: "test", ModelCode: "test"},
	}
	requested := make(chan permission.Request, 1)
	resolved := make(chan permission.Resolution, 1)
	state.Emit = func(ctx context.Context, event agentloop.Event) error {
		switch event.Type {
		case permission.EventRequested:
			request := event.Payload.(permission.Request)
			requested <- request
			if session.CurrentPermissionMode() == conversation.PermissionAsk {
				if !session.SetPermissionMode(conversation.PermissionAuto, round.ID) {
					t.Fatal("failed to switch to auto mode")
				}
				return manager.PermissionModeChanged(ctx, session.ID, conversation.PermissionAuto)
			}
		case permission.EventResolved:
			resolved <- event.Payload.(permission.Resolution)
		}
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- manager.Middleware().BeforeToolCall(t.Context(), state) }()
	request := <-requested
	<-reviewStarted
	if err := manager.Resolve(t.Context(), permission.Resolution{
		SessionID: session.ID, RoundID: round.ID, ApprovalID: request.ApprovalID, Decision: permission.Deny,
	}); err != nil {
		t.Fatal(err)
	}
	close(releaseReview)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	resolution := <-resolved
	if resolution.Decision != permission.Deny {
		t.Fatalf("resolution = %#v", resolution)
	}
	if state.Result == nil || state.Result.Content[0].(conversation.TextBlock).Text != permission.DeniedMessage {
		t.Fatalf("result = %#v", state.Result)
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

func TestMiddlewareShowsAskAndFailureReasons(t *testing.T) {
	for _, scenario := range []string{"ask", "request error", "manual recheck"} {
		t.Run(scenario, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				if scenario == "request error" {
					w.WriteHeader(401)
					fmt.Fprint(w, `{"error":{"message":"secret must not be displayed"}}`)
					return
				}
				answer := `{"decision":"ask","message":"Confirm discarding uncommitted changes."}`
				fmt.Fprintf(w, `{"id":"review","choices":[{"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, answer)
			}))
			defer server.Close()
			manager := permission.NewPermissionManager()
			cwd := t.TempDir()
			state := &middleware.ToolCallContext{
				Session: &conversation.Session{ID: uuid.New(), PermissionMode: conversation.PermissionAuto},
				Round:   &conversation.Round{ID: uuid.New(), Cwd: &cwd},
				Call:    &conversation.ToolUseBlock{ID: "shell", Name: "shell", Input: []byte(`{"commands":["git reset --hard"]}`)},
				Model:   modelcall.ModelCallConfig{APIType: modelcall.APIOpenAICompletions, BaseURL: server.URL, APIKey: "test", ModelCode: "test"},
			}
			if scenario == "manual recheck" {
				state.Session.PermissionMode = conversation.PermissionAsk
			}
			seen := 0
			state.Emit = func(ctx context.Context, event agentloop.Event) error {
				if event.Type != permission.EventRequested {
					return nil
				}
				request := event.Payload.(permission.Request)
				seen++
				if scenario == "manual recheck" && seen == 1 {
					if request.Message != "" {
						t.Fatal("manual mode unexpectedly has a review reason")
					}
					if !state.Session.SetPermissionMode(conversation.PermissionAuto, state.Round.ID) {
						t.Fatal("failed to switch session to auto mode")
					}
					return manager.PermissionModeChanged(ctx, state.Session.ID, conversation.PermissionAuto)
				}
				if request.Message == "" || strings.Contains(request.Message, "secret must") {
					t.Fatalf("bad approval reason: %q", request.Message)
				}
				if (scenario == "ask" || scenario == "manual recheck") && request.Message != "Confirm discarding uncommitted changes." {
					t.Fatalf("message = %q", request.Message)
				}
				return manager.Resolve(ctx, permission.Resolution{SessionID: state.Session.ID, RoundID: state.Round.ID, ApprovalID: request.ApprovalID, Decision: permission.Allow})
			}
			if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			if seen == 0 || state.Result != nil {
				t.Fatalf("approvals = %d, result = %#v", seen, state.Result)
			}
			if requests != 1 {
				t.Fatalf("requests = %d", requests)
			}
		})
	}
}

func TestInvalidReviewAfterRetryDeniesCurrentCall(t *testing.T) {
	for _, answer := range []string{
		`{"decision":"ask"}`,
		`{"message":"Missing decision."}`,
		`{"decision":true,"message":null}`,
		`{"decision":"allow",`,
	} {
		for _, initialMode := range []conversation.PermissionMode{conversation.PermissionAuto, conversation.PermissionAsk} {
			t.Run(string(initialMode)+"/"+answer, func(t *testing.T) {
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests++
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"id":"review","choices":[{"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, answer)
				}))
				defer server.Close()
				manager := permission.NewPermissionManager()
				cwd := t.TempDir()
				state := &middleware.ToolCallContext{
					Session: &conversation.Session{ID: uuid.New(), PermissionMode: initialMode},
					Round:   &conversation.Round{ID: uuid.New(), Cwd: &cwd},
					Call:    &conversation.ToolUseBlock{ID: "current-call", Name: "shell", Input: []byte(`{"commands":["printf hello"]}`)},
					Model:   modelcall.ModelCallConfig{APIType: modelcall.APIOpenAICompletions, BaseURL: server.URL, APIKey: "test", ModelCode: "test"},
				}
				approvals, reviews, resolutions := 0, 0, 0
				state.Emit = func(ctx context.Context, event agentloop.Event) error {
					switch event.Type {
					case permission.EventRequested:
						approvals++
						if initialMode == conversation.PermissionAuto || approvals > 1 {
							t.Fatal("invalid reviewer output fell back to manual approval")
						}
						state.Session.SetPermissionMode(conversation.PermissionAuto, state.Round.ID)
						return manager.PermissionModeChanged(ctx, state.Session.ID, conversation.PermissionAuto)
					case permission.EventReviewResolved:
						reviews++
					case permission.EventResolved:
						resolutions++
						if event.Payload.(permission.Resolution).Decision != permission.Deny {
							t.Fatal("pending approval was not resolved as deny")
						}
					}
					return nil
				}
				if err := manager.Middleware().BeforeToolCall(t.Context(), state); err != nil {
					t.Fatal(err)
				}
				if requests != 2 || reviews != 1 {
					t.Fatalf("requests = %d, review resolutions = %d", requests, reviews)
				}
				if initialMode == conversation.PermissionAsk && (approvals != 1 || resolutions != 1) {
					t.Fatalf("approvals = %d, resolutions = %d", approvals, resolutions)
				}
				if state.Result == nil || !state.Result.IsError || state.Result.ToolUseID != "current-call" {
					t.Fatalf("call was not rejected: %#v", state.Result)
				}
				message, ok := state.Result.Content[0].(conversation.TextBlock)
				if !ok || message.Text != permission.AutoDeniedMessage+" Auto review failed: invalid output after retry." {
					t.Fatalf("rejection reason = %#v", state.Result.Content)
				}
				if state.Session.CurrentPermissionMode() != conversation.PermissionAuto {
					t.Fatal("changed session permission mode")
				}
			})
		}
	}
}
