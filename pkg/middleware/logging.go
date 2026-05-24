package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.FullPath(),
			"rawPath", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", time.Since(start),
			"clientIp", c.ClientIP(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}

		if c.Writer.Status() >= 500 || len(c.Errors) > 0 {
			slog.WarnContext(c.Request.Context(), "http request completed with error", attrs...)
			return
		}
		slog.DebugContext(c.Request.Context(), "http request completed", attrs...)
	}
}

func RecoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		response.Abort(c, recovered)
	})
}
