package middlewares

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/api/response"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(c.Request.Context(), "HTTP handler panic", "panic", recovered, "path", c.Request.URL.Path)
				if !c.Writer.Written() {
					response.Failure(c.Writer, http.StatusInternalServerError, "internal", "internal server error")
					c.Abort()
					return
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
