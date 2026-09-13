package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"olixops-permission-center/pkg/logger"
)

func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info(c.Request.Context(), "http request",
			zap.String("method", c.Request.Method), zap.String("route", c.FullPath()),
			zap.Int("status", c.Writer.Status()), zap.Duration("latency", time.Since(started)))
	}
}
