package rpc

// Chunked upload protocol
//
// A sender whose request params exceed the per-line cap (see
// defaultMaxLineBytes) cannot fit a single NDJSON line. Instead it uploads the
// raw params as base64-encoded shards via a small control protocol, then
// commits the session to fire the real method:
//
//	1. chunk.begin    -> {requestId, method, totalSize?, chunkCount?}
//	2. chunk.part  (x N) -> {requestId, index, data}   // data = base64(shard)
//	3. chunk.commit  -> {requestId}                    // response = real method's result
//	4. chunk.abort   -> {requestId}                    // optional cancel
//
// NDJSON is ordered and the server dispatches on a single goroutine, so a
// sender may pipeline begin + parts + commit without waiting for intermediate
// responses. The commit response carries the real method's result (or its
// structured error, with the real method's error code). Shards are base64 so
// any split point is safe and the payload survives JSON string escaping.
//
// Sessions live in process memory and are dropped on process exit; a sender
// must restart an interrupted upload. Idle sessions are reaped after the TTL.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	// defaultMaxChunkPayload caps the assembled payload of a single chunked
	// upload at 256 MiB (4x the per-line cap). It bounds memory a hostile or
	// buggy sender can pin via in-flight sessions.
	defaultMaxChunkPayload int64 = 1 << 28
	// defaultChunkTTL is how long an idle chunk session is kept before reaping.
	defaultChunkTTL = 5 * time.Minute
)

type chunkSession struct {
	method     string
	totalSize  int64 // 0 = undeclared
	chunkCount int   // 0 = undeclared
	parts      map[int][]byte
	received   int
	totalBytes int64
	createdAt  time.Time
	lastAt     time.Time
}

// ChunkAssembler holds in-flight chunked uploads and assembles them on commit.
type ChunkAssembler struct {
	d          *Dispatcher
	mu         sync.Mutex
	sessions   map[string]*chunkSession
	maxPayload int64
	ttl        time.Duration
	now        func() time.Time
}

// NewChunkAssembler returns an assembler that dispatches committed uploads via
// d. Defaults: 256 MiB max payload, 5 min idle TTL.
func NewChunkAssembler(d *Dispatcher) *ChunkAssembler {
	return &ChunkAssembler{
		d:          d,
		sessions:   make(map[string]*chunkSession),
		maxPayload: defaultMaxChunkPayload,
		ttl:        defaultChunkTTL,
		now:        time.Now,
	}
}

// SetMaxPayload overrides the assembled-payload cap. Intended for tests.
func (a *ChunkAssembler) SetMaxPayload(n int64) {
	if n > 0 {
		a.maxPayload = n
	}
}

// SetTTL overrides the idle-session TTL. Intended for tests.
func (a *ChunkAssembler) SetTTL(d time.Duration) {
	if d > 0 {
		a.ttl = d
	}
}

// SetNow overrides the clock used for session timestamps. Intended for tests.
func (a *ChunkAssembler) SetNow(f func() time.Time) {
	if f != nil {
		a.now = f
	}
}

