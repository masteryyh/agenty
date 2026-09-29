package response

import (
	"log/slog"
	"net/http"

	json "github.com/bytedance/sonic"
)

type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	ErrorCode string `json:"errorCode,omitempty"`
}

func Success(writer http.ResponseWriter, status int, data any) {
	write(writer, status, Body{Code: status, Message: "ok", Data: data})
}

func Failure(writer http.ResponseWriter, status int, code, message string) {
	write(writer, status, Body{Code: status, Message: message, ErrorCode: code})
}

func write(writer http.ResponseWriter, status int, body Body) {
	encoded, err := json.Marshal(body)
	if err != nil {
		slog.Error("failed to encode HTTP response", "error", err)
		status = http.StatusInternalServerError
		encoded, _ = json.Marshal(Body{Code: status, Message: "internal server error", ErrorCode: "internal"})
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if _, err := writer.Write(append(encoded, '\n')); err != nil {
		slog.Error("failed to write HTTP response", "error", err)
	}
}
