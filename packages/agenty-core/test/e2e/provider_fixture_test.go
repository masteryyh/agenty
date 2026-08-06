//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type providerRequest struct {
	Call   int
	Method string
	Path   string
	Header http.Header
	Body   map[string]any
}

type providerReply struct {
	Status        int
	Body          string
	WaitForCancel bool
}

type providerResponder func(providerRequest) providerReply

type providerFixture struct {
	server   *httptest.Server
	requests chan providerRequest
	calls    atomic.Int64
}

func newProviderFixture(t *testing.T, responder providerResponder) *providerFixture {
	t.Helper()

	fixture := &providerFixture{requests: make(chan providerRequest, 64)}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode provider request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}

		recorded := providerRequest{
			Call:   int(fixture.calls.Add(1)),
			Method: request.Method,
			Path:   request.URL.Path,
			Header: request.Header.Clone(),
			Body:   body,
		}
		fixture.requests <- recorded
		reply := responder(recorded)
		if reply.WaitForCancel {
			<-request.Context().Done()
			return
		}

		status := reply.Status
		if status == 0 {
			status = http.StatusOK
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		if reply.Body != "" {
			if _, err := writer.Write([]byte(reply.Body)); err != nil {
				t.Errorf("write provider response: %v", err)
			}
		}
	}))
	t.Cleanup(fixture.server.Close)

	return fixture
}

func (f *providerFixture) BaseURL(apiType string) string {
	if apiType == "openai" || apiType == "openai_completions" {
		return f.server.URL + "/v1"
	}
	return f.server.URL
}

func providerSuccess(apiType, text string, call int) providerReply {
	switch apiType {
	case "openai":
		return providerReply{Body: fmt.Sprintf(`{
			"id":"resp_%d","object":"response","created_at":1,"model":"model-e2e","status":"completed",
			"output":[{
				"type":"message","id":"msg_%d","role":"assistant","status":"completed",
				"content":[{"type":"output_text","text":%q,"annotations":[]}]
			}],
			"usage":{
				"input_tokens":2,"output_tokens":3,"total_tokens":5,
				"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}
			}
		}`, call, call, text)}
	case "openai_completions":
		return providerReply{Body: fmt.Sprintf(`{
			"id":"chat_%d","object":"chat.completion","created":1,"model":"model-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`, call, text)}
	case "anthropic":
		return providerReply{Body: fmt.Sprintf(`{
			"id":"msg_%d","model":"model-e2e","role":"assistant","type":"message","stop_reason":"end_turn",
			"content":[{"type":"text","text":%q}],
			"usage":{"input_tokens":2,"output_tokens":3}
		}`, call, text)}
	case "gemini":
		return providerReply{Body: fmt.Sprintf(`{
			"responseId":"gemini_%d","modelVersion":"model-e2e",
			"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":%q}]}}],
			"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}
		}`, call, text)}
	default:
		return providerReply{Status: http.StatusBadRequest, Body: `{"error":"unsupported fixture api"}`}
	}
}

func requestMatchesAPI(request providerRequest, apiType string) bool {
	switch apiType {
	case "openai":
		return request.Method == http.MethodPost && request.Path == "/v1/responses"
	case "openai_completions":
		return request.Method == http.MethodPost && request.Path == "/v1/chat/completions"
	case "anthropic":
		return request.Method == http.MethodPost && request.Path == "/v1/messages"
	case "gemini":
		return request.Method == http.MethodPost && strings.HasSuffix(request.Path, ":generateContent")
	default:
		return false
	}
}
