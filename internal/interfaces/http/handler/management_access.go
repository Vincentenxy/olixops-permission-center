package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	app "olixops-permission-center/internal/application/management"
	m "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/pkg/page"
)

type myTenantsRequest struct {
	page.Request
	Code string `json:"code" binding:"max=64"`
	Name string `json:"name" binding:"max=128"`
}

type systemIDRequest struct {
	SystemID string `json:"systemId" binding:"required,uuid4"`
}

type credentialCreationCheckDTO struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// MyTenants lists only the caller's active tenant-administrator memberships.
// @Summary 当前账号实际管理的有效租户（超管也按成员关系过滤）
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body myTenantsRequest true "分页与租户编码/名称筛选"
// @Success 200 {object} response.Envelope
// @Failure 400,401,503 {object} response.Envelope
// @Router /api/v1/auth/tenant/list [post]
func (h *Management) MyTenants(c *gin.Context) {
	post(func(ctx context.Context, id string, q myTenantsRequest) (m.Page[m.Tenant], error) {
		return h.service.MyTenants(ctx, id, m.Filter{PageNum: q.PageNum, PageSize: q.PageSize, Code: q.Code, Name: q.Name})
	}, listDTO(tenantDTOOf))(c)
}

// CheckCredentialCreation reports current eligibility without creating a token.
// @Summary 查询系统固定 Token 的当前签发资格
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body systemIDRequest true "目标系统"
// @Success 200 {object} response.Envelope
// @Failure 400,401,404,503 {object} response.Envelope
// @Router /api/v1/system/credential/checkCreate [post]
func (h *Management) CheckCredentialCreation(c *gin.Context) {
	post(func(ctx context.Context, id string, q systemIDRequest) (app.CredentialCreationCheck, error) {
		return h.service.CheckCredentialCreation(ctx, id, q.SystemID)
	}, func(v app.CredentialCreationCheck) any {
		return credentialCreationCheckDTO{Allowed: v.Allowed, Reason: v.Reason}
	})(c)
}
