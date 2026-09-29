//go:build e2e

package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type agentyClient struct {
	process *coreProcess
	events  chan SessionEvent

	streamMu     sync.Mutex
	streamWriter *io.PipeWriter
	streamCancel context.CancelFunc
	streamReady  map[string]chan struct{}
	streamErrors chan error
}

type streamFrame struct {
	Type  string
	Topic string
	Event json.RawMessage
}

func newAgentyClient(process *coreProcess) *agentyClient {
	return &agentyClient{
		process:      process,
		events:       make(chan SessionEvent, 1024),
		streamReady:  make(map[string]chan struct{}),
		streamErrors: make(chan error, 1),
	}
}

func (c *agentyClient) InitializeAlready(ctx context.Context) (InitializeResult, error) {
	var result InitializeResult
	err := c.process.Request(ctx, http.MethodGet, "/v1/initialization", nil, &result)
	return result, err
}

func (c *agentyClient) CompleteInitialization(ctx context.Context, providerCode, modelCode string) (InitializeResult, error) {
	var result InitializeResult
	err := c.process.Request(ctx, http.MethodPost, "/v1/initialization", map[string]any{
		"providerCode": providerCode,
		"modelCode":    modelCode,
	}, &result)
	return result, err
}

func (c *agentyClient) CreateProvider(ctx context.Context, input ProviderCreateInput) (Provider, error) {
	var result Provider
	err := c.process.Request(ctx, http.MethodPost, "/v1/providers", input, &result)
	return result, err
}

func (c *agentyClient) GetProvider(ctx context.Context, code string) (Provider, error) {
	var result Provider
	err := c.process.Request(ctx, http.MethodGet, "/v1/providers/"+url.PathEscape(code), nil, &result)
	return result, err
}

func (c *agentyClient) ListProviders(ctx context.Context) ([]Provider, error) {
	var result []Provider
	err := c.process.Request(ctx, http.MethodGet, "/v1/providers", nil, &result)
	return result, err
}

func (c *agentyClient) ListProviderModels(ctx context.Context, providerCode string) ([]AvailableModel, error) {
	var result []AvailableModel
	err := c.process.Request(ctx, http.MethodGet, "/v1/providers/"+url.PathEscape(providerCode)+"/models", nil, &result)
	return result, err
}

func (c *agentyClient) UpdateProvider(ctx context.Context, input ProviderUpdateInput) (Provider, error) {
	var result Provider
	err := c.process.Request(ctx, http.MethodPatch, "/v1/providers/"+url.PathEscape(input.Code), input, &result)
	return result, err
}

func (c *agentyClient) DeleteProvider(ctx context.Context, code string) (DeleteResult, error) {
	var result DeleteResult
	err := c.process.Request(ctx, http.MethodDelete, "/v1/providers/"+url.PathEscape(code), nil, &result)
	return result, err
}

func (c *agentyClient) AddModel(ctx context.Context, input ModelInput) (Provider, error) {
	var result Provider
	err := c.process.Request(ctx, http.MethodPost, "/v1/providers/"+url.PathEscape(input.ProviderCode)+"/models", input, &result)
	return result, err
}

func (c *agentyClient) RemoveModel(ctx context.Context, providerCode, modelCode string) (Provider, error) {
	var result Provider
	path := "/v1/providers/" + url.PathEscape(providerCode) + "/models/" + url.PathEscape(modelCode)
	err := c.process.Request(ctx, http.MethodDelete, path, nil, &result)
	return result, err
}

func (c *agentyClient) CreateSession(ctx context.Context, input SessionCreateInput) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodPost, "/v1/sessions", input, &result)
	return result, err
}

func (c *agentyClient) GetSession(ctx context.Context, id string) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(id), nil, &result)
	return result, err
}

