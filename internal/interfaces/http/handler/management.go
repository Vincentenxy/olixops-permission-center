package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	app "olixops-permission-center/internal/application/management"
	m "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/pkg/page"
	"olixops-permission-center/pkg/response"
	"olixops-permission-center/pkg/userctx"
)

type Management struct{ service *app.Service }

func NewManagement(service *app.Service) *Management { return &Management{service: service} }

type UserProfile struct {
	UserID       string `json:"userId"`
	Username     string `json:"username"`
	Name         string `json:"name"`
	PlatformRole string `json:"platformRole"`
}

func profile(u m.User) UserProfile { return UserProfile{u.ID, u.Username, u.Name, u.PlatformRole} }

type baseDTO struct {
	ID       string    `json:"id"`
	Version  int64     `json:"version"`
	CreateAt time.Time `json:"createAt"`
	UpdateAt time.Time `json:"updateAt"`
}

func baseDTOOf(v m.Base) baseDTO { return baseDTO{v.ID, v.Version, v.CreateAt, v.UpdateAt} }

type userDTO struct {
	baseDTO
	Username     string `json:"username"`
	Name         string `json:"name"`
	PlatformRole string `json:"platformRole"`
	Status       string `json:"status"`
	Remark       string `json:"remark"`
}

func userDTOOf(v m.User) any {
	return userDTO{baseDTOOf(v.Base), v.Username, v.Name, v.PlatformRole, v.Status, v.Remark}
}

type tenantDTO struct {
	baseDTO
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Remark string `json:"remark"`
}

func tenantDTOOf(v m.Tenant) any {
	return tenantDTO{baseDTOOf(v.Base), v.Code, v.Name, v.Status, v.Remark}
}

type systemDTO struct {
	baseDTO
	TenantID string `json:"tenantId"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Remark   string `json:"remark"`
}

func systemDTOOf(v m.System) any {
	return systemDTO{baseDTOOf(v.Base), v.TenantID, v.Code, v.Name, v.Status, v.Remark}
}

type accountDTO struct {
	baseDTO
	SystemID string `json:"systemId"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

func accountDTOOf(v m.Account) any {
	return accountDTO{baseDTOOf(v.Base), v.SystemID, v.Name, v.Status}
}
func systemDetailDTOOf(v app.SystemDetail) any {
	return struct {
		System  any `json:"system"`
		Account any `json:"account"`
	}{systemDTOOf(v.System), accountDTOOf(v.Account)}
}

type memberDTO struct {
	baseDTO
	TenantID   string `json:"tenantId"`
	UserID     string `json:"userId"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	MemberRole string `json:"memberRole"`
	Status     string `json:"status"`
}

func memberDTOOf(v m.Member) any {
	return memberDTO{baseDTOOf(v.Base), v.TenantID, v.UserID, v.UserName, v.Name, v.MemberRole, v.Status}
}

type credentialDTO struct {
	baseDTO
	AccountID string     `json:"accountId"`
	KeyID     string     `json:"keyId"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	RevokedAt *time.Time `json:"revokedAt"`
}

func credentialDTOOf(v m.Credential) any {
	return credentialDTO{baseDTOOf(v.Base), v.AccountID, v.KeyID, v.Name, v.Status, v.RevokedAt}
}
func issuedDTOOf(v app.IssuedCredential) any {
	return struct {
		Credential any    `json:"credential"`
		Token      string `json:"token"`
	}{credentialDTOOf(v.Credential), v.Token}
}

type auditDTO struct {
	ID         string          `json:"id"`
	ActorType  string          `json:"actorType"`
	ActorID    *string         `json:"actorId"`
	ActorName  string          `json:"actorName"`
	TenantID   *string         `json:"tenantId"`
	TenantName string          `json:"tenantName"`
	SystemID   *string         `json:"systemId"`
	SystemName string          `json:"systemName"`
	AccountID  *string         `json:"accountId"`
	KeyID      string          `json:"keyId"`
	TargetType string          `json:"targetType"`
	TargetID   *string         `json:"targetId"`
	TargetName string          `json:"targetName"`
	Action     string          `json:"action"`
	Result     string          `json:"result"`
	ReasonCode string          `json:"reasonCode"`
	RequestID  string          `json:"requestId"`
	Changes    json.RawMessage `json:"changes"`
	CreateAt   time.Time       `json:"createAt"`
}

func auditDTOOf(v m.Audit) any {
	return auditDTO{v.ID, v.ActorType, v.ActorID, v.ActorName, v.TenantID, v.TenantName, v.SystemID, v.SystemName, v.AccountID, v.KeyID, v.TargetType, v.TargetID, v.TargetName, v.Action, v.Result, v.ReasonCode, v.RequestID, json.RawMessage(v.Changes), v.CreateAt}
}

type loginRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=72"`
}

