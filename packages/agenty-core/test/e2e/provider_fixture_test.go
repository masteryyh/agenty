//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
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
		streaming, _ := body["stream"].(bool)
		streaming = streaming || strings.Contains(recorded.Path, ":streamGenerateContent")
		if streaming && status == http.StatusOK {
			if err := writeProviderStream(writer, recorded.Path, reply.Body, recorded.Call); err != nil {
				t.Errorf("write provider stream: %v", err)
			}
			return
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

func writeProviderStream(writer http.ResponseWriter, path, body string, call int) error {
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return writeOpenAIChatStream(writer, body, call)
	case strings.HasSuffix(path, "/responses"):
		return writeOpenAIResponsesStream(writer, body)
	case strings.HasSuffix(path, "/messages"):
		return writeAnthropicStream(writer, body, call)
	case strings.Contains(path, ":streamGenerateContent"):
		return writeGoogleStream(writer, body)
	default:
		return fmt.Errorf("unsupported streaming fixture path %q", path)
	}
}

func writeOpenAIChatStream(writer http.ResponseWriter, body string, call int) error {
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &completion); err != nil {
		return fmt.Errorf("decode fixture completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return fmt.Errorf("fixture completion has no choices")
	}

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	frames := []string{
		fmt.Sprintf(`{"id":"chat_%d","object":"chat.completion.chunk","created":1,"model":"model-e2e","choices":[{"index":0,"delta":{"role":"assistant","content":%q},"finish_reason":null}]}`, call, completion.Choices[0].Message.Content),
		fmt.Sprintf(`{"id":"chat_%d","object":"chat.completion.chunk","created":1,"model":"model-e2e","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, call),
		"[DONE]",
	}
	for _, frame := range frames {
		if _, err := fmt.Fprintf(writer, "data: %s\n\n", frame); err != nil {
			return err
		}
	}
	return nil
}

func writeOpenAIResponsesStream(writer http.ResponseWriter, body string) error {
	compact, err := compactJSON(body)
	if err != nil {
		return err
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	_, err = fmt.Fprintf(writer, "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":%s}\n\n", compact)
	return err
}

func writeAnthropicStream(writer http.ResponseWriter, body string, call int) error {
	var message struct {
		Model   string `json:"model"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(body), &message); err != nil {
		return fmt.Errorf("decode anthropic fixture message: %w", err)
	}
	if len(message.Content) == 0 {
		return fmt.Errorf("anthropic fixture message has no content")
	}

	events := []struct {
		name string
		data string
	}{
		{name: "message_start", data: fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_%d","type":"message","role":"assistant","content":[],"model":%q,"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":2,"output_tokens":0}}}`, call, message.Model)},
		{name: "content_block_start", data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{name: "content_block_delta", data: fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`, message.Content[0].Text)},
		{name: "content_block_stop", data: `{"type":"content_block_stop","index":0}`},
		{name: "message_delta", data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}`},
		{name: "message_stop", data: `{"type":"message_stop"}`},
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	for _, event := range events {
		if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.name, event.data); err != nil {
			return err
		}
	}
	return nil
}

func writeGoogleStream(writer http.ResponseWriter, body string) error {
	compact, err := compactJSON(body)
	if err != nil {
		return err
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	_, err = fmt.Fprintf(writer, "data: %s\n\n", compact)
	return err
}

func compactJSON(value string) (string, error) {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return "", fmt.Errorf("decode fixture JSON: %w", err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("encode fixture JSON: %w", err)
	}
	return string(encoded), nil
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
		return request.Method == http.MethodPost && (strings.HasSuffix(request.Path, ":generateContent") || strings.HasSuffix(request.Path, ":streamGenerateContent"))
	default:
		return false
	}
}

func providerToolNames(request providerRequest, apiType string) []string {
	tools, _ := request.Body["tools"].([]any)
	names := []string{}
	for _, rawTool := range tools {
		tool, _ := rawTool.(map[string]any)
		switch apiType {
		case "openai", "anthropic":
			if name, ok := tool["name"].(string); ok {
				names = append(names, name)
			}
		case "openai_completions":
			function, _ := tool["function"].(map[string]any)
			if name, ok := function["name"].(string); ok {
				names = append(names, name)
			}
		case "gemini":
			declarations, _ := tool["functionDeclarations"].([]any)
			for _, rawDeclaration := range declarations {
				declaration, _ := rawDeclaration.(map[string]any)
				if name, ok := declaration["name"].(string); ok {
					names = append(names, name)
				}
			}
		}
	}
	sort.Strings(names)
	return names
}