func (c *agentyClient) ListSessions(ctx context.Context, input SessionListInput) ([]SessionSummary, error) {
	query := url.Values{}
	if input.Limit != 0 {
		query.Set("limit", fmt.Sprint(input.Limit))
	}
	if input.Offset != 0 {
		query.Set("offset", fmt.Sprint(input.Offset))
	}
	path := "/v1/sessions"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var result []SessionSummary
	err := c.process.Request(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (c *agentyClient) DeleteSession(ctx context.Context, id string) (DeleteResult, error) {
	var result DeleteResult
	err := c.process.Request(ctx, http.MethodDelete, "/v1/sessions/"+url.PathEscape(id), nil, &result)
	return result, err
}

func (c *agentyClient) SetSessionTitle(ctx context.Context, id, title string) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodPut, "/v1/sessions/"+url.PathEscape(id)+"/title", map[string]any{"title": title}, &result)
	return result, err
}

func (c *agentyClient) SetSessionModel(ctx context.Context, id string, model ModelRef) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodPut, "/v1/sessions/"+url.PathEscape(id)+"/model", map[string]any{
		"providerCode": model.ProviderCode,
		"modelCode":    model.ModelCode,
	}, &result)
	return result, err
}

func (c *agentyClient) SetSessionReasoningEffort(ctx context.Context, id, reasoningEffort string) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodPut, "/v1/sessions/"+url.PathEscape(id)+"/reasoning-effort", map[string]any{
		"reasoningEffort": reasoningEffort,
	}, &result)
	return result, err
}

func (c *agentyClient) SetSessionCwd(ctx context.Context, id string, cwd *string) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodPut, "/v1/sessions/"+url.PathEscape(id)+"/cwd", map[string]any{"cwd": cwd}, &result)
	return result, err
}

