package session_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	infracompaction "github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/metadata"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infrarpc "github.com/masteryyh/agenty-core/pkg/infra/rpc"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	infrastorage "github.com/masteryyh/agenty-core/pkg/infra/storage"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
)

type scriptedCaller struct {
	mu        sync.Mutex
	requests  []modelcall.ModelCallRequest
	responses []*modelcall.ModelCallResponse
	index     int
	started   chan struct{}
	release   <-chan struct{}
}

func (caller *scriptedCaller) Invoke(ctx context.Context, request modelcall.ModelCallRequest) (*modelcall.ModelCallResponse, error) {
	caller.mu.Lock()
	caller.requests = append(caller.requests, request)
	index := caller.index
	caller.index++
	caller.mu.Unlock()

	if caller.started != nil {
		caller.started <- struct{}{}
	}
	if caller.release != nil {
		select {
		case <-caller.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	caller.mu.Lock()
	defer caller.mu.Unlock()
	if index >= len(caller.responses) {
		return nil, errors.New("unexpected LLM invocation")
	}
	response := *caller.responses[index]
	response.Content = append(conversation.Content{}, response.Content...)
	return &response, nil
}

func (caller *scriptedCaller) Stream(
	ctx context.Context,
	request modelcall.ModelCallRequest,
	handler modelcall.ModelCallStreamHandler,
) (*modelcall.ModelCallResponse, error) {
	response, err := caller.Invoke(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := handler(modelcall.ModelCallStreamEvent{
		Type:  modelcall.ModelCallStreamEventTextDelta,
		Index: 0,
		Delta: "streamed",
	}); err != nil {
		return nil, err
	}
	if err := handler(modelcall.ModelCallStreamEvent{
		Type:     modelcall.ModelCallStreamEventCompleted,
		Response: response,
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (caller *scriptedCaller) Call(
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

func (caller *scriptedCaller) Requests() []modelcall.ModelCallRequest {
	caller.mu.Lock()
	defer caller.mu.Unlock()

	return append([]modelcall.ModelCallRequest{}, caller.requests...)
}

type executionTestTool struct {
	definition modelcall.ToolDefinition
	execute    func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error)
}

func (tool *executionTestTool) Definition() modelcall.ToolDefinition {
	return tool.definition
}

func (tool *executionTestTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	return tool.execute(ctx, callContext, input)
}

type executionFixture struct {
	catalog  *providerRepositoryFake
	sessions *sessionRepositoryFake
	registry *infratools.Registry
}

func newExecutionFixture(t *testing.T, maxOutputTokens int64) *executionFixture {
	t.Helper()

	catalogRepository := newProviderRepositoryFake()
	sessions := newSessionRepositoryFake()
	registry := infratools.NewRegistry()

	provider, err := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	provider.APIKey = "test-key"
	provider.AddModel(catalog.Model{
		Code:            "gpt-5",
		Name:            "GPT-5",
		ContextWindow:   128_000,
		MaxOutputTokens: maxOutputTokens,
	})
	if err := catalogRepository.Save(t.Context(), provider); err != nil {
		t.Fatal(err)
	}

	return &executionFixture{
		catalog:  catalogRepository,
		sessions: sessions,
		registry: registry,
	}
}

func (fixture *executionFixture) createSession(t *testing.T) *conversation.Session {
	t.Helper()

	session := conversation.StartSession(
		shared.NewModelRef("openai", "gpt-5"),
		128_000,
		shared.ReasoningOff,
		new("/workspace"),
	)
	if err := fixture.sessions.Save(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()
	return session
}

func (fixture *executionFixture) newEngine(
	t *testing.T,
	invokeModel modelcall.InvokeFunc,
) *infrasession.Engine {
	return fixture.newEngineWithEvents(t, invokeModel, nil)
}

func (fixture *executionFixture) newEngineWithEvents(
	t *testing.T,
	invokeModel modelcall.InvokeFunc,
	events func(context.Context, infrarpc.SessionEvent) error,
) *infrasession.Engine {
	return fixture.newEngineWithHandlers(t, invokeModel, events, nil)
}

func (fixture *executionFixture) newEngineWithHandlers(
	t *testing.T,
	invokeModel modelcall.InvokeFunc,
	events func(context.Context, infrarpc.SessionEvent) error,
	compactions func(context.Context, infracompaction.Event) error,
) *infrasession.Engine {
	t.Helper()

	middlewareManager := inframiddleware.NewManager()
	if err := middlewareManager.Register(metadata.NewMiddleware()); err != nil {
		t.Fatal(err)
	}
	if err := middlewareManager.Register(infracompaction.NewMiddleware()); err != nil {
		t.Fatal(err)
	}
	if err := middlewareManager.Register(infrastorage.NewSessionMiddleware(fixture.sessions)); err != nil {
		t.Fatal(err)
	}
	if err := middlewareManager.Register(infrarpc.NewSessionNotificationMiddleware(
		func(ctx context.Context, method string, payload any) error {
			switch method {
			case "session.event":
				if events == nil {
					return nil
				}
				event, ok := payload.(infrarpc.SessionEvent)
				if !ok {
					return errors.New("unexpected session event payload")
				}
				return events(ctx, event)
			case "session.compaction":
				if compactions == nil {
					return nil
				}
				event, ok := payload.(infracompaction.Event)
				if !ok {
					return errors.New("unexpected compaction event payload")
				}
				return compactions(ctx, event)
			default:
				return errors.New("unexpected notification method")
			}
		},
	)); err != nil {
		t.Fatal(err)
	}
	middlewareChain, err := middlewareManager.Compile()
	if err != nil {
		t.Fatal(err)
	}
	engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
		Sessions:    fixture.sessions,
		Catalog:     fixture.catalog,
		Tools:       fixture.registry,
		InvokeModel: invokeModel,
		LoopHooks:   middlewareChain.AgentLoopHooks(),
		Lifecycle:   middlewareChain.LifecycleHooks(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := engine.Shutdown(shutdownCtx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	return engine
}

func TestEngineStreamsOrderedSessionEvents(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content:    conversation.Text("done"),
		Usage:      conversation.TokenUsage{Input: 2, Output: 3, Total: 5},
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}}}
	events := make(chan infrarpc.SessionEvent, 16)
	engine := fixture.newEngineWithEvents(
		t,
		caller.Call,
		func(_ context.Context, event infrarpc.SessionEvent) error {
			events <- event
			return nil
		},
	)
	session := fixture.createSession(t)
	started, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello"))
	if err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	got := make([]infrarpc.SessionEvent, 0, 6)
	for len(got) < 6 {
		select {
		case event := <-events:
			got = append(got, event)
		case <-time.After(time.Second):
			t.Fatalf("received %d events, want 6", len(got))
		}
	}
	wantTypes := []infrarpc.SessionEventType{
		infrarpc.SessionEventRoundStarted,
		infrarpc.SessionEventMessageAppended,
		infrarpc.SessionEventModelStream,
		infrarpc.SessionEventModelStream,
		infrarpc.SessionEventMessageAppended,
		infrarpc.SessionEventRoundEnded,
	}
	for i, event := range got {
		if event.Type != wantTypes[i] {
			t.Errorf("event %d type = %q, want %q", i, event.Type, wantTypes[i])
		}
		if event.Sequence != uint64(i+1) {
			t.Errorf("event %d sequence = %d, want %d", i, event.Sequence, i+1)
		}
		if event.SessionID != session.ID || event.RoundID != started.RoundID {
			t.Errorf("event %d scope = %s/%s", i, event.SessionID, event.RoundID)
		}
	}
	if got[2].Stream == nil || got[2].Stream.Delta != "streamed" {
		t.Fatalf("text delta event = %+v", got[2])
	}
	if got[1].Message == nil || got[1].Message.IsHidden() {
		t.Fatalf("user event exposed hidden metadata = %+v", got[1])
	}
	if got[5].Status != conversation.RoundCompleted || got[5].Usage == nil || got[5].Usage.Total != 5 {
		t.Fatalf("terminal event = %+v", got[5])
	}
}

func TestEngineCompletesRoundWhenInitialNotificationFails(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content:    conversation.Text("done"),
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}}}
	engine := fixture.newEngineWithEvents(
		t,
		caller.Call,
		func(_ context.Context, event infrarpc.SessionEvent) error {
			if event.Type == infrarpc.SessionEventRoundStarted {
				return errors.New("client disconnected")
			}
			return nil
		},
	)
	session := fixture.createSession(t)
	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForExecution(t, engine, session.ID)

	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Rounds) != 1 || loaded.Rounds[0].Status != conversation.RoundFailed {
		t.Fatalf("persisted rounds = %+v", loaded.Rounds)
	}
}

func TestEngineCompletesToolLoopAndPersistsRound(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 100_000)
	var toolContext agentloop.CallContext
	if err := fixture.registry.Register(&executionTestTool{
		definition: modelcall.ToolDefinition{
			Name: "lookup",
			InputSchema: modelcall.JSONSchema{
				Type: modelcall.JSONSchemaTypeObject,
			},
		},
		execute: func(_ context.Context, callContext agentloop.CallContext, input []byte) (conversation.Content, error) {
			toolContext = callContext
			if string(input) != `{"query":"agenty"}` {
				t.Errorf("tool input = %s", input)
			}
			return conversation.Text("tool result"), nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{
		{
			Content: conversation.Content{
				conversation.TextBlock{Text: "checking"},
				conversation.ToolUseBlock{ID: "call-1", Name: "lookup", Input: []byte(`{"query":"agenty"}`)},
			},
			Usage:      conversation.TokenUsage{Input: 10, Output: 3, Total: 13},
			StopReason: modelcall.ModelCallStopReasonToolUse,
		},
		{
			Content:    conversation.Text("done"),
			Usage:      conversation.TokenUsage{Input: 20, Output: 4, Total: 24},
			StopReason: modelcall.ModelCallStopReasonEndTurn,
		},
	}}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)

	started, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello"))
	if err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != conversation.RoundRunning || len(loaded.Rounds) != 1 {
		t.Fatalf("started = %+v, rounds = %+v", started, loaded.Rounds)
	}
	round := loaded.Rounds[0]
	if round.Status != conversation.RoundCompleted || len(round.Messages) != 5 {
		t.Fatalf("round = %+v", round)
	}
	if round.Usage != (conversation.TokenUsage{Input: 30, Output: 7, Total: 37}) {
		t.Errorf("round usage = %+v", round.Usage)
	}
	if toolContext.SessionID != session.ID || toolContext.RoundID != round.ID || toolContext.Cwd != "/workspace" {
		t.Errorf("tool context = %+v", toolContext)
	}

	requests := caller.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if requests[0].MaxOutputTokens != 100_000 {
		t.Errorf("max output tokens = %d, want %d", requests[0].MaxOutputTokens, 100_000)
	}
	if len(requests[0].Messages) != 2 || len(requests[1].Messages) != 4 {
		t.Fatalf("request message counts = %d, %d", len(requests[0].Messages), len(requests[1].Messages))
	}
	if requests[0].Messages[0].Role != conversation.RoleDeveloper || requests[0].Messages[1].Role != conversation.RoleUser {
		t.Errorf("first request message roles = %+v", requests[0].Messages[:2])
	}
	metadataText, ok := requests[0].Messages[0].Content[0].(conversation.TextBlock)
	if !ok || !strings.Contains(metadataText.Text, "<model>gpt-5</model>") {
		t.Errorf("first metadata message = %+v", requests[0].Messages[0])
	}
	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Name != "lookup" {
		t.Errorf("request tools = %+v", requests[0].Tools)
	}
	if !strings.Contains(requests[0].SystemPrompt, "<basic>") {
		t.Errorf("system prompt = %q", requests[0].SystemPrompt)
	}
	toolResults := toolResultBlocks(round.Messages[3].Content)
	if len(toolResults) != 1 || toolResults[0].ToolUseID != "call-1" || toolResults[0].IsError {
		t.Errorf("tool results = %+v", toolResults)
	}
}

func TestEngineUsesGlobalModelOutputLimit(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content:    conversation.Text("done"),
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}}}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)

	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	requests := caller.Requests()
	if len(requests) != 1 || requests[0].MaxOutputTokens != infrasession.DefaultMaxOutputTokens {
		t.Fatalf("requests = %+v, want maxOutputTokens %d", requests, infrasession.DefaultMaxOutputTokens)
	}
}

