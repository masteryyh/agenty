//go:build integration

package adapter_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc/adapter"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func newDispatcher(t *testing.T) *rpc.Dispatcher {
	return newDispatcherWithCaller(t, &adapterTestCaller{})
}

func newDispatcherWithCaller(t *testing.T, caller agentloop.Caller) *rpc.Dispatcher {
	t.Helper()
	dir := t.TempDir()
	agentRepo := storage.NewAgentRepository(filepath.Join(dir, "agents"))
	catalogRepo := storage.NewCatalogRepository(filepath.Join(dir, "providers"))
	db, err := storage.OpenIsolatedDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	convRepo := storage.NewConversationRepository(db, filepath.Join(dir, "sessions"))
	execution, err := agentloop.NewEngine(t.Context(), agentloop.Dependencies{
		Sessions: convRepo,
		Agents:   agentRepo,
		Catalog:  catalogRepo,
		Tools:    agentloop.NewRegistry(),
		NewCaller: func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
			return caller, nil
		},
	})
	if err != nil {
		t.Fatalf("create execution engine: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := execution.Shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown execution engine: %v", err)
		}
	})
	sessionService := application.NewSessionService(
		convRepo,
		application.WithSessionExecutionState(execution),
	)
	agentService := application.NewAgentService(agentRepo)
	providerService := application.NewProviderService(catalogRepo)
	initialization := &initializationState{}
	d := rpc.NewDispatcher()
	adapter.RegisterAll(
		d,
		agentService,
		providerService,
		application.NewInitializeService(agentService, providerService, initialization),
		sessionService,
		execution,
	)
	return d
}

type initializationState struct {
	initialized bool
}

func (s *initializationState) Initialized() bool {
	return s.initialized
}

func (s *initializationState) SetInitialized(initialized bool) error {
	s.initialized = initialized
	return nil
}

type adapterTestCaller struct{}

func (*adapterTestCaller) Invoke(context.Context, agentloop.Request) (*agentloop.Response, error) {
	return &agentloop.Response{
		Content:    conversation.Text("completed"),
		StopReason: agentloop.StopReasonEndTurn,
	}, nil
}

func (*adapterTestCaller) Stream(
	context.Context,
	agentloop.Request,
	agentloop.StreamHandler,
) (*agentloop.Response, error) {
	return nil, fmt.Errorf("unexpected stream invocation")
}

type blockingAdapterCaller struct {
	started chan struct{}
}

func (caller *blockingAdapterCaller) Invoke(
	ctx context.Context,
	_ agentloop.Request,
) (*agentloop.Response, error) {
	close(caller.started)
	<-ctx.Done()

	return nil, ctx.Err()
}

func (*blockingAdapterCaller) Stream(
	context.Context,
	agentloop.Request,
	agentloop.StreamHandler,
) (*agentloop.Response, error) {
	return nil, fmt.Errorf("unexpected stream invocation")
}

func request(id int, method string, params any) string {
	b, _ := json.Marshal(params)
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, string(b))
}

// call runs a single JSON-RPC request through an in-memory server and returns
// the decoded response.
func call(t *testing.T, d *rpc.Dispatcher, req string) map[string]any {
	t.Helper()
	out := &bytes.Buffer{}
	srv := rpc.NewServer(d, strings.NewReader(req+"\n"), out)
	srv.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode response %q: %v", out.String(), err)
	}
	return resp
}

func errCode(resp map[string]any) int {
	e, ok := resp["error"].(map[string]any)
	if !ok {
		return 0
	}
	code, _ := e["code"].(float64)
	return int(code)
}

func TestAdapterAgentCreateAndGet(t *testing.T) {
	d := newDispatcher(t)

	create := call(t, d, request(1, "agent.create", map[string]any{
		"slug": "coder", "name": "Code Assistant", "soul": "You code.",
	}))
	if errCode(create) != 0 {
		t.Fatalf("create error: %+v", create["error"])
	}
	result := create["result"].(map[string]any)
	if result["slug"] != "coder" {
		t.Errorf("slug = %v, want coder", result["slug"])
	}

	got := call(t, d, request(2, "agent.get", map[string]any{"slug": "coder"}))
	if errCode(got) != 0 {
		t.Fatalf("get error: %+v", got["error"])
	}
	gotResult := got["result"].(map[string]any)
	if gotResult["name"] != "Code Assistant" {
		t.Errorf("name = %v", gotResult["name"])
	}
}

