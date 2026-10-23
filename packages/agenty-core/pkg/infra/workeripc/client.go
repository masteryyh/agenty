// Package workeripc implements Agenty's versioned stdio NDJSON worker protocol.
package workeripc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const Version = 1
const MaxFrameBytes = 16 << 20

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

type Frame struct {
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Method    string          `json:"method,omitempty"`
	Params    any             `json:"params,omitempty"`
	Event     string          `json:"event,omitempty"`
	Sequence  uint64          `json:"sequence,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Failure        `json:"error,omitempty"`
}

type Initialization struct {
	ProtocolVersion          int    `json:"protocolVersion"`
	Backend                  string `json:"backend"`
	EngineVersion            string `json:"engineVersion"`
	MaxFrameBytes            int    `json:"maxFrameBytes"`
	MaxInputTokens           int    `json:"maxInputTokens"`
	MaxOutputTokens          int64  `json:"maxOutputTokens"`
	MaxConcurrentGenerations int    `json:"maxConcurrentGenerations"`
	SupportsJSONSchema       bool   `json:"supportsJsonSchema"`
}

type Client struct {
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	writeMu         sync.Mutex
	mu              sync.Mutex
	pending         map[string]chan Frame
	done            chan struct{}
	exited          chan struct{}
	failure         error
	once            sync.Once
	next            atomic.Uint64
	Limit           int
	MaxInputTokens  int
	MaxOutputTokens int64
}

func Start(ctx context.Context, command string, args, environment []string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if environment != nil {
		cmd.Env = environment
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start worker: %w", err)
	}
	c := &Client{cmd: cmd, stdin: stdin, pending: make(map[string]chan Frame), done: make(chan struct{}), exited: make(chan struct{}), Limit: MaxFrameBytes}
	go c.read(stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			slog.DebugContext(ctx, "inference worker", "message", scanner.Text())
		}
		// Drain even after an overlong log line so stderr never blocks inference.
		_, _ = io.Copy(io.Discard, stderr)
	}()
	return c, nil
}

func (c *Client) fail(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.failure = err
		close(c.done)
		c.mu.Unlock()
		_ = c.stdin.Close()
		_ = c.cmd.Process.Kill()
	})
}

func (c *Client) read(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), MaxFrameBytes)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			return newline + 1, data[:newline], nil
		}
		if atEOF && len(data) > 0 {
			return 0, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil
	})
	for scanner.Scan() {
		var frame Frame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil || frame.Version != Version || frame.RequestID == "" || (frame.Type != "event" && frame.Type != "response") {
			c.fail(errors.New("worker protocol: invalid frame"))
			break
		}
		if frame.Type == "response" && ((frame.Result == nil) == (frame.Error == nil)) {
			c.fail(errors.New("worker protocol: response must contain exactly one result or error"))
			break
		}
		c.mu.Lock()
		output := c.pending[frame.RequestID]
		c.mu.Unlock()
		if output == nil {
			c.fail(errors.New("worker protocol: unknown request ID"))
			break
		}
		select {
		case output <- frame:
		case <-c.done:
			break
		default:
			c.fail(errors.New("worker protocol: consumer queue overflow"))
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	c.fail(fmt.Errorf("worker stdout closed: %w", err))
	// Wait only after both pipes have been drained; stderr has its own reader.
	_ = c.cmd.Wait()
	close(c.exited)
}

func (c *Client) write(frame Frame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(data)+1 > c.Limit {
		return fmt.Errorf("worker frame exceeds %d bytes", c.Limit)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *Client) Call(ctx context.Context, method string, params, result any, event func(Frame) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strconv.FormatUint(c.next.Add(1), 10)
	output := make(chan Frame, 64)
	c.mu.Lock()
	c.pending[id] = output
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	written := make(chan error, 1)
	go func() {
		written <- c.write(Frame{Version: Version, Type: "request", RequestID: id, Method: method, Params: params})
	}()
	select {
	case err := <-written:
		if err != nil {
			c.fail(err)
			return err
		}
	case <-ctx.Done():
		c.fail(ctx.Err())
		return ctx.Err()
	case <-c.done:
		if len(output) == 0 {
			return c.Err()
		}
	}

	var sequence uint64
	var callErr error
	cancelled := ctx.Done()
	var grace <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		var frame Frame
		select {
		case <-cancelled:
			callErr = ctx.Err()
			cancelled = nil
			if method != "generate" {
				c.fail(callErr)
				return callErr
			}
			go func() {
				cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = c.Call(cancelCtx, "cancel", map[string]string{"requestId": id}, nil, nil)
			}()
			timer = time.NewTimer(5 * time.Second)
			grace = timer.C
			continue
		case <-grace:
			c.fail(callErr)
			return callErr
		case <-c.done:
			select {
			case frame = <-output:
			default:
				if callErr != nil {
					return callErr
				}
				return c.Err()
			}
		case frame = <-output:
		}
		if method == "generate" {
			sequence++
			if frame.Sequence != sequence {
				c.fail(errors.New("worker token sequence gap"))
				return c.Err()
			}
		}
		if frame.Type == "event" {
			if callErr == nil && event != nil {
				if err := event(frame); err != nil {
					c.fail(err)
					return err
				}
			}
			continue
		}
		if callErr != nil {
			return callErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if frame.Error != nil {
			return frame.Error
		}
		if result != nil {
			return json.Unmarshal(frame.Result, result)
		}
		return nil
	}
}

func (c *Client) Err() error              { c.mu.Lock(); defer c.mu.Unlock(); return c.failure }
func (c *Client) Exited() <-chan struct{} { return c.exited }
func (c *Client) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}
func (c *Client) Close(ctx context.Context) error {
	var err error
	if c.Alive() {
		err = c.Call(ctx, "shutdown", struct{}{}, nil, nil)
		c.fail(errors.New("worker stopped"))
	}
	select {
	case <-c.exited:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
