package management

import (
	"context"
	"errors"

	m "olixops-permission-center/internal/domain/management"
)

// CredentialCreationCheck describes the current snapshot only. Creation always
// repeats this check inside its write transaction; it does not accept a permit.
type CredentialCreationCheck struct {
	Allowed bool
	Reason  string
}

const (
	CredentialTenantDisabled    = "tenantDisabled"
	CredentialTenantAdminNeeded = "tenantAdminRequired"
	CredentialSystemDisabled    = "systemDisabled"
	CredentialAccountDisabled   = "accountDisabled"
	CredentialAlreadyActive     = "activeCredentialExists"
)

func (s *Service) CheckCredentialCreation(ctx context.Context, id, systemID string) (CredentialCreationCheck, error) {
	return read(ctx, s, id, func(tx m.Tx, u m.User) (CredentialCreationCheck, error) {
		sys, tenant, err := systemScope(tx, u, systemID)
		if err != nil {
			return CredentialCreationCheck{}, err
		}
		account, err := tx.SystemAccount(sys.ID)
		if err != nil {
			return CredentialCreationCheck{}, err
		}
		return checkCredentialCreation(tx, u, sys, tenant, account)
	})
}

func checkCredentialCreation(tx m.Tx, u m.User, sys m.System, tenant m.Tenant, account m.Account) (CredentialCreationCheck, error) {
	if tenant.Status != m.Active {
		return CredentialCreationCheck{Reason: CredentialTenantDisabled}, nil
	}
	if _, err := tenantScope(tx, u, tenant.ID, true); err != nil {
		if errors.Is(err, m.ErrNotFound) || errors.Is(err, m.ErrForbidden) {
			return CredentialCreationCheck{Reason: CredentialTenantAdminNeeded}, nil
		}
		return CredentialCreationCheck{}, err
	}
	if sys.Status != m.Active {
		return CredentialCreationCheck{Reason: CredentialSystemDisabled}, nil
	}
	if account.Status != m.Active {
		return CredentialCreationCheck{Reason: CredentialAccountDisabled}, nil
	}
	count, err := tx.ActiveCredentialCount(account.ID)
	if err != nil {
		return CredentialCreationCheck{}, err
	}
	if count != 0 {
		return CredentialCreationCheck{Reason: CredentialAlreadyActive}, nil
	}
	return CredentialCreationCheck{Allowed: true}, nil
}

func (c CredentialCreationCheck) creationError() error {
	if c.Allowed {
		return nil
	}
	if c.Reason == CredentialAlreadyActive {
		return m.ErrActiveCredential
	}
	return m.ErrForbidden
}
