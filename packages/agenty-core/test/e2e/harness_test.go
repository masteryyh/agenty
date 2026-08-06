//go:build e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const processTimeout = 10 * time.Second

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type readResult struct {
	line []byte
	err  error
}

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type coreProcess struct {
	dataDir  string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	lines    chan readResult
	waitDone chan struct{}
	stderr   *synchronizedBuffer
	cancel   context.CancelFunc

	ids      atomic.Int64
	ioMu     sync.Mutex
	waitMu   sync.Mutex
	waitErr  error
	stopOnce sync.Once
	stopErr  error
}

func startCore(t *testing.T) *coreProcess {
	t.Helper()
	dataDir := t.TempDir()
	return startCoreAt(t, dataDir, coreEnv(dataDir))
}

func startCoreAt(t *testing.T, dataDir string, env []string) *coreProcess {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, coreBinary)
	cmd.Dir = moduleRoot
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatalf("create core stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatalf("create core stdout: %v", err)
	}
	stderr := new(synchronizedBuffer)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start core: %v", err)
	}

	p := &coreProcess{
		dataDir:  dataDir,
		cmd:      cmd,
		stdin:    stdin,
		lines:    make(chan readResult, 32),
		waitDone: make(chan struct{}),
		stderr:   stderr,
		cancel:   cancel,
	}
	go p.readStdout(stdout)
	go func() {
		p.waitMu.Lock()
		p.waitErr = cmd.Wait()
		p.waitMu.Unlock()
		close(p.waitDone)
	}()

	t.Cleanup(func() {
		if err := p.stop(); err != nil {
			t.Errorf("stop core: %v\nstderr:\n%s", err, p.stderr.String())
		}
	})
	return p
}

func (p *coreProcess) readStdout(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			p.lines <- readResult{line: bytes.TrimSpace(line)}
		}
		if err != nil {
			p.lines <- readResult{err: err}
			return
		}
	}
}

func (p *coreProcess) Call(t *testing.T, method string, params any) rpcResponse {
	t.Helper()

	id := p.ids.Add(1)
	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("encode %s request: %v", method, err)
	}

	line := p.exchange(t, payload)
	var response rpcResponse
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode %s response %q: %v", method, line, err)
	}
	if response.JSONRPC != "2.0" {
		t.Fatalf("%s response jsonrpc = %q, want 2.0", method, response.JSONRPC)
	}
	var responseID int64
	if err := json.Unmarshal(response.ID, &responseID); err != nil || responseID != id {
		t.Fatalf("%s response id = %s, want %d", method, response.ID, id)
	}
	return response
}

func (p *coreProcess) CallChunked(t *testing.T, method string, params any, shardSize int) rpcResponse {
	t.Helper()
	if shardSize <= 0 {
		t.Fatal("chunk shard size must be positive")
	}

	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode chunk params: %v", err)
	}
	requestID := fmt.Sprintf("e2e-%d", p.ids.Add(1))
	chunkCount := (len(raw) + shardSize - 1) / shardSize
	if chunkCount == 0 {
		chunkCount = 1
	}

	requireSuccess(t, p.Call(t, "chunk.begin", map[string]any{
		"requestId":  requestID,
		"method":     method,
		"totalSize":  len(raw),
		"chunkCount": chunkCount,
	}))
	for index := range chunkCount {
		start := min(index*shardSize, len(raw))
		end := min(start+shardSize, len(raw))
		requireSuccess(t, p.Call(t, "chunk.part", map[string]any{
			"requestId": requestID,
			"index":     index,
			"data":      base64.StdEncoding.EncodeToString(raw[start:end]),
		}))
	}
	return p.Call(t, "chunk.commit", map[string]any{"requestId": requestID})
}

func (p *coreProcess) Notify(t *testing.T, method string, params any) {
	t.Helper()
	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		t.Fatalf("encode %s notification: %v", method, err)
	}
	p.write(t, payload)
}

func (p *coreProcess) ExchangeRaw(t *testing.T, payload string) []byte {
	t.Helper()
	return p.exchange(t, []byte(payload))
}

func (p *coreProcess) SendRaw(t *testing.T, payload string) {
	t.Helper()
	p.write(t, []byte(payload))
}

