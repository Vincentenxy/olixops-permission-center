package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	app "olixops-permission-center/internal/application/management"
	m "olixops-permission-center/internal/domain/management"
)

// UserList executes the shared management application use case.
// @Summary user/list
// @Tags user
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/user/list [post]
func (h *Management) UserList(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q listRequest) (m.Page[m.User], error) {
		return s.Users(ctx, id, q.filter())
	}, listDTO(userDTOOf))(c)
}

// UserDetail executes the shared management application use case.
// @Summary user/detail
// @Tags user
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body idRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/user/detail [post]
func (h *Management) UserDetail(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q idRequest) (m.User, error) { return s.User(ctx, id, q.ID) }, userDTOOf)(c)
}

// UserCreate executes the shared management application use case.
// @Summary user/create
// @Tags user
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createUserRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/user/create [post]
func (h *Management) UserCreate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q createUserRequest) (m.User, error) {
		return s.CreateUser(ctx, id, app.CreateUser{Username: q.Username, Password: q.Password, Name: q.Name, Remark: q.Remark})
	}, userDTOOf)(c)
}

// UserUpdate executes the shared management application use case.
// @Summary user/update
// @Tags user
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body updateRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/user/update [post]
func (h *Management) UserUpdate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q updateRequest) (m.User, error) {
		return s.UpdateUser(ctx, id, q.command())
	}, userDTOOf)(c)
}

// UserSetStatus executes the shared management application use case.
// @Summary user/setStatus
// @Tags user
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body statusRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/user/setStatus [post]
func (h *Management) UserSetStatus(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q statusRequest) (m.User, error) {
		return s.SetUserStatus(ctx, id, q.command())
	}, userDTOOf)(c)
}

// UserSetPlatformRole executes the shared management application use case.
// @Summary user/setPlatformRole
// @Tags user
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body roleRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/user/setPlatformRole [post]
func (h *Management) UserSetPlatformRole(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q roleRequest) (m.User, error) {
		return s.SetPlatformRole(ctx, id, app.SetRole{ID: q.ID, Version: q.Version, PlatformRole: q.PlatformRole})
	}, userDTOOf)(c)
}

// TenantList executes the shared management application use case.
// @Summary tenant/list
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/list [post]
func (h *Management) TenantList(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q listRequest) (m.Page[m.Tenant], error) {
		return s.Tenants(ctx, id, q.filter())
	}, listDTO(tenantDTOOf))(c)
}

// TenantDetail executes the shared management application use case.
// @Summary tenant/detail
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body idRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/detail [post]
func (h *Management) TenantDetail(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q idRequest) (m.Tenant, error) { return s.Tenant(ctx, id, q.ID) }, tenantDTOOf)(c)
}

// TenantCreate executes the shared management application use case.
// @Summary tenant/create
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/create [post]
func (h *Management) TenantCreate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q createRequest) (m.Tenant, error) {
		return s.CreateTenant(ctx, id, q.command())
	}, tenantDTOOf)(c)
}

// TenantUpdate executes the shared management application use case.
// @Summary tenant/update
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body updateRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/update [post]
func (h *Management) TenantUpdate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q updateRequest) (m.Tenant, error) {
		return s.UpdateTenant(ctx, id, q.command())
	}, tenantDTOOf)(c)
}

// TenantSetStatus executes the shared management application use case.
// @Summary tenant/setStatus
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body statusRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/setStatus [post]
func (h *Management) TenantSetStatus(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q statusRequest) (m.Tenant, error) {
		return s.SetTenantStatus(ctx, id, q.command())
	}, tenantDTOOf)(c)
}

// TenantMemberList executes the shared management application use case.
// @Summary tenant/member/list
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/member/list [post]
func (h *Management) TenantMemberList(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q listRequest) (m.Page[m.Member], error) {
		return s.Members(ctx, id, q.filter())
	}, listDTO(memberDTOOf))(c)
}

// TenantMemberAllocate executes the shared management application use case.
// @Summary tenant/member/allocate
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body memberRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/member/allocate [post]
func (h *Management) TenantMemberAllocate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q memberRequest) (m.Member, error) {
		return s.AllocateMember(ctx, id, q.TenantID, q.UserID)
	}, memberDTOOf)(c)
}