func TestEngineProjectsApplyPatchByProviderCapability(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name            string
		freeFormTool    bool
		wantApplyPatch  bool
		wantShellPrompt bool
	}{
		{name: "free-form provider", freeFormTool: true, wantApplyPatch: true},
		{name: "shell fallback", wantShellPrompt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newExecutionFixture(t, 8_192)
			provider, err := fixture.catalog.Get(t.Context(), "openai")
			if err != nil {
				t.Fatal(err)
			}
			provider.FreeFormTool = test.freeFormTool
			if err := fixture.catalog.Save(t.Context(), provider); err != nil {
				t.Fatal(err)
			}
			if err := fixture.registry.Register(&executionTestTool{
				definition: modelcall.ToolDefinition{
					Type: modelcall.ToolTypeApplyPatch,
					Name: "apply_patch",
				},
			}); err != nil {
				t.Fatal(err)
			}

			caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
				Content:    conversation.Text("done"),
				StopReason: modelcall.ModelCallStopReasonEndTurn,
			}}}
			engine := fixture.newEngine(t, caller.Call)
			session := fixture.createSession(t)
			if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("edit")); err != nil {
				t.Fatal(err)
			}
			waitForExecution(t, engine, session.ID)

			requests := caller.Requests()
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			gotApplyPatch := len(requests[0].Tools) == 1 && requests[0].Tools[0].Name == "apply_patch"
			if gotApplyPatch != test.wantApplyPatch {
				t.Errorf("apply_patch registered = %v, want %v", gotApplyPatch, test.wantApplyPatch)
			}
			gotShellPrompt := strings.Contains(requests[0].SystemPrompt, "shell tool with one complete apply_patch command")
			if gotShellPrompt != test.wantShellPrompt {
				t.Errorf("shell fallback prompt present = %v, want %v", gotShellPrompt, test.wantShellPrompt)
			}
		})
	}
}

