package response

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/customerrors"
)

type GenericResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *T     `json:"data,omitempty"`
}

func NewGenericResponse[T any](code int, message string, data T) *GenericResponse[T] {
	return &GenericResponse[T]{
		Code:    code,
		Message: message,
		Data:    &data,
	}
}

func OK[T any](c *gin.Context, data T) {
	c.JSON(200, NewGenericResponse(200, "ok", data))
}

func Failed(c *gin.Context, err error) {
	bizErr := customerrors.GetBusinessError(err)
	if bizErr != nil {
		c.JSON(200, NewGenericResponse[any](bizErr.Code, bizErr.Message, nil))
	} else {
		c.JSON(200, NewGenericResponse[any](500, "internal server error", nil))
	}
}

func Abort(c *gin.Context, reason any) {
	err, ok := reason.(error)
	if ok {
		bizErr := customerrors.GetBusinessError(err)
		if bizErr != nil {
			c.AbortWithStatusJSON(200, NewGenericResponse[any](bizErr.Code, bizErr.Message, nil))
		} else {
			c.AbortWithStatusJSON(200, NewGenericResponse[any](500, "internal server error", nil))
		}
	} else {
		slog.Error("an error occurred or panic recovered", "reason", reason)
		c.AbortWithStatusJSON(500, NewGenericResponse[any](500, "internal server error", nil))
	}
}
