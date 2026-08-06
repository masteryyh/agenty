package application

import "github.com/masteryyh/agenty-core/pkg/application/apperrors"

type (
	Code  = apperrors.Code
	Error = apperrors.Error
)

const (
	CodeInternal      = apperrors.CodeInternal
	CodeNotFound      = apperrors.CodeNotFound
	CodeAlreadyExists = apperrors.CodeAlreadyExists
	CodeValidation    = apperrors.CodeValidation
)

func NewError(code Code, message string) *Error {
	return apperrors.NewError(code, message)
}

func WrapError(code Code, message string, cause error) *Error {
	return apperrors.WrapError(code, message, cause)
}

func NotFound(message string) *Error {
	return apperrors.NotFound(message)
}

func AlreadyExists(message string) *Error {
	return apperrors.AlreadyExists(message)
}

func Validation(message string) *Error {
	return apperrors.Validation(message)
}

func Internal(message string) *Error {
	return apperrors.Internal(message)
}
