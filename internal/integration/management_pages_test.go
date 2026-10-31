package integration

import "testing"

type credentialCheck struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

func TestManagementPageQueriesRealPostgres(t *testing.T) {
	f := newFixture(t)
	initial := f.bootstrap(t)
	admin := f.login(t, initial.Username).AccessToken
	a := decode[resource](t, f.ok(t, "/tenant/create", admin, map[string]any{"code": "a", "name": "Tenant A"}))
	b := decode[resource](t, f.ok(t, "/tenant/create", admin, map[string]any{"code": "b", "name": "Tenant B"}))
	u := f.createUser(t, admin, "scope_admin")
	other := f.createUser(t, admin, "scope.admin")
	for _, tenant := range []resource{a, b} {
		f.ok(t, "/tenant/member/allocate", admin, map[string]any{"tenantId": tenant.ID, "userId": u.ID})
	}
	userToken := f.login(t, u.Username).AccessToken
	otherToken := f.login(t, other.Username).AccessToken
	sys := decode[systemDetail](t, f.ok(t, "/system/create", admin, map[string]any{"tenantId": a.ID, "code": "backend", "name": "Backend"}))
	checkBody := map[string]any{"systemId": sys.System.ID}
	check := func(token string, allowed bool, reason string) {
		t.Helper()
		r := f.ok(t, "/system/credential/checkCreate", token, checkBody)
		v := decode[credentialCheck](t, r)
		if v.Allowed != allowed || v.Reason != reason {
			t.Fatalf("unexpected credential eligibility: %+v", v)
		}
		noSecrets(t, r, fixturePassword, token)
	}
	for _, path := range []string{"/auth/tenant/list", "/system/credential/checkCreate"} {
		f.expect(t, path, "", map[string]any{}, 401, 401)
	}
	f.expect(t, "/auth/tenant/list", admin, map[string]any{"tenantId": b.ID}, 400, 400)
	f.expect(t, "/system/credential/checkCreate", admin, map[string]any{}, 400, 400)
	f.expect(t, "/system/credential/checkCreate", admin, map[string]any{"systemId": "invalid"}, 400, 400)
	f.expect(t, "/system/credential/checkCreate", otherToken, checkBody, 404, 404)
	if decodePage[resource](t, f.ok(t, "/tenant/list", admin, map[string]any{})).Total != 2 ||
		decodePage[resource](t, f.ok(t, "/auth/tenant/list", admin, map[string]any{})).Total != 0 ||
		decodePage[resource](t, f.ok(t, "/auth/tenant/list", otherToken, map[string]any{})).Total != 0 {
		t.Fatal("platform authority or unrelated identity became a tenant membership")
	}
	first := decodePage[resource](t, f.ok(t, "/auth/tenant/list", userToken, map[string]any{"pageSize": 1}))
	second := decodePage[resource](t, f.ok(t, "/auth/tenant/list", userToken, map[string]any{"pageSize": 1, "pageNum": 2}))
	if first.Total != 2 || second.Total != 2 || len(first.List) != 1 || len(second.List) != 1 || first.List[0].ID == second.List[0].ID {
		t.Fatal("membership list/count/pagination disagree")
	}
	filtered := decodePage[resource](t, f.ok(t, "/auth/tenant/list", userToken, map[string]any{"code": "a"}))
	if filtered.Total != 1 || len(filtered.List) != 1 || filtered.List[0].ID != a.ID {
		t.Fatal("membership filter not applied")
	}
	for _, path := range []string{"/user/list", "/tenant/member/list"} {
		q := map[string]any{"username": "SCOPE_"}
		if path == "/tenant/member/list" {
			q["tenantId"] = a.ID
		}
		matches := decodePage[resource](t, f.ok(t, path, admin, q))
		if matches.Total != 1 || len(matches.List) != 1 || matches.List[0].Username != u.Username {
			t.Fatal("username search must ignore case and treat underscore literally")
		}
		q["username"] = "%"
		if decodePage[resource](t, f.ok(t, path, admin, q)).Total != 0 {
			t.Fatal("percent sign became a username wildcard")
		}
	}
	f.expect(t, "/user/list", userToken, map[string]any{"username": other.Username}, 403, 403)
	f.expect(t, "/tenant/member/list", otherToken, map[string]any{"tenantId": a.ID, "username": u.Username}, 404, 404)
	check(admin, false, "tenantAdminRequired")
	check(userToken, true, "")
	if f.scalar(t, "SELECT count(*) FROM system_credential_info") != 0 {
		t.Fatal("preflight created a credential")
	}
	issued := decode[issuedCredential](t, f.ok(t, "/system/credential/create", userToken, map[string]any{"accountId": sys.Account.ID, "name": "Backend"}))
	check(userToken, false, "activeCredentialExists")
	f.ok(t, "/system/credential/revoke", userToken, map[string]any{"id": issued.Credential.ID, "version": issued.Credential.Version})
	check(userToken, true, "")
	f.setStatus(t, "/system/account/setStatus", admin, sys.Account, "disabled")
	check(userToken, false, "accountDisabled")
	f.expect(t, "/system/credential/create", userToken, map[string]any{"accountId": sys.Account.ID, "name": "Stale preflight"}, 403, 403)
	f.setStatus(t, "/tenant/setStatus", admin, b, "disabled")
	if decodePage[resource](t, f.ok(t, "/auth/tenant/list", userToken, map[string]any{})).Total != 1 {
		t.Fatal("disabled tenant remained in actual scope")
	}
	members := decodePage[resource](t, f.ok(t, "/tenant/member/list", admin, map[string]any{"tenantId": a.ID, "username": u.Username}))
	f.setStatus(t, "/tenant/member/setStatus", admin, members.List[0], "disabled")
	if decodePage[resource](t, f.ok(t, "/auth/tenant/list", userToken, map[string]any{})).Total != 0 {
		t.Fatal("disabled membership remained in actual scope")
	}
	f.expect(t, "/system/credential/checkCreate", userToken, checkBody, 404, 404)
}
