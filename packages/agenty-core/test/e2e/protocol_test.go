//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestStdioJSONRPCSupportsClientTrafficPatterns(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)

	writeRequestFrame(t, ctx, process, rpcRequest{
		JSONRPC: "2.0",
		Method:  "agent.create",
		Params: AgentCreateInput{
			Code: "notification-agent",
			Name: "通知创建的 Agent 🐈",
			Soul: "line one\nline two",
		},
	})
	writeRequestFrame(t, ctx, process, rpcRequest{
		JSONRPC: "2.0",
		ID:      "barrier",
		Method:  "agent.get",
		Params:  map[string]any{"code": "notification-agent"},
	})
	barrier := readSingleResponse(t, ctx, process)
	if string(barrier.ID) != `"barrier"` || barrier.Error != nil {
		t.Fatalf("notification barrier response = %+v", barrier)
	}
	var created Agent
	requireNoError(t, json.Unmarshal(barrier.Result, &created))
	if created.Name != "通知创建的 Agent 🐈" || created.Soul != "line one\nline two" {
		t.Fatalf("notification-created agent = %+v", created)
	}

	batch := []any{
		rpcRequest{JSONRPC: "2.0", ID: 101, Method: "agent.list", Params: struct{}{}},
		rpcRequest{JSONRPC: "2.0", Method: "agent.list", Params: struct{}{}},
		rpcRequest{JSONRPC: "2.0", ID: 102, Method: "missing.method", Params: struct{}{}},
		1,
	}
	writeJSONFrame(
		t,
		ctx,
		process,
		batch,
	)
	frame, err := process.ReadFrame(ctx)
	requireNoError(t, err)
	var responses []rpcResponse
	requireNoError(t, json.Unmarshal(frame, &responses))
	if len(responses) != 3 || responses[0].Error != nil {
		t.Fatalf("batch responses = %+v", responses)
	}
	if responses[1].Error == nil || responses[1].Error.Code != errMethodMissing {
		t.Fatalf("missing method response = %+v", responses[1])
	}
	if responses[2].Error == nil || responses[2].Error.Code != errInvalidRequest {
		t.Fatalf("invalid batch member response = %+v", responses[2])
	}

	requireNoError(t, process.WriteFrame(ctx, []byte(`{"jsonrpc":"2.0",`)))
	malformed := readSingleResponse(t, ctx, process)
	if malformed.Error == nil || malformed.Error.Code != errParse || string(malformed.ID) != "null" {
		t.Fatalf("malformed response = %+v", malformed)
	}
	writeRequestFrame(t, ctx, process, rpcRequest{
		JSONRPC: "2.0",
		ID:      "after-error",
		Method:  "agent.list",
	})
	recovered := readSingleResponse(t, ctx, process)
	if recovered.Error != nil || string(recovered.ID) != `"after-error"` {
		t.Fatalf("recovery response = %+v", recovered)
	}
}

func TestJSONRPCRequestIDsRoundTripExactly(t *testing.T) {
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
			ctx, cancel := testContext(t)
			defer cancel()
			process := startCore(t)

			frame := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"agent.list"}`, tt.id)
			requireNoError(t, process.WriteFrame(ctx, []byte(frame)))
			response := readSingleResponse(t, ctx, process)
			if response.Error != nil || string(response.ID) != tt.id {
				t.Fatalf(
					"response = %+v, want id %s",
					response,
					tt.id,
				)
			}
		})
	}
}

func TestRemovedInitializeSetMethodsAreNotRegistered(t *testing.T) {
	t.Parallel()

	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)
	for id, method := range []string{
		"initialize.set.provider",
		"initialize.set.model",
		"initialize.set.agent",
		"initialize.set.completed",
	} {
		writeRequestFrame(t, ctx, process, rpcRequest{
			JSONRPC: "2.0",
			ID:      id + 1,
			Method:  method,
			Params:  struct{}{},
		})
		response := readSingleResponse(t, ctx, process)
		if response.Error == nil || response.Error.Code != errMethodMissing {
			t.Fatalf("%s response = %+v, want method not found", method, response)
		}
	}
}

func TestStdioProcessesFinalFrameWithoutNewlineAndExitsOnEOF(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)

	payload := []byte(`{"jsonrpc":"2.0","id":"final-line","method":"agent.list"}`)
	requireNoError(t, process.WriteFinalFrame(ctx, payload))
	response := readSingleResponse(t, ctx, process)
	if response.Error != nil || string(response.ID) != `"final-line"` {
		t.Fatalf("final frame response = %+v", response)
	}
	requireNoError(t, process.Close())
}

func TestStdioEmptyInputExitsCleanly(t *testing.T) {
	t.Parallel()
	process := startCore(t)
	requireNoError(t, process.Close())
}

func writeRequestFrame(t *testing.T, ctx context.Context, process *coreProcess, request rpcRequest) {
	t.Helper()
	writeJSONFrame(
		t,
		ctx,
		process,
		request,
	)
}

func writeJSONFrame(t *testing.T, ctx context.Context, process *coreProcess, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	requireNoError(t, err)
	requireNoError(t, process.WriteFrame(ctx, payload))
}

func readSingleResponse(t *testing.T, ctx context.Context, process *coreProcess) rpcResponse {
	t.Helper()
	frame, err := process.ReadFrame(ctx)
	requireNoError(t, err)

	var response rpcResponse
	requireNoError(t, json.Unmarshal(frame, &response))
	if response.JSONRPC != "2.0" {
		t.Fatalf("response jsonrpc = %q, want 2.0", response.JSONRPC)
	}
	return response
}
