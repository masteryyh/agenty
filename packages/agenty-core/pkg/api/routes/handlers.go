package routes

import (
	stdjson "encoding/json"
	"errors"
	"net/http"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty-core/pkg/application"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err *APIError) Error() string {
	return err.Message
}

func invalidParams(message string) error {
	return &APIError{Status: http.StatusBadRequest, Code: "invalid_params", Message: message}
}

func notFound(message string) error {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: message}
}

func toAPIError(err error) error {
	if appErr, ok := errors.AsType[*application.Error](err); ok {
		switch appErr.Code {
		case application.CodeNotFound:
			return notFound(appErr.Message)
		case application.CodeAlreadyExists:
			return &APIError{Status: http.StatusConflict, Code: "already_exists", Message: appErr.Message}
		case application.CodeValidation:
			return invalidParams(appErr.Message)
		default:
			return &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: appErr.Message}
		}
	}
	return &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}
}

func decodeParams(params stdjson.RawMessage, destination any) error {
	if len(params) == 0 {
		params = stdjson.RawMessage("{}")
	}
	return json.Unmarshal(params, destination)
}
