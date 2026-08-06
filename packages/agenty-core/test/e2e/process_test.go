//go:build e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const processTimeout = 15 * time.Second

type frameResult struct {
	data []byte
	err  error
}

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
	dataDir string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	frames  chan frameResult
	stderr  *synchronizedBuffer
	cancel  context.CancelFunc

	writeMu  sync.Mutex
	waitMu   sync.Mutex
	waitDone chan struct{}
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

	process := &coreProcess{
		dataDir:  dataDir,
		cmd:      cmd,
		stdin:    stdin,
		frames:   make(chan frameResult, 128),
		stderr:   stderr,
		cancel:   cancel,
		waitDone: make(chan struct{}),
	}
	go process.readStdout(stdout)
	go process.wait()

	t.Cleanup(func() {
		if err := process.Close(); err != nil {
			t.Errorf("close core: %v\nstderr:\n%s", err, process.stderr.String())
		}
	})

	return process
}

func (p *coreProcess) readStdout(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			p.frames <- frameResult{data: bytes.Clone(trimmed)}
		}
		if err != nil {
			p.frames <- frameResult{err: err}
			return
		}
	}
}

func (p *coreProcess) wait() {
	err := p.cmd.Wait()
	p.waitMu.Lock()
	p.waitErr = err
	p.waitMu.Unlock()
	close(p.waitDone)
}

func (p *coreProcess) WriteFrame(ctx context.Context, payload []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	line := make([]byte, 0, len(payload)+1)
	line = append(line, payload...)
	line = append(line, '\n')
	if _, err := p.stdin.Write(line); err != nil {
		return fmt.Errorf("write core frame: %w", err)
	}
	return nil
}

func (p *coreProcess) WriteFinalFrame(ctx context.Context, payload []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := p.stdin.Write(payload); err != nil {
		return fmt.Errorf("write final core frame: %w", err)
	}
	if err := p.stdin.Close(); err != nil {
		return fmt.Errorf("close core stdin: %w", err)
	}
	return nil
}

func (p *coreProcess) ReadFrame(ctx context.Context) ([]byte, error) {
	select {
	case result := <-p.frames:
		if result.err != nil {
			return nil, result.err
		}
		return result.data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *coreProcess) Close() error {
	p.stopOnce.Do(func() {
		_ = p.stdin.Close()
		timer := time.NewTimer(processTimeout)
		defer timer.Stop()

		select {
		case <-p.waitDone:
			p.stopErr = p.processError()
		case <-timer.C:
			p.cancel()
			p.stopErr = fmt.Errorf("process did not exit after stdin EOF within %s", processTimeout)
		}

		p.cancel()
		diagnostics := strings.TrimSpace(p.stderr.String())
		if diagnostics != "" && p.stopErr == nil {
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

func coreEnv(dataDir string) []string {
	env := replaceEnv(os.Environ(), "AGENTY_DATA_DIR", dataDir)
	env = replaceEnv(env, "AGENTY_LOG_LEVEL", "")
	return replaceEnv(env, "AGENTY_LOG_FORMAT", "")
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
