// Package integration exercises the actual HTTP, application and PostgreSQL
// boundaries. Set TEST_DATABASE_DSN to an expendable database ending in _test.
package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	management "olixops-permission-center/internal/application/management"
	domain "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/internal/infrastructure/persistence"
	httpinterface "olixops-permission-center/internal/interfaces/http"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/config"
)

const fixturePassword = "Fictional-integration-password-2026!"

type fixture struct {
	ctx     context.Context
	db      *sql.DB
	server  *httptest.Server
	client  *http.Client
	service *management.Service
	schema  string
}

type reply struct {
	Status    int
	Code      int
	Msg       string
	Data      json.RawMessage
	RequestID string
}

type httpCall struct {
	path, token string
	body        any
}

type resource struct {
	ID           string `json:"id"`
	Version      int64  `json:"version"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	Status       string `json:"status"`
	TenantID     string `json:"tenantId"`
	SystemID     string `json:"systemId"`
	AccountID    string `json:"accountId"`
	PlatformRole string `json:"platformRole"`
}

type profile struct {
	UserID       string `json:"userId"`
	Username     string `json:"username"`
	PlatformRole string `json:"platformRole"`
}

type loginResult struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        profile   `json:"user"`
}

type systemDetail struct {
	System  resource `json:"system"`
	Account resource `json:"account"`
}

type issuedCredential struct {
	Credential resource `json:"credential"`
	Token      string   `json:"token"`
}

type page[T any] struct {
	Total int64 `json:"total"`
	List  []T   `json:"list"`
}

type auditEvent struct {
	ID        string  `json:"id"`
	ActorType string  `json:"actorType"`
	ActorID   *string `json:"actorId"`
	ActorName string  `json:"actorName"`
	TenantID  *string `json:"tenantId"`
	SystemID  *string `json:"systemId"`
	AccountID *string `json:"accountId"`
	TargetID  *string `json:"targetId"`
	Result    string  `json:"result"`
	RequestID string  `json:"requestId"`
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("real PostgreSQL integration skipped: set TEST_DATABASE_DSN to a database ending in _test")
	}
	connection, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(connection.Database, "_test") {
		t.Fatal("TEST_DATABASE_DSN must parse and explicitly select a database ending in _test")
	}
	connection.ConnectTimeout = 5 * time.Second
	connection.RuntimeParams["statement_timeout"] = "10000"
	connection.RuntimeParams["lock_timeout"] = "5000"
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("cannot generate scratch schema identifier")
	}
	schema := "opc_integration_" + hex.EncodeToString(random[:])
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	f := &fixture{ctx: ctx, schema: quotedSchema}
	control := stdlib.OpenDB(*connection)
	created := false
	t.Cleanup(func() {
		if f.server != nil {
			f.client.CloseIdleConnections()
			f.server.Close()
		}
		if f.db != nil {
			_ = f.db.Close()
		}
		_ = control.Close()
		cancel()
		if !created {
			return
		}
		// Only the exact random schema created by this fixture is removed, after
		// its HTTP server and application pool have stopped. Never drop public.
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		cleanupDB := stdlib.OpenDB(*connection)
		defer cleanupDB.Close()
		if _, err := cleanupDB.ExecContext(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("could not remove owned integration schema %s", schema)
		}
	})
	if _, err := control.ExecContext(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal("cannot create isolated test schema; check test database connectivity and CREATE permission")
	}
	created = true
	ddl := canonicalDDL(t)
	ddl = strings.ReplaceAll(ddl, "public.", quotedSchema+".")
	if _, err := control.ExecContext(ctx, ddl); err != nil {
		t.Fatal("canonical seven-table SQL could not initialize the isolated schema")
	}
	if err := control.Close(); err != nil {
		t.Fatal("cannot close schema initialization connection")
	}
	scoped := connection.Copy()
	scoped.RuntimeParams["search_path"] = schema
	f.db = stdlib.OpenDB(*scoped)
	f.db.SetMaxOpenConns(8)
	f.db.SetMaxIdleConns(4)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: f.db}), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatal("cannot connect management persistence to isolated schema")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("cannot generate ephemeral test signing key")
	}
	private := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	public := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)}))
	cfg := config.Config{AuthJWTPublicKey: public, AuthJWTIssuer: "integration-issuer", AuthJWTAudience: "integration-audience", HTTPWriteTimeout: 10 * time.Second}
	signer, err := auth.NewSigner(private, public, cfg.AuthJWTIssuer, cfg.AuthJWTAudience, time.Hour)
	if err != nil {
		t.Fatal("cannot initialize ephemeral test signer")
	}
	f.service = management.New(persistence.NewManagementStore(db), signer, cfg.AuthJWTIssuer)
	f.server = httptest.NewServer(httpinterface.NewRouter(cfg, f.service))
	f.client = f.server.Client()
	f.client.Timeout = 15 * time.Second
	return f
}

func canonicalDDL(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "design", "database.md"))
	if err != nil {
		t.Fatal("cannot read canonical database design")
	}
	var found string
	for _, block := range strings.Split(string(contents), "```sql")[1:] {
		end := strings.Index(block, "```")
		if end < 0 {
			t.Fatal("database design has an unterminated SQL block")
		}
		sqlText := strings.TrimSpace(block[:end])
		if !strings.Contains(sqlText, "CREATE TABLE public.tenant_info") {
			continue
		}
		if found != "" || strings.Count(sqlText, "CREATE TABLE public.") != 7 || !strings.HasPrefix(sqlText, "BEGIN;") || !strings.HasSuffix(sqlText, "COMMIT;") {
			t.Fatal("expected exactly one canonical transaction defining all seven management tables")
		}
		found = sqlText
	}
	if found == "" {
		t.Fatal("canonical management DDL was not found")
	}
	return found
}