func TestEngineCompactsAutomaticallyAndPreservesTranscript(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	provider, err := fixture.catalog.Get(t.Context(), "openai")
	if err != nil {
		t.Fatal(err)
	}
	model, err := provider.Model("gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	model.ContextWindow = 100
	if err := fixture.catalog.Save(t.Context(), provider); err != nil {
		t.Fatal(err)
	}
	compactionEvents := make(chan infracompaction.Event, 2)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{
		{
			Content: conversation.Text("Task goals: finish the task\nCompleted: initial work\nIncomplete: follow up"),
			Usage:   conversation.TokenUsage{Input: 10, Output: 20, Total: 30},
		},
		{
			Content:    conversation.Text("continued"),
			Usage:      conversation.TokenUsage{Input: 3, Output: 2, Total: 5},
			StopReason: modelcall.ModelCallStopReasonEndTurn,
		},
	}}
	engine := fixture.newEngineWithHandlers(
		t,
		caller.Call,
		nil,
		func(_ context.Context, event infracompaction.Event) error {
			compactionEvents <- event
			return nil
		},
	)
	session := fixture.createSession(t)
	session.SetModel(*session.CurrentModel, 100)
	if err := fixture.sessions.Save(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()

	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("finish the task")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Rounds) != 1 || len(loaded.Rounds[0].Messages) != 3 {
		t.Fatalf("raw transcript rounds/messages = %d/%d", len(loaded.Rounds), len(loaded.Rounds[0].Messages))
	}
	if len(loaded.ContextMessages()) != 4 {
		t.Fatalf("effective context messages = %d, want retained user, summary, metadata, and response", len(loaded.ContextMessages()))
	}
	requests := caller.Requests()
	if len(requests) != 2 {
		t.Fatalf("LLM requests = %d, want compaction plus normal", len(requests))
	}
	if !strings.Contains(requests[0].SystemPrompt, "<basic>") {
		t.Errorf("compaction system prompt = %q", requests[0].SystemPrompt)
	}
	compactionPromptBlock, ok := requests[0].Messages[2].Content[0].(conversation.TextBlock)
	if !ok || len(requests[0].Messages) != 3 || !strings.Contains(compactionPromptBlock.Text, "session-compaction-request") {
		t.Fatalf("compaction request messages = %+v", requests[0].Messages)
	}
	if len(requests[0].Tools) != 0 {
		t.Fatalf("compaction request tools = %+v", requests[0].Tools)
	}
	if requests[0].MaxOutputTokens != infrasession.DefaultMaxOutputTokens || requests[1].MaxOutputTokens != infrasession.DefaultMaxOutputTokens {
		t.Errorf("max output tokens = %d/%d", requests[0].MaxOutputTokens, requests[1].MaxOutputTokens)
	}
	if len(requests[1].Messages) != 2 {
		t.Fatalf("post-compaction message count = %d", len(requests[1].Messages))
	}
	started := <-compactionEvents
	completed := <-compactionEvents
	if started.Type != infracompaction.EventStarted || completed.Type != infracompaction.EventCompleted || started.CompactionID != completed.CompactionID {
		t.Errorf("compaction events = %+v, %+v", started, completed)
	}
}

