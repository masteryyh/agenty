package modelcall

import (
	"context"
	"fmt"
	"net/http"
)

type caller interface {
	Invoke(context.Context, ModelCallRequest) (*ModelCallResponse, error)
	Stream(context.Context, ModelCallRequest, ModelCallStreamHandler) (*ModelCallResponse, error)
}

type callConfig struct {
	httpClient    *http.Client
	streamHandler ModelCallStreamHandler
}

// Option modifies a single model call. No option is retained after Call
// returns.
type Option func(*callConfig) error

func WithHTTPClient(client *http.Client) Option {
	return func(config *callConfig) error {
		if client == nil {
			return invalidRequest("HTTP client must not be nil")
		}

		config.httpClient = client
		return nil
	}
}

func WithStreamHandler(handler ModelCallStreamHandler) Option {
	return func(config *callConfig) error {
		if handler == nil {
			return invalidRequest("stream handler must not be nil")
		}

		config.streamHandler = handler
		return nil
	}
}

// Call invokes a model once. It creates protocol clients only for the current
// request and neither reads nor changes session or provider business data.
func Call(
	ctx context.Context,
	model ModelCallConfig,
	request ModelCallRequest,
	options ...Option,
) (*ModelCallResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	config := callConfig{}
	for _, option := range options {
		if err := option(&config); err != nil {
			return nil, fmt.Errorf("modelcall: apply call option: %w", err)
		}
	}

	caller, err := newCaller(ctx, model, config)
	if err != nil {
		return nil, err
	}
	if err := validateRequest(request); err != nil {
		return nil, err
	}

	if config.streamHandler == nil {
		response, err := caller.Invoke(ctx, request)
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, fmt.Errorf("model call returned an empty response")
		}
		return response, nil
	}

	response, err := caller.Stream(ctx, request, config.streamHandler)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("model call returned an empty response")
	}
	return response, nil
}
