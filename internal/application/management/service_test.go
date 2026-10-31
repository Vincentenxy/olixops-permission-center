package management

import (
	"context"
	"errors"
	"testing"
	"time"

	m "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/credential"
)

const testUserID = "11111111-1111-4111-8111-111111111111"
const testTenantID = "22222222-2222-4222-8222-222222222222"
const testSystemID = "33333333-3333-4333-8333-333333333333"
const testAccountID = "44444444-4444-4444-8444-444444444444"

type testTx struct {
	m.Tx
	u            m.User
	t            m.Tenant
	s            m.System
	a            m.Account
	member       m.Member
	credential   m.Credential
	memberErr    error
	auditErr     error
	activeTokens int64
	countErr     error
	filter       m.Filter
	scope        m.Scope
	events       []m.Audit
}

func (t *testTx) User(string) (m.User, error) { return t.u, nil }
func (t *testTx) UserByUsername(username string) (m.User, error) {
	if username != t.u.Username {
		return m.User{}, m.ErrNotFound
	}
	return t.u, nil
}
func (t *testTx) Tenant(string) (m.Tenant, error)             { return t.t, nil }
func (t *testTx) System(string) (m.System, error)             { return t.s, nil }
func (t *testTx) Account(string) (m.Account, error)           { return t.a, nil }
func (t *testTx) Membership(string, string) (m.Member, error) { return t.member, t.memberErr }
func (t *testTx) ActiveCredentialCount(string) (int64, error) { return t.activeTokens, t.countErr }
func (t *testTx) SystemAccount(string) (m.Account, error)     { return t.a, nil }
func (t *testTx) Tenants(f m.Filter, scope m.Scope) (m.Page[m.Tenant], error) {
	t.filter, t.scope = f, scope
	return m.Page[m.Tenant]{List: []m.Tenant{}}, nil
}
func (t *testTx) CreateCredential(c m.Credential) error { t.credential = c; return nil }
func (t *testTx) CredentialByKeyID(key string) (m.Credential, error) {
	if key != t.credential.KeyID {
		return m.Credential{}, m.ErrNotFound
	}
	return t.credential, nil
}
func (t *testTx) AppendAudit(a m.Audit) error {
	if t.auditErr != nil {
		return t.auditErr
	}
	t.events = append(t.events, a)
	return nil
}

type testStore struct {
	tx      *testTx
	readErr error
}

func (s *testStore) Read(_ context.Context, fn func(m.Tx) error) error {
	if s.readErr != nil {
		return s.readErr
	}
	return fn(s.tx)
}
func (s *testStore) Write(_ context.Context, fn func(m.Tx) error) error {
	before := *s.tx
	if err := fn(s.tx); err != nil {
		*s.tx = before
		return err
	}
	return nil
}
func fixture() (*Service, *testTx) {
	tx := &testTx{u: m.User{Base: m.Base{ID: testUserID}, Username: "test.admin", Name: "Test Admin", Status: m.Active, PlatformRole: m.PlatformAdmin}, t: m.Tenant{Base: m.Base{ID: testTenantID}, Status: m.Active}, s: m.System{Base: m.Base{ID: testSystemID}, TenantID: testTenantID, Status: m.Active}, a: m.Account{Base: m.Base{ID: testAccountID}, SystemID: testSystemID, Status: m.Active}, member: m.Member{MemberRole: m.TenantAdmin, Status: m.Active}}
	return New(&testStore{tx: tx}, nil, "test-issuer"), tx
}
func TestTokenIssueRequiresActualTenantAdministrator(t *testing.T) {
	for _, tc := range []struct {
		name               string
		role, memberStatus string
		missing            bool
		want               error
	}{
		{"superadmin without membership", m.PlatformAdmin, "", true, m.ErrForbidden},
		{"superadmin disabled membership", m.PlatformAdmin, m.Disabled, false, m.ErrForbidden},
		{"superadmin explicit membership", m.PlatformAdmin, m.Active, false, nil},
		{"tenant admin explicit membership", m.NoPlatformRole, m.Active, false, nil},
		{"other tenant", m.NoPlatformRole, "", true, m.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tx := fixture()
			tx.u.PlatformRole = tc.role
			tx.member.Status = tc.memberStatus
			if tc.missing {
				tx.memberErr = m.ErrNotFound
			}
			issued, err := s.CreateCredential(context.Background(), testUserID, testAccountID, "Backend")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error %v, want %v", err, tc.want)
			}
			if tc.want != nil {
				if issued.Token != "" || tx.credential.ID != "" {
					t.Fatal("denied creation exposed/persisted token")
				}
				return
			}
			if !credential.Verify(issued.Token, tx.credential.TokenDigest) {
				t.Fatal("issued token cannot authenticate")
			}
			if tx.events[0].TenantID == nil || *tx.events[0].TenantID != testTenantID {
				t.Fatal("audit lost trusted tenant")
			}
		})
	}
}
func TestTokenIssueRollsBackIfAuditFailsAndRejectsSecondActiveToken(t *testing.T) {
	s, tx := fixture()
	tx.auditErr = m.ErrUnavailable
	issued, err := s.CreateCredential(context.Background(), testUserID, testAccountID, "Backend")
	if !errors.Is(err, m.ErrUnavailable) || issued.Token != "" || tx.credential.ID != "" {
		t.Fatal("failed audit must roll back and erase issued response")
	}
	tx.auditErr = nil
	tx.activeTokens = 1
	if _, err = s.CreateCredential(context.Background(), testUserID, testAccountID, "Backend"); !errors.Is(err, m.ErrActiveCredential) {
		t.Fatal("second active token permitted")
	}
}
func TestSystemAuthenticationRechecksEveryParentAndRevocation(t *testing.T) {
	for _, target := range []string{"credential", "account", "system", "tenant", "valid"} {
		t.Run(target, func(t *testing.T) {
			s, tx := fixture()
			issued, _ := credential.Generate()
			tx.credential = m.Credential{AccountID: testAccountID, KeyID: issued.ID, TokenDigest: issued.Digest, Status: m.Active}
			switch target {
			case "credential":
				tx.credential.Status = m.Revoked
			case "account":
				tx.a.Status = m.Disabled
			case "system":
				tx.s.Status = m.Disabled
			case "tenant":
				tx.t.Status = m.Disabled
			}
			identity, err := s.AuthenticateSystem(context.Background(), "Bearer "+issued.Token)
			if target == "valid" {
				if err != nil || identity.TenantID != testTenantID || identity.SystemID != testSystemID || identity.AccountID != testAccountID {
					t.Fatal("wrong scope")
				}
			} else if !errors.Is(err, m.ErrUnauthorized) || identity.TenantID != "" {
				t.Fatal("disabled chain accepted")
			}
		})
	}
}

