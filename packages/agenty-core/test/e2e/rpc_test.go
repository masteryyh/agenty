//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestRPCProcessContract(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	core.Notify(t, "agent.create", map[string]any{
		"slug": "notified",
		"name": "通知创建的 Agent 🐈",
		"soul": "line one\nline two",
	})
	created := decodeResult[agentView](t, core.Call(t, "agent.get", map[string]any{"slug": "notified"}))
	if created.Name != "通知创建的 Agent 🐈" || created.Soul != "line one\nline two" {
		t.Fatalf("notification-created agent = %+v", created)
	}

	batch, err := json.Marshal([]any{
		rpcRequest{JSONRPC: "2.0", ID: 101, Method: "agent.list", Params: map[string]any{}},
		rpcRequest{JSONRPC: "2.0", Method: "agent.list", Params: map[string]any{}},
		rpcRequest{JSONRPC: "2.0", ID: 102, Method: "missing.method", Params: map[string]any{}},
		1,
	})
	if err != nil {
		t.Fatalf("encode batch: %v", err)
	}
	var responses []rpcResponse
	if err := json.Unmarshal(core.ExchangeRaw(t, string(batch)), &responses); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(responses) != 3 {
		t.Fatalf("batch responses = %d, want 3: %+v", len(responses), responses)
	}
	if responses[0].Error != nil {
		t.Fatalf("batch list error = %+v", responses[0].Error)
	}
	requireRPCError(t, responses[1], errMethodMissing)
	requireRPCError(t, responses[2], errInvalidRequest)
	if string(responses[2].ID) != "null" {
		t.Fatalf("invalid batch member id = %s, want null", responses[2].ID)
	}

	malformed := decodeRawResponse(t, core.ExchangeRaw(t, `{"jsonrpc":"2.0",`))
	requireRPCError(t, malformed, errParse)
	if string(malformed.ID) != "null" {
		t.Fatalf("parse error id = %s, want null", malformed.ID)
	}
	listed := decodeResult[[]agentView](t, core.Call(t, "agent.list", map[string]any{}))
	if len(listed) != 1 || listed[0].Slug != "notified" {
		t.Fatalf("server did not recover after malformed input: %+v", listed)
	}
}

func TestRPCRequestIDRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{name: "string", id: `"request-id"`},
		{name: "large integer", id: `9007199254740993`},
		{name: "explicit null", id: `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			core := startCore(t)
			line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"agent.list"}`, tt.id)
			response := decodeRawResponse(t, core.ExchangeRaw(t, line))
			requireSuccess(t, response)
			if string(response.ID) != tt.id {
				t.Fatalf("response id = %s, want %s", response.ID, tt.id)
			}
		})
	}
}

func TestRPCInvalidInputAndParamsRemainRecoverable(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	emptyLine := decodeRawResponse(t, core.ExchangeRaw(t, ""))
	requireRPCError(t, emptyLine, errParse)
	if string(emptyLine.ID) != "null" {
		t.Fatalf("empty-line id = %s, want null", emptyLine.ID)
	}

	scalar := decodeRawResponse(t, core.ExchangeRaw(t, "1"))
	requireRPCError(t, scalar, errInvalidRequest)
	if string(scalar.ID) != "null" {
		t.Fatalf("scalar request id = %s, want null", scalar.ID)
	}

	emptyBatch := decodeRawResponse(t, core.ExchangeRaw(t, "[]"))
	requireRPCError(t, emptyBatch, errInvalidRequest)
	if string(emptyBatch.ID) != "null" {
		t.Fatalf("empty batch id = %s, want null", emptyBatch.ID)
	}

	requireRPCError(t, core.Call(t, "agent.list", []string{"unexpected"}), errInvalidParams)

	withoutParams := decodeRawResponse(t, core.ExchangeRaw(t,
		`{"jsonrpc":"2.0","id":"no-params","method":"agent.list"}`+"\r",
	))
	listed := decodeResult[[]agentView](t, withoutParams)
	if string(withoutParams.ID) != `"no-params"` || len(listed) != 0 {
		t.Fatalf("CRLF request without params = id %s, result %+v", withoutParams.ID, listed)
	}
}

func TestRPCAllNotificationBatchProducesNoResponse(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	batch, err := json.Marshal([]rpcRequest{
		{JSONRPC: "2.0", Method: "agent.create", Params: map[string]any{"slug": "batch-notified", "name": "Batch Notification"}},
		{JSONRPC: "2.0", Method: "missing.method", Params: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("encode notification batch: %v", err)
	}
	core.SendRaw(t, string(batch))

	created := decodeResult[agentView](t, core.Call(t, "agent.get", map[string]any{"slug": "batch-notified"}))
	if created.Name != "Batch Notification" {
		t.Fatalf("notification batch result = %+v", created)
	}
}

func TestChunkedRequestUsesRegisteredBusinessHandler(t *testing.T) {
	t.Parallel()
	core := startCore(t)
	params := map[string]any{
		"slug": "chunked-agent",
		"name": "Chunked Agent",
		"soul": "A payload intentionally split across many shards for the process-level protocol test.",
	}

	created := decodeResult[agentView](t, core.CallChunked(t, "agent.create", params, 17))
	if created.Slug != "chunked-agent" || created.Name != "Chunked Agent" {
		t.Fatalf("chunked agent = %+v", created)
	}
	got := decodeResult[agentView](t, core.Call(t, "agent.get", map[string]any{"slug": "chunked-agent"}))
	if got.Soul != params["soul"] {
		t.Fatalf("reloaded chunked soul = %q", got.Soul)
	}

	requireRPCError(t, core.CallChunked(t, "agent.create", params, 23), errAlreadyExists)
}