// call is safe to use concurrently. Errors omit tokens, request payloads, DSNs,
// and response bodies; assertions inspect secrets without printing them.
func (f *fixture) call(path, token string, body any, requestID string) (reply, error) {
	var payload io.Reader
	method := http.MethodGet
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return reply{}, errors.New("cannot encode fixture request")
		}
		payload = bytes.NewReader(encoded)
		method = http.MethodPost
	}
	request, err := http.NewRequestWithContext(f.ctx, method, f.server.URL+"/api/v1"+path, payload)
	if err != nil {
		return reply{}, errors.New("cannot construct fixture request")
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if requestID != "" {
		request.Header.Set("x-request-id", requestID)
	}
	response, err := f.client.Do(request)
	if err != nil {
		return reply{}, errors.New("fixture HTTP request failed")
	}
	defer response.Body.Close()
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&fields); err != nil {
		return reply{}, errors.New("HTTP response is not a JSON object")
	}
	r := reply{Status: response.StatusCode, RequestID: response.Header.Get("x-request-id"), Data: fields["data"]}
	if len(fields) != 3 || fields["code"] == nil || fields["msg"] == nil || fields["data"] == nil || json.Unmarshal(fields["code"], &r.Code) != nil || json.Unmarshal(fields["msg"], &r.Msg) != nil {
		return reply{}, errors.New("HTTP response violates code/msg/data contract")
	}
	if r.Status >= 400 && (r.Code != r.Status || r.Msg != http.StatusText(r.Status) || string(r.Data) != "null") {
		return reply{}, errors.New("HTTP failure violates standard status envelope")
	}
	if r.RequestID == "" || (requestID != "" && r.RequestID != requestID) {
		return reply{}, errors.New("HTTP response lost request ID")
	}
	return r, nil
}

func (f *fixture) expect(t *testing.T, path, token string, body any, status, code int) reply {
	t.Helper()
	r, err := f.call(path, token, body, "")
	if err != nil {
		t.Fatalf("%s: %s", path, err)
	}
	if r.Status != status || r.Code != code {
		t.Fatalf("%s: got HTTP/code %d/%d, want %d/%d", path, r.Status, r.Code, status, code)
	}
	return r
}

func (f *fixture) ok(t *testing.T, path, token string, body any) reply {
	t.Helper()
	return f.expect(t, path, token, body, http.StatusOK, 0)
}

func decode[T any](t *testing.T, r reply) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(r.Data, &value); err != nil {
		t.Fatal("successful response has unexpected data shape")
	}
	return value
}

func decodePage[T any](t *testing.T, r reply) page[T] {
	t.Helper()
	fields := decode[map[string]json.RawMessage](t, r)
	if len(fields) != 2 || fields["total"] == nil || fields["list"] == nil || string(fields["list"]) == "null" {
		t.Fatal("pagination must return only total/list with a non-null list")
	}
	return decode[page[T]](t, r)
}

