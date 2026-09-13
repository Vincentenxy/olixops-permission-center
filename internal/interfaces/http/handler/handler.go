package handler

import (
	"github.com/gin-gonic/gin"
	"olixops-permission-center/internal/application/health"
	"olixops-permission-center/pkg/response"
)

type Handler struct {
	health health.Service
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