func (p *coreProcess) ExchangeFinalRaw(t *testing.T, payload string) []byte {
	t.Helper()
	p.ioMu.Lock()
	defer p.ioMu.Unlock()

	p.writeExactLocked(t, []byte(payload))
	if err := p.stdin.Close(); err != nil {
		t.Fatalf("close core stdin: %v\nstderr:\n%s", err, p.stderr.String())
	}
	return p.awaitResponseLocked(t)
}

func (p *coreProcess) exchange(t *testing.T, payload []byte) []byte {
	t.Helper()
	p.ioMu.Lock()
	defer p.ioMu.Unlock()

	p.writeLocked(t, payload)
	return p.awaitResponseLocked(t)
}

func (p *coreProcess) awaitResponseLocked(t *testing.T) []byte {
	t.Helper()
	timer := time.NewTimer(processTimeout)
	defer timer.Stop()
	select {
	case result := <-p.lines:
		if result.err != nil {
			t.Fatalf("read core response: %v\nstderr:\n%s", result.err, p.stderr.String())
		}
		return result.line
	case <-timer.C:
		p.cancel()
		t.Fatalf("core response timed out after %s\nstderr:\n%s", processTimeout, p.stderr.String())
	}
	return nil
}

func (p *coreProcess) write(t *testing.T, payload []byte) {
	t.Helper()
	p.ioMu.Lock()
	defer p.ioMu.Unlock()
	p.writeLocked(t, payload)
}

func (p *coreProcess) writeLocked(t *testing.T, payload []byte) {
	t.Helper()
	line := make([]byte, 0, len(payload)+1)
	line = append(line, payload...)
	line = append(line, '\n')
	p.writeExactLocked(t, line)
}

func (p *coreProcess) writeExactLocked(t *testing.T, payload []byte) {
	t.Helper()
	if _, err := p.stdin.Write(payload); err != nil {
		t.Fatalf("write core request: %v\nstderr:\n%s", err, p.stderr.String())
	}
}

func (p *coreProcess) Close(t *testing.T) {
	t.Helper()
	if err := p.stop(); err != nil {
		t.Fatalf("stop core: %v\nstderr:\n%s", err, p.stderr.String())
	}
}

func (p *coreProcess) stop() error {
	p.stopOnce.Do(func() {
		_ = p.stdin.Close()
		timer := time.NewTimer(processTimeout)
		defer timer.Stop()
		select {
		case <-p.waitDone:
			if err := p.processError(); err != nil {
				p.stopErr = err
			}
		case <-timer.C:
			p.cancel()
			p.stopErr = fmt.Errorf("process did not exit after stdin EOF within %s", processTimeout)
		}
		p.cancel()
		if diagnostics := strings.TrimSpace(p.stderr.String()); diagnostics != "" && p.stopErr == nil {
			p.stopErr = fmt.Errorf("unexpected stderr: %s", diagnostics)
		}
	})
	return p.stopErr
}

func (p *coreProcess) processError() error {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	return p.waitErr
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

func coreEnv(dataDir string) []string {
	env := replaceEnv(os.Environ(), "AGENTY_DATA_DIR", dataDir)
	// Clear logging env so the child process is driven by its config file
	// (InitializeDataDir seeds info/text defaults). Tests that need to
	// exercise env overrides set these explicitly via replaceEnv.
	env = replaceEnv(env, "AGENTY_LOG_LEVEL", "")
	return replaceEnv(env, "AGENTY_LOG_FORMAT", "")
}

func requireSuccess(t *testing.T, response rpcResponse) json.RawMessage {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("unexpected RPC error %d: %s", response.Error.Code, response.Error.Message)
	}
	return response.Result
}

func requireRPCError(t *testing.T, response rpcResponse, code int) *rpcError {
	t.Helper()
	if response.Error == nil {
		t.Fatalf("RPC error = nil, want code %d; result = %s", code, response.Result)
	}
	if response.Error.Code != code {
		t.Fatalf("RPC error code = %d, want %d: %s", response.Error.Code, code, response.Error.Message)
	}
	return response.Error
}

func decodeResult[T any](t *testing.T, response rpcResponse) T {
	t.Helper()
	raw := requireSuccess(t, response)
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode RPC result %s: %v", raw, err)
	}
	return result
}

func decodeRawResponse(t *testing.T, line []byte) rpcResponse {
	t.Helper()
	var response rpcResponse
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode raw response %q: %v", line, err)
	}
	return response
}
