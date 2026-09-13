package management

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	m "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/logger"
	"olixops-permission-center/pkg/requestid"
)

type TokenSigner interface {
	Sign(string, string) (string, time.Time, error)
}
type Service struct {
	store  m.Store
	signer TokenSigner
	issuer string
}

func New(store m.Store, signer TokenSigner, issuer string) *Service {
	return &Service{store: store, signer: signer, issuer: issuer}
}

type LoginResult struct {
	AccessToken string
	ExpiresAt   time.Time
	User        m.User
}
type SystemDetail struct {
	System  m.System
	Account m.Account
}
type IssuedCredential struct {
	Credential m.Credential
	Token      string
}
type CreateUser struct{ Username, Password, Name, Remark string }
type CreateResource struct{ TenantID, Code, Name, Remark string }
type UpdateResource struct {
	ID, Name, Remark string
	Version          int64
}
type SetStatus struct {
	ID, Status string
	Version    int64
}
type SetRole struct {
	ID, PlatformRole string
	Version          int64
}

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@-]{2,63}$`)
var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }
func nameValid(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 128
}
func detailsValid(name, remark string) bool {
	return nameValid(name) && utf8.RuneCountInString(remark) <= 2000
}
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func newBase(actor string) m.Base {
	now := time.Now().UTC()
	return m.Base{ID: newID(), Version: 1, CreateAt: now, UpdateAt: now, CreateBy: actor, UpdateBy: actor}
}
func changed(b *m.Base, actor string, version int64) error {
	if version <= 0 {
		return m.ErrInvalid
	}
	if b.Version != version {
		return m.ErrConflict
	}
	b.Version++
	b.UpdateAt = time.Now().UTC()
	b.UpdateBy = actor
	return nil
}
func activeUser(tx m.Tx, id string) (m.User, error) {
	if !ValidID(id) {
		return m.User{}, m.ErrUnauthorized
	}
	u, err := tx.User(id)
	if errors.Is(err, m.ErrNotFound) {
		return u, m.ErrUnauthorized
	}
	if err != nil {
		return u, err
	}
	if u.Status != m.Active || u.IsDeleted {
		return u, m.ErrUnauthorized
	}
	return u, nil
}
func requirePlatform(u m.User) error {
	if u.PlatformRole != m.PlatformAdmin {
		return m.ErrForbidden
	}
	return nil
}
func userScope(u m.User) m.Scope {
	return m.Scope{UserID: u.ID, Global: u.PlatformRole == m.PlatformAdmin}
}

// Tenant membership is always required for issuing credentials, including for
// platform administrators. Other management operations retain platform authority.
func tenantScope(tx m.Tx, u m.User, id string, issuing bool) (m.Tenant, error) {
	if !ValidID(id) {
		return m.Tenant{}, m.ErrInvalid
	}
	tenant, err := tx.Tenant(id)
	if err != nil {
		return tenant, err
	}
	if !issuing && u.PlatformRole == m.PlatformAdmin {
		return tenant, nil
	}
	member, err := tx.Membership(id, u.ID)
	if err != nil && !errors.Is(err, m.ErrNotFound) {
		return tenant, err
	}
	if err != nil || member.Status != m.Active || member.MemberRole != m.TenantAdmin || tenant.Status != m.Active {
		return tenant, m.ErrNotFound
	}
	return tenant, nil
}
func systemScope(tx m.Tx, u m.User, id string) (m.System, m.Tenant, error) {
	if !ValidID(id) {
		return m.System{}, m.Tenant{}, m.ErrInvalid
	}
	sys, err := tx.System(id)
	if err != nil {
		return sys, m.Tenant{}, err
	}
	tenant, err := tenantScope(tx, u, sys.TenantID, false)
	return sys, tenant, err
}
func ptr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func audit(ctx context.Context, u m.User, action, targetType, id, name string, tenant *m.Tenant, system *m.System, changes any) m.Audit {
	data, _ := json.Marshal(changes)
	if changes == nil {
		data = []byte("{}")
	}
	a := m.Audit{ID: newID(), ActorType: "user", ActorID: ptr(u.ID), ActorName: u.Name, TargetType: targetType, TargetID: ptr(id), TargetName: name, Action: action, Result: "success", RequestID: requestid.FromContext(ctx), Changes: data, CreateAt: time.Now().UTC(), CreateBy: "user:" + u.ID}
	if tenant != nil {
		a.TenantID = ptr(tenant.ID)
		a.TenantName = tenant.Name
	}
	if system != nil {
		a.SystemID = ptr(system.ID)
		a.SystemName = system.Name
	}
	return a
}
func (s *Service) failure(ctx context.Context, action string, err error, verified *m.User) {
	if s == nil || s.store == nil {
		return
	}
	result, reason := "failed", "operation_failed"
	if errors.Is(err, m.ErrUnauthorized) || errors.Is(err, m.ErrForbidden) || errors.Is(err, m.ErrNotFound) {
		result, reason = "denied", "access_denied"
	}
	// Unverified callers and failed target input never establish audit scope.
	// A previously resolved active local user remains attributable even when the
	// target is outside that user's scope. Only platform admins see these events.
	a := m.Audit{ID: newID(), ActorType: "anonymous", TargetType: "request", Action: action, Result: result, ReasonCode: reason, RequestID: requestid.FromContext(ctx), Changes: []byte("{}"), CreateAt: time.Now().UTC(), CreateBy: "anonymous"}
	if verified != nil {
		a.ActorType = "user"
		a.ActorID = ptr(verified.ID)
		a.ActorName = verified.Name
		a.CreateBy = "user:" + verified.ID
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if auditErr := s.store.Write(auditCtx, func(tx m.Tx) error { return tx.AppendAudit(a) }); auditErr != nil {
		logger.Error(auditCtx, "management failure audit unavailable")
	}
}
func read[T any](ctx context.Context, s *Service, id string, fn func(m.Tx, m.User) (T, error)) (out T, err error) {
	if s == nil || s.store == nil {
		return out, m.ErrUnavailable
	}
	err = s.store.Read(ctx, func(tx m.Tx) error {
		u, e := activeUser(tx, id)
		if e != nil {
			return e
		}
		out, e = fn(tx, u)
		return e
	})
	return
}
func mutate[T any](ctx context.Context, s *Service, id, action string, fn func(m.Tx, m.User) (T, *m.Audit, error)) (out T, err error) {
	if s == nil || s.store == nil {
		return out, m.ErrUnavailable
	}
	var verified *m.User
	err = s.store.Write(ctx, func(tx m.Tx) error {
		u, e := activeUser(tx, id)
		if e != nil {
			return e
		}
		verified = &u
		var event *m.Audit
		out, event, e = fn(tx, u)
		if e != nil {
			return e
		}
		if event != nil {
			return tx.AppendAudit(*event)
		}
		return nil
	})
	if err != nil {
		s.failure(ctx, action, err, verified)
		var zero T
		out = zero
	}
	return
}
func (s *Service) Login(ctx context.Context, username, password string) (out LoginResult, err error) {
	if s == nil || s.store == nil || s.signer == nil {
		return out, m.ErrUnavailable
	}
	username = strings.ToLower(strings.TrimSpace(username))
	var u m.User
	err = s.store.Read(ctx, func(tx m.Tx) error { var e error; u, e = tx.UserByUsername(username); return e })
	if err != nil && !errors.Is(err, m.ErrNotFound) {
		return out, err
	}
	verified := auth.VerifyPassword(password, u.PasswordHash)
	if err != nil || !verified || u.Status != m.Active || u.IsDeleted {
		s.failure(ctx, "auth.login", m.ErrUnauthorized, nil)
		return out, m.ErrUnauthorized
	}
	err = s.store.Write(ctx, func(tx m.Tx) error {
		current, e := activeUser(tx, u.ID)
		if e != nil {
			return e
		}
		if current.PasswordHash != u.PasswordHash {
			return m.ErrUnauthorized
		}
		u = current
		event := audit(ctx, u, "auth.login", "user", u.ID, u.Name, nil, nil, nil)
		return tx.AppendAudit(event)
	})
	if err != nil {
		return out, err
	}
	token, expires, err := s.signer.Sign(u.ID, u.Name)
	if err != nil {
		return out, m.ErrUnavailable
	}
	return LoginResult{AccessToken: token, ExpiresAt: expires, User: u}, nil
}
func (s *Service) Me(ctx context.Context, id string) (m.User, error) {
	return read(ctx, s, id, func(_ m.Tx, u m.User) (m.User, error) { return u, nil })
}
func (s *Service) Bootstrap(ctx context.Context, request CreateUser) (m.User, error) {
	if s == nil || s.store == nil || s.issuer == "" {
		return m.User{}, m.ErrUnavailable
	}
	request.Username = strings.ToLower(strings.TrimSpace(request.Username))
	if !usernamePattern.MatchString(request.Username) || !detailsValid(request.Name, request.Remark) {
		return m.User{}, m.ErrInvalid
	}
	hash, err := auth.HashPassword(request.Password)
	if err != nil {
		return m.User{}, m.ErrInvalid
	}
	u := m.User{Base: newBase("bootstrap"), Username: request.Username, PasswordHash: hash, ExternalIssuer: s.issuer, SubjectClaim: "sub", Name: request.Name, PlatformRole: m.PlatformAdmin, Status: m.Active, Remark: request.Remark}
	u.ExternalSubject = u.ID
	err = s.store.Write(ctx, func(tx m.Tx) error {
		n, e := tx.UserCount()
		if e != nil {
			return e
		}
		if n != 0 {
			return m.ErrBootstrap
		}
		if e = tx.CreateUser(u); e != nil {
			return e
		}
		a := audit(ctx, u, "user.bootstrap", "user", u.ID, u.Name, nil, nil, nil)
		a.ActorType = "bootstrap"
		a.ActorID = nil
		a.CreateBy = "bootstrap"
		return tx.AppendAudit(a)
	})
	if err != nil {
		return m.User{}, err
	}
	return u, nil
}
func (s *Service) CreateUser(ctx context.Context, id string, r CreateUser) (m.User, error) {
	// Reject non-platform callers before performing expensive password hashing;
	// the transaction checks the actor again after hashing to close revocation races.
	var verified *m.User
	if _, err := read(ctx, s, id, func(_ m.Tx, u m.User) (struct{}, error) { verified = &u; return struct{}{}, requirePlatform(u) }); err != nil {
		s.failure(ctx, "user.create", err, verified)
		return m.User{}, err
	}
	r.Username = strings.ToLower(strings.TrimSpace(r.Username))
	if !usernamePattern.MatchString(r.Username) || !detailsValid(r.Name, r.Remark) {
		return m.User{}, m.ErrInvalid
	}
	hash, err := auth.HashPassword(r.Password)
	if err != nil {
		return m.User{}, m.ErrInvalid
	}
	return mutate(ctx, s, id, "user.create", func(tx m.Tx, u m.User) (v m.User, a *m.Audit, e error) {
		if e = requirePlatform(u); e != nil {
			return
		}
		v = m.User{Base: newBase("user:" + u.ID), Username: r.Username, PasswordHash: hash, ExternalIssuer: s.issuer, SubjectClaim: "sub", Name: r.Name, PlatformRole: m.NoPlatformRole, Status: m.Active, Remark: r.Remark}
		v.ExternalSubject = v.ID
		if e = tx.CreateUser(v); e != nil {
			return
		}
		event := audit(ctx, u, "user.create", "user", v.ID, v.Name, nil, nil, map[string]string{"username": v.Username, "platformRole": v.PlatformRole})
		a = &event
		return
	})
}
func (s *Service) Users(ctx context.Context, id string, f m.Filter) (m.Page[m.User], error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Page[m.User], error) {
		if e := requirePlatform(u); e != nil {
			return m.Page[m.User]{}, e
		}
		return tx.Users(f)
	})
}
func (s *Service) User(ctx context.Context, id, target string) (m.User, error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.User, error) {
		if e := requirePlatform(u); e != nil {
			return m.User{}, e
		}
		return tx.User(target)
	})
}
func (s *Service) UpdateUser(ctx context.Context, id string, r UpdateResource) (m.User, error) {
	if !detailsValid(r.Name, r.Remark) {
		return m.User{}, m.ErrInvalid
	}
	return s.changeUser(ctx, id, r.ID, r.Version, "user.update", func(v *m.User) { v.Name = r.Name; v.Remark = r.Remark })
}
func (s *Service) SetUserStatus(ctx context.Context, id string, r SetStatus) (m.User, error) {
	if r.Status != m.Active && r.Status != m.Disabled {
		return m.User{}, m.ErrInvalid
	}
	return s.changeUser(ctx, id, r.ID, r.Version, "user.setStatus", func(v *m.User) { v.Status = r.Status })
}
func (s *Service) SetPlatformRole(ctx context.Context, id string, r SetRole) (m.User, error) {
	if r.PlatformRole != m.PlatformAdmin && r.PlatformRole != m.NoPlatformRole {
		return m.User{}, m.ErrInvalid
	}
	return s.changeUser(ctx, id, r.ID, r.Version, "user.setPlatformRole", func(v *m.User) { v.PlatformRole = r.PlatformRole })
}
func (s *Service) changeUser(ctx context.Context, id, target string, version int64, action string, change func(*m.User)) (m.User, error) {
	return mutate(ctx, s, id, action, func(tx m.Tx, u m.User) (v m.User, a *m.Audit, e error) {
		if e = requirePlatform(u); e != nil {
			return
		}
		v, e = tx.User(target)
		if e != nil {
			return
		}
		before := map[string]string{"name": v.Name, "status": v.Status, "platformRole": v.PlatformRole}
		if e = changed(&v.Base, "user:"+u.ID, version); e != nil {
			return
		}
		change(&v)
		if e = tx.UpdateUser(v, version); e != nil {
			return
		}
		n, countErr := tx.ActiveAdminCount()
		if countErr != nil {
			e = countErr
			return
		}
		if n == 0 {
			e = m.ErrLastAdmin
			return
		}
		event := audit(ctx, u, action, "user", v.ID, v.Name, nil, nil, map[string]any{"before": before, "after": map[string]string{"name": v.Name, "status": v.Status, "platformRole": v.PlatformRole}})
		a = &event
		return
	})
}
func (s *Service) Tenants(ctx context.Context, id string, f m.Filter) (m.Page[m.Tenant], error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Page[m.Tenant], error) { return tx.Tenants(f, userScope(u)) })
}
func (s *Service) Tenant(ctx context.Context, id, target string) (m.Tenant, error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Tenant, error) { return tenantScope(tx, u, target, false) })
}
func (s *Service) CreateTenant(ctx context.Context, id string, r CreateResource) (m.Tenant, error) {
	r.Code = strings.ToLower(strings.TrimSpace(r.Code))
	if !codePattern.MatchString(r.Code) || !detailsValid(r.Name, r.Remark) {
		return m.Tenant{}, m.ErrInvalid
	}
	return mutate(ctx, s, id, "tenant.create", func(tx m.Tx, u m.User) (v m.Tenant, a *m.Audit, e error) {
		if e = requirePlatform(u); e != nil {
			return
		}
		v = m.Tenant{Base: newBase("user:" + u.ID), Code: r.Code, Name: r.Name, Status: m.Active, Remark: r.Remark}
		if e = tx.CreateTenant(v); e != nil {
			return
		}
		event := audit(ctx, u, "tenant.create", "tenant", v.ID, v.Name, &v, nil, map[string]string{"code": v.Code, "name": v.Name})
		a = &event
		return
	})
}
func (s *Service) UpdateTenant(ctx context.Context, id string, r UpdateResource) (m.Tenant, error) {
	if !detailsValid(r.Name, r.Remark) {
		return m.Tenant{}, m.ErrInvalid
	}
	return s.changeTenant(ctx, id, r.ID, r.Version, "tenant.update", func(v *m.Tenant) { v.Name = r.Name; v.Remark = r.Remark })
}
func (s *Service) SetTenantStatus(ctx context.Context, id string, r SetStatus) (m.Tenant, error) {
	if r.Status != m.Active && r.Status != m.Disabled {
		return m.Tenant{}, m.ErrInvalid
	}
	return s.changeTenant(ctx, id, r.ID, r.Version, "tenant.setStatus", func(v *m.Tenant) { v.Status = r.Status })
}
func (s *Service) changeTenant(ctx context.Context, id, target string, version int64, action string, change func(*m.Tenant)) (m.Tenant, error) {
	return mutate(ctx, s, id, action, func(tx m.Tx, u m.User) (v m.Tenant, a *m.Audit, e error) {
		if e = requirePlatform(u); e != nil {
			return
		}
		v, e = tx.Tenant(target)
		if e != nil {
			return
		}
		before := map[string]string{"name": v.Name, "status": v.Status}
		if e = changed(&v.Base, "user:"+u.ID, version); e != nil {
			return
		}
		change(&v)
		if e = tx.UpdateTenant(v, version); e != nil {
			return
		}
		event := audit(ctx, u, action, "tenant", v.ID, v.Name, &v, nil, map[string]any{"before": before, "after": map[string]string{"name": v.Name, "status": v.Status}})
		a = &event
		return
	})
}
func (s *Service) Members(ctx context.Context, id string, f m.Filter) (m.Page[m.Member], error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Page[m.Member], error) {
		if _, e := tenantScope(tx, u, f.TenantID, false); e != nil {
			return m.Page[m.Member]{}, e
		}
		return tx.Members(f)
	})
}
func (s *Service) AllocateMember(ctx context.Context, id, tenantID, userID string) (m.Member, error) {
	return mutate(ctx, s, id, "tenantMember.allocate", func(tx m.Tx, u m.User) (v m.Member, a *m.Audit, e error) {
		if e = requirePlatform(u); e != nil {
			return
		}
		tenant, e := tenantScope(tx, u, tenantID, false)
		if e != nil {
			return
		}
		if tenant.Status != m.Active {
			e = m.ErrForbidden
			return
		}
		target, e := tx.User(userID)
		if e != nil {
			return
		}
		if target.Status != m.Active {
			e = m.ErrForbidden
			return
		}
		v, e = tx.Membership(tenantID, userID)
		if e == nil {
			if v.Status != m.Active {
				e = m.ErrConflict
			}
			return
		}
		if !errors.Is(e, m.ErrNotFound) {
			return
		}
		v = m.Member{Base: newBase("user:" + u.ID), TenantID: tenantID, UserID: userID, MemberRole: m.TenantAdmin, Status: m.Active, UserName: target.Username, Name: target.Name}
		if e = tx.CreateMember(v); e != nil {
			return
		}
		event := audit(ctx, u, "tenantMember.allocate", "tenant_member", v.ID, target.Name, &tenant, nil, map[string]string{"userId": userID, "memberRole": v.MemberRole})
		a = &event
		return
	})
}
func (s *Service) SetMemberStatus(ctx context.Context, id string, r SetStatus) (m.Member, error) {
	if r.Status != m.Active && r.Status != m.Disabled {
		return m.Member{}, m.ErrInvalid
	}
	return mutate(ctx, s, id, "tenantMember.setStatus", func(tx m.Tx, u m.User) (v m.Member, a *m.Audit, e error) {
		if e = requirePlatform(u); e != nil {
			return
		}
		v, e = tx.Member(r.ID)
		if e != nil {
			return
		}
		tenant, e := tenantScope(tx, u, v.TenantID, false)
		if e != nil {
			return
		}
		if e = changed(&v.Base, "user:"+u.ID, r.Version); e != nil {
			return
		}
		v.Status = r.Status
		if e = tx.UpdateMember(v, r.Version); e != nil {
			return
		}
		event := audit(ctx, u, "tenantMember.setStatus", "tenant_member", v.ID, "", &tenant, nil, map[string]string{"status": v.Status})
		a = &event
		return
	})
}
