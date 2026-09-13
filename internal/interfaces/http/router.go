package httpinterface

import (
	"net/http"

	"github.com/gin-gonic/gin"
	management "olixops-permission-center/internal/application/management"
	"olixops-permission-center/internal/interfaces/http/handler"
	"olixops-permission-center/internal/interfaces/http/middleware"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/response"
)

func NewRouter(cfg config.Config, services ...*management.Service) *gin.Engine {
	// All diagnostic output goes through pkg/logger, including in development.
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	if err := router.SetTrustedProxies(cfg.HTTPTrustedProxies); err != nil {
		_ = router.SetTrustedProxies(nil)
	}
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.Use(middleware.RequestID(), middleware.AccessLog(), middleware.Recovery(), middleware.RequestTimeout(cfg.HTTPWriteTimeout))
	router.NoRoute(func(c *gin.Context) { response.HTTPError(c, http.StatusNotFound) })
	router.NoMethod(func(c *gin.Context) { response.HTTPError(c, http.StatusMethodNotAllowed) })

	h := handler.New()
	public := router.Group("/api/v1/pub")
	public.GET("/healthz", h.Health)
	var service *management.Service
	if len(services) > 0 {
		service = services[0]
	}
	mh := handler.NewManagement(service)
	public.POST("/auth/login", middleware.LoginRateLimit(), mh.Login)
	router.GET("/api/v1/systemAccess/me", mh.SystemMe)

	secured := router.Group("/api/v1")
	secured.Use(middleware.Auth(cfg))
	mh.Register(secured)
	return router
}