func TestEngineCompactsManually(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content: conversation.Text("manual summary"),
		Usage:   conversation.TokenUsage{Input: 4, Output: 5, Total: 9},
	}}}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, conversation.Text("manual goal")); err != nil {
		t.Fatal(err)
	}
	if err := fixture.sessions.Save(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()

	result, err := engine.Compact(t.Context(), session.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if result.Trigger != conversation.CompactionTriggerManual || result.Usage.Total != 9 {
		t.Fatalf("compact result = %+v", result)
	}
	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Rounds[0].Messages) != 1 || len(loaded.ContextMessages()) != 2 {
		t.Fatalf("manual compact transcript/context = %d/%d", len(loaded.Rounds[0].Messages), len(loaded.ContextMessages()))
	}
}

func TestManualCompactionRejectsToolCalls(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content: conversation.Content{
			conversation.ToolUseBlock{ID: "compact-call", Name: "lookup", Input: []byte(`{"query":"session"}`)},
		},
		Usage:      conversation.TokenUsage{Input: 10, Output: 2, Total: 12},
		StopReason: modelcall.ModelCallStopReasonToolUse,
	}}}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, conversation.Text("manual goal")); err != nil {
		t.Fatal(err)
	}
	if err := fixture.sessions.Save(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()

	if _, err := engine.Compact(t.Context(), session.ID.String()); err == nil {
		t.Fatal("tool request compaction succeeded")
	}
	requests := caller.Requests()
	if len(requests) != 1 {
		t.Fatalf("compaction requests = %d, want 1", len(requests))
	}
	if len(requests[0].Tools) != 0 {
		t.Fatalf("compaction request tools = %+v", requests[0].Tools)
	}

	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Rounds[0].Messages) != 1 {
		t.Fatalf("raw transcript messages = %d, want 1", len(loaded.Rounds[0].Messages))
	}
	for _, event := range fixture.sessions.events[session.ID] {
		if _, ok := event.(conversation.SessionCompacted); ok {
			t.Fatalf("failed compaction was persisted: %+v", event)
		}
	}
}

