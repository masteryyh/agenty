package agentloop_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type scriptedCaller struct {
	mu        sync.Mutex
	requests  []agentloop.Request
	responses []*agentloop.Response
	index     int
	started   chan struct{}
	release   <-chan struct{}
}

func (caller *scriptedCaller) Invoke(ctx context.Context, request agentloop.Request) (*agentloop.Response, error) {
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
	context.Context,
	agentloop.Request,
	agentloop.StreamHandler,
) (*agentloop.Response, error) {
	return nil, errors.New("unexpected stream invocation")
}

func (caller *scriptedCaller) Requests() []agentloop.Request {
	caller.mu.Lock()
	defer caller.mu.Unlock()

	return append([]agentloop.Request{}, caller.requests...)
}

type executionTestTool struct {
	definition agentloop.ToolDefinition
	execute    func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error)
}

func (tool *executionTestTool) Definition() agentloop.ToolDefinition {
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
	agents   *agentRepositoryFake
	catalog  *providerRepositoryFake
	sessions *sessionRepositoryFake
	registry *agentloop.Registry
}

func newExecutionFixture(t *testing.T, maxOutputTokens int64) *executionFixture {
	t.Helper()

	agents := newAgentRepositoryFake()
	catalogRepository := newProviderRepositoryFake()
	sessions := newSessionRepositoryFake()
	registry := agentloop.NewRegistry()

	agentDefinition, err := agent.New("coder", "Code Assistant")
	if err != nil {
		t.Fatal(err)
	}
	agentDefinition.Soul = "Be precise."
	if err := agents.Save(t.Context(), agentDefinition); err != nil {
		t.Fatal(err)
	}

	provider, err := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	provider.APIKey = "test-key"
	provider.AddModel(catalog.Model{
		Slug:            "gpt-5",
		Name:            "GPT-5",
		ContextWindow:   128_000,
		MaxOutputTokens: maxOutputTokens,
	})
	if err := catalogRepository.Save(t.Context(), provider); err != nil {
		t.Fatal(err)
	}

	return &executionFixture{
		agents:   agents,
		catalog:  catalogRepository,
		sessions: sessions,
		registry: registry,
	}
}

func (fixture *executionFixture) createSession(t *testing.T) *conversation.Session {
	t.Helper()

	session := conversation.StartSession(
		"coder",
		shared.NewModelRef("openai", "gpt-5"),
		128_000,
		shared.ReasoningOff,
		ptr("/workspace"),
	)
	if err := fixture.sessions.Save(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()
	return session
}

func (fixture *executionFixture) newEngine(
	t *testing.T,
	callerFactory agentloop.CallerFactory,
) *agentloop.Engine {
	t.Helper()

	engine, err := agentloop.NewEngine(t.Context(), agentloop.Dependencies{
		Sessions:  fixture.sessions,
		Agents:    fixture.agents,
		Catalog:   fixture.catalog,
		Tools:     fixture.registry,
		NewCaller: callerFactory,
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

func TestEngineCompletesToolLoopAndPersistsRound(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 100_000)
	var toolContext agentloop.CallContext
	if err := fixture.registry.Register(&executionTestTool{
		definition: agentloop.ToolDefinition{
			Name: "lookup",
			InputSchema: agentloop.JSONSchema{
				Type: agentloop.JSONSchemaTypeObject,
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

	caller := &scriptedCaller{responses: []*agentloop.Response{
		{
			Content: conversation.Content{
				conversation.TextBlock{Text: "checking"},
				conversation.ToolUseBlock{ID: "call-1", Name: "lookup", Input: []byte(`{"query":"agenty"}`)},
			},
			Usage:      conversation.TokenUsage{Input: 10, Output: 3, Total: 13},
			StopReason: agentloop.StopReasonToolUse,
		},
		{
			Content:    conversation.Text("done"),
			Usage:      conversation.TokenUsage{Input: 20, Output: 4, Total: 24},
			StopReason: agentloop.StopReasonEndTurn,
		},
	}}
	engine := fixture.newEngine(t, func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
		return caller, nil
	})
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
	if round.Status != conversation.RoundCompleted || len(round.Messages) != 4 {
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
	if requests[0].MaxOutputTokens != agentloop.DefaultMaxOutputTokens {
		t.Errorf("max output tokens = %d, want %d", requests[0].MaxOutputTokens, agentloop.DefaultMaxOutputTokens)
	}
	if len(requests[0].Messages) != 1 || len(requests[1].Messages) != 3 {
		t.Errorf("request message counts = %d, %d", len(requests[0].Messages), len(requests[1].Messages))
	}
	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Name != "lookup" {
		t.Errorf("request tools = %+v", requests[0].Tools)
	}
	if !strings.Contains(requests[0].SystemPrompt, "Be precise.") {
		t.Errorf("system prompt does not contain soul: %q", requests[0].SystemPrompt)
	}
	toolResults := toolResultBlocks(round.Messages[2].Content)
	if len(toolResults) != 1 || toolResults[0].ToolUseID != "call-1" || toolResults[0].IsError {
		t.Errorf("tool results = %+v", toolResults)
	}
}

func TestEngineUsesSmallerModelOutputLimit(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*agentloop.Response{{
		Content:    conversation.Text("done"),
		StopReason: agentloop.StopReasonEndTurn,
	}}}
	engine := fixture.newEngine(t, func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
		return caller, nil
	})
	session := fixture.createSession(t)

	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)

	requests := caller.Requests()
	if len(requests) != 1 || requests[0].MaxOutputTokens != 8_192 {
		t.Fatalf("requests = %+v, want maxOutputTokens 8192", requests)
	}
}

func TestEngineRunsMultipleSessionsInParallel(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8_192)
	release := make(chan struct{})
	caller := &scriptedCaller{
		responses: []*agentloop.Response{
			{Content: conversation.Text("one"), StopReason: agentloop.StopReasonEndTurn},
			{Content: conversation.Text("two"), StopReason: agentloop.StopReasonEndTurn},
			{Content: conversation.Text("three"), StopReason: agentloop.StopReasonEndTurn},
		},
		started: make(chan struct{}, 3),
		release: release,
	}
	engine := fixture.newEngine(t, func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
		return caller, nil
	})

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
		responses: []*agentloop.Response{{Content: conversation.Text("done")}},
		started:   make(chan struct{}, 1),
		release:   make(chan struct{}),
	}
	engine := fixture.newEngine(t, func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
		return caller, nil
	})
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
	caller := &scriptedCaller{responses: []*agentloop.Response{{
		Content:    conversation.Text("done"),
		StopReason: agentloop.StopReasonEndTurn,
	}}}
	engine := fixture.newEngine(t, func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
		return caller, nil
	})
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
		responses: []*agentloop.Response{
			{Content: conversation.Text("one")},
			{Content: conversation.Text("two")},
		},
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	engine := fixture.newEngine(t, func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
		return caller, nil
	})
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

func waitForExecution(t *testing.T, engine *agentloop.Engine, sessionID uuid.UUID) {
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
