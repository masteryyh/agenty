package response

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/customerrors"
)

type GenericResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func NewGenericResponse(code int, message string, data any) *GenericResponse {
	return &GenericResponse{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

func OK(c *gin.Context, data any) {
	c.JSON(200, NewGenericResponse(200, "ok", data))
}

func Failed(c *gin.Context, err error) {
	bizErr := customerrors.GetBusinessError(err)
	if bizErr != nil {
		c.JSON(200, NewGenericResponse(bizErr.Code, bizErr.Message, nil))
	} else {
		c.JSON(200, NewGenericResponse(500, "internal server error", nil))
	}
}

func Abort(c *gin.Context, reason any) {
	err, ok := reason.(error)
	if ok {
		bizErr := customerrors.GetBusinessError(err)
		if bizErr != nil {
			c.AbortWithStatusJSON(200, NewGenericResponse(bizErr.Code, bizErr.Message, nil))
		} else {
			c.AbortWithStatusJSON(200, NewGenericResponse(500, "internal server error", nil))
		}
	} else {
		slog.Error("an error occurred or panic recovered", "reason", reason)
		c.AbortWithStatusJSON(500, NewGenericResponse(500, "internal server error", nil))
	}
}