// Login authenticates an active local user and signs a bounded RS256 JWT.
// @Summary 管理员登录
// @Tags auth
// @Accept json
// @Produce json
// @Param body body loginRequest true "用户名和密码"
// @Success 200 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /api/v1/pub/auth/login [post]
func (h *Management) Login(c *gin.Context) {
	var r loginRequest
	if !bind(c, &r) {
		return
	}
	v, e := h.service.Login(c.Request.Context(), r.Username, r.Password)
	if e != nil {
		respondError(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, struct {
		AccessToken string      `json:"accessToken"`
		ExpiresAt   time.Time   `json:"expiresAt"`
		User        UserProfile `json:"user"`
	}{v.AccessToken, v.ExpiresAt, profile(v.User)})
}

// Me returns a live local identity rather than trusting JWT role claims.
// @Summary 当前管理员
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /api/v1/auth/me [get]
func (h *Management) Me(c *gin.Context) {
	v, e := h.service.Me(c.Request.Context(), userctx.MustFromContext(c).UserID)
	if e != nil {
		respondError(c, e)
		return
	}
	response.Success(c, profile(v))
}

// SystemMe validates the fixed API token and all of its parent scopes.
// @Summary 当前系统调用身份
// @Tags systemAccess
// @Security SystemToken
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /api/v1/systemAccess/me [get]
func (h *Management) SystemMe(c *gin.Context) {
	v, e := h.service.AuthenticateSystem(c.Request.Context(), c.GetHeader("Authorization"))
	if e != nil {
		respondError(c, e)
		return
	}
	response.Success(c, struct {
		TenantID  string `json:"tenantId"`
		SystemID  string `json:"systemId"`
		AccountID string `json:"accountId"`
		KeyID     string `json:"keyId"`
	}{v.TenantID, v.SystemID, v.AccountID, v.KeyID})
}
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, m.ErrInvalid):
		response.HTTPError(c, http.StatusBadRequest)
	case errors.Is(err, m.ErrUnauthorized):
		response.HTTPError(c, http.StatusUnauthorized)
	case errors.Is(err, m.ErrForbidden):
		response.HTTPError(c, http.StatusForbidden)
	case errors.Is(err, m.ErrNotFound):
		response.HTTPError(c, http.StatusNotFound)
	case errors.Is(err, m.ErrConflict):
		response.HTTPError(c, http.StatusConflict)
	case errors.Is(err, m.ErrUnavailable):
		response.HTTPError(c, http.StatusServiceUnavailable)
	case errors.Is(err, m.ErrDuplicate), errors.Is(err, m.ErrLastAdmin), errors.Is(err, m.ErrActiveCredential):
		response.Error(c, err.Error())
	default:
		response.HTTPError(c, http.StatusInternalServerError)
	}
}
func bind(c *gin.Context, q any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(q); err != nil {
		response.HTTPError(c, http.StatusBadRequest)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		response.HTTPError(c, http.StatusBadRequest)
		return false
	}
	if err := binding.Validator.ValidateStruct(q); err != nil {
		response.HTTPError(c, http.StatusBadRequest)
		return false
	}
	if normalizer, ok := q.(interface{ Normalize() }); ok {
		normalizer.Normalize()
	}
	return true
}
func post[Q, T any](fn func(context.Context, string, Q) (T, error), dto func(T) any) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q Q
		if !bind(c, &q) {
			return
		}
		v, e := fn(c.Request.Context(), userctx.MustFromContext(c).UserID, q)
		if e != nil {
			respondError(c, e)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, dto(v))
	}
}
func listDTO[T any](convert func(T) any) func(m.Page[T]) any {
	return func(v m.Page[T]) any {
		items := make([]any, 0, len(v.List))
		for _, item := range v.List {
			items = append(items, convert(item))
		}
		return page.Response[any]{Total: v.Total, List: items}
	}
}

type idRequest struct {
	ID string `json:"id" binding:"required,uuid4"`
}
type versionRequest struct {
	ID      string `json:"id" binding:"required,uuid4"`
	Version int64  `json:"version" binding:"required,min=1"`
}
type createRequest struct {
	TenantID string `json:"tenantId" binding:"omitempty,uuid4"`
	Code     string `json:"code" binding:"required,max=64"`
	Name     string `json:"name" binding:"required,max=128"`
	Remark   string `json:"remark" binding:"max=2000"`
}