func noSecrets(t *testing.T, r reply, values ...string) {
	t.Helper()
	for _, forbidden := range append([]string{"passwordHash", "password_hash", "tokenDigest", "token_digest"}, values...) {
		if forbidden != "" && bytes.Contains(r.Data, []byte(forbidden)) {
			t.Fatal("public response exposed a forbidden credential value or field")
		}
	}
}

func (f *fixture) bootstrap(t *testing.T) domain.User {
	t.Helper()
	u, err := f.service.Bootstrap(f.ctx, management.CreateUser{Username: "initial.admin", Password: fixturePassword, Name: "Initial administrator"})
	if err != nil {
		t.Fatal("offline bootstrap failed against empty scratch schema")
	}
	return u
}

func (f *fixture) login(t *testing.T, username string) loginResult {
	t.Helper()
	r := f.ok(t, "/pub/auth/login", "", map[string]any{"username": username, "password": fixturePassword})
	v := decode[loginResult](t, r)
	if v.AccessToken == "" || !v.ExpiresAt.After(time.Now()) || v.User.Username != username {
		t.Fatal("login did not return a bounded token and expected identity")
	}
	noSecrets(t, r, fixturePassword)
	return v
}

func (f *fixture) createUser(t *testing.T, token, username string) resource {
	t.Helper()
	r := f.ok(t, "/user/create", token, map[string]any{"username": username, "password": fixturePassword, "name": username})
	noSecrets(t, r, fixturePassword)
	return decode[resource](t, r)
}

func (f *fixture) setStatus(t *testing.T, path, token string, target resource, status string) resource {
	t.Helper()
	return decode[resource](t, f.ok(t, path, token, map[string]any{"id": target.ID, "version": target.Version, "status": status}))
}

func (f *fixture) scalar(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var result int64
	if err := f.db.QueryRowContext(f.ctx, query, args...).Scan(&result); err != nil {
		t.Fatal("isolated verification query failed")
	}
	return result
}

func concurrentCalls(t *testing.T, f *fixture, requests ...httpCall) []reply {
	t.Helper()
	results := make([]reply, len(requests))
	errorsByRequest := make([]error, len(requests))
	start := make(chan struct{})
	var done sync.WaitGroup
	for i, request := range requests {
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			results[i], errorsByRequest[i] = f.call(request.path, request.token, request.body, "")
		}()
	}
	close(start)
	done.Wait()
	for _, err := range errorsByRequest {
		if err != nil {
			t.Fatalf("concurrent fixture request: %s", err)
		}
	}
	return results
}

