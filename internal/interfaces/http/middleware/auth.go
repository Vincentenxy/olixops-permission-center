package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/logger"
	"olixops-permission-center/pkg/response"
	"olixops-permission-center/pkg/userctx"
)

// Auth guards every route in its group, regardless of the requested URL text.
func Auth(cfg config.Config) gin.HandlerFunc {
	verifier, configErr := auth.NewLocalVerifier(cfg.AuthJWTPublicKey, cfg.AuthJWTIssuer, cfg.AuthJWTAudience)
	return func(c *gin.Context) {
		if configErr != nil {
			logger.Error(c.Request.Context(), "authentication configuration is unavailable")
			response.HTTPError(c, http.StatusServiceUnavailable)
			return
		}
		user, err := verifier.Authenticate(c.GetHeader("Authorization"), c.GetHeader("Cookie"))
		if err != nil {
			response.HTTPError(c, http.StatusUnauthorized)
			return
		}
		c.Request = c.Request.WithContext(userctx.WithContext(c.Request.Context(), user))
		c.Next()
	}
}
