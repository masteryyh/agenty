package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// newTestServer builds a Server over input with the given handlers and a
// discard logger so tests stay quiet on stderr.
func newTestServer(t *testing.T, input string, handlers map[string]Handler) (*Server, *bytes.Buffer) {
	t.Helper()
	d := NewDispatcher()
	for m, h := range handlers {
		d.Register(m, h)
	}
	out := &bytes.Buffer{}
	srv := NewServer(d, strings.NewReader(input), out)
	srv.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return srv, out
}

// outputLines splits the NDJSON output into raw message bytes, skipping blanks.
func outputLines(t *testing.T, out *bytes.Buffer) [][]byte {
	t.Helper()
	var lines [][]byte
	for _, line := range bytes.Split(out.Bytes(), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func mustDecodeResponse(t *testing.T, raw []byte) response {
	t.Helper()
	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response %s: %v", raw, err)
	}
	return resp
}

func TestServerSingleRequest(t *testing.T) {
	echo := func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			V string `json:"v"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		return map[string]any{"echo": p.V}, nil
	}
	srv, out := newTestServer(t,
		`{"jsonrpc":"2.0","id":1,"method":"echo","params":{"v":"hi"}}`+"\n",
		map[string]Handler{"echo": echo},
	)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	lines := outputLines(t, out)
	if len(lines) != 1 {
		t.Fatalf("got %d output lines, want 1", len(lines))
	}
	resp := mustDecodeResponse(t, lines[0])
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != "1" {
		t.Errorf("id = %s, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["echo"] != "hi" {
		t.Errorf("result = %v, want echo=hi", result)
	}
}

func TestServerNotificationProducesNoResponse(t *testing.T) {
	echo := func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil }
	srv, out := newTestServer(t,
		`{"jsonrpc":"2.0","method":"echo","params":{"v":"hi"}}`+"\n",
		map[string]Handler{"echo": echo},
	)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("notification produced output %q, want none", out.String())
	}
}

func TestServerMethodNotFound(t *testing.T) {
	srv, out := newTestServer(t,
		`{"jsonrpc":"2.0","id":2,"method":"nope"}`+"\n",
		map[string]Handler{},
	)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := mustDecodeResponse(t, outputLines(t, out)[0])
	if resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("error = %+v, want method not found", resp.Error)
	}
	if string(resp.ID) != "2" {
		t.Errorf("id = %s, want 2", resp.ID)
	}
}

func TestServerBatch(t *testing.T) {
	echo := func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			V string `json:"v"`
		}
		json.Unmarshal(params, &p)
		return p.V, nil
	}
	srv, out := newTestServer(t,
		`[{"jsonrpc":"2.0","id":1,"method":"echo","params":{"v":"a"}},{"jsonrpc":"2.0","id":2,"method":"echo","params":{"v":"b"}}]`+"\n",
		map[string]Handler{"echo": echo},
	)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := outputLines(t, out)
	if len(lines) != 1 {
		t.Fatalf("got %d output lines, want 1 batch response", len(lines))
	}
	var resps []response
	if err := json.Unmarshal(lines[0], &resps); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2", len(resps))
	}
	if string(resps[0].Result) != `"a"` || string(resps[1].Result) != `"b"` {
		t.Errorf("results = %s, %s, want \"a\", \"b\"", resps[0].Result, resps[1].Result)
	}
}

func TestServerBatchWithNotification(t *testing.T) {
	echo := func(_ context.Context, _ json.RawMessage) (any, error) { return "ok", nil }
	// A request and a notification in the same batch: only one response.
	srv, out := newTestServer(t,
		`[{"jsonrpc":"2.0","id":1,"method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`+"\n",
		map[string]Handler{"echo": echo},
	)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resps []response
	if err := json.Unmarshal(outputLines(t, out)[0], &resps); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1 (notification excluded)", len(resps))
	}
	if string(resps[0].ID) != "1" {
		t.Errorf("response id = %s, want 1", resps[0].ID)
	}
}

func TestServerBatchAllNotificationsNoResponse(t *testing.T) {
	echo := func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil }
	srv, out := newTestServer(t,
		`[{"jsonrpc":"2.0","method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`+"\n",
		map[string]Handler{"echo": echo},
	)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("all-notification batch produced output %q, want none", out.String())
	}
}

