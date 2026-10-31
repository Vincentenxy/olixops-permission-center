package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	m "olixops-permission-center/internal/domain/management"
	"olixops-permission-center/pkg/userctx"
)

func TestPaginationAndStrictBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		body              string
		num, size, status int
	}{{`{}`, 1, 20, 200}, {`{"pageNum":0,"pageSize":-1}`, 1, 20, 200}, {`{"pageNum":3,"pageSize":999}`, 3, 200, 200}, {`{"page":1}`, 0, 0, 400}, {`{} {}`, 0, 0, 400}} {
		t.Run(tc.body, func(t *testing.T) {
			r := gin.New()
			called := false
			r.POST("/list", func(c *gin.Context) {
				c.Request = c.Request.WithContext(userctx.WithContext(c.Request.Context(), userctx.User{UserID: "test"}))
				c.Next()
			}, post(func(_ context.Context, _ string, q myTenantsRequest) (m.Page[m.Tenant], error) {
				called = true
				if q.PageNum != tc.num || q.PageSize != tc.size {
					t.Fatalf("application received %d/%d", q.PageNum, q.PageSize)
				}
				return m.Page[m.Tenant]{}, nil
			}, listDTO(tenantDTOOf)))
			w := httptest.NewRecorder()
			q := httptest.NewRequest(http.MethodPost, "/list", strings.NewReader(tc.body))
			q.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, q)
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			if tc.status == 400 {
				if called {
					t.Fatal("invalid request reached application")
				}
				return
			}
			var envelope struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if e := json.Unmarshal(w.Body.Bytes(), &envelope); e != nil {
				t.Fatal(e)
			}
			if len(envelope.Data) != 2 || string(envelope.Data["list"]) != "[]" {
				t.Fatalf("wrong page envelope: %s", w.Body)
			}
		})
	}
}
func TestPublicDTOsCannotExposeSecrets(t *testing.T) {
	u := m.User{PasswordHash: "fake-password-hash", ExternalSubject: "fake-external-subject"}
	c := m.Credential{TokenDigest: "fake-token-digest"}
	for _, v := range []any{userDTOOf(u), profile(u), credentialDTOOf(c)} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "fake-") {
			t.Fatalf("internal data leaked: %s", data)
		}
	}
}