func TestAdapterEmptyCollectionsUseArrays(t *testing.T) {
	d := newDispatcher(t)

	for _, tt := range []struct {
		name   string
		method string
		id     int
	}{
		{name: "agents", method: "agent.list", id: 1},
		{name: "providers", method: "provider.list", id: 2},
		{name: "sessions", method: "session.list", id: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := call(t, d, request(tt.id, tt.method, map[string]any{}))
			if errCode(response) != 0 {
				t.Fatalf("%s error: %+v", tt.method, response["error"])
			}
			if _, ok := response["result"].([]any); !ok {
				t.Errorf("%s result = %T, want array", tt.method, response["result"])
			}
		})
	}
}

func TestAdapterAgentNotFound(t *testing.T) {
	d := newDispatcher(t)
	resp := call(t, d, request(1, "agent.get", map[string]any{"slug": "missing"}))
	if code := errCode(resp); code != rpc.ErrCodeNotFound {
		t.Errorf("code = %d, want %d (not found)", code, rpc.ErrCodeNotFound)
	}
}

func TestAdapterAgentInvalidSlug(t *testing.T) {
	d := newDispatcher(t)
	resp := call(t, d, request(1, "agent.create", map[string]any{"slug": "Bad Slug", "name": "x"}))
	if code := errCode(resp); code != rpc.ErrCodeInvalidParams {
		t.Errorf("code = %d, want %d (invalid params)", code, rpc.ErrCodeInvalidParams)
	}
}

func TestAdapterAgentDuplicate(t *testing.T) {
	d := newDispatcher(t)
	call(t, d, request(1, "agent.create", map[string]any{"slug": "coder", "name": "A"}))
	resp := call(t, d, request(2, "agent.create", map[string]any{"slug": "coder", "name": "B"}))
	if code := errCode(resp); code != rpc.ErrCodeAlreadyExists {
		t.Errorf("code = %d, want %d (already exists)", code, rpc.ErrCodeAlreadyExists)
	}
}

func TestAdapterProviderAddModel(t *testing.T) {
	d := newDispatcher(t)

	call(t, d, request(1, "provider.create", map[string]any{
		"slug": "anthropic", "name": "Anthropic", "type": "anthropic",
	}))
	resp := call(t, d, request(2, "provider.addModel", map[string]any{
		"providerSlug":    "anthropic",
		"modelSlug":       "claude-opus-4-8",
		"name":            "Claude Opus 4.8",
		"contextWindow":   200000,
		"maxOutputTokens": 32000,
	}))
	if errCode(resp) != 0 {
		t.Fatalf("addModel error: %+v", resp["error"])
	}
	result := resp["result"].(map[string]any)
	models := result["models"].([]any)
	if len(models) != 1 {
		t.Errorf("models = %d, want 1", len(models))
	}
	model := models[0].(map[string]any)
	if model["maxOutputTokens"] != float64(32000) {
		t.Errorf("maxOutputTokens = %v, want 32000", model["maxOutputTokens"])
	}
}

func TestAdapterInitializeJourney(t *testing.T) {
	d := newDispatcher(t)

	already := call(t, d, request(1, "initialize.already", struct{}{}))
	if errCode(already) != 0 {
		t.Fatalf("already error: %+v", already["error"])
	}
	if already["result"].(map[string]any)["initialized"] != false {
		t.Fatalf("already result = %+v", already["result"])
	}

	for id, step := range []struct {
		method string
		params map[string]any
	}{
		{method: "provider.create", params: map[string]any{
			"slug": "openai", "name": "OpenAI", "type": "openai", "apiKey": "test",
		}},
		{method: "provider.addModel", params: map[string]any{
			"providerSlug": "openai", "modelSlug": "gpt-test", "name": "GPT Test",
			"contextWindow": 128000, "maxOutputTokens": 16384, "isDefault": true,
		}},
		{method: "agent.create", params: map[string]any{
			"slug": "default", "name": "Default", "soul": "Be helpful.", "isDefault": true,
			"defaultContextWindow": 128000,
			"defaultModel":         map[string]any{"providerSlug": "openai", "modelSlug": "gpt-test"},
		}},
		{method: "initialize.complete", params: map[string]any{
			"agentSlug": "default", "providerSlug": "openai", "modelSlug": "gpt-test",
		}},
	} {
		resp := call(t, d, request(id+2, step.method, step.params))
		if errCode(resp) != 0 {
			t.Fatalf("%s error: %+v", step.method, resp["error"])
		}
	}

	already = call(t, d, request(10, "initialize.already", struct{}{}))
	if already["result"].(map[string]any)["initialized"] != true {
		t.Fatalf("already result after completion = %+v", already["result"])
	}
}