func TestModelSwitchCompactsWithCurrentModelBeforePersistingTarget(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	provider, err := fixture.catalog.Get(t.Context(), "openai")
	if err != nil {
		t.Fatal(err)
	}
	provider.AddModel(catalog.Model{
		Code:            "small-model",
		Name:            "Small Model",
		ContextWindow:   4_000,
		MaxOutputTokens: 1_024,
	})
	if err := fixture.catalog.Save(t.Context(), provider); err != nil {
		t.Fatal(err)
	}
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content:    conversation.Text("model switch summary"),
		Usage:      conversation.TokenUsage{Input: 10, Output: 5, Total: 15},
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}}}
	engine := fixture.newEngine(t, func(
		ctx context.Context,
		model modelcall.ModelCallConfig,
		request modelcall.ModelCallRequest,
		handler modelcall.ModelCallStreamHandler,
	) (*modelcall.ModelCallResponse, error) {
		if model.ModelCode != "gpt-5" {
			t.Fatalf("compression used target model %q", model.ModelCode)
		}
		return caller.Call(ctx, model, request, handler)
	})
	session := fixture.createSession(t)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, conversation.Text(strings.Repeat("important context ", 1_000))); err != nil {
		t.Fatal(err)
	}
	if err := fixture.sessions.Save(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()

	updated, err := engine.SetModel(t.Context(), session.ID.String(), "openai", "small-model")
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentModel == nil || updated.CurrentModel.ModelCode != "small-model" || updated.ContextWindow != 4_000 {
		t.Fatalf("updated session model = %+v, context window = %d", updated.CurrentModel, updated.ContextWindow)
	}
	if len(caller.Requests()) != 1 {
		t.Fatalf("model switch LLM requests = %d, want 1 compaction request", len(caller.Requests()))
	}
	if caller.Requests()[0].MaxOutputTokens != 8_192 {
		t.Fatalf("model switch compaction max output = %d, want 8192", caller.Requests()[0].MaxOutputTokens)
	}

	events := fixture.sessions.events[session.ID]
	compactedIndex := -1
	modelSetIndex := -1
	for index, event := range events {
		switch event.(type) {
		case conversation.SessionCompacted:
			compactedIndex = index
		case conversation.SessionModelSet:
			modelSetIndex = index
		}
	}
	if compactedIndex < 0 || modelSetIndex < 0 || compactedIndex >= modelSetIndex {
		t.Fatalf("event order = %+v", events)
	}
}

