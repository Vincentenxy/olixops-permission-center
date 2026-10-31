package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	m "olixops-permission-center/internal/domain/management"
)

type ManagementStore struct{ db *gorm.DB }

func NewManagementStore(db *gorm.DB) *ManagementStore { return &ManagementStore{db: db} }
func (s *ManagementStore) Read(ctx context.Context, fn func(m.Tx) error) error {
	if s == nil || s.db == nil {
		return m.ErrUnavailable
	}
	return translate(s.db.WithContext(ctx).Transaction(func(db *gorm.DB) error { return fn(&managementTx{db}) }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}))
}
func (s *ManagementStore) Write(ctx context.Context, fn func(m.Tx) error) error {
	if s == nil || s.db == nil {
		return m.ErrUnavailable
	}
	return translate(s.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		if err := db.Exec("SELECT pg_advisory_xact_lock(147, 1)").Error; err != nil {
			return err
		}
		return fn(&managementTx{db})
	}))
}

type managementTx struct{ db *gorm.DB }

func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return m.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return m.ErrDuplicate
	}
	for _, known := range []error{m.ErrUnavailable, m.ErrUnauthorized, m.ErrForbidden, m.ErrNotFound, m.ErrConflict, m.ErrInvalid, m.ErrDuplicate, m.ErrLastAdmin, m.ErrActiveCredential, m.ErrBootstrap} {
		if errors.Is(err, known) {
			return known
		}
	}
	return m.ErrUnavailable
}
func (t *managementTx) one(table, id string, out any) error {
	query := t.db.Table(table).Where("id = ? AND is_deleted = false", id)
	if table == "user_info" {
		query = query.Select("*, display_name AS name")
	}
	return translate(query.Take(out).Error)
}
func base(b m.Base) map[string]any {
	return map[string]any{"id": b.ID, "version": b.Version, "create_at": b.CreateAt, "create_by": b.CreateBy, "update_at": b.UpdateAt, "update_by": b.UpdateBy, "is_deleted": false}
}
func insert(t *managementTx, table string, b m.Base, fields map[string]any) error {
	for k, v := range base(b) {
		fields[k] = v
	}
	return translate(t.db.Table(table).Create(fields).Error)
}
func update(t *managementTx, table string, b m.Base, version int64, fields map[string]any) error {
	fields["version"] = version + 1
	fields["update_at"] = b.UpdateAt
	fields["update_by"] = b.UpdateBy
	r := t.db.Table(table).Where("id = ? AND version = ? AND is_deleted = false", b.ID, version).Updates(fields)
	if r.Error != nil {
		return translate(r.Error)
	}
	if r.RowsAffected != 1 {
		return m.ErrConflict
	}
	return nil
}
func pageQuery[T any](q *gorm.DB, f m.Filter, order string) (m.Page[T], error) {
	result := m.Page[T]{List: make([]T, 0)}
	// GORM interprets Select("alias.*") as one count column and emits the
	// invalid COUNT("alias"."*"). Count rows on a statement clone so joins and
	// scope filters remain identical while the list keeps its DTO projection.
	if err := q.Session(&gorm.Session{}).Select("count(*)").Count(&result.Total).Error; err != nil {
		return result, translate(err)
	}
	if err := q.Order(order).Limit(f.PageSize).Offset((f.PageNum - 1) * f.PageSize).Find(&result.List).Error; err != nil {
		return result, translate(err)
	}
	return result, nil
}
func filtered(q *gorm.DB, f m.Filter, prefix string) *gorm.DB {
	if f.Code != "" {
		q = q.Where(prefix+"code = ?", f.Code)
	}
	if f.Name != "" {
		q = q.Where(prefix+"name ILIKE ?", "%"+escapeLike(f.Name)+"%")
	}
	if f.Status != "" {
		q = q.Where(prefix+"status = ?", f.Status)
	}
	return q
}
func escapeLike(v string) string {
	result := ""
	for _, r := range v {
		if r == '%' || r == '_' || r == '\\' {
			result += "\\"
		}
		result += string(r)
	}
	return result
}
func scoped(q *gorm.DB, scope m.Scope, tenantColumn string) *gorm.DB {
	if scope.Global {
		return q
	}
	return q.Where("EXISTS (SELECT 1 FROM tenant_member_info tm JOIN tenant_info ti ON ti.id = tm.tenant_id WHERE tm.tenant_id = "+tenantColumn+" AND tm.user_id = ? AND tm.status = 'active' AND tm.member_role = 'tenant_admin' AND tm.is_deleted = false AND ti.status = 'active' AND ti.is_deleted = false)", scope.UserID)
}
func (t *managementTx) User(id string) (v m.User, e error) { e = t.one("user_info", id, &v); return }
func (t *managementTx) UserByUsername(username string) (v m.User, e error) {
	e = translate(t.db.Table("user_info").Select("*, display_name AS name").Where("username = ? AND is_deleted = false", username).Take(&v).Error)
	return
}
func (t *managementTx) Users(f m.Filter) (m.Page[m.User], error) {
	q := t.db.Table("user_info").Select("*, display_name AS name").Where("is_deleted = false")
	if f.Username != "" {
		q = q.Where("username ILIKE ?", "%"+escapeLike(f.Username)+"%")
	}
	if f.Name != "" {
		q = q.Where("display_name ILIKE ?", "%"+escapeLike(f.Name)+"%")
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.PlatformRole != "" {
		q = q.Where("platform_role = ?", f.PlatformRole)
	}
	return pageQuery[m.User](q, f, "create_at DESC, id DESC")
}
func (t *managementTx) UserCount() (n int64, e error) {
	e = translate(t.db.Table("user_info").Count(&n).Error)
	return
}
func (t *managementTx) ActiveAdminCount() (n int64, e error) {
	e = translate(t.db.Table("user_info").Where("is_deleted = false AND status = ? AND platform_role = ?", m.Active, m.PlatformAdmin).Count(&n).Error)
	return
}
func (t *managementTx) CreateUser(v m.User) error {
	return insert(t, "user_info", v.Base, map[string]any{"username": v.Username, "password_hash": v.PasswordHash, "external_issuer": v.ExternalIssuer, "subject_claim": v.SubjectClaim, "external_subject": v.ExternalSubject, "display_name": v.Name, "platform_role": v.PlatformRole, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) UpdateUser(v m.User, version int64) error {
	return update(t, "user_info", v.Base, version, map[string]any{"display_name": v.Name, "platform_role": v.PlatformRole, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) Tenant(id string) (v m.Tenant, e error) {
	e = t.one("tenant_info", id, &v)
	return
}
func (t *managementTx) Tenants(f m.Filter, s m.Scope) (m.Page[m.Tenant], error) {
	q := filtered(t.db.Table("tenant_info").Where("is_deleted = false"), f, "")
	return pageQuery[m.Tenant](scoped(q, s, "tenant_info.id"), f, "create_at DESC, id DESC")
}
func (t *managementTx) CreateTenant(v m.Tenant) error {
	return insert(t, "tenant_info", v.Base, map[string]any{"code": v.Code, "name": v.Name, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) UpdateTenant(v m.Tenant, n int64) error {
	return update(t, "tenant_info", v.Base, n, map[string]any{"name": v.Name, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) Member(id string) (v m.Member, e error) {
	e = t.one("tenant_member_info", id, &v)
	return
}
func (t *managementTx) Membership(tenant, user string) (v m.Member, e error) {
	e = translate(t.db.Table("tenant_member_info").Where("tenant_id = ? AND user_id = ? AND is_deleted = false", tenant, user).Take(&v).Error)
	return
}
func (t *managementTx) Members(f m.Filter) (m.Page[m.Member], error) {
	q := t.db.Table("tenant_member_info tm").Select("tm.*, ui.username AS user_name, ui.display_name AS name").Joins("JOIN user_info ui ON ui.id = tm.user_id AND ui.is_deleted = false").Where("tm.tenant_id = ? AND tm.is_deleted = false", f.TenantID)
	if f.Username != "" {
		q = q.Where("ui.username ILIKE ?", "%"+escapeLike(f.Username)+"%")
	}
	if f.Name != "" {
		q = q.Where("ui.display_name ILIKE ?", "%"+escapeLike(f.Name)+"%")
	}
	if f.Status != "" {
		q = q.Where("tm.status = ?", f.Status)
	}
	return pageQuery[m.Member](q, f, "tm.create_at DESC, tm.id DESC")
}
func (t *managementTx) CreateMember(v m.Member) error {
	return insert(t, "tenant_member_info", v.Base, map[string]any{"tenant_id": v.TenantID, "user_id": v.UserID, "member_role": v.MemberRole, "status": v.Status})
}
func (t *managementTx) UpdateMember(v m.Member, n int64) error {
	return update(t, "tenant_member_info", v.Base, n, map[string]any{"status": v.Status})
}
func (t *managementTx) System(id string) (v m.System, e error) {
	e = t.one("system_info", id, &v)
	return
}
func (t *managementTx) Systems(f m.Filter, s m.Scope) (m.Page[m.System], error) {
	q := filtered(t.db.Table("system_info").Where("is_deleted = false"), f, "")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	return pageQuery[m.System](scoped(q, s, "system_info.tenant_id"), f, "create_at DESC, id DESC")
}
func (t *managementTx) CreateSystem(v m.System) error {
	return insert(t, "system_info", v.Base, map[string]any{"tenant_id": v.TenantID, "code": v.Code, "name": v.Name, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) UpdateSystem(v m.System, n int64) error {
	return update(t, "system_info", v.Base, n, map[string]any{"name": v.Name, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) Account(id string) (v m.Account, e error) {
	e = t.one("system_account_info", id, &v)
	return
}
func (t *managementTx) SystemAccount(system string) (v m.Account, e error) {
	e = translate(t.db.Table("system_account_info").Where("system_id = ? AND is_deleted = false", system).Take(&v).Error)
	return
}
func (t *managementTx) CreateAccount(v m.Account) error {
	return insert(t, "system_account_info", v.Base, map[string]any{"system_id": v.SystemID, "name": v.Name, "status": v.Status, "remark": v.Remark})
}
func (t *managementTx) UpdateAccount(v m.Account, n int64) error {
	return update(t, "system_account_info", v.Base, n, map[string]any{"status": v.Status})
}
func (t *managementTx) Credential(id string) (v m.Credential, e error) {
	e = t.one("system_credential_info", id, &v)
	return
}
func (t *managementTx) CredentialByKeyID(key string) (v m.Credential, e error) {
	e = translate(t.db.Table("system_credential_info").Where("key_id = ? AND is_deleted = false", key).Take(&v).Error)
	return
}
func (t *managementTx) Credentials(f m.Filter) (m.Page[m.Credential], error) {
	q := t.db.Table("system_credential_info sc").Select("sc.*").Joins("JOIN system_account_info sa ON sa.id = sc.account_id AND sa.is_deleted = false").Where("sa.system_id = ? AND sc.is_deleted = false", f.SystemID)
	if f.Status != "" {
		q = q.Where("sc.status = ?", f.Status)
	}
	return pageQuery[m.Credential](q, f, "sc.create_at DESC, sc.id DESC")
}
func (t *managementTx) ActiveCredentialCount(account string) (n int64, e error) {
	e = translate(t.db.Table("system_credential_info").Where("account_id = ? AND status = ? AND is_deleted = false", account, m.Active).Count(&n).Error)
	return
}
func (t *managementTx) CreateCredential(v m.Credential) error {
	return insert(t, "system_credential_info", v.Base, map[string]any{"account_id": v.AccountID, "key_id": v.KeyID, "token_digest": v.TokenDigest, "name": v.Name, "status": v.Status, "revoked_by": v.RevokedBy})
}
func (t *managementTx) UpdateCredential(v m.Credential, n int64) error {
	return update(t, "system_credential_info", v.Base, n, map[string]any{"status": v.Status, "revoked_at": v.RevokedAt, "revoked_by": v.RevokedBy})
}
func (t *managementTx) Audit(id string) (v m.Audit, e error) {
	e = translate(t.db.Table("authorization_audit_log").Where("id = ?", id).Take(&v).Error)
	return
}
func (t *managementTx) Audits(f m.Filter, s m.Scope) (m.Page[m.Audit], error) {
	q := scoped(t.db.Table("authorization_audit_log"), s, "authorization_audit_log.tenant_id")
	for col, v := range map[string]string{"tenant_id": f.TenantID, "system_id": f.SystemID, "actor_id": f.ActorID, "action": f.Action, "result": f.Result, "request_id": f.RequestID} {
		if v != "" {
			q = q.Where(col+" = ?", v)
		}
	}
	if f.From != nil {
		q = q.Where("create_at >= ?", f.From)
	}
	if f.To != nil {
		q = q.Where("create_at <= ?", f.To)
	}
	return pageQuery[m.Audit](q, f, "create_at DESC, id DESC")
}
func (t *managementTx) AppendAudit(v m.Audit) error {
	return translate(t.db.Table("authorization_audit_log").Create(map[string]any{"id": v.ID, "actor_type": v.ActorType, "actor_id": v.ActorID, "actor_name": v.ActorName, "tenant_id": v.TenantID, "tenant_name": v.TenantName, "system_id": v.SystemID, "system_name": v.SystemName, "account_id": v.AccountID, "key_id": v.KeyID, "target_type": v.TargetType, "target_id": v.TargetID, "target_name": v.TargetName, "action": v.Action, "result": v.Result, "reason_code": v.ReasonCode, "request_id": v.RequestID, "changes": gorm.Expr("?::jsonb", string(v.Changes)), "create_at": v.CreateAt, "create_by": v.CreateBy}).Error)
}
