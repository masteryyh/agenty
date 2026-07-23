package application

import "fmt"

// Code classifies an application-level error so the interface layer can map it
// to a structured JSON-RPC error code instead of a generic internal error.
type Code int

const (
	CodeInternal Code = iota
	CodeNotFound
	CodeAlreadyExists
	CodeValidation
)

func (c Code) String() string {
	switch c {
	case CodeNotFound:
		return "not_found"
	case CodeAlreadyExists:
		return "already_exists"
	case CodeValidation:
		return "validation"
	default:
		return "internal"
	}
}

// Error is an application-layer error carrying a classification Code. The
// adapter layer maps Code to a JSON-RPC error code.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("application: %s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("application: %s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// NewError builds an Error without a cause.
func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError builds an Error that wraps cause.
func WrapError(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// Convenience constructors for the common classifications.
func NotFound(message string) *Error      { return NewError(CodeNotFound, message) }
func AlreadyExists(message string) *Error { return NewError(CodeAlreadyExists, message) }
func Validation(message string) *Error    { return NewError(CodeValidation, message) }
func Internal(message string) *Error      { return NewError(CodeInternal, message) }
