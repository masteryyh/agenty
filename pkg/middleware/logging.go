package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

func RecoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		response.Abort(c, recovered)
	})
}
