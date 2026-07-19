package conn

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	json "github.com/bytedance/sonic"
)

type HTTPRequest struct {
	URL     string
	Params  map[string]string
	Headers map[string]string
	Body    any
}

type SSEEvent struct {
	Data string
	Err  error
}

func buildRequest(ctx context.Context, method string, req HTTPRequest) (*http.Request, error) {
	rawURL := req.URL
	if len(req.Params) > 0 {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		q := u.Query()
		for k, v := range req.Params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}

	var bodyReader io.Reader
	if req.Body != nil {
		data, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	return httpReq, nil
}

func doRequest[T any](ctx context.Context, method string, req HTTPRequest) (T, error) {
	var zero T
	httpReq, err := buildRequest(ctx, method, req)
	if err != nil {
		return zero, err
	}

	resp, err := GetHTTPClient().Do(httpReq)
	if err != nil {
		return zero, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func Get[T any](ctx context.Context, req HTTPRequest) (T, error) {
	return doRequest[T](ctx, http.MethodGet, req)
}

func Post[T any](ctx context.Context, req HTTPRequest) (T, error) {
	return doRequest[T](ctx, http.MethodPost, req)
}

func Put[T any](ctx context.Context, req HTTPRequest) (T, error) {
	return doRequest[T](ctx, http.MethodPut, req)
}

func Patch[T any](ctx context.Context, req HTTPRequest) (T, error) {
	return doRequest[T](ctx, http.MethodPatch, req)
}

func Delete[T any](ctx context.Context, req HTTPRequest) (T, error) {
	return doRequest[T](ctx, http.MethodDelete, req)
}

func PostSSE(ctx context.Context, req HTTPRequest) (<-chan SSEEvent, error) {
	httpReq, err := buildRequest(ctx, http.MethodPost, req)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := GetHTTPClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan SSEEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 512*1024), 512*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			select {
			case ch <- SSEEvent{Data: data}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case ch <- SSEEvent{Err: err}:
			case <-ctx.Done():
			}
		}
	}()

	return ch, nil
}
