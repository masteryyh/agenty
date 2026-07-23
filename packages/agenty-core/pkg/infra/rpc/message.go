package rpc

import (
	"encoding/json"
	"fmt"
)

type ID = json.RawMessage

type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("rpc: [%d] %s", e.Code, e.Message)
}

func NewError(code int, message string, data any) *Error {
	e := &Error{Code: code, Message: message}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return &Error{
				Code:    ErrCodeInternalError,
				Message: "rpc: failed to marshal error data",
			}
		}
		e.Data = raw
	}
	return e
}

func ParseError(message string) *Error {
	return &Error{Code: ErrCodeParseError, Message: message}
}

func InvalidRequest(message string) *Error {
	return &Error{Code: ErrCodeInvalidRequest, Message: message}
}

func MethodNotFound(message string) *Error {
	return &Error{Code: ErrCodeMethodNotFound, Message: message}
}

func InvalidParams(message string) *Error {
	return &Error{Code: ErrCodeInvalidParams, Message: message}
}

func InternalError(message string) *Error {
	return &Error{Code: ErrCodeInternalError, Message: message}
}

// MessageTooLarge reports that an inbound line exceeded the per-line byte cap.
// The cap is echoed in data so the sender knows the limit and can switch to
// the chunked upload protocol instead of giving up.
func MessageTooLarge(max int) *Error {
	return NewError(ErrCodeMessageTooLarge, "rpc: message too large", map[string]any{"maxLineBytes": max})
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func (r request) isNotification() bool {
	return len(r.ID) == 0
}