func TestEngineAppendsOnlyChangedMetadataAfterTheFirstRound(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{
		{Content: conversation.Text("first"), StopReason: modelcall.ModelCallStopReasonEndTurn},
		{Content: conversation.Text("second"), StopReason: modelcall.ModelCallStopReasonEndTurn},
	}}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)

	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("first input")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	updated, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated.SetCwd(new("/workspace/two"))
	updated.SetReasoningEffort(shared.ReasoningMax)
	if err := fixture.sessions.Save(t.Context(), updated); err != nil {
		t.Fatal(err)
	}
	updated.ClearPending()

	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("second input")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	requests := caller.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if len(requests[1].Messages) != 5 {
		t.Fatalf("second request message count = %d, want 5", len(requests[1].Messages))
	}
	metadata, ok := requests[1].Messages[3].Content[0].(conversation.TextBlock)
	if !ok {
		t.Fatalf("second metadata message = %+v", requests[1].Messages[3])
	}
	if !strings.Contains(metadata.Text, "<cwd>/workspace/two</cwd>") ||
		!strings.Contains(metadata.Text, "<reasoning-effort>max</reasoning-effort>") {
		t.Errorf("second metadata = %q", metadata.Text)
	}
	for _, unchanged := range []string{"<model>", "<provider>", "<timezone>"} {
		if strings.Contains(metadata.Text, unchanged) {
			t.Errorf("second metadata unexpectedly contains %q: %q", unchanged, metadata.Text)
		}
	}
}

