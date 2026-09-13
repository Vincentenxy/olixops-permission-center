package middleware

import (
	"github.com/gin-gonic/gin"
	"olixops-permission-center/pkg/requestid"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, requestID := requestid.Ensure(c.Request.Context(), c.GetHeader("X-Request-ID"))
		c.Header("X-Request-ID", requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