// Begin opens a chunked upload session for method under requestID.
func (a *ChunkAssembler) Begin(method, requestID string, totalSize int64, chunkCount int) error {
	if requestID == "" {
		return InvalidParams("rpc: chunk.begin requires requestId")
	}
	if method == "" {
		return InvalidParams("rpc: chunk.begin requires method")
	}
	if totalSize < 0 {
		return InvalidParams("rpc: chunk.begin totalSize must not be negative")
	}
	if chunkCount < 0 {
		return InvalidParams("rpc: chunk.begin chunkCount must not be negative")
	}
	if _, ok := a.d.handlers[method]; !ok {
		return MethodNotFound(fmt.Sprintf("rpc: method %q not found", method))
	}
	if totalSize > a.maxPayload {
		return NewError(ErrCodeChunkPayloadTooLarge, fmt.Sprintf("rpc: payload size %d exceeds max %d", totalSize, a.maxPayload), nil)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.sessions[requestID]; ok {
		return NewError(ErrCodeAlreadyExists, "rpc: chunk session "+requestID+" already exists", nil)
	}
	now := a.now()
	a.sessions[requestID] = &chunkSession{
		method:     method,
		totalSize:  totalSize,
		chunkCount: chunkCount,
		parts:      make(map[int][]byte),
		createdAt:  now,
		lastAt:     now,
	}
	return nil
}

// Part appends one base64-encoded shard at index to the named session.
func (a *ChunkAssembler) Part(requestID string, index int, data string) error {
	if index < 0 {
		return InvalidParams("rpc: chunk part index must not be negative")
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return ParseError("rpc: chunk part data is not valid base64: " + err.Error())
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	sess, ok := a.sessions[requestID]
	if !ok {
		return NewError(ErrCodeNotFound, "rpc: chunk session "+requestID+" not found", nil)
	}
	if _, dup := sess.parts[index]; dup {
		return InvalidParams(fmt.Sprintf("rpc: chunk part %d already received", index))
	}
	if sess.chunkCount > 0 && index >= sess.chunkCount {
		return InvalidParams(fmt.Sprintf("rpc: chunk part index %d exceeds declared chunkCount %d", index, sess.chunkCount))
	}
	if sess.totalBytes+int64(len(decoded)) > a.maxPayload {
		delete(a.sessions, requestID)
		return NewError(ErrCodeChunkPayloadTooLarge, fmt.Sprintf("rpc: accumulated payload exceeds max %d", a.maxPayload), nil)
	}
	sess.parts[index] = decoded
	sess.received++
	sess.totalBytes += int64(len(decoded))
	sess.lastAt = a.now()
	return nil
}

// Commit assembles the session's shards in index order, validates the result
// as JSON, and returns the target method name and its raw params. The session
// is removed regardless of outcome; a failed commit must be restarted with a
// new chunk.begin.
func (a *ChunkAssembler) Commit(ctx context.Context, requestID string) (string, json.RawMessage, error) {
	a.mu.Lock()
	sess, ok := a.sessions[requestID]
	if ok {
		delete(a.sessions, requestID)
	}
	a.mu.Unlock()
	if !ok {
		return "", nil, NewError(ErrCodeNotFound, "rpc: chunk session "+requestID+" not found", nil)
	}

	if sess.chunkCount > 0 && sess.received != sess.chunkCount {
		return "", nil, InvalidParams(fmt.Sprintf("rpc: chunk session %s incomplete: received %d of %d parts", requestID, sess.received, sess.chunkCount))
	}

	indices := make([]int, 0, len(sess.parts))
	for i := range sess.parts {
		indices = append(indices, i)
	}
	slices.Sort(indices)
	assembled := make([]byte, 0, sess.totalBytes)
	for position, i := range indices {
		if i != position {
			return "", nil, InvalidParams(fmt.Sprintf("rpc: chunk session %s is missing part %d", requestID, position))
		}
		assembled = append(assembled, sess.parts[i]...)
	}

	if sess.totalSize > 0 && int64(len(assembled)) != sess.totalSize {
		return "", nil, InvalidParams(fmt.Sprintf("rpc: chunk session %s size mismatch: got %d want %d", requestID, len(assembled), sess.totalSize))
	}
	if !json.Valid(assembled) {
		return "", nil, ParseError("rpc: assembled chunk payload is not valid JSON")
	}
	return sess.method, json.RawMessage(assembled), nil
}

// Abort drops an in-flight session.
func (a *ChunkAssembler) Abort(requestID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.sessions[requestID]; !ok {
		return NewError(ErrCodeNotFound, "rpc: chunk session "+requestID+" not found", nil)
	}
	delete(a.sessions, requestID)
	return nil
}

// StartCleanup launches a goroutine that reaps idle sessions older than the
// TTL. It returns when ctx is cancelled.
func (a *ChunkAssembler) StartCleanup(ctx context.Context) {
	go func() {
		interval := a.ttl / 2
		if interval < time.Second {
			interval = time.Second
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.sweep()
			}
		}
	}()
}

func (a *ChunkAssembler) sweep() {
	cutoff := a.now().Add(-a.ttl)
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, sess := range a.sessions {
		if sess.lastAt.Before(cutoff) {
			delete(a.sessions, id)
		}
	}
}

// RegisterChunkHandlers registers chunk.begin/part/commit/abort on d.
func RegisterChunkHandlers(d *Dispatcher, a *ChunkAssembler) {
	d.Register("chunk.begin", a.beginHandler)
	d.Register("chunk.part", a.partHandler)
	d.Register("chunk.commit", a.commitHandler)
	d.Register("chunk.abort", a.abortHandler)
}

func (a *ChunkAssembler) beginHandler(_ context.Context, params json.RawMessage) (any, error) {
	var p struct {
		RequestID  string `json:"requestId"`
		Method     string `json:"method"`
		TotalSize  int64  `json:"totalSize,omitempty"`
		ChunkCount int    `json:"chunkCount,omitempty"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, InvalidParams("invalid params: " + err.Error())
	}
	if err := a.Begin(p.Method, p.RequestID, p.TotalSize, p.ChunkCount); err != nil {
		return nil, err
	}
	return map[string]any{"requestId": p.RequestID, "accepted": true}, nil
}

func (a *ChunkAssembler) partHandler(_ context.Context, params json.RawMessage) (any, error) {
	var p struct {
		RequestID string `json:"requestId"`
		Index     int    `json:"index"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, InvalidParams("invalid params: " + err.Error())
	}
	if err := a.Part(p.RequestID, p.Index, p.Data); err != nil {
		return nil, err
	}
	return map[string]any{"requestId": p.RequestID, "index": p.Index, "received": true}, nil
}

func (a *ChunkAssembler) commitHandler(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, InvalidParams("invalid params: " + err.Error())
	}
	method, rawParams, err := a.Commit(ctx, p.RequestID)
	if err != nil {
		return nil, err
	}
	// Dispatch the real method in-process and surface its result/error
	// verbatim so the commit response is indistinguishable from a direct call.
	resp := a.d.dispatch(ctx, request{JSONRPC: "2.0", Method: method, Params: rawParams})
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

func (a *ChunkAssembler) abortHandler(_ context.Context, params json.RawMessage) (any, error) {
	var p struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, InvalidParams("invalid params: " + err.Error())
	}
	if err := a.Abort(p.RequestID); err != nil {
		return nil, err
	}
	return map[string]any{"requestId": p.RequestID, "aborted": true}, nil
}
