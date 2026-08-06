// Package adapter wires application services to the JSON-RPC protocol layer:
// it decodes params, invokes the matching use-case, and maps application
// errors to structured JSON-RPC errors.
package adapter

import (
	"encoding/json"
	"errors"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func decodeParams(params json.RawMessage, dst any) error {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	return json.Unmarshal(params, dst)
}

func toRPCError(err error) *rpc.Error {
	if appErr, ok := errors.AsType[*application.Error](err); ok {
		switch appErr.Code {
		case application.CodeNotFound:
			return rpc.NewError(rpc.ErrCodeNotFound, appErr.Message, nil)
		case application.CodeAlreadyExists:
			return rpc.NewError(rpc.ErrCodeAlreadyExists, appErr.Message, nil)
		case application.CodeValidation:
			return rpc.NewError(rpc.ErrCodeInvalidParams, appErr.Message, nil)
		default:
			return rpc.NewError(rpc.ErrCodeInternalError, appErr.Message, nil)
		}
	}
	return rpc.NewError(rpc.ErrCodeInternalError, err.Error(), nil)
}

func wrap(v any, err error) (any, error) {
	if err != nil {
		return nil, toRPCError(err)
	}
	return v, nil
}
