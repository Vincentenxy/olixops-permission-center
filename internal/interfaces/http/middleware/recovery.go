package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"olixops-permission-center/pkg/logger"
	"olixops-permission-center/pkg/response"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				// Gin's default recovery also dumps request headers and panic values.
				logger.Error(c.Request.Context(), "http panic recovered")
				if !c.Writer.Written() {
					response.HTTPError(c, http.StatusInternalServerError)
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