func TestManagementHTTPRealPostgres(t *testing.T) {
	f := newFixture(t)
	initial := f.bootstrap(t)
	if _, err := f.service.Bootstrap(f.ctx, management.CreateUser{Username: "second.bootstrap", Password: fixturePassword, Name: "Must not initialize again"}); !errors.Is(err, domain.ErrBootstrap) {
		t.Fatal("bootstrap did not reject a nonempty user table")
	}
	f.expect(t, "/auth/me", "", nil, 401, 401)
	f.expect(t, "/pub/auth/login", "", map[string]any{"username": initial.Username, "password": "Incorrect-fixture-password"}, 401, 401)
	adminLogin := f.login(t, initial.Username)
	admin := adminLogin.AccessToken
	me := f.ok(t, "/auth/me", admin, nil)
	if decode[profile](t, me).UserID != initial.ID || adminLogin.User.PlatformRole != "platform_admin" {
		t.Fatal("JWT login did not bind the bootstrapped local identity")
	}
	noSecrets(t, me, fixturePassword, admin)
	f.expect(t, "/user/setStatus", admin, map[string]any{"id": initial.ID, "version": 1, "status": "disabled"}, 200, -1)
	f.expect(t, "/user/setPlatformRole", admin, map[string]any{"id": initial.ID, "version": 1, "platformRole": "none"}, 200, -1)

	a := decode[resource](t, f.ok(t, "/tenant/create", admin, map[string]any{"code": "tenant_a", "name": "Tenant A"}))
	b := decode[resource](t, f.ok(t, "/tenant/create", admin, map[string]any{"code": "tenant_b", "name": "Tenant B"}))
	alice := f.createUser(t, admin, "alice.admin")
	multi := f.createUser(t, admin, "multi.admin")
	member := decode[resource](t, f.ok(t, "/tenant/member/allocate", admin, map[string]any{"tenantId": a.ID, "userId": alice.ID}))
	for _, tenant := range []resource{a, b} {
		f.ok(t, "/tenant/member/allocate", admin, map[string]any{"tenantId": tenant.ID, "userId": multi.ID})
	}
	aliceToken := f.login(t, alice.Username).AccessToken
	multiToken := f.login(t, multi.Username).AccessToken
	visible := decodePage[resource](t, f.ok(t, "/tenant/list", aliceToken, map[string]any{"pageNum": 0, "pageSize": 0}))
	if visible.Total != 1 || len(visible.List) != 1 || visible.List[0].ID != a.ID {
		t.Fatal("tenant list or total leaked another tenant")
	}
	if decodePage[resource](t, f.ok(t, "/tenant/list", multiToken, map[string]any{})).Total != 2 {
		t.Fatal("one administrator cannot manage both assigned tenants")
	}
	f.expect(t, "/tenant/detail", aliceToken, map[string]any{"id": b.ID}, 404, 404)
	f.expect(t, "/tenant/create", aliceToken, map[string]any{"code": "forbidden", "name": "Forbidden"}, 403, 403)
	sa := decode[systemDetail](t, f.ok(t, "/system/create", aliceToken, map[string]any{"tenantId": a.ID, "code": "same_code", "name": "System A"}))
	sb := decode[systemDetail](t, f.ok(t, "/system/create", admin, map[string]any{"tenantId": b.ID, "code": "same_code", "name": "System B"}))
	if sa.Account.SystemID != sa.System.ID || sb.System.TenantID != b.ID {
		t.Fatal("system/account creation lost its tenant ownership")
	}
	if f.scalar(t, "SELECT count(*) FROM system_credential_info") != 0 {
		t.Fatal("system creation issued a token without tenant-admin approval")
	}
	systems := decodePage[resource](t, f.ok(t, "/system/list", aliceToken, map[string]any{}))
	if systems.Total != 1 || len(systems.List) != 1 || systems.List[0].ID != sa.System.ID {
		t.Fatal("system list or total escaped tenant scope")
	}
	f.expect(t, "/system/detail", aliceToken, map[string]any{"id": sb.System.ID}, 404, 404)
	f.expect(t, "/system/update", aliceToken, map[string]any{"id": sb.System.ID, "version": 1, "name": "Forbidden rename"}, 404, 404)
	if decode[systemDetail](t, f.ok(t, "/system/detail", admin, map[string]any{"id": sb.System.ID})).System.Name != "System B" {
		t.Fatal("denied cross-tenant mutation changed the target")
	}
	f.expect(t, "/system/credential/create", admin, map[string]any{"accountId": sb.Account.ID, "name": "Superadmin alone"}, 403, 403)
	denialTrace := "integration-cross-tenant-denial"
	denied, err := f.call("/system/credential/create", aliceToken, map[string]any{"accountId": sb.Account.ID, "name": "Forbidden issuance"}, denialTrace)
	if err != nil || denied.Status != 404 {
		t.Fatal("cross-tenant token issuance was not safely denied")
	}
	denialPage := decodePage[auditEvent](t, f.ok(t, "/audit/list", admin, map[string]any{"requestId": denialTrace}))
	if denialPage.Total != 1 || len(denialPage.List) != 1 {
		t.Fatal("cross-tenant denial audit missing or duplicated")
	}
	event := denialPage.List[0]
	if event.ActorType != "user" || event.ActorID == nil || *event.ActorID != alice.ID || event.ActorName != alice.Name || event.Result != "denied" || event.RequestID != denialTrace || event.TenantID != nil || event.SystemID != nil || event.AccountID != nil || event.TargetID != nil {
		t.Fatal("denial audit lost verified actor or trusted an unauthorized target")
	}
	f.expect(t, "/audit/detail", aliceToken, map[string]any{"id": event.ID}, 404, 404)
	f.ok(t, "/tenant/member/allocate", admin, map[string]any{"tenantId": b.ID, "userId": initial.ID})
	f.ok(t, "/system/credential/create", admin, map[string]any{"accountId": sb.Account.ID, "name": "Explicit tenant administrator"})

	results := concurrentCalls(t, f,
		httpCall{"/system/credential/create", aliceToken, map[string]any{"accountId": sa.Account.ID, "name": "Concurrent A"}},
		httpCall{"/system/credential/create", aliceToken, map[string]any{"accountId": sa.Account.ID, "name": "Concurrent B"}},
	)
	var issued issuedCredential
	successes, conflicts := 0, 0
	for _, r := range results {
		switch {
		case r.Status == 200 && r.Code == 0:
			successes++
			issued = decode[issuedCredential](t, r)
		case r.Status == 200 && r.Code == -1:
			conflicts++
		default:
			t.Fatal("concurrent issuance returned an unexpected outcome")
		}
	}
	if successes != 1 || conflicts != 1 || issued.Token == "" || f.scalar(t, "SELECT count(*) FROM system_credential_info WHERE account_id = $1 AND status = 'active'", sa.Account.ID) != 1 {
		t.Fatal("concurrent issuance did not preserve exactly one active fixed token")
	}
	identity := decode[resource](t, f.ok(t, "/systemAccess/me", issued.Token, nil))
	if identity.TenantID != a.ID || identity.SystemID != sa.System.ID || identity.AccountID != sa.Account.ID {
		t.Fatal("system token did not resolve its server-owned identity chain")
	}
	f.expect(t, "/auth/me", issued.Token, nil, 401, 401)
	f.expect(t, "/systemAccess/me", admin, nil, 401, 401)
	credentialList := f.ok(t, "/system/credential/list", aliceToken, map[string]any{"systemId": sa.System.ID})
	noSecrets(t, credentialList, issued.Token, "expiresAt")
	if decodePage[resource](t, credentialList).Total != 1 {
		t.Fatal("credential metadata list does not match stored credentials")
	}

	sa.System = decode[resource](t, f.ok(t, "/system/update", aliceToken, map[string]any{"id": sa.System.ID, "version": sa.System.Version, "name": "Updated system A"}))
	f.expect(t, "/system/update", aliceToken, map[string]any{"id": sa.System.ID, "version": 1, "name": "Stale edit"}, 409, 409)
	sa.System = f.setStatus(t, "/system/setStatus", aliceToken, sa.System, "disabled")
	f.expect(t, "/systemAccess/me", issued.Token, nil, 401, 401)
	sa.System = f.setStatus(t, "/system/setStatus", aliceToken, sa.System, "active")
	f.ok(t, "/systemAccess/me", issued.Token, nil)
	sa.Account = f.setStatus(t, "/system/account/setStatus", aliceToken, sa.Account, "disabled")
	f.expect(t, "/systemAccess/me", issued.Token, nil, 401, 401)
	sa.Account = f.setStatus(t, "/system/account/setStatus", aliceToken, sa.Account, "active")
	f.ok(t, "/systemAccess/me", issued.Token, nil)
	a = f.setStatus(t, "/tenant/setStatus", admin, a, "disabled")
	f.expect(t, "/systemAccess/me", issued.Token, nil, 401, 401)
	empty := decodePage[resource](t, f.ok(t, "/tenant/list", aliceToken, map[string]any{}))
	if empty.Total != 0 || len(empty.List) != 0 {
		t.Fatal("disabled tenant remained visible")
	}
	a = f.setStatus(t, "/tenant/setStatus", admin, a, "active")
	f.ok(t, "/systemAccess/me", issued.Token, nil)
	member = f.setStatus(t, "/tenant/member/setStatus", admin, member, "disabled")
	f.expect(t, "/system/detail", aliceToken, map[string]any{"id": sa.System.ID}, 404, 404)
	f.ok(t, "/systemAccess/me", issued.Token, nil)
	f.setStatus(t, "/tenant/member/setStatus", admin, member, "active")

	audits := f.ok(t, "/audit/list", aliceToken, map[string]any{"pageSize": 200})
	noSecrets(t, audits, issued.Token, fixturePassword, admin, aliceToken)
	entries := decodePage[auditEvent](t, audits)
	if entries.Total == 0 || entries.Total != int64(len(entries.List)) || entries.Total != f.scalar(t, "SELECT count(*) FROM authorization_audit_log WHERE tenant_id = $1", a.ID) {
		t.Fatal("scoped audit list and total disagree")
	}
	for _, entry := range entries.List {
		if entry.TenantID == nil || *entry.TenantID != a.ID {
			t.Fatal("audit list exposed a different tenant or platform event")
		}
	}
	f.expect(t, "/audit/list", aliceToken, map[string]any{"tenantId": b.ID}, 404, 404)
	bAudits := decodePage[auditEvent](t, f.ok(t, "/audit/list", admin, map[string]any{"tenantId": b.ID}))
	if len(bAudits.List) == 0 {
		t.Fatal("fixture has no tenant B audit to check")
	}
	f.expect(t, "/audit/detail", aliceToken, map[string]any{"id": bAudits.List[0].ID}, 404, 404)
	f.ok(t, "/system/credential/revoke", aliceToken, map[string]any{"id": issued.Credential.ID, "version": issued.Credential.Version})
	f.expect(t, "/systemAccess/me", issued.Token, nil, 401, 401)
	a = f.setStatus(t, "/tenant/setStatus", admin, a, "disabled")
	f.setStatus(t, "/tenant/setStatus", admin, a, "active")
	f.expect(t, "/systemAccess/me", issued.Token, nil, 401, 401)

	t.Run("audit insert failure rolls back business write", func(t *testing.T) {
		before := f.scalar(t, "SELECT count(*) FROM tenant_info")
		if _, err := f.db.ExecContext(f.ctx, "ALTER TABLE "+f.schema+".authorization_audit_log RENAME TO test_audit_unavailable"); err != nil {
			t.Fatal("cannot isolate audit write failure")
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := f.db.ExecContext(ctx, "ALTER TABLE "+f.schema+".test_audit_unavailable RENAME TO authorization_audit_log"); err != nil {
				t.Error("cannot restore isolated audit table")
			}
		}()
		f.expect(t, "/tenant/create", admin, map[string]any{"code": "must_rollback", "name": "Must roll back"}, 503, 503)
		if f.scalar(t, "SELECT count(*) FROM tenant_info") != before {
			t.Fatal("business write committed without its audit")
		}
	})
	var hash string
	if err := f.db.QueryRowContext(f.ctx, "SELECT password_hash FROM user_info WHERE id = $1", alice.ID).Scan(&hash); err != nil || !strings.HasPrefix(hash, "$2") || hash == fixturePassword {
		t.Fatal("user password was not stored exclusively as a bcrypt hash")
	}
	f.setStatus(t, "/user/setStatus", admin, alice, "disabled")
	f.expect(t, "/auth/me", aliceToken, nil, 401, 401)
}

