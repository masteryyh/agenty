//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/sha3"
)

const processTimeout = 15 * time.Second

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type coreProcess struct {
	dataDir   string
	transport string
	address   string
	cmd       *exec.Cmd
	client    *http.Client
	stderr    *synchronizedBuffer
	cancel    context.CancelFunc
	waitDone  chan struct{}

	waitMu   sync.Mutex
	waitErr  error
	stopOnce sync.Once
	stopErr  error
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (err *APIError) Error() string {
	return err.Message
}

func startCore(t *testing.T) *coreProcess {
	t.Helper()
	dataDir := t.TempDir()
	return startCoreAt(t, dataDir, coreEnv(dataDir))
}

func startCoreAt(t *testing.T, dataDir string, env []string) *coreProcess {
	t.Helper()
	canonical, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatalf("resolve data directory: %v", err)
	}
	if err := os.MkdirAll(canonical, 0o700); err != nil {
		t.Fatalf("create data directory: %v", err)
	}
	canonical, err = filepath.EvalSymlinks(canonical)
	if err != nil {
		t.Fatalf("canonicalize data directory: %v", err)
	}
	canonical = filepath.Clean(canonical)
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	transportName, address := addressForDataDir(canonical)
	env = replaceEnv(env, "AGENTY_DATA_DIR", canonical)
	env = replaceEnv(env, "AGENTY_TRANSPORT", transportName)
	env = replaceEnv(env, "AGENTY_CORE_ADDR", address)
	env = replaceEnv(env, "AGENTY_IPC_VERSION", "1")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, coreBinary)
	cmd.Dir = moduleRoot
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout = io.Discard
	stderr := new(synchronizedBuffer)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start core: %v", err)
	}

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{
		Protocols: protocols,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialLocal(ctx, transportName, address)
		},
	}
	process := &coreProcess{
		dataDir:   canonical,
		transport: transportName,
		address:   address,
		cmd:       cmd,
		client:    &http.Client{Transport: transport},
		stderr:    stderr,
		cancel:    cancel,
		waitDone:  make(chan struct{}),
	}
	go process.wait()
	process.waitReady(t)

	t.Cleanup(func() {
		if err := process.Close(); err != nil {
			t.Errorf("close core: %v\nstderr:\n%s", err, process.stderr.String())
		}
	})
	return process
}

func (p *coreProcess) wait() {
	err := p.cmd.Wait()
	p.waitMu.Lock()
	p.waitErr = err
	p.waitMu.Unlock()
	close(p.waitDone)
}

func (p *coreProcess) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(processTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		var info map[string]json.RawMessage
		lastErr = p.Request(ctx, http.MethodGet, "/v1/system", nil, &info)
		cancel()
		if lastErr == nil {
			var dataDir, ipcVersion, apiContract string
			_ = json.Unmarshal(info["dataDir"], &dataDir)
			_ = json.Unmarshal(info["ipcVersion"], &ipcVersion)
			_ = json.Unmarshal(info["apiContract"], &apiContract)
			if dataDir != p.dataDir || ipcVersion != "1" || apiContract != "v1" {
				t.Fatalf("core handshake = %+v, want dataDir=%q ipc=1 api=v1", info, p.dataDir)
			}
			return
		}
		select {
		case <-p.waitDone:
			t.Fatalf("core exited before HTTP/2 readiness: %v\nstderr:\n%s", p.processError(), p.stderr.String())
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("core did not become ready: %v\nstderr:\n%s", lastErr, p.stderr.String())
}

func (p *coreProcess) Request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://agenty.local"+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("HTTP/2 %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read HTTP response: %w", err)
	}
	var result struct {
		Code      int             `json:"code"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
		ErrorCode string          `json:"errorCode"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode HTTP response: %w", err)
	}
	if result.Code != response.StatusCode || len(result.Data) == 0 {
		return fmt.Errorf("HTTP response envelope does not match status %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		apiError := &APIError{Status: response.StatusCode, Code: result.ErrorCode, Message: result.Message}
		if apiError.Message == "" {
			apiError.Message = fmt.Sprintf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
		}
		return apiError
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(result.Data, output); err != nil {
		return fmt.Errorf("decode HTTP response: %w", err)
	}
	return nil
}

func (p *coreProcess) Close() error {
	p.stopOnce.Do(func() {
		select {
		case <-p.waitDone:
			p.stopErr = p.processError()
		default:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			shutdownErr := p.Request(ctx, http.MethodPost, "/v1/system/shutdown", nil, nil)
			cancel()
			p.client.CloseIdleConnections()
			if shutdownErr != nil {
				select {
				case <-p.waitDone:
				default:
					p.cancel()
				}
			}
			timer := time.NewTimer(processTimeout)
			defer timer.Stop()
			select {
			case <-p.waitDone:
				p.stopErr = p.processError()
			case <-timer.C:
				p.cancel()
				p.stopErr = fmt.Errorf("core did not exit after HTTP shutdown within %s", processTimeout)
				if !p.waitForExit(2 * time.Second) {
					var killErr error
					if p.cmd.Process != nil {
						killErr = p.cmd.Process.Kill()
						if errors.Is(killErr, os.ErrProcessDone) {
							killErr = nil
						}
					}
					if !p.waitForExit(2 * time.Second) {
						pid := -1
						if p.cmd.Process != nil {
							pid = p.cmd.Process.Pid
						}
						p.stopErr = errors.Join(p.stopErr, fmt.Errorf("core process %d was not reaped after forced termination", pid))
					} else if killErr != nil {
						p.stopErr = errors.Join(p.stopErr, fmt.Errorf("force-terminate core process: %w", killErr))
					}
				}
			}
		}
		p.cancel()
		p.client.CloseIdleConnections()
		diagnostics := strings.TrimSpace(p.stderr.String())
		if diagnostics != "" && p.stopErr == nil {
			p.stopErr = fmt.Errorf("unexpected stderr: %s", diagnostics)
		}
	})
	return p.stopErr
}

func (p *coreProcess) waitForExit(limit time.Duration) bool {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-p.waitDone:
		return true
	case <-timer.C:
		return false
	}
}

func (p *coreProcess) processError() error {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	return p.waitErr
}

func coreEnv(dataDir string) []string {
	canonical, err := filepath.Abs(dataDir)
	if err == nil {
		if createErr := os.MkdirAll(canonical, 0o700); createErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
				canonical = filepath.Clean(resolved)
			}
		}
	}
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	transport, address := addressForDataDir(canonical)
	env := replaceEnv(os.Environ(), "AGENTY_DATA_DIR", dataDir)
	env = replaceEnv(env, "AGENTY_LOG_LEVEL", "")
	env = replaceEnv(env, "AGENTY_LOG_FORMAT", "")
	env = replaceEnv(env, "AGENTY_DATA_DIR", canonical)
	env = replaceEnv(env, "AGENTY_TRANSPORT", transport)
	env = replaceEnv(env, "AGENTY_CORE_ADDR", address)
	return replaceEnv(env, "AGENTY_IPC_VERSION", "1")
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

func addressForDataDir(dataDir string) (string, string) {
	digest := sha3.Sum256([]byte(dataDir))
	key := hex.EncodeToString(digest[:16])
	if runtime.GOOS == "windows" {
		return "named_pipe", "\\\\.\\pipe\\agenty-core-" + key
	}
	return "uds", filepath.Join(os.TempDir(), "agenty-"+key+".sock")
}

func testErrorCode(err error) string {
	var apiError *APIError
	if errors.As(err, &apiError) {
		return apiError.Code
	}
	return ""
}
