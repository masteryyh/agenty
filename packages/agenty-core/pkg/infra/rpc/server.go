package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
)

const defaultMaxLineBytes = 1 << 26

type Server struct {
	dispatcher   *Dispatcher
	in           io.Reader
	out          io.Writer
	logger       *slog.Logger
	mu           sync.Mutex
	maxLineBytes int
}

func NewServer(d *Dispatcher, in io.Reader, out io.Writer) *Server {
	return &Server{
		dispatcher:   d,
		in:           in,
		out:          out,
		logger:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		maxLineBytes: defaultMaxLineBytes,
	}
}

func (s *Server) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

func (s *Server) SetMaxLineBytes(max int) {
	if max > 0 {
		s.maxLineBytes = max
	}
}

type lineEvent struct {
	line     []byte
	tooLarge bool
}

func (s *Server) Serve(ctx context.Context) error {
	events := make(chan lineEvent)
	errCh := make(chan error, 1)

	go func() {
		reader := bufio.NewReaderSize(s.in, 65*1024)
		for {
			line, tooLarge, err := readLine(reader, s.maxLineBytes)

			if tooLarge {
				select {
				case events <- lineEvent{tooLarge: true}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
					return
				}
				errCh <- err
				return
			}
			if tooLarge {
				continue
			}

			select {
			case events <- lineEvent{line: line}:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case ev := <-events:
			if ev.tooLarge {
				s.write(response{
					JSONRPC: "2.0",
					ID:      json.RawMessage("null"),
					Error:   MessageTooLarge(s.maxLineBytes),
				})
				continue
			}
			s.handleLine(ctx, ev.line)
		}
	}
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			s.write(response{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   ParseError("rpc: parse error: " + err.Error()),
			})
			return
		}
		s.handleBatch(ctx, batch)
		return
	}

	resp, respond := s.handleSingle(ctx, line)
	if respond {
		s.write(resp)
	}
}

func (s *Server) handleSingle(ctx context.Context, line []byte) (response, bool) {
	if !json.Valid(line) {
		return response{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   ParseError("rpc: parse error: invalid JSON"),
		}, true
	}

	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return response{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   InvalidRequest("rpc: invalid request: " + err.Error()),
		}, true
	}

	if req.JSONRPC != "2.0" || req.Method == "" || !validRequestID(req.ID) {
		id := req.ID
		if !validRequestID(id) || len(id) == 0 {
			id = json.RawMessage("null")
		}

		return response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   InvalidRequest(`rpc: invalid request: jsonrpc must be "2.0" and method is required`),
		}, true
	}

	resp := s.dispatcher.dispatch(ctx, req)
	if req.isNotification() {
		return resp, false
	}
	return resp, true
}

func validRequestID(id json.RawMessage) bool {
	if len(id) == 0 || bytes.Equal(id, []byte("null")) {
		return true
	}
	var value any
	if err := json.Unmarshal(id, &value); err != nil {
		return false
	}
	switch value.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}

func (s *Server) handleBatch(ctx context.Context, batch []json.RawMessage) {
	if len(batch) == 0 {
		s.write(response{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   InvalidRequest("rpc: empty batch"),
		})
		return
	}

	responses := make([]response, 0, len(batch))
	for _, raw := range batch {
		resp, respond := s.handleSingle(ctx, raw)
		if respond {
			responses = append(responses, resp)
		}
	}

	if len(responses) == 0 {
		return
	}
	s.writeBatch(responses)
}

func (s *Server) write(resp response) {
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("rpc: failed to encode response", "error", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.out.Write(data)
	_, _ = s.out.Write([]byte("\n"))
}

func (s *Server) writeBatch(resps []response) {
	data, err := json.Marshal(resps)
	if err != nil {
		s.logger.Error("rpc: failed to encode batch response", "error", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.out.Write(data)
	_, _ = s.out.Write([]byte("\n"))
}

func readLine(r *bufio.Reader, max int) (line []byte, tooLarge bool, err error) {
	var buf []byte
	for {
		fragment, ferr := r.ReadSlice('\n')
		if len(fragment) > 0 {
			buf = append(buf, fragment...)
		}

		switch ferr {
		case nil:
			line := trimLineTerminator(buf)
			if len(line) > max {
				return nil, true, nil
			}
			return line, false, nil
		case bufio.ErrBufferFull:
			if len(buf) > max {
				return nil, true, drainLine(r)
			}
			continue
		default:
			if len(buf) > 0 {
				line := trimLineTerminator(buf)
				if len(line) > max {
					return nil, true, nil
				}
				return line, false, nil
			}
			return nil, false, ferr
		}
	}
}

func drainLine(r *bufio.Reader) error {
	for {
		_, ferr := r.ReadSlice('\n')
		switch ferr {
		case nil:
			return nil
		case bufio.ErrBufferFull:
			continue
		default:
			return ferr
		}
	}
}

func trimLineTerminator(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return b
}