func TestLastAdministratorConcurrentChangesRealPostgres(t *testing.T) {
	for _, operation := range []string{"demote", "disable"} {
		t.Run(operation, func(t *testing.T) {
			f := newFixture(t)
			first := f.bootstrap(t)
			firstToken := f.login(t, first.Username).AccessToken
			second := f.createUser(t, firstToken, "second.admin")
			second = decode[resource](t, f.ok(t, "/user/setPlatformRole", firstToken, map[string]any{"id": second.ID, "version": second.Version, "platformRole": "platform_admin"}))
			secondToken := f.login(t, second.Username).AccessToken
			path, field, value, deniedStatus := "/user/setPlatformRole", "platformRole", "none", 403
			if operation == "disable" {
				path, field, value, deniedStatus = "/user/setStatus", "status", "disabled", 401
			}
			results := concurrentCalls(t, f,
				httpCall{path, firstToken, map[string]any{"id": second.ID, "version": second.Version, field: value}},
				httpCall{path, secondToken, map[string]any{"id": first.ID, "version": first.Version, field: value}},
			)
			successes, denials := 0, 0
			for _, result := range results {
				if result.Status == 200 && result.Code == 0 {
					successes++
				} else if result.Status == deniedStatus && result.Code == deniedStatus {
					denials++
				}
			}
			if successes != 1 || denials != 1 || f.scalar(t, "SELECT count(*) FROM user_info WHERE status = 'active' AND platform_role = 'platform_admin' AND is_deleted = false") != 1 {
				t.Fatal("concurrent administrator changes did not leave exactly one authorized active administrator")
			}
		})
	}
}
