package modelcall

import (
	"context"
	"errors"
	"testing"
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
