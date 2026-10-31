package management

import (
	"context"
	"errors"
	"testing"

	m "olixops-permission-center/internal/domain/management"
)

func TestMyTenantsNeverUsesPlatformScope(t *testing.T) {
	for _, role := range []string{m.PlatformAdmin, m.NoPlatformRole} {
		s, tx := fixture()
		tx.u.PlatformRole = role
		f := m.Filter{PageNum: 3, PageSize: 17, Name: "Company"}
		if _, err := s.MyTenants(context.Background(), testUserID, f); err != nil {
			t.Fatal(err)
		}
		if tx.scope.Global || tx.scope.UserID != testUserID || tx.filter != f {
			t.Fatal("actual memberships must use the authenticated user and unchanged pagination")
		}
	}
}

func TestCredentialCheckMatchesCreationRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testTx)
		reason string
		want   error
	}{
		{"eligible", func(*testTx) {}, "", nil},
		{"tenant admin", func(tx *testTx) { tx.u.PlatformRole = m.NoPlatformRole }, "", nil},
		{"platform alone", func(tx *testTx) { tx.memberErr = m.ErrNotFound }, CredentialTenantAdminNeeded, m.ErrForbidden},
		{"disabled membership", func(tx *testTx) { tx.member.Status = m.Disabled }, CredentialTenantAdminNeeded, m.ErrForbidden},
		{"tenant disabled", func(tx *testTx) { tx.t.Status = m.Disabled }, CredentialTenantDisabled, m.ErrForbidden},
		{"system disabled", func(tx *testTx) { tx.s.Status = m.Disabled }, CredentialSystemDisabled, m.ErrForbidden},
		{"account disabled", func(tx *testTx) { tx.a.Status = m.Disabled }, CredentialAccountDisabled, m.ErrForbidden},
		{"active token", func(tx *testTx) { tx.activeTokens = 1 }, CredentialAlreadyActive, m.ErrActiveCredential},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tx := fixture()
			tc.change(tx)
			check, err := s.CheckCredentialCreation(context.Background(), testUserID, testSystemID)
			if err != nil || check.Allowed != (tc.want == nil) || check.Reason != tc.reason {
				t.Fatalf("unexpected eligibility: %+v, %v", check, err)
			}
			if len(tx.events) != 0 || tx.credential.ID != "" {
				t.Fatal("eligibility check performed a write")
			}
			_, err = s.CreateCredential(context.Background(), testUserID, testAccountID, "Backend")
			if !errors.Is(err, tc.want) {
				t.Fatalf("creation error %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCredentialCheckRejectsScopeAndPropagatesStorageErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testTx)
		want   error
	}{
		{"cross tenant", func(tx *testTx) { tx.u.PlatformRole = m.NoPlatformRole; tx.memberErr = m.ErrNotFound }, m.ErrNotFound},
		{"disabled user", func(tx *testTx) { tx.u.Status = m.Disabled }, m.ErrUnauthorized},
		{"membership storage", func(tx *testTx) { tx.memberErr = m.ErrUnavailable }, m.ErrUnavailable},
		{"credential storage", func(tx *testTx) { tx.countErr = m.ErrUnavailable }, m.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tx := fixture()
			tc.change(tx)
			check, err := s.CheckCredentialCreation(context.Background(), testUserID, testSystemID)
			if !errors.Is(err, tc.want) || check != (CredentialCreationCheck{}) {
				t.Fatalf("failure became a successful eligibility result: %+v, %v", check, err)
			}
		})
	}
}
