package customerrors

import (
	"errors"
	"fmt"
	"net/http"
)

type BusinessError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *BusinessError) Error() string {
	return fmt.Sprintf("code: %d, message: %s", e.Code, e.Message)
}

func NewBusinessError(code int, message string) *BusinessError {
	return &BusinessError{
		Code:    code,
		Message: message,
	}
}

var (
	ErrUnauthorized        = NewBusinessError(http.StatusUnauthorized, "unauthorized")
	ErrForbidden           = NewBusinessError(http.StatusForbidden, "forbidden")
	ErrInvalidParams       = NewBusinessError(http.StatusBadRequest, "invalid params")
	ErrInternalServerError = NewBusinessError(http.StatusInternalServerError, "internal server error")

	ErrSessionNotFound       = NewBusinessError(http.StatusNotFound, "session not found")
	ErrModelNotFound         = NewBusinessError(http.StatusNotFound, "model not found")
	ErrModelAlreadyExists    = NewBusinessError(http.StatusConflict, "model already exists")
	ErrProviderNotFound      = NewBusinessError(http.StatusNotFound, "provider not found")
	ErrProviderAlreadyExists = NewBusinessError(http.StatusConflict, "provider already exists")
	ErrProviderInUse         = NewBusinessError(http.StatusBadRequest, "provider is in use and cannot be deleted")
)

func GetBusinessError(err error) *BusinessError {
	if err == nil {
		return nil
	}
	var businessErr *BusinessError
	if errors.As(err, &businessErr) {
		return businessErr
	}
	return nil
}
