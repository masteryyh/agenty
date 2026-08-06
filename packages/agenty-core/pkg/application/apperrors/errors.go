package apperrors

import "fmt"

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

func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func WrapError(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func NotFound(message string) *Error {
	return NewError(CodeNotFound, message)
}

func AlreadyExists(message string) *Error {
	return NewError(CodeAlreadyExists, message)
}

func Validation(message string) *Error {
	return NewError(CodeValidation, message)
}

func Internal(message string) *Error {
	return NewError(CodeInternal, message)
}
