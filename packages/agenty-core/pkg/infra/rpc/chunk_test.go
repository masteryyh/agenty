package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func newAssembler(t *testing.T) (*Dispatcher, *ChunkAssembler) {
	t.Helper()
	d := NewDispatcher()
	d.Register("echo", func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			V string `json:"v"`
		}
		_ = json.Unmarshal(params, &p)
		return map[string]any{"echo": p.V}, nil
	})
	d.Register("fail", func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, InvalidParams("bad input")
	})
	asm := NewChunkAssembler(d)
	RegisterChunkHandlers(d, asm)
	return d, asm
}

func dispatchRaw(t *testing.T, d *Dispatcher, id, method string, params any) response {
	t.Helper()
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return d.dispatch(context.Background(), request{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method, Params: b})
}

func dispatchOK(t *testing.T, d *Dispatcher, id, method string, params any) response {
	t.Helper()
	resp := dispatchRaw(t, d, id, method, params)
	if resp.Error != nil {
		t.Fatalf("%s error: %+v", method, resp.Error)
	}
	return resp
}

func uploadShards(t *testing.T, d *Dispatcher, requestID, method string, shards map[int][]byte) response {
	t.Helper()
	dispatchOK(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": requestID, "method": method})
	for i, s := range shards {
		dispatchOK(t, d, `"p"`, "chunk.part", map[string]any{"requestId": requestID, "index": i, "data": base64.StdEncoding.EncodeToString(s)})
	}
	return dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": requestID})
}

func TestChunkHappyPath(t *testing.T) {
	d, _ := newAssembler(t)
	params := []byte(`{"v":"hello"}`)
	resp := uploadShards(t, d, "r1", "echo", map[int][]byte{
		0: params[:3],
		1: params[3:7],
		2: params[7:],
	})
	if resp.Error != nil {
		t.Fatalf("commit error: %+v", resp.Error)
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["echo"] != "hello" {
		t.Errorf("echo = %v, want hello", result["echo"])
	}
}

func TestChunkShardsOutOfOrder(t *testing.T) {
	d, _ := newAssembler(t)
	params := []byte(`{"v":"xyz"}`)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	dispatchRaw(t, d, `"p2"`, "chunk.part", map[string]any{"requestId": "r", "index": 2, "data": base64.StdEncoding.EncodeToString(params[6:])})
	dispatchRaw(t, d, `"p0"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString(params[:3])})
	dispatchRaw(t, d, `"p1"`, "chunk.part", map[string]any{"requestId": "r", "index": 1, "data": base64.StdEncoding.EncodeToString(params[3:6])})
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error != nil {
		t.Fatalf("commit error: %+v", resp.Error)
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["echo"] != "xyz" {
		t.Errorf("echo = %v, want xyz", result["echo"])
	}
}

func TestChunkCommitIncomplete(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo", "chunkCount": 2})
	dispatchRaw(t, d, `"p"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString([]byte("{}"))})
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

func TestChunkDuplicateIndex(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	dispatchRaw(t, d, `"p1"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString([]byte("{}"))})
	resp := dispatchRaw(t, d, `"p2"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString([]byte("{}"))})
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

func TestChunkUnknownSession(t *testing.T) {
	d, _ := newAssembler(t)
	for _, method := range []string{"chunk.part", "chunk.commit", "chunk.abort"} {
		t.Run(method, func(t *testing.T) {
			resp := dispatchRaw(t, d, `"1"`, method, map[string]any{"requestId": "ghost"})
			if resp.Error == nil || resp.Error.Code != ErrCodeNotFound {
				t.Errorf("error = %+v, want not found", resp.Error)
			}
		})
	}
}

func TestChunkSizeMismatch(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo", "totalSize": 100})
	dispatchRaw(t, d, `"p"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString([]byte("{}"))})
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("error = %+v, want invalid params (size mismatch)", resp.Error)
	}
}

func TestChunkPartBadBase64(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	resp := dispatchRaw(t, d, `"p"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": "!!!not base64!!!"})
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Fatalf("error = %+v, want parse error", resp.Error)
	}
}

func TestChunkAssembledNotJSON(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	dispatchRaw(t, d, `"p"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString([]byte("not json"))})
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Fatalf("error = %+v, want parse error", resp.Error)
	}
}

func TestChunkAbortThenCommit(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	dispatchRaw(t, d, `"a"`, "chunk.abort", map[string]any{"requestId": "r"})
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error == nil || resp.Error.Code != ErrCodeNotFound {
		t.Fatalf("error = %+v, want not found", resp.Error)
	}
}

func TestChunkBeginPayloadTooLarge(t *testing.T) {
	d, asm := newAssembler(t)
	asm.SetMaxPayload(100)
	resp := dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo", "totalSize": 200})
	if resp.Error == nil || resp.Error.Code != ErrCodeChunkPayloadTooLarge {
		t.Fatalf("error = %+v, want chunk payload too large", resp.Error)
	}
}

func TestChunkBeginUnknownMethod(t *testing.T) {
	d, _ := newAssembler(t)
	resp := dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "nope"})
	if resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("error = %+v, want method not found", resp.Error)
	}
}

func TestChunkBeginDuplicate(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b1"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	resp := dispatchRaw(t, d, `"b2"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	if resp.Error == nil || resp.Error.Code != ErrCodeAlreadyExists {
		t.Fatalf("error = %+v, want already exists", resp.Error)
	}
}

func TestChunkCommitSurfacesMethodError(t *testing.T) {
	d, _ := newAssembler(t)
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "fail"})
	dispatchRaw(t, d, `"p"`, "chunk.part", map[string]any{"requestId": "r", "index": 0, "data": base64.StdEncoding.EncodeToString([]byte("{}"))})
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams || resp.Error.Message != "bad input" {
		t.Fatalf("error = %+v, want invalid params 'bad input'", resp.Error)
	}
}

func TestChunkSweepIdle(t *testing.T) {
	d, asm := newAssembler(t)
	asm.SetTTL(time.Minute)
	base := time.Now()
	asm.SetNow(func() time.Time { return base })
	dispatchRaw(t, d, `"b"`, "chunk.begin", map[string]any{"requestId": "r", "method": "echo"})
	asm.SetNow(func() time.Time { return base.Add(2 * time.Minute) })
	asm.sweep()
	resp := dispatchRaw(t, d, `"c"`, "chunk.commit", map[string]any{"requestId": "r"})
	if resp.Error == nil || resp.Error.Code != ErrCodeNotFound {
		t.Fatalf("error = %+v, want not found after sweep", resp.Error)
	}
}

func TestChunkRejectsInvalidShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*ChunkAssembler) error
	}{
		{name: "negative total size", call: func(a *ChunkAssembler) error { return a.Begin("echo", "r", -1, 0) }},
		{name: "negative chunk count", call: func(a *ChunkAssembler) error { return a.Begin("echo", "r", 0, -1) }},
		{name: "negative index", call: func(a *ChunkAssembler) error {
			if err := a.Begin("echo", "r", 0, 0); err != nil {
				return err
			}
			return a.Part("r", -1, base64.StdEncoding.EncodeToString([]byte(`{}`)))
		}},
		{name: "index exceeds count", call: func(a *ChunkAssembler) error {
			if err := a.Begin("echo", "r", 0, 2); err != nil {
				return err
			}
			return a.Part("r", 2, base64.StdEncoding.EncodeToString([]byte(`{}`)))
		}},
		{name: "missing index", call: func(a *ChunkAssembler) error {
			if err := a.Begin("echo", "r", 0, 0); err != nil {
				return err
			}
			if err := a.Part("r", 0, base64.StdEncoding.EncodeToString([]byte(`{"v":"`))); err != nil {
				return err
			}
			if err := a.Part("r", 2, base64.StdEncoding.EncodeToString([]byte(`x"}`))); err != nil {
				return err
			}
			_, _, err := a.Commit(context.Background(), "r")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, assembler := newAssembler(t)
			var rpcErr *Error
			if err := tt.call(assembler); !errors.As(err, &rpcErr) || rpcErr.Code != ErrCodeInvalidParams {
				t.Errorf("error = %v, want invalid params", err)
			}
		})
	}
}

