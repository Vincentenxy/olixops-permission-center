package management

import (
	"context"
	"errors"
	"strings"
	"time"

	m "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/pkg/credential"
)

func (s *Service) Systems(ctx context.Context, id string, f m.Filter) (m.Page[m.System], error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Page[m.System], error) {
		if f.TenantID != "" {
			if _, e := tenantScope(tx, u, f.TenantID, false); e != nil {
				return m.Page[m.System]{}, e
			}
		}
		return tx.Systems(f, userScope(u))
	})
}
func (s *Service) System(ctx context.Context, id, target string) (SystemDetail, error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (SystemDetail, error) {
		sys, _, err := systemScope(tx, u, target)
		if err != nil {
			return SystemDetail{}, err
		}
		account, err := tx.SystemAccount(sys.ID)
		return SystemDetail{System: sys, Account: account}, err
	})
}
func (s *Service) CreateSystem(ctx context.Context, id string, r CreateResource) (SystemDetail, error) {
	r.Code = strings.ToLower(strings.TrimSpace(r.Code))
	if !codePattern.MatchString(r.Code) || !detailsValid(r.Name, r.Remark) {
		return SystemDetail{}, m.ErrInvalid
	}
	return mutate(ctx, s, id, "system.create", func(tx m.Tx, u m.User) (v SystemDetail, a *m.Audit, e error) {
		tenant, e := tenantScope(tx, u, r.TenantID, false)
		if e != nil {
			return
		}
		if tenant.Status != m.Active {
			e = m.ErrForbidden
			return
		}
		v.System = m.System{Base: newBase("user:" + u.ID), TenantID: tenant.ID, Code: r.Code, Name: r.Name, Status: m.Active, Remark: r.Remark}
		v.Account = m.Account{Base: newBase("user:" + u.ID), SystemID: v.System.ID, Name: r.Name, Status: m.Active}
		if e = tx.CreateSystem(v.System); e != nil {
			return
		}
		if e = tx.CreateAccount(v.Account); e != nil {
			return
		}
		event := audit(ctx, u, "system.create", "system", v.System.ID, v.System.Name, &tenant, &v.System, map[string]string{"code": v.System.Code, "name": v.System.Name, "accountId": v.Account.ID})
		event.AccountID = ptr(v.Account.ID)
		a = &event
		return
	})
}
func (s *Service) UpdateSystem(ctx context.Context, id string, r UpdateResource) (m.System, error) {
	if !detailsValid(r.Name, r.Remark) {
		return m.System{}, m.ErrInvalid
	}
	return s.changeSystem(ctx, id, r.ID, r.Version, "system.update", func(v *m.System) { v.Name = r.Name; v.Remark = r.Remark })
}
func (s *Service) SetSystemStatus(ctx context.Context, id string, r SetStatus) (m.System, error) {
	if r.Status != m.Active && r.Status != m.Disabled {
		return m.System{}, m.ErrInvalid
	}
	return s.changeSystem(ctx, id, r.ID, r.Version, "system.setStatus", func(v *m.System) { v.Status = r.Status })
}
func (s *Service) changeSystem(ctx context.Context, id, target string, version int64, action string, change func(*m.System)) (m.System, error) {
	return mutate(ctx, s, id, action, func(tx m.Tx, u m.User) (v m.System, a *m.Audit, e error) {
		v, tenant, e := systemScope(tx, u, target)
		if e != nil {
			return v, nil, e
		}
		if tenant.Status != m.Active {
			return v, nil, m.ErrForbidden
		}
		before := map[string]string{"name": v.Name, "status": v.Status}
		if e = changed(&v.Base, "user:"+u.ID, version); e != nil {
			return v, nil, e
		}
		change(&v)
		if e = tx.UpdateSystem(v, version); e != nil {
			return v, nil, e
		}
		event := audit(ctx, u, action, "system", v.ID, v.Name, &tenant, &v, map[string]any{"before": before, "after": map[string]string{"name": v.Name, "status": v.Status}})
		return v, &event, nil
	})
}
func (s *Service) SetAccountStatus(ctx context.Context, id string, r SetStatus) (m.Account, error) {
	if r.Status != m.Active && r.Status != m.Disabled {
		return m.Account{}, m.ErrInvalid
	}
	return mutate(ctx, s, id, "systemAccount.setStatus", func(tx m.Tx, u m.User) (v m.Account, a *m.Audit, e error) {
		v, e = tx.Account(r.ID)
		if e != nil {
			return
		}
		sys, tenant, e := systemScope(tx, u, v.SystemID)
		if e != nil {
			return
		}
		if tenant.Status != m.Active {
			e = m.ErrForbidden
			return
		}
		if e = changed(&v.Base, "user:"+u.ID, r.Version); e != nil {
			return
		}
		v.Status = r.Status
		if e = tx.UpdateAccount(v, r.Version); e != nil {
			return
		}
		event := audit(ctx, u, "systemAccount.setStatus", "system_account", v.ID, v.Name, &tenant, &sys, map[string]string{"status": v.Status})
		event.AccountID = ptr(v.ID)
		a = &event
		return
	})
}
func (s *Service) Credentials(ctx context.Context, id string, f m.Filter) (m.Page[m.Credential], error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Page[m.Credential], error) {
		if _, _, e := systemScope(tx, u, f.SystemID); e != nil {
			return m.Page[m.Credential]{}, e
		}
		return tx.Credentials(f)
	})
}
func (s *Service) CreateCredential(ctx context.Context, id, accountID, name string) (IssuedCredential, error) {
	if !nameValid(name) {
		return IssuedCredential{}, m.ErrInvalid
	}
	return mutate(ctx, s, id, "systemCredential.create", func(tx m.Tx, u m.User) (v IssuedCredential, a *m.Audit, e error) {
		account, e := tx.Account(accountID)
		if e != nil {
			return
		}
		sys, tenant, e := systemScope(tx, u, account.SystemID)
		if e != nil {
			return
		}
		if _, e = tenantScope(tx, u, tenant.ID, true); e != nil {
			if errors.Is(e, m.ErrNotFound) {
				e = m.ErrForbidden
			}
			return
		}
		if tenant.Status != m.Active || sys.Status != m.Active || account.Status != m.Active {
			e = m.ErrForbidden
			return
		}
		count, e := tx.ActiveCredentialCount(account.ID)
		if e != nil {
			return
		}
		if count != 0 {
			e = m.ErrActiveCredential
			return
		}
		issued, e := credential.Generate()
		if e != nil {
			e = m.ErrUnavailable
			return
		}
		v.Credential = m.Credential{Base: newBase("user:" + u.ID), AccountID: account.ID, KeyID: issued.ID, TokenDigest: issued.Digest, Name: name, Status: m.Active}
		v.Token = issued.Token
		if e = tx.CreateCredential(v.Credential); e != nil {
			return
		}
		event := audit(ctx, u, "systemCredential.create", "system_credential", v.Credential.ID, name, &tenant, &sys, map[string]string{"keyId": issued.ID, "name": name})
		event.AccountID = ptr(account.ID)
		event.KeyID = issued.ID
		a = &event
		return
	})
}
func (s *Service) RevokeCredential(ctx context.Context, id, target string, version int64) (m.Credential, error) {
	return mutate(ctx, s, id, "systemCredential.revoke", func(tx m.Tx, u m.User) (v m.Credential, a *m.Audit, e error) {
		v, e = tx.Credential(target)
		if e != nil {
			return
		}
		account, e := tx.Account(v.AccountID)
		if e != nil {
			return
		}
		sys, tenant, e := systemScope(tx, u, account.SystemID)
		if e != nil {
			return
		}
		if v.Status == m.Revoked {
			return
		}
		if e = changed(&v.Base, "user:"+u.ID, version); e != nil {
			return
		}
		now := time.Now().UTC()
		v.Status = m.Revoked
		v.RevokedAt = &now
		v.RevokedBy = "user:" + u.ID
		if e = tx.UpdateCredential(v, version); e != nil {
			return
		}
		event := audit(ctx, u, "systemCredential.revoke", "system_credential", v.ID, v.Name, &tenant, &sys, map[string]string{"keyId": v.KeyID, "status": v.Status})
		event.AccountID = ptr(account.ID)
		event.KeyID = v.KeyID
		a = &event
		return
	})
}