func TestServerParseError(t *testing.T) {
	srv, out := newTestServer(t,
		`{bad json`+"\n",
		map[string]Handler{},
	)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := mustDecodeResponse(t, outputLines(t, out)[0])
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Fatalf("error = %+v, want parse error", resp.Error)
	}
	if string(resp.ID) != "null" {
		t.Errorf("id = %s, want null", resp.ID)
	}
}

func TestServerEmptyBatch(t *testing.T) {
	srv, out := newTestServer(t, `[]`+"\n", map[string]Handler{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := mustDecodeResponse(t, outputLines(t, out)[0])
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidRequest {
		t.Fatalf("error = %+v, want invalid request", resp.Error)
	}
}

func TestServerInvalidRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		batch bool
	}{
		{name: "empty object", input: `{}`},
		{name: "wrong version", input: `{"jsonrpc":"1.0","id":1,"method":"echo"}`},
		{name: "missing method", input: `{"jsonrpc":"2.0","id":1}`},
		{name: "scalar", input: `1`},
		{name: "string", input: `"request"`},
		{name: "method has wrong type", input: `{"jsonrpc":"2.0","id":1,"method":1}`},
		{name: "object id", input: `{"jsonrpc":"2.0","id":{},"method":"echo"}`},
		{name: "batch scalar member", input: `[1]`, batch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, out := newTestServer(t, tt.input+"\n", map[string]Handler{})
			if err := srv.Serve(t.Context()); err != nil {
				t.Fatalf("Serve: %v", err)
			}
			lines := outputLines(t, out)
			if len(lines) != 1 {
				t.Fatalf("output lines = %d, want 1", len(lines))
			}
			var resp response
			if tt.batch {
				var responses []response
				if err := json.Unmarshal(lines[0], &responses); err != nil {
					t.Fatalf("decode batch: %v", err)
				}
				if len(responses) != 1 {
					t.Fatalf("batch responses = %d, want 1", len(responses))
				}
				resp = responses[0]
			} else {
				resp = mustDecodeResponse(t, lines[0])
			}
			if resp.Error == nil || resp.Error.Code != ErrCodeInvalidRequest {
				t.Errorf("error = %+v, want invalid request", resp.Error)
			}
			if string(resp.ID) != "null" && (tt.name == "empty object" || tt.name == "scalar" || tt.name == "string" || tt.name == "batch scalar member") {
				t.Errorf("id = %s, want null", resp.ID)
			}
		})
	}
}

func TestServerHandlerReturnsRPCError(t *testing.T) {
	h := func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, InvalidParams("value is required")
	}
	srv, out := newTestServer(t,
		`{"jsonrpc":"2.0","id":1,"method":"m"}`+"\n",
		map[string]Handler{"m": h},
	)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := mustDecodeResponse(t, outputLines(t, out)[0])
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams || resp.Error.Message != "value is required" {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

func TestServerHandlerReturnsPlainError(t *testing.T) {
	h := func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, errors.New("boom")
	}
	srv, out := newTestServer(t,
		`{"jsonrpc":"2.0","id":1,"method":"m"}`+"\n",
		map[string]Handler{"m": h},
	)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := mustDecodeResponse(t, outputLines(t, out)[0])
	if resp.Error == nil || resp.Error.Code != ErrCodeInternalError {
		t.Fatalf("error = %+v, want internal error", resp.Error)
	}
}

func TestServerIDRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{name: "string", id: `"a"`},
		{name: "number", id: `42`},
		{name: "null", id: `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			echo := func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil }
			input := `{"jsonrpc":"2.0","id":` + tt.id + `,"method":"echo"}` + "\n"
			srv, out := newTestServer(t, input, map[string]Handler{"echo": echo})
			if err := srv.Serve(t.Context()); err != nil {
				t.Fatalf("Serve: %v", err)
			}
			if got := string(mustDecodeResponse(t, outputLines(t, out)[0]).ID); got != tt.id {
				t.Errorf("id = %s, want %s", got, tt.id)
			}
		})
	}
}

func TestDispatcherWrapErrorPassesRPCErrorThrough(t *testing.T) {
	original := InvalidParams("bad")
	wrapped := wrapError(original)
	if wrapped != original {
		t.Error("wrapError should return *Error values verbatim")
	}
}

func TestDispatcherWrapErrorWrapsUnknown(t *testing.T) {
	wrapped := wrapError(errors.New("boom"))
	if wrapped.Code != ErrCodeInternalError {
		t.Errorf("code = %d, want internal error", wrapped.Code)
	}
}

func TestServerLineTooLarge(t *testing.T) {
	echo := func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil }
	oversize := strings.Repeat("x", 2048)
	srv, out := newTestServer(t, oversize+"\n", map[string]Handler{"echo": echo})
	srv.SetMaxLineBytes(1024)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := outputLines(t, out)
	if len(lines) != 1 {
		t.Fatalf("got %d output lines, want 1 (too-large)", len(lines))
	}
	resp := mustDecodeResponse(t, lines[0])
	if resp.Error == nil || resp.Error.Code != ErrCodeMessageTooLarge {
		t.Fatalf("error = %+v, want message too large", resp.Error)
	}
	if string(resp.ID) != "null" {
		t.Errorf("id = %s, want null", resp.ID)
	}
	var data struct {
		MaxLineBytes int `json:"maxLineBytes"`
	}
	if err := json.Unmarshal(resp.Error.Data, &data); err != nil {
		t.Fatalf("decode error data: %v", err)
	}
	if data.MaxLineBytes != 1024 {
		t.Errorf("data.maxLineBytes = %d, want 1024", data.MaxLineBytes)
	}
}

func TestServerContinuesAfterTooLarge(t *testing.T) {
	echo := func(_ context.Context, _ json.RawMessage) (any, error) { return "ok", nil }
	oversize := strings.Repeat("x", 2048)
	input := oversize + "\n" + `{"jsonrpc":"2.0","id":1,"method":"echo"}` + "\n"
	srv, out := newTestServer(t, input, map[string]Handler{"echo": echo})
	srv.SetMaxLineBytes(1024)

	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := outputLines(t, out)
	if len(lines) != 2 {
		t.Fatalf("got %d output lines, want 2 (too-large + echo)", len(lines))
	}
	if r := mustDecodeResponse(t, lines[0]); r.Error == nil || r.Error.Code != ErrCodeMessageTooLarge {
		t.Errorf("line 0 error = %+v, want message too large", r.Error)
	}
	if r := mustDecodeResponse(t, lines[1]); r.Error != nil || string(r.Result) != `"ok"` {
		t.Errorf("line 1 = %+v, want result \"ok\"", r)
	}
}

func TestReadLineBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		max      int
		wantLine string
		tooLarge bool
	}{
		{name: "lf", input: "hello\n", max: 5, wantLine: "hello"},
		{name: "crlf", input: "hello\r\n", max: 5, wantLine: "hello"},
		{name: "eof without newline", input: "hello", max: 5, wantLine: "hello"},
		{name: "one byte over with newline", input: "hello!\n", max: 5, tooLarge: true},
		{name: "one byte over at eof", input: "hello!", max: 5, tooLarge: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line, tooLarge, err := readLine(bufio.NewReaderSize(strings.NewReader(tt.input), 16), tt.max)
			if err != nil {
				t.Fatalf("readLine: %v", err)
			}
			if tooLarge != tt.tooLarge || string(line) != tt.wantLine {
				t.Errorf("readLine = %q, %v; want %q, %v", line, tooLarge, tt.wantLine, tt.tooLarge)
			}
		})
	}
}

func TestReadLineDrainsOversizedInput(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 100)+"\nnext\n"), 16)
	if _, tooLarge, err := readLine(reader, 20); err != nil || !tooLarge {
		t.Fatalf("first read = tooLarge %v, error %v", tooLarge, err)
	}
	line, tooLarge, err := readLine(reader, 20)
	if err != nil || tooLarge || string(line) != "next" {
		t.Errorf("second read = %q, tooLarge %v, error %v", line, tooLarge, err)
	}
}

type blockingReader struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingReader() *blockingReader {
	return &blockingReader{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (r *blockingReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	close(r.done)
	return 0, io.EOF
}

func TestServerCancellationReleasesAfterReaderCloses(t *testing.T) {
	t.Parallel()

	reader := newBlockingReader()
	srv := NewServer(NewDispatcher(), reader, io.Discard)
	ctx, cancel := context.WithCancel(t.Context())
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx) }()
	<-reader.started
	cancel()
	if err := <-serveDone; !errors.Is(err, context.Canceled) {
		t.Errorf("Serve error = %v, want context.Canceled", err)
	}
	close(reader.release)
	<-reader.done
}
