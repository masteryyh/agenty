package modelcall

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func TestCallValidatesStatelessInputsBeforeNetwork(t *testing.T) {
	validModel := ModelCallConfig{
		APIType:   APIOpenAI,
		APIKey:    "test-key",
		ModelCode: "test-model",
	}
	validRequest := ModelCallRequest{MaxOutputTokens: 1}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		model   ModelCallConfig
		request ModelCallRequest
		options []Option
		want    error
	}{
		{
			name:    "canceled context",
			ctx:     canceled,
			model:   ModelCallConfig{},
			request: ModelCallRequest{},
			want:    context.Canceled,
		},
		{
			name:    "missing API key",
			ctx:     t.Context(),
			model:   ModelCallConfig{APIType: APIOpenAI, ModelCode: "test-model"},
			request: validRequest,
			want:    ErrInvalidRequest,
		},
		{
			name:    "missing model code",
			ctx:     t.Context(),
			model:   ModelCallConfig{APIType: APIOpenAI, APIKey: "test-key"},
			request: validRequest,
			want:    ErrInvalidRequest,
		},
		{
			name: "unsupported API type",
			ctx:  t.Context(),
			model: ModelCallConfig{
				APIType:   APIType("unknown"),
				APIKey:    "test-key",
				ModelCode: "test-model",
			},
			request: validRequest,
			want:    ErrUnsupportedAPI,
		},
		{
			name:    "invalid request",
			ctx:     t.Context(),
			model:   validModel,
			request: ModelCallRequest{},
			want:    ErrInvalidRequest,
		},
		{
			name:    "nil stream handler",
			ctx:     t.Context(),
			model:   validModel,
			request: validRequest,
			options: []Option{WithStreamHandler(nil)},
			want:    ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Call(tt.ctx, tt.model, tt.request, tt.options...)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Call() error = %v, want errors.Is(_, %v)", err, tt.want)
			}
		})
	}
}

func TestCallStreamsMalformedOpenAIResponsesToolArgumentsForMiddlewareValidation(t *testing.T) {
	t.Parallel()

	const toolCall = `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{\"commands\"}","status":"completed"}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(writer, "event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":0,\"sequence_number\":1,\"item\":%s}\n\n", toolCall)
		_, _ = fmt.Fprintf(writer, "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_1\",\"model\":\"test-model\",\"status\":\"completed\",\"output\":[%s],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n", toolCall)
	}))
	t.Cleanup(server.Close)

	var events []ModelCallStreamEvent
	response, err := Call(t.Context(), ModelCallConfig{
		BaseURL: server.URL, APIType: APIOpenAI, APIKey: "test-key", ModelCode: "test-model",
	}, ModelCallRequest{MaxOutputTokens: 128}, WithStreamHandler(func(event ModelCallStreamEvent) error {
		events = append(events, event)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || len(response.Content) != 1 {
		t.Fatalf("response = %#v", response)
	}
	call := response.Content[0].(conversation.ToolUseBlock)
	if string(call.Input) != `{}` || call.InputError == "" {
		t.Fatalf("response tool input = %q, error = %q", call.Input, call.InputError)
	}
	if len(events) != 2 || events[0].Type != ModelCallStreamEventToolUseDone ||
		string(events[0].ToolInput) != `{"commands"}` || events[1].Type != ModelCallStreamEventCompleted {
		t.Fatalf("stream events = %#v", events)
	}
}