// AuthenticateSystem uses one database snapshot, so the entire credential chain
// is observed consistently. Every request rechecks revocation and parent status.
func (s *Service) AuthenticateSystem(ctx context.Context, header string) (out m.SystemIdentity, err error) {
	if s == nil || s.store == nil {
		return out, m.ErrUnavailable
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return out, m.ErrUnauthorized
	}
	key, err := credential.ParseID(parts[1])
	if err != nil {
		return out, m.ErrUnauthorized
	}
	err = s.store.Read(ctx, func(tx m.Tx) error {
		c, e := tx.CredentialByKeyID(key)
		if e != nil {
			return e
		}
		if !credential.Verify(parts[1], c.TokenDigest) || c.Status != m.Active || c.RevokedAt != nil {
			return m.ErrUnauthorized
		}
		account, e := tx.Account(c.AccountID)
		if e != nil {
			return e
		}
		sys, e := tx.System(account.SystemID)
		if e != nil {
			return e
		}
		tenant, e := tx.Tenant(sys.TenantID)
		if e != nil {
			return e
		}
		if account.Status != m.Active || sys.Status != m.Active || tenant.Status != m.Active {
			return m.ErrUnauthorized
		}
		out = m.SystemIdentity{TenantID: tenant.ID, SystemID: sys.ID, AccountID: account.ID, KeyID: c.KeyID}
		return nil
	})
	if errors.Is(err, m.ErrNotFound) {
		err = m.ErrUnauthorized
	}
	if err != nil {
		return m.SystemIdentity{}, err
	}
	return
}
func (s *Service) Audits(ctx context.Context, id string, f m.Filter) (m.Page[m.Audit], error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Page[m.Audit], error) {
		if f.TenantID != "" {
			if _, e := tenantScope(tx, u, f.TenantID, false); e != nil {
				return m.Page[m.Audit]{}, e
			}
		}
		if f.SystemID != "" {
			if _, _, e := systemScope(tx, u, f.SystemID); e != nil {
				return m.Page[m.Audit]{}, e
			}
		}
		return tx.Audits(f, userScope(u))
	})
}
func (s *Service) Audit(ctx context.Context, id, target string) (m.Audit, error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (m.Audit, error) {
		v, e := tx.Audit(target)
		if e != nil {
			return v, e
		}
		if u.PlatformRole == m.PlatformAdmin {
			return v, nil
		}
		if v.TenantID == nil {
			return m.Audit{}, m.ErrNotFound
		}
		if _, e = tenantScope(tx, u, *v.TenantID, false); e != nil {
			return m.Audit{}, e
		}
		return v, nil
	})
}