type testSigner struct{}

func (testSigner) Sign(id, name string) (string, time.Time, error) {
	return "fake-signed-token", time.Now().UTC().Add(time.Hour), nil
}
func TestLocalLoginAndDisabledUser(t *testing.T) {
	s, tx := fixture()
	s.signer = testSigner{}
	hash, err := auth.HashPassword("Fictional-test-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	tx.u.PasswordHash = hash
	result, err := s.Login(context.Background(), " TEST.ADMIN ", "Fictional-test-password-2026")
	if err != nil || result.AccessToken == "" || result.User.ID != testUserID {
		t.Fatal("valid local login failed")
	}
	tx.u.Status = m.Disabled
	if _, err = s.Login(context.Background(), "test.admin", "Fictional-test-password-2026"); !errors.Is(err, m.ErrUnauthorized) {
		t.Fatal("disabled user logged in")
	}
	if _, err = s.Me(context.Background(), testUserID); !errors.Is(err, m.ErrUnauthorized) {
		t.Fatal("old JWT may use disabled identity")
	}
}
func TestKnownStorageFailureDoesNotBecomeUnauthorized(t *testing.T) {
	s, tx := fixture()
	s.signer = testSigner{}
	s.store = &testStore{tx: tx, readErr: m.ErrUnavailable}
	if _, err := s.Login(context.Background(), "test.admin", "Fictional-test-password-2026"); !errors.Is(err, m.ErrUnavailable) {
		t.Fatal("storage failure was disguised as credentials failure")
	}
}

func TestDeniedAuditPreservesVerifiedActorWithoutUntrustedTarget(t *testing.T) {
	s, tx := fixture()
	tx.u.PlatformRole = m.NoPlatformRole
	tx.memberErr = m.ErrNotFound
	if _, err := s.CreateCredential(context.Background(), testUserID, testAccountID, "Backend"); !errors.Is(err, m.ErrNotFound) {
		t.Fatal("cross-tenant access not denied")
	}
	if len(tx.events) != 1 {
		t.Fatal("denial event missing")
	}
	event := tx.events[0]
	if event.ActorType != "user" || event.ActorID == nil || *event.ActorID != testUserID || event.ActorName != tx.u.Name {
		t.Fatal("verified actor missing from denial")
	}
	if event.TenantID != nil || event.SystemID != nil || event.AccountID != nil || event.TargetID != nil {
		t.Fatal("unentitled target scope leaked into audit")
	}
	tx.events = nil
	if _, err := s.CreateUser(context.Background(), testUserID, CreateUser{}); !errors.Is(err, m.ErrForbidden) {
		t.Fatal("non-platform preflight was not denied")
	}
	if len(tx.events) != 1 || tx.events[0].ActorID == nil || *tx.events[0].ActorID != testUserID {
		t.Fatal("preflight denial lost verified actor")
	}
	tx.events = nil
	_, _ = s.CreateCredential(context.Background(), "unverified-input", testAccountID, "Backend")
	if len(tx.events) != 1 || tx.events[0].ActorID != nil || tx.events[0].ActorType != "anonymous" {
		t.Fatal("unverified input became an audit identity")
	}
}