// TenantMemberSetStatus executes the shared management application use case.
// @Summary tenant/member/setStatus
// @Tags tenant
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body statusRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/tenant/member/setStatus [post]
func (h *Management) TenantMemberSetStatus(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q statusRequest) (m.Member, error) {
		return s.SetMemberStatus(ctx, id, q.command())
	}, memberDTOOf)(c)
}

// SystemList executes the shared management application use case.
// @Summary system/list
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/list [post]
func (h *Management) SystemList(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q listRequest) (m.Page[m.System], error) {
		return s.Systems(ctx, id, q.filter())
	}, listDTO(systemDTOOf))(c)
}

// SystemDetail executes the shared management application use case.
// @Summary system/detail
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body idRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/detail [post]
func (h *Management) SystemDetail(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q idRequest) (app.SystemDetail, error) {
		return s.System(ctx, id, q.ID)
	}, systemDetailDTOOf)(c)
}

// SystemCreate executes the shared management application use case.
// @Summary system/create
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/create [post]
func (h *Management) SystemCreate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q createRequest) (app.SystemDetail, error) {
		return s.CreateSystem(ctx, id, q.command())
	}, systemDetailDTOOf)(c)
}

// SystemUpdate executes the shared management application use case.
// @Summary system/update
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body updateRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/update [post]
func (h *Management) SystemUpdate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q updateRequest) (m.System, error) {
		return s.UpdateSystem(ctx, id, q.command())
	}, systemDTOOf)(c)
}

// SystemSetStatus executes the shared management application use case.
// @Summary system/setStatus
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body statusRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/setStatus [post]
func (h *Management) SystemSetStatus(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q statusRequest) (m.System, error) {
		return s.SetSystemStatus(ctx, id, q.command())
	}, systemDTOOf)(c)
}

// SystemAccountSetStatus executes the shared management application use case.
// @Summary system/account/setStatus
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body statusRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/account/setStatus [post]
func (h *Management) SystemAccountSetStatus(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q statusRequest) (m.Account, error) {
		return s.SetAccountStatus(ctx, id, q.command())
	}, accountDTOOf)(c)
}

// SystemCredentialList executes the shared management application use case.
// @Summary system/credential/list
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/credential/list [post]
func (h *Management) SystemCredentialList(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q listRequest) (m.Page[m.Credential], error) {
		return s.Credentials(ctx, id, q.filter())
	}, listDTO(credentialDTOOf))(c)
}

// SystemCredentialCreate executes the shared management application use case.
// @Summary system/credential/create
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body credentialRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/credential/create [post]
func (h *Management) SystemCredentialCreate(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q credentialRequest) (app.IssuedCredential, error) {
		return s.CreateCredential(ctx, id, q.AccountID, q.Name)
	}, issuedDTOOf)(c)
}

// SystemCredentialRevoke executes the shared management application use case.
// @Summary system/credential/revoke
// @Tags system
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body versionRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/system/credential/revoke [post]
func (h *Management) SystemCredentialRevoke(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q versionRequest) (m.Credential, error) {
		return s.RevokeCredential(ctx, id, q.ID, q.Version)
	}, credentialDTOOf)(c)
}

// AuditList executes the shared management application use case.
// @Summary audit/list
// @Tags audit
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/audit/list [post]
func (h *Management) AuditList(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q listRequest) (m.Page[m.Audit], error) {
		return s.Audits(ctx, id, q.filter())
	}, listDTO(auditDTOOf))(c)
}

// AuditDetail executes the shared management application use case.
// @Summary audit/detail
// @Tags audit
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body idRequest true "Request"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409,503 {object} response.Envelope
// @Router /api/v1/audit/detail [post]
func (h *Management) AuditDetail(c *gin.Context) {
	s := h.service
	post(func(ctx context.Context, id string, q idRequest) (m.Audit, error) { return s.Audit(ctx, id, q.ID) }, auditDTOOf)(c)
}
