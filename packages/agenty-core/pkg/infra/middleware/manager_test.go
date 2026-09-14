package middleware_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

func TestManagerCompilesHooksInRegistrationOrderAndFreezesRegistration(t *testing.T) {
	manager := middleware.NewManager()
	var calls []string
	for _, name := range []string{"first", "second", "third"} {
		name := name
		if err := manager.Register(middleware.Middleware{
			Name: name,
			BeforeModelCall: func(context.Context, *middleware.ModelCallContext) error {
				calls = append(calls, name)
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	chain, err := manager.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(middleware.Middleware{Name: "late"}); err == nil {
		t.Fatal("Register after Compile succeeded")
	}
	if _, err := manager.Compile(); err != nil {
		t.Fatal(err)
	}

	if err := chain.BeforeModelCall(t.Context(), &middleware.ModelCallContext{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"first", "second", "third"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("hook order = %v, want %v", calls, want)
	}
}

func TestManagerAfterHooksRunAllAndJoinErrors(t *testing.T) {
	manager := middleware.NewManager()
	var calls []string
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	for _, name := range []string{"first", "second"} {
		name := name
		if err := manager.Register(middleware.Middleware{
			Name: name,
			AfterRound: func(context.Context, *middleware.RoundContext) error {
				calls = append(calls, name)
				if name == "first" {
					return firstErr
				}
				return secondErr
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	chain, err := manager.Compile()
	if err != nil {
		t.Fatal(err)
	}

	err = chain.AfterRound(t.Context(), &middleware.RoundContext{})
	if err == nil || !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("AfterRound error = %v", err)
	}
	if want := []string{"first", "second"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("after hook order = %v, want %v", calls, want)
	}
}

func TestManagerDeliversEventsInOrderAndStopsAfterFailure(t *testing.T) {
	manager := middleware.NewManager()
	var calls []string
	stop := errors.New("stop event delivery")
	for _, name := range []string{"storage", "notifications", "later"} {
		name := name
		if err := manager.Register(middleware.Middleware{
			Name: name,
			OnEvent: func(context.Context, *middleware.EventContext) error {
				calls = append(calls, name)
				if name == "notifications" {
					return stop
				}
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	chain, err := manager.Compile()
	if err != nil {
		t.Fatal(err)
	}

	err = chain.OnEvent(t.Context(), &middleware.EventContext{
		Event: &agentloop.Event{Type: agentloop.EventRoundStarted},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("OnEvent error = %v, want wrapped %v", err, stop)
	}
	if want := []string{"storage", "notifications"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("event hook order = %v, want %v", calls, want)
	}
}

func TestAgentLoopHooksCanModifyRequestAndToolResult(t *testing.T) {
	tool := &testTool{}
	hooks := agentloop.LoopHooks{
		BeforeModelCall: func(_ context.Context, state *agentloop.ModelCallState) error {
			state.Request.SystemPrompt = "changed"
			return nil
		},
		BeforeToolCall: func(_ context.Context, state *agentloop.ToolCallState) error {
			state.Call.Input = []byte(`{"value":"changed"}`)
			return nil
		},
		AfterToolCall: func(_ context.Context, state *agentloop.ToolCallState) error {
			state.Result.IsError = false
			return nil
		},
	}
	caller := &testCaller{}
	loop, err := agentloop.NewAgentLoop(agentloop.AgentLoopOptions{
		BuildRequest: func(context.Context, int) (modelcall.ModelCallRequest, conversation.TokenUsage, error) {
			return modelcall.ModelCallRequest{SystemPrompt: "original"}, conversation.TokenUsage{}, nil
		},
		InvokeModel: caller.Call,
		ToolRuntime: tool,
		Hooks:       hooks,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if caller.request.SystemPrompt != "changed" {
		t.Fatalf("system prompt = %q, want changed", caller.request.SystemPrompt)
	}
	if string(tool.input) != `{"value":"changed"}` {
		t.Fatalf("tool input = %s, want changed", tool.input)
	}
}

func TestAgentLoopHooksPropagateDerivedContext(t *testing.T) {
	type contextKey string
	const key contextKey = "hook"
	manager := middleware.NewManager()
	if err := manager.Register(middleware.Middleware{
		Name: "context",
		BeforeModelCall: func(_ context.Context, state *middleware.ModelCallContext) error {
			state.Context = context.WithValue(state.Context, key, "derived")
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	chain, err := manager.Compile()
	if err != nil {
		t.Fatal(err)
	}

	var invokedWith context.Context
	loop, err := agentloop.NewAgentLoop(agentloop.AgentLoopOptions{
		BuildRequest: func(context.Context, int) (modelcall.ModelCallRequest, conversation.TokenUsage, error) {
			return modelcall.ModelCallRequest{}, conversation.TokenUsage{}, nil
		},
		InvokeModel: (&contextRecordingCaller{
			invokedWith: &invokedWith,
		}).Call,
		Hooks: chain.AgentLoopHooks(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if invokedWith == nil || invokedWith.Value(key) != "derived" {
		t.Fatalf("model context = %v, want derived value", invokedWith)
	}
}

type testCaller struct {
	request modelcall.ModelCallRequest
	calls   int
}

type contextRecordingCaller struct {
	invokedWith *context.Context
}

func (caller *contextRecordingCaller) Invoke(
	ctx context.Context,
	_ modelcall.ModelCallRequest,
) (*modelcall.ModelCallResponse, error) {
	*caller.invokedWith = ctx
	return &modelcall.ModelCallResponse{
		Content:    conversation.Text("done"),
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}, nil
}

func (caller *contextRecordingCaller) Stream(
	ctx context.Context,
	request modelcall.ModelCallRequest,
	_ modelcall.ModelCallStreamHandler,
) (*modelcall.ModelCallResponse, error) {
	return caller.Invoke(ctx, request)
}

func (caller *contextRecordingCaller) Call(
	ctx context.Context,
	_ modelcall.ModelCallConfig,
	request modelcall.ModelCallRequest,
	handler modelcall.ModelCallStreamHandler,
) (*modelcall.ModelCallResponse, error) {
	if handler == nil {
		return caller.Invoke(ctx, request)
	}
	return caller.Stream(ctx, request, handler)
}

func (caller *testCaller) Invoke(_ context.Context, request modelcall.ModelCallRequest) (*modelcall.ModelCallResponse, error) {
	caller.request = request
	caller.calls++
	if caller.calls == 1 {
		return &modelcall.ModelCallResponse{
			Content:    conversation.Content{conversation.ToolUseBlock{ID: "tool-1", Name: "test", Input: []byte(`{"value":"original"}`)}},
			StopReason: modelcall.ModelCallStopReasonToolUse,
		}, nil
	}
	return &modelcall.ModelCallResponse{Content: conversation.Text("done"), StopReason: modelcall.ModelCallStopReasonEndTurn}, nil
}

func (caller *testCaller) Stream(ctx context.Context, request modelcall.ModelCallRequest, _ modelcall.ModelCallStreamHandler) (*modelcall.ModelCallResponse, error) {
	return caller.Invoke(ctx, request)
}

func (caller *testCaller) Call(
	ctx context.Context,
	_ modelcall.ModelCallConfig,
	request modelcall.ModelCallRequest,
	handler modelcall.ModelCallStreamHandler,
) (*modelcall.ModelCallResponse, error) {
	if handler == nil {
		return caller.Invoke(ctx, request)
	}
	return caller.Stream(ctx, request, handler)
}

type testTool struct {
	input []byte
}

func (tool *testTool) Definition() modelcall.ToolDefinition {
	return modelcall.ToolDefinition{Name: "test", InputSchema: modelcall.JSONSchema{}}
}

func (tool *testTool) Execute(_ context.Context, _ agentloop.CallContext, input []byte) (conversation.Content, error) {
	tool.input = append([]byte(nil), input...)
	return conversation.Text("ok"), nil
}

func (tool *testTool) Definitions() []modelcall.ToolDefinition {
	return []modelcall.ToolDefinition{tool.Definition()}
}

func (tool *testTool) ExecuteBatch(ctx context.Context, callContext agentloop.CallContext, calls []conversation.ToolUseBlock) []conversation.ToolResultBlock {
	results := make([]conversation.ToolResultBlock, len(calls))
	for index, call := range calls {
		content, err := tool.Execute(ctx, callContext, call.Input)
		results[index] = conversation.ToolResultBlock{ToolUseID: call.ID, Content: content, IsError: err != nil}
	}
	return results
}