func TestAdapterSessionCreateAndGet(t *testing.T) {
	d := newDispatcher(t)

	create := call(t, d, request(1, "session.create", map[string]any{
		"agentSlug":     "coder",
		"providerSlug":  "anthropic",
		"modelSlug":     "claude-opus-4-8",
		"contextWindow": 200000,
	}))
	if errCode(create) != 0 {
		t.Fatalf("create error: %+v", create["error"])
	}
	result := create["result"].(map[string]any)
	id := result["id"].(string)
	if rounds, ok := result["rounds"].([]any); !ok || len(rounds) != 0 {
		t.Errorf("created rounds = %v, want an empty array", result["rounds"])
	}

	got := call(t, d, request(2, "session.get", map[string]any{"id": id}))
	if errCode(got) != 0 {
		t.Fatalf("get error: %+v", got["error"])
	}
	gotResult := got["result"].(map[string]any)
	if gotResult["id"] != id {
		t.Errorf("id = %v, want %s", gotResult["id"], id)
	}
	if rounds, ok := gotResult["rounds"].([]any); !ok || len(rounds) != 0 {
		t.Errorf("loaded rounds = %v, want an empty array", gotResult["rounds"])
	}
}

func TestAdapterSessionList(t *testing.T) {
	d := newDispatcher(t)
	for range 3 {
		call(t, d, request(1, "session.create", map[string]any{
			"agentSlug":    "coder",
			"providerSlug": "anthropic",
			"modelSlug":    "claude-opus-4-8",
		}))
	}
	resp := call(t, d, request(2, "session.list", map[string]any{}))
	if errCode(resp) != 0 {
		t.Fatalf("list error: %+v", resp["error"])
	}
	result := resp["result"].([]any)
	if len(result) != 3 {
		t.Errorf("list returned %d, want 3", len(result))
	}
}

func TestAdapterSessionStartCompletesRound(t *testing.T) {
	d := newDispatcher(t)
	id := createExecutableSession(t, d)

	started := call(t, d, request(10, "session.start", map[string]any{
		"id": id,
		"content": []map[string]any{{
			"type": "text",
			"text": "hello",
		}},
	}))
	if errCode(started) != 0 {
		t.Fatalf("start error: %+v", started["error"])
	}
	startResult := started["result"].(map[string]any)
	if startResult["sessionId"] != id || startResult["status"] != "running" {
		t.Fatalf("start result = %+v", startResult)
	}

	round := waitForAdapterRoundStatus(t, d, id, "completed")
	messages := round["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	assistant := messages[1].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("assistant message = %+v", assistant)
	}
}

func TestAdapterSessionStopCancelsRound(t *testing.T) {
	caller := &blockingAdapterCaller{started: make(chan struct{})}
	d := newDispatcherWithCaller(t, caller)
	id := createExecutableSession(t, d)

	started := call(t, d, request(10, "session.start", map[string]any{
		"id": id,
		"content": []map[string]any{{
			"type": "text",
			"text": "wait",
		}},
	}))
	if errCode(started) != 0 {
		t.Fatalf("start error: %+v", started["error"])
	}
	select {
	case <-caller.started:
	case <-time.After(time.Second):
		t.Fatal("LLM caller did not start")
	}

	duplicate := call(t, d, request(11, "session.start", map[string]any{
		"id": id,
		"content": []map[string]any{{
			"type": "text",
			"text": "again",
		}},
	}))
	if code := errCode(duplicate); code != rpc.ErrCodeAlreadyExists {
		t.Fatalf("duplicate start code = %d, want %d", code, rpc.ErrCodeAlreadyExists)
	}

	stopped := call(t, d, request(12, "session.stop", map[string]any{"id": id}))
	if errCode(stopped) != 0 {
		t.Fatalf("stop error: %+v", stopped["error"])
	}
	stopResult := stopped["result"].(map[string]any)
	if stopResult["sessionId"] != id || stopResult["stopRequested"] != true {
		t.Fatalf("stop result = %+v", stopResult)
	}

	waitForAdapterRoundStatus(t, d, id, "cancelled")
	secondStop := call(t, d, request(13, "session.stop", map[string]any{"id": id}))
	if code := errCode(secondStop); code != rpc.ErrCodeNotFound {
		t.Fatalf("second stop code = %d, want %d", code, rpc.ErrCodeNotFound)
	}
}

