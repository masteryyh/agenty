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

// decodeParams unmarshals params into dst, treating an absent params payload as
// an empty object so handlers with no required fields still validate cleanly.
func decodeParams(params json.RawMessage, dst any) error {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	return json.Unmarshal(params, dst)
}

// toRPCError maps an application error to a JSON-RPC error with a structured
// code. Unclassified errors become internal errors.
func toRPCError(err error) *rpc.Error {
	var appErr *application.Error
	if errors.As(err, &appErr) {
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

// wrap adapts a service call's (result, error) return to a handler return,
// mapping any application error to a structured JSON-RPC error. Handlers must
// route service results through wrap (or toRPCError) so classification is not
// lost to the dispatcher's generic internal-error fallback.
func wrap(v any, err error) (any, error) {
	if err != nil {
		return nil, toRPCError(err)
	}
	return v, nil
}
