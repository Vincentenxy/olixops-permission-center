package handler

import (
	"github.com/gin-gonic/gin"
	"olixops-permission-center/internal/application/health"
	"olixops-permission-center/pkg/response"
	"olixops-permission-center/pkg/userctx"
)

type Handler struct {
	health health.Service
}

// CurrentUser is the public identity projection; credentials never enter responses.
type CurrentUser struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
}

func New() *Handler {
	return &Handler{}
}

// Health godoc
// @Summary Process liveness
// @Tags health
// @Produce json
// @Success 200 {object} response.Envelope{data=health.Status}
// @Router /api/v1/pub/healthz [get]
func (h *Handler) Health(c *gin.Context) {
	response.Success(c, h.health.Check())
}

// Me godoc
// @Summary Current authenticated identity
// @Tags authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=CurrentUser}
// @Failure 401 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /api/v1/auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	user := userctx.MustFromContext(c)
	response.Success(c, CurrentUser{UserID: user.UserID, Name: user.Name})
}