func createExecutableSession(t *testing.T, d *rpc.Dispatcher) string {
	t.Helper()

	createAgent := call(t, d, request(1, "agent.create", map[string]any{
		"slug": "coder",
		"name": "Coder",
		"soul": "Complete the task.",
	}))
	if errCode(createAgent) != 0 {
		t.Fatalf("create agent error: %+v", createAgent["error"])
	}
	createProvider := call(t, d, request(2, "provider.create", map[string]any{
		"slug":   "openai",
		"name":   "OpenAI",
		"type":   "openai_completions",
		"apiKey": "test-key",
	}))
	if errCode(createProvider) != 0 {
		t.Fatalf("create provider error: %+v", createProvider["error"])
	}
	addModel := call(t, d, request(3, "provider.addModel", map[string]any{
		"providerSlug":    "openai",
		"modelSlug":       "gpt-test",
		"name":            "GPT Test",
		"contextWindow":   128000,
		"maxOutputTokens": 100000,
	}))
	if errCode(addModel) != 0 {
		t.Fatalf("add model error: %+v", addModel["error"])
	}
	created := call(t, d, request(4, "session.create", map[string]any{
		"agentSlug":     "coder",
		"providerSlug":  "openai",
		"modelSlug":     "gpt-test",
		"contextWindow": 128000,
	}))
	if errCode(created) != 0 {
		t.Fatalf("create session error: %+v", created["error"])
	}

	return created["result"].(map[string]any)["id"].(string)
}

func waitForAdapterRoundStatus(
	t *testing.T,
	d *rpc.Dispatcher,
	sessionID string,
	want string,
) map[string]any {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		response := call(t, d, request(20, "session.get", map[string]any{"id": sessionID}))
		if errCode(response) != 0 {
			t.Fatalf("get session error: %+v", response["error"])
		}
		session := response["result"].(map[string]any)
		rounds := session["rounds"].([]any)
		if len(rounds) == 1 {
			round := rounds[0].(map[string]any)
			if round["status"] == want {
				return round
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s did not reach round status %q", sessionID, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAdapterUnknownMethod(t *testing.T) {
	d := newDispatcher(t)
	resp := call(t, d, request(1, "bogus.method", map[string]any{}))
	if code := errCode(resp); code != rpc.ErrCodeMethodNotFound {
		t.Errorf("code = %d, want %d (method not found)", code, rpc.ErrCodeMethodNotFound)
	}
}

// callChunked uploads params via the chunk.* protocol (2 shards) and returns
// the commit response, which carries the real method's result.
func callChunked(t *testing.T, d *rpc.Dispatcher, id int, method string, params any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(params)
	mid := len(raw) / 2
	if mid == 0 {
		mid = len(raw)
	}
	shards := [][]byte{raw[:mid], raw[mid:]}

	var input strings.Builder
	bp, _ := json.Marshal(map[string]any{"requestId": "r", "method": method})
	input.WriteString(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"chunk.begin","params":%s}`+"\n", id, bp))
	for i, s := range shards {
		pp, _ := json.Marshal(map[string]any{"requestId": "r", "index": i, "data": base64.StdEncoding.EncodeToString(s)})
		input.WriteString(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"chunk.part","params":%s}`+"\n", id, pp))
	}
	cp, _ := json.Marshal(map[string]any{"requestId": "r"})
	input.WriteString(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"chunk.commit","params":%s}`+"\n", id, cp))

	out := &bytes.Buffer{}
	srv := rpc.NewServer(d, strings.NewReader(input.String()), out)
	srv.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := bytes.Split(out.Bytes(), []byte("\n"))
	var last []byte
	for i := len(lines) - 1; i >= 0; i-- {
		if len(bytes.TrimSpace(lines[i])) > 0 {
			last = lines[i]
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal(last, &resp); err != nil {
		t.Fatalf("decode commit response %q: %v", out.String(), err)
	}
	return resp
}

func TestAdapterChunkedAgentCreate(t *testing.T) {
	d := newDispatcher(t)
	asm := rpc.NewChunkAssembler(d)
	rpc.RegisterChunkHandlers(d, asm)

	resp := callChunked(t, d, 1, "agent.create", map[string]any{
		"slug": "chunked", "name": "Chunked Agent", "soul": "You shard.",
	})
	if errCode(resp) != 0 {
		t.Fatalf("chunked create error: %+v", resp["error"])
	}
	result := resp["result"].(map[string]any)
	if result["slug"] != "chunked" {
		t.Errorf("slug = %v, want chunked", result["slug"])
	}

	// The committed upload must be indistinguishable from a direct call: a
	// subsequent agent.get reads back the same record.
	got := call(t, d, request(2, "agent.get", map[string]any{"slug": "chunked"}))
	if errCode(got) != 0 {
		t.Fatalf("get error: %+v", got["error"])
	}
}
