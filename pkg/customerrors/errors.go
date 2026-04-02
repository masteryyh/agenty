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

	ErrSessionNotFound            = NewBusinessError(http.StatusNotFound, "session not found")
	ErrAgentNotFound              = NewBusinessError(http.StatusNotFound, "agent not found")
	ErrAgentAlreadyExists         = NewBusinessError(http.StatusConflict, "agent already exists")
	ErrDeletingDefaultAgent       = NewBusinessError(http.StatusBadRequest, "default agent cannot be deleted")
	ErrModelNotFound              = NewBusinessError(http.StatusNotFound, "model not found")
	ErrModelAlreadyExists         = NewBusinessError(http.StatusConflict, "model already exists")
	ErrDeletingDefaultModel       = NewBusinessError(http.StatusBadRequest, "default model cannot be deleted")
	ErrModelTypeMutuallyExclusive = NewBusinessError(http.StatusBadRequest, "embedding model and context compression model cannot be enabled at the same time")
	ErrProviderNotFound           = NewBusinessError(http.StatusNotFound, "provider not found")
	ErrProviderAlreadyExists      = NewBusinessError(http.StatusConflict, "provider already exists")
	ErrProviderInUse              = NewBusinessError(http.StatusBadRequest, "provider is in use and cannot be deleted")
	ErrProviderNotConfigured      = NewBusinessError(http.StatusBadRequest, "provider is not configured")
	ErrEmbeddingMigrating         = NewBusinessError(http.StatusConflict, "embedding migration in progress, please try again later")

	ErrMCPServerNotFound         = NewBusinessError(http.StatusNotFound, "mcp server not found")
	ErrMCPServerAlreadyExists    = NewBusinessError(http.StatusConflict, "mcp server already exists")
	ErrMCPServerConnectionFailed = NewBusinessError(http.StatusBadGateway, "mcp server connection failed")
	ErrMCPServerNotConnected     = NewBusinessError(http.StatusBadRequest, "mcp server not connected")

	ErrKnowledgeItemNotFound = NewBusinessError(http.StatusNotFound, "knowledge item not found")
	ErrKnowledgeContentEmpty = NewBusinessError(http.StatusBadRequest, "knowledge item content is required")
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
