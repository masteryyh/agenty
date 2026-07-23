package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type Handler func(ctx context.Context, params json.RawMessage) (any, error)

type Dispatcher struct {
	handlers map[string]Handler
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]Handler)}
}

func (d *Dispatcher) Register(method string, h Handler) {
	d.handlers[method] = h
}

func (d *Dispatcher) dispatch(ctx context.Context, req request) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}

	h, ok := d.handlers[req.Method]
	if !ok {
		resp.Error = MethodNotFound(fmt.Sprintf("rpc: method %q not found", req.Method))
		return resp
	}

	result, err := h(ctx, req.Params)
	if err != nil {
		resp.Error = wrapError(err)
		return resp
	}

	raw, err := json.Marshal(result)
	if err != nil {
		resp.Error = InternalError(fmt.Sprintf("rpc: failed to marshal result: %v", err))
		return resp
	}
	resp.Result = raw
	return resp
}

func wrapError(err error) *Error {
	if rpcErr, ok := errors.AsType[*Error](err); ok {
		return rpcErr
	}
	return InternalError(err.Error())
}