func TestChunkAccumulatedPayloadLimitDropsSession(t *testing.T) {
	t.Parallel()

	_, assembler := newAssembler(t)
	assembler.SetMaxPayload(4)
	if err := assembler.Begin("echo", "r", 0, 0); err != nil {
		t.Fatal(err)
	}
	err := assembler.Part("r", 0, base64.StdEncoding.EncodeToString([]byte("12345")))
	var rpcErr *Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != ErrCodeChunkPayloadTooLarge {
		t.Fatalf("Part error = %v, want payload too large", err)
	}
	err = assembler.Abort("r")
	if !errors.As(err, &rpcErr) || rpcErr.Code != ErrCodeNotFound {
		t.Errorf("Abort error = %v, want session not found", err)
	}
}

func TestChunkStartCleanupExpiresIdleSession(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		_, assembler := newAssembler(t)
		assembler.SetTTL(2 * time.Second)
		ctx, cancel := context.WithCancel(t.Context())
		assembler.StartCleanup(ctx)
		if err := assembler.Begin("echo", "idle", 0, 0); err != nil {
			t.Fatal(err)
		}

		time.Sleep(3 * time.Second)
		synctest.Wait()
		var rpcErr *Error
		if err := assembler.Abort("idle"); !errors.As(err, &rpcErr) || rpcErr.Code != ErrCodeNotFound {
			t.Errorf("Abort error = %v, want expired session", err)
		}

		cancel()
		synctest.Wait()
	})
}