func (q createRequest) command() app.CreateResource {
	return app.CreateResource{TenantID: q.TenantID, Code: q.Code, Name: q.Name, Remark: q.Remark}
}

type updateRequest struct {
	versionRequest
	Name   string `json:"name" binding:"required,max=128"`
	Remark string `json:"remark" binding:"max=2000"`
}

func (q updateRequest) command() app.UpdateResource {
	return app.UpdateResource{ID: q.ID, Version: q.Version, Name: q.Name, Remark: q.Remark}
}

type statusRequest struct {
	versionRequest
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

func (q statusRequest) command() app.SetStatus {
	return app.SetStatus{ID: q.ID, Version: q.Version, Status: q.Status}
}

type roleRequest struct {
	versionRequest
	PlatformRole string `json:"platformRole" binding:"required,oneof=none platform_admin"`
}
type createUserRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=72"`
	Name     string `json:"name" binding:"required,max=128"`
	Remark   string `json:"remark" binding:"max=2000"`
}
type memberRequest struct {
	TenantID string `json:"tenantId" binding:"required,uuid4"`
	UserID   string `json:"userId" binding:"required,uuid4"`
}
type credentialRequest struct {
	AccountID string `json:"accountId" binding:"required,uuid4"`
	Name      string `json:"name" binding:"required,max=128"`
}
type listRequest struct {
	page.Request
	Username     string     `json:"username" binding:"max=64"`
	Code         string     `json:"code" binding:"max=64"`
	Name         string     `json:"name" binding:"max=128"`
	Status       string     `json:"status" binding:"omitempty,oneof=active disabled revoked"`
	PlatformRole string     `json:"platformRole" binding:"omitempty,oneof=none platform_admin"`
	TenantID     string     `json:"tenantId" binding:"omitempty,uuid4"`
	SystemID     string     `json:"systemId" binding:"omitempty,uuid4"`
	ActorID      string     `json:"actorId" binding:"omitempty,uuid4"`
	Action       string     `json:"action" binding:"max=64"`
	Result       string     `json:"result" binding:"omitempty,oneof=success denied failed"`
	RequestID    string     `json:"requestId" binding:"max=128"`
	From         *time.Time `json:"from"`
	To           *time.Time `json:"to"`
}

func (q listRequest) filter() m.Filter {
	return m.Filter{PageNum: q.PageNum, PageSize: q.PageSize, Code: q.Code, Name: q.Name, Username: q.Username, Status: q.Status, PlatformRole: q.PlatformRole, TenantID: q.TenantID, SystemID: q.SystemID, ActorID: q.ActorID, Action: q.Action, Result: q.Result, RequestID: q.RequestID, From: q.From, To: q.To}
}

// Register exposes explicit management use cases; binding and response policy is
// shared while each route retains its own command and authorization behavior.
func (h *Management) Register(r *gin.RouterGroup) {
	r.GET("/auth/me", h.Me)
	r.POST("/auth/tenant/list", h.MyTenants)
	r.POST("/user/list", h.UserList)
	r.POST("/user/detail", h.UserDetail)
	r.POST("/user/create", h.UserCreate)
	r.POST("/user/update", h.UserUpdate)
	r.POST("/user/setStatus", h.UserSetStatus)
	r.POST("/user/setPlatformRole", h.UserSetPlatformRole)
	r.POST("/tenant/list", h.TenantList)
	r.POST("/tenant/detail", h.TenantDetail)
	r.POST("/tenant/create", h.TenantCreate)
	r.POST("/tenant/update", h.TenantUpdate)
	r.POST("/tenant/setStatus", h.TenantSetStatus)
	r.POST("/tenant/member/list", h.TenantMemberList)
	r.POST("/tenant/member/allocate", h.TenantMemberAllocate)
	r.POST("/tenant/member/setStatus", h.TenantMemberSetStatus)
	r.POST("/system/list", h.SystemList)
	r.POST("/system/detail", h.SystemDetail)
	r.POST("/system/create", h.SystemCreate)
	r.POST("/system/update", h.SystemUpdate)
	r.POST("/system/setStatus", h.SystemSetStatus)
	r.POST("/system/account/setStatus", h.SystemAccountSetStatus)
	r.POST("/system/credential/list", h.SystemCredentialList)
	r.POST("/system/credential/checkCreate", h.CheckCredentialCreation)
	r.POST("/system/credential/create", h.SystemCredentialCreate)
	r.POST("/system/credential/revoke", h.SystemCredentialRevoke)
	r.POST("/audit/list", h.AuditList)
	r.POST("/audit/detail", h.AuditDetail)
}
