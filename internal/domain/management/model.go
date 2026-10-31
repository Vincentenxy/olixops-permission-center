// Package management contains the permission center's own management model.
// Business resources and their permissions remain in the separate SpiceDB context.
package management

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnavailable      = errors.New("management storage unavailable")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")
	ErrInvalid          = errors.New("invalid input")
	ErrDuplicate        = errors.New("code or identity already exists")
	ErrLastAdmin        = errors.New("at least one active platform administrator is required")
	ErrActiveCredential = errors.New("account already has an active token")
	ErrBootstrap        = errors.New("administrator initialization requires an empty user table")
)

const (
	Active         = "active"
	Disabled       = "disabled"
	PlatformAdmin  = "platform_admin"
	NoPlatformRole = "none"
	TenantAdmin    = "tenant_admin"
	Revoked        = "revoked"
)

type Base struct {
	ID        string
	Version   int64
	CreateAt  time.Time
	CreateBy  string
	UpdateAt  time.Time
	UpdateBy  string
	IsDeleted bool
}
type Tenant struct {
	Base
	Code, Name, Status, Remark string
}
type User struct {
	Base
	Username, PasswordHash, ExternalIssuer, SubjectClaim, ExternalSubject string
	Name, PlatformRole, Status, Remark                                    string
}
type Member struct {
	Base
	TenantID, UserID, MemberRole, Status, UserName, Name string
}
type System struct {
	Base
	TenantID, Code, Name, Status, Remark string
}
type Account struct {
	Base
	SystemID, Name, Status, Remark string
}
type Credential struct {
	Base
	AccountID, KeyID, TokenDigest, Name, Status, RevokedBy string
	RevokedAt                                              *time.Time
}
type Audit struct {
	ID, ActorType, ActorName, TenantName, SystemName, KeyID, TargetType, TargetName string
	ActorID, TenantID, SystemID, AccountID, TargetID                                *string
	Action, Result, ReasonCode, RequestID, CreateBy                                 string
	Changes                                                                         []byte
	CreateAt                                                                        time.Time
}
type SystemIdentity struct{ TenantID, SystemID, AccountID, KeyID string }
type CredentialChain struct {
	Credential Credential
	Account    Account
	System     System
	Tenant     Tenant
}
type Filter struct {
	PageNum, PageSize                                                                                  int
	Code, Name, Username, Status, PlatformRole, TenantID, SystemID, ActorID, Action, Result, RequestID string
	From, To                                                                                           *time.Time
}

// Scope is derived from an active local user, never directly from request fields.
type Scope struct {
	UserID string
	Global bool
}
type Page[T any] struct {
	Total int64
	List  []T
}

// Store owns snapshots and transaction lifetime. Every management mutation uses
// the same advisory lock, including bootstrap and last-administrator protection.
// This deliberately conservative first-phase policy makes revocation ordering explicit.
type Store interface {
	Read(context.Context, func(Tx) error) error
	Write(context.Context, func(Tx) error) error
}
type Tx interface {
	User(string) (User, error)
	UserByUsername(string) (User, error)
	Users(Filter) (Page[User], error)
	UserCount() (int64, error)
	ActiveAdminCount() (int64, error)
	CreateUser(User) error
	UpdateUser(User, int64) error
	Tenant(string) (Tenant, error)
	Tenants(Filter, Scope) (Page[Tenant], error)
	CreateTenant(Tenant) error
	UpdateTenant(Tenant, int64) error
	Member(string) (Member, error)
	Membership(string, string) (Member, error)
	Members(Filter) (Page[Member], error)
	CreateMember(Member) error
	UpdateMember(Member, int64) error
	System(string) (System, error)
	Systems(Filter, Scope) (Page[System], error)
	CreateSystem(System) error
	UpdateSystem(System, int64) error
	Account(string) (Account, error)
	SystemAccount(string) (Account, error)
	CreateAccount(Account) error
	UpdateAccount(Account, int64) error
	Credential(string) (Credential, error)
	CredentialByKeyID(string) (Credential, error)
	Credentials(Filter) (Page[Credential], error)
	ActiveCredentialCount(string) (int64, error)
	CreateCredential(Credential) error
	UpdateCredential(Credential, int64) error
	Audit(string) (Audit, error)
	Audits(Filter, Scope) (Page[Audit], error)
	AppendAudit(Audit) error
}
