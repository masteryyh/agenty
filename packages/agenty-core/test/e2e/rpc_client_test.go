//go:build e2e

package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

type rpcClient struct {
	process *coreProcess
	ids     atomic.Int64

	pendingMu sync.Mutex
	pending   map[string]chan rpcResponse
	called    map[string]int
	orphans   chan rpcResponse
	events    chan SessionEvent
	done      chan struct{}
	routeErr  error
}

func newRPCClient(process *coreProcess) *rpcClient {
	client := &rpcClient{
		process: process,
		pending: map[string]chan rpcResponse{},
		called:  map[string]int{},
		orphans: make(chan rpcResponse, 16),
		events:  make(chan SessionEvent, 128),
		done:    make(chan struct{}),
	}
	go client.routeResponses()
	return client
}

func (c *rpcClient) SessionEvents() <-chan SessionEvent {
	return c.events
}

func (c *rpcClient) Call(ctx context.Context, method string, params, result any) error {
	id := c.ids.Add(1)
	responseChannel := make(chan rpcResponse, 1)
	key := strconv.FormatInt(id, 10)

	c.pendingMu.Lock()
	c.pending[key] = responseChannel
	c.called[method]++
	c.pendingMu.Unlock()

	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		c.removePending(key)
		return fmt.Errorf("encode %s request: %w", method, err)
	}
	if err := c.process.WriteFrame(ctx, payload); err != nil {
		c.removePending(key)
		return err
	}

	select {
	case response := <-responseChannel:
		if response.Error != nil {
			return response.Error
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(key)
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("RPC transport closed: %w", c.routeError())
	}
}

func (c *rpcClient) Notify(ctx context.Context, method string, params any) error {
	c.pendingMu.Lock()
	c.called[method]++
	c.pendingMu.Unlock()

	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("encode %s notification: %w", method, err)
	}
	return c.process.WriteFrame(ctx, payload)
}

func (c *rpcClient) CallChunked(
	ctx context.Context,
	method string,
	params any,
	shardSize int,
	result any,
) error {
	if shardSize <= 0 {
		return fmt.Errorf("chunk shard size must be positive")
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode chunk params: %w", err)
	}
	requestID := "e2e-" + strconv.FormatInt(c.ids.Add(1), 10)
	chunkCount := max(1, (len(raw)+shardSize-1)/shardSize)

	if err := c.Call(ctx, "chunk.begin", map[string]any{
		"requestId":  requestID,
		"method":     method,
		"totalSize":  len(raw),
		"chunkCount": chunkCount,
	}, nil); err != nil {
		return err
	}
	for index := range chunkCount {
		start := min(index*shardSize, len(raw))
		end := min(start+shardSize, len(raw))
		if err := c.Call(ctx, "chunk.part", map[string]any{
			"requestId": requestID,
			"index":     index,
			"data":      base64.StdEncoding.EncodeToString(raw[start:end]),
		}, nil); err != nil {
			return err
		}
	}
	return c.Call(
		ctx,
		"chunk.commit",
		map[string]any{"requestId": requestID},
		result,
	)
}

func (c *rpcClient) AbortChunk(ctx context.Context, requestID, method string) error {
	if err := c.Call(ctx, "chunk.begin", map[string]any{
		"requestId": requestID,
		"method":    method,
	}, nil); err != nil {
		return err
	}
	return c.Call(
		ctx,
		"chunk.abort",
		map[string]any{"requestId": requestID},
		nil,
	)
}

func (c *rpcClient) CalledMethods() map[string]int {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	called := make(map[string]int, len(c.called))
	for method, count := range c.called {
		called[method] = count
	}
	return called
}

func (c *rpcClient) routeResponses() {
	defer close(c.done)
	for {
		frame, err := c.process.ReadFrame(context.Background())
		if err != nil {
			if err == io.EOF {
				c.setRouteError(io.EOF)
				return
			}
			c.setRouteError(err)
			return
		}

		notification, responses, err := decodeRPCFrame(frame)
		if err != nil {
			c.setRouteError(err)
			return
		}
		if notification != nil {
			if notification.Method == "session.compaction" {
				continue
			}
			if notification.Method != "session.event" {
				c.setRouteError(fmt.Errorf("unexpected RPC notification %q", notification.Method))
				return
			}
			var event SessionEvent
			if err := json.Unmarshal(notification.Params, &event); err != nil {
				c.setRouteError(fmt.Errorf("decode session event: %w", err))
				return
			}
			c.events <- event
			continue
		}
		for _, response := range responses {
			c.dispatchResponse(response)
		}
	}
}

func decodeRPCFrame(frame []byte) (*rpcNotification, []rpcResponse, error) {
	responses := []rpcResponse{}
	if len(frame) > 0 && frame[0] == '[' {
		if err := json.Unmarshal(frame, &responses); err != nil {
			return nil, nil, fmt.Errorf("decode RPC batch response: %w", err)
		}
		return nil, responses, nil
	}

	var envelope struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return nil, nil, fmt.Errorf("decode RPC frame: %w", err)
	}
	if envelope.Method != "" {
		var notification rpcNotification
		if err := json.Unmarshal(frame, &notification); err != nil {
			return nil, nil, fmt.Errorf("decode RPC notification: %w", err)
		}
		return &notification, nil, nil
	}

	var response rpcResponse
	if err := json.Unmarshal(frame, &response); err != nil {
		return nil, nil, fmt.Errorf("decode RPC response: %w", err)
	}
	return nil, append(responses, response), nil
}

func (c *rpcClient) dispatchResponse(response rpcResponse) {
	key := string(response.ID)
	c.pendingMu.Lock()
	responseChannel, ok := c.pending[key]
	if ok {
		delete(c.pending, key)
	}
	c.pendingMu.Unlock()

	if ok {
		responseChannel <- response
		return
	}
	select {
	case c.orphans <- response:
	default:
	}
}

func (c *rpcClient) removePending(key string) {
	c.pendingMu.Lock()
	delete(c.pending, key)
	c.pendingMu.Unlock()
}

func (c *rpcClient) setRouteError(err error) {
	c.pendingMu.Lock()
	c.routeErr = err
	c.pendingMu.Unlock()
}

func (c *rpcClient) routeError() error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	return c.routeErr
}
