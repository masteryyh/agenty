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

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc/adapter"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func newDispatcher(t *testing.T) *rpc.Dispatcher {
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
	d := rpc.NewDispatcher()
	adapter.RegisterAll(d, application.NewAgentService(agentRepo), application.NewProviderService(catalogRepo), application.NewSessionService(convRepo))
	return d
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
		"providerSlug":  "anthropic",
		"modelSlug":     "claude-opus-4-8",
		"name":          "Claude Opus 4.8",
		"contextWindow": 200000,
	}))
	if errCode(resp) != 0 {
		t.Fatalf("addModel error: %+v", resp["error"])
	}
	result := resp["result"].(map[string]any)
	models := result["models"].([]any)
	if len(models) != 1 {
		t.Errorf("models = %d, want 1", len(models))
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

	got := call(t, d, request(2, "session.get", map[string]any{"id": id}))
	if errCode(got) != 0 {
		t.Fatalf("get error: %+v", got["error"])
	}
	gotResult := got["result"].(map[string]any)
	if gotResult["id"] != id {
		t.Errorf("id = %v, want %s", gotResult["id"], id)
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