func TestEngineRunsMultipleSessionsInParallel(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	release := make(chan struct{})
	caller := &scriptedCaller{
		responses: []*modelcall.ModelCallResponse{
			{Content: conversation.Text("one"), StopReason: modelcall.ModelCallStopReasonEndTurn},
			{Content: conversation.Text("two"), StopReason: modelcall.ModelCallStopReasonEndTurn},
			{Content: conversation.Text("three"), StopReason: modelcall.ModelCallStopReasonEndTurn},
		},
		started: make(chan struct{}, 3),
		release: release,
	}
	engine := fixture.newEngine(t, caller.Call)

	sessions := []*conversation.Session{
		fixture.createSession(t),
		fixture.createSession(t),
		fixture.createSession(t),
	}
	for _, session := range sessions {
		if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello")); err != nil {
			t.Fatal(err)
		}
	}
	for range sessions {
		select {
		case <-caller.started:
		case <-time.After(time.Second):
			t.Fatal("sessions did not reach the LLM caller concurrently")
		}
	}
	close(release)

	for _, session := range sessions {
		waitForExecution(t, engine, session.ID)
		loaded, err := fixture.sessions.Load(t.Context(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Rounds[0].Status != conversation.RoundCompleted {
			t.Errorf("session %s round = %+v", session.ID, loaded.Rounds[0])
		}
	}
}

func TestEngineRejectsDuplicateAndCancelsTargetSession(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{
		responses: []*modelcall.ModelCallResponse{{Content: conversation.Text("done")}},
		started:   make(chan struct{}, 1),
		release:   make(chan struct{}),
	}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)

	started, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello"))
	if err != nil {
		t.Fatal(err)
	}
	<-caller.started
	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("again")); appErrorCode(err) != application.CodeAlreadyExists {
		t.Fatalf("duplicate start error = %v, want already exists", err)
	}

	sessionService := application.NewSessionService(
		fixture.sessions,
		application.WithSessionExecutionState(engine),
	)
	if err := sessionService.Delete(t.Context(), session.ID.String()); appErrorCode(err) != application.CodeAlreadyExists {
		t.Fatalf("delete running session error = %v, want already exists", err)
	}

	stopped, err := engine.Stop(t.Context(), session.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if stopped.RoundID != started.RoundID || !stopped.StopRequested {
		t.Errorf("stop result = %+v", stopped)
	}
	waitForExecution(t, engine, session.ID)

	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Rounds[0].Status != conversation.RoundCancelled || loaded.Rounds[0].Error != nil {
		t.Errorf("cancelled round = %+v", loaded.Rounds[0])
	}
	if _, err := engine.Stop(t.Context(), session.ID.String()); appErrorCode(err) != application.CodeNotFound {
		t.Errorf("second stop error = %v, want not found", err)
	}
}

func TestEngineSerializesSessionDeleteWithStart(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content:    conversation.Text("done"),
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}}}
	engine := fixture.newEngine(t, caller.Call)
	session := fixture.createSession(t)
	fixture.sessions.loadAttempted = make(chan struct{})
	fixture.sessions.deleteStarted = make(chan struct{})
	fixture.sessions.deleteRelease = make(chan struct{})
	sessionService := application.NewSessionService(
		fixture.sessions,
		application.WithSessionExecutionState(engine),
	)

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- sessionService.Delete(t.Context(), session.ID.String())
	}()
	select {
	case <-fixture.sessions.deleteStarted:
	case <-time.After(time.Second):
		t.Fatal("session delete did not start")
	}

	startResult := make(chan error, 1)
	go func() {
		_, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello"))
		startResult <- err
	}()
	select {
	case <-fixture.sessions.loadAttempted:
		t.Fatal("session start reached repository load before delete completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(fixture.sessions.deleteRelease)
	if err := <-deleteResult; err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.sessions.loadAttempted:
	case <-time.After(time.Second):
		t.Fatal("session start did not resume after delete completed")
	}
	if err := <-startResult; appErrorCode(err) != application.CodeNotFound {
		t.Fatalf("start after delete error = %v, want not found", err)
	}
}

func TestEngineShutdownCancelsAllSessions(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{
		responses: []*modelcall.ModelCallResponse{
			{Content: conversation.Text("one")},
			{Content: conversation.Text("two")},
		},
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	engine := fixture.newEngine(t, caller.Call)
	sessions := []*conversation.Session{fixture.createSession(t), fixture.createSession(t)}
	for _, session := range sessions {
		if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello")); err != nil {
			t.Fatal(err)
		}
	}
	for range sessions {
		<-caller.started
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := engine.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		loaded, err := fixture.sessions.Load(t.Context(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Rounds[0].Status != conversation.RoundCancelled {
			t.Errorf("session %s round = %+v", session.ID, loaded.Rounds[0])
		}
	}
}

func waitForExecution(t *testing.T, engine *infrasession.Engine, sessionID uuid.UUID) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for engine.IsRunning(sessionID) {
		if time.Now().After(deadline) {
			t.Fatalf("session %s did not finish", sessionID)
		}
		time.Sleep(time.Millisecond)
	}
}

func toolResultBlocks(content conversation.Content) []conversation.ToolResultBlock {
	results := make([]conversation.ToolResultBlock, 0)
	for _, block := range content {
		result, ok := block.(conversation.ToolResultBlock)
		if ok {
			results = append(results, result)
		}
	}

	return results
}