func (c *agentyClient) EnableCodexMode(ctx context.Context, id string) (Session, error) {
	var result Session
	err := c.process.Request(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/codex-mode", nil, &result)
	return result, err
}

func (c *agentyClient) StartSession(ctx context.Context, id string, content []ContentInput) (ExecutionStart, error) {
	if err := c.subscribeSession(ctx, id); err != nil {
		return ExecutionStart{}, err
	}
	var result ExecutionStart
	err := c.process.Request(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/rounds", map[string]any{"content": content}, &result)
	return result, err
}

func (c *agentyClient) StopSession(ctx context.Context, id string, roundIDs ...string) (ExecutionStop, error) {
	roundID := ""
	if len(roundIDs) > 0 {
		roundID = roundIDs[0]
	} else {
		session, err := c.GetSession(ctx, id)
		if err != nil {
			return ExecutionStop{}, err
		}
		for index := len(session.Rounds) - 1; index >= 0; index-- {
			if session.Rounds[index].Status == "running" {
				roundID = session.Rounds[index].ID
				break
			}
		}
	}
	if roundID == "" {
		return ExecutionStop{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_params", Message: "no active round"}
	}
	var result ExecutionStop
	path := "/v1/sessions/" + url.PathEscape(id) + "/rounds/" + url.PathEscape(roundID) + "/cancel"
	err := c.process.Request(ctx, http.MethodPost, path, nil, &result)
	return result, err
}

func (c *agentyClient) CompactSession(ctx context.Context, id string) (map[string]any, error) {
	var result map[string]any
	err := c.process.Request(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/compact", nil, &result)
	return result, err
}

func (c *agentyClient) subscribeSession(ctx context.Context, id string) error {
	topic := "session:" + id
	c.streamMu.Lock()
	if c.streamReady[topic] != nil {
		ready := c.streamReady[topic]
		c.streamMu.Unlock()
		select {
		case <-ready:
			return nil
		case err := <-c.streamErrors:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ready := make(chan struct{})
	c.streamReady[topic] = ready
	started := c.streamWriter != nil
	c.streamMu.Unlock()

	if !started {
		if err := c.openStream(); err != nil {
			return err
		}
	}
	command, err := json.Marshal(map[string]string{"type": "subscribe", "topic": topic})
	if err != nil {
		return err
	}
	c.streamMu.Lock()
	writer := c.streamWriter
	c.streamMu.Unlock()
	if writer == nil {
		return fmt.Errorf("HTTP event stream is not available")
	}
	if _, err := writer.Write(append(command, '\n')); err != nil {
		return fmt.Errorf("subscribe to %s: %w", topic, err)
	}
	select {
	case <-ready:
		return nil
	case err := <-c.streamErrors:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *agentyClient) openStream() error {
	c.streamMu.Lock()
	if c.streamWriter != nil {
		c.streamMu.Unlock()
		return nil
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	request, err := http.NewRequestWithContext(streamCtx, http.MethodPost, "http://agenty.local/v1/stream", reader)
	if err != nil {
		c.streamMu.Unlock()
		cancel()
		return err
	}
	request.Header.Set("Content-Type", "application/x-ndjson")
	c.streamCancel = cancel
	c.streamWriter = writer
	c.streamMu.Unlock()

	response, err := c.process.client.Do(request)
	if err != nil {
		cancel()
		return fmt.Errorf("open HTTP/2 event stream: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("event stream returned HTTP %d: %s", response.StatusCode, string(body))
	}
	go c.readStream(response.Body)
	select {
	case err := <-c.streamErrors:
		return err
	default:
		return nil
	}
}

func (c *agentyClient) readStream(body io.ReadCloser) {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var frame streamFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			c.failStream(fmt.Errorf("decode event stream frame: %w", err))
			return
		}
		if frame.Type == "snapshot" || frame.Type == "ready" {
			c.streamMu.Lock()
			ready := c.streamReady[frame.Topic]
			c.streamMu.Unlock()
			if ready != nil {
				select {
				case <-ready:
				default:
					close(ready)
				}
			}
		}
		if frame.Type == "event" && len(frame.Event) > 0 {
			var envelope struct {
				Kind string
			}
			if err := json.Unmarshal(frame.Event, &envelope); err != nil {
				c.failStream(fmt.Errorf("decode event stream event: %w", err))
				return
			}
			if envelope.Kind == "session" {
				var event SessionEvent
				if err := json.Unmarshal(frame.Event, &event); err != nil {
					c.failStream(fmt.Errorf("decode session event: %w", err))
					return
				}
				select {
				case c.events <- event:
				default:
					c.failStream(fmt.Errorf("session event test queue is full"))
					return
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		c.failStream(err)
	}
}

func (c *agentyClient) failStream(err error) {
	select {
	case c.streamErrors <- err:
	default:
	}
}

func (c *agentyClient) Close() {
	c.streamMu.Lock()
	writer := c.streamWriter
	cancel := c.streamCancel
	c.streamWriter = nil
	c.streamCancel = nil
	c.streamMu.Unlock()
	if writer != nil {
		_ = writer.Close()
	}
	if cancel != nil {
		cancel()
	}
}

func (c *agentyClient) WaitForRoundStatus(ctx context.Context, sessionID, roundID, want string) (Session, error) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		session, err := c.GetSession(ctx, sessionID)
		if err != nil {
			return Session{}, err
		}
		for _, round := range session.Rounds {
			if round.ID != roundID {
				continue
			}
			if round.Status == want {
				return session, nil
			}
			if round.Status == "completed" || round.Status == "failed" || round.Status == "cancelled" {
				errorText := ""
				if round.Error != nil {
					errorText = *round.Error
				}
				return Session{}, fmt.Errorf("session %s round %s reached %q while waiting for %q: %s",
					sessionID, roundID, round.Status, want, errorText)
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return Session{}, fmt.Errorf("session %s round %s did not reach %q: %w", sessionID, roundID, want, ctx.Err())
		}
	}
}

func (c *agentyClient) WaitForRoundEvent(ctx context.Context, sessionID, roundID, want string) (SessionEvent, error) {
	for {
		select {
		case event := <-c.events:
			if event.SessionID != sessionID || event.RoundID != roundID {
				continue
			}
			if event.Type == want {
				return event, nil
			}
		case err := <-c.streamErrors:
			return SessionEvent{}, err
		case <-ctx.Done():
			return SessionEvent{}, fmt.Errorf("session %s round %s did not emit %q: %w", sessionID, roundID, want, ctx.Err())
		}
	}
}
