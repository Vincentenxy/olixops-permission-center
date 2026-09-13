package httpinterface

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"olixops-permission-center/internal/interfaces/http/handler"
	"olixops-permission-center/internal/interfaces/http/middleware"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/response"
)

func NewRouter(cfg config.Config) *gin.Engine {
	// All diagnostic output goes through pkg/logger, including in development.
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.Use(middleware.RequestID(), middleware.AccessLog(), middleware.Recovery())
	router.NoRoute(func(c *gin.Context) { response.HTTPError(c, http.StatusNotFound) })
	router.NoMethod(func(c *gin.Context) { response.HTTPError(c, http.StatusMethodNotAllowed) })

	h := handler.New()
	public := router.Group("/api/v1/pub")
	public.GET("/healthz", h.Health)

	secured := router.Group("/api/v1")
	secured.Use(middleware.Auth(cfg))
	secured.GET("/auth/me", h.Me)
	return router
}
