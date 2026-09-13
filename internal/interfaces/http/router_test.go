package httpinterface

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"olixops-permission-center/internal/interfaces/http/middleware"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/logger"
	"olixops-permission-center/pkg/response"
)

func TestHTTPAuthenticationAndEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AuthJWTPublicKey: base64.StdEncoding.EncodeToString(der), AuthJWTIssuer: "test-issuer", AuthJWTAudience: "test-audience"}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "test-issuer", "aud": "test-audience", "exp": time.Now().Add(time.Hour).Unix(), "staffuserid": "test-user", "name": "Test User"}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(cfg)
	router.GET("/panic", func(*gin.Context) { panic("fake-sensitive-panic") })
	router.GET("/business-error", func(c *gin.Context) { response.Error(c, "role not found") })
	secured := router.Group("/api/v1/resource", middleware.Auth(cfg))
	secured.GET("/pub/secret", func(c *gin.Context) { response.Success(c, "must be protected") })
	core, logs := observer.New(zap.InfoLevel)
	previous := logger.L()
	logger.Replace(zap.New(core))
	t.Cleanup(func() { logger.Replace(previous) })
	for _, tc := range []struct {
		name, method, path, token string
		status, code              int
		msg                       string
	}{
		{"public health", "GET", "/api/v1/pub/healthz", "", 200, 0, ""},
		{"authenticated identity", "GET", "/api/v1/auth/me", token, 200, 0, ""},
		{"missing token", "GET", "/api/v1/auth/me", "", 401, 401, "Unauthorized"},
		{"invalid token", "GET", "/api/v1/auth/me", "fake-sensitive-token", 401, 401, "Unauthorized"},
		{"no substring bypass", "GET", "/api/v1/resource/pub/secret", "", 401, 401, "Unauthorized"},
		{"unknown route", "GET", "/absent?value=fake-sensitive-query", "", 404, 404, "Not Found"},
		{"method not allowed", "DELETE", "/api/v1/pub/healthz", "", 405, 405, "Method Not Allowed"},
		{"panic", "GET", "/panic", "", 500, 500, "Internal Server Error"},
		{"business error", "GET", "/business-error", "", 200, -1, "role not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set("X-Request-ID", "test-request-id")
			request.Header.Set("Cookie", "fake-sensitive-cookie")
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("status %d, want %d", recorder.Code, tc.status)
			}
			var body struct {
				Code int             `json:"code"`
				Msg  string          `json:"msg"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != tc.code || body.Msg != tc.msg || body.Data == nil {
				t.Fatalf("unexpected response envelope: %s", recorder.Body)
			}
			if recorder.Header().Get("X-Request-ID") != "test-request-id" {
				t.Fatal("request id not echoed")
			}
			if strings.Contains(recorder.Body.String(), "fake-sensitive") || strings.Contains(recorder.Body.String(), token) || strings.Contains(recorder.Body.String(), "\"jwt\"") {
				t.Fatal("response leaked credentials or internal details")
			}
			if tc.name == "authenticated identity" && string(body.Data) != `{"userId":"test-user","name":"Test User"}` {
				t.Fatalf("wrong public identity: %s", body.Data)
			}
		})
	}
	for _, entry := range logs.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "fake-sensitive") || strings.Contains(string(encoded), token) || strings.Contains(entry.Message, "fake-sensitive") {
			t.Fatal("logs leaked request or panic details")
		}
	}
	if logs.FilterMessage("http request").Len() != 9 {
		t.Fatal("not all requests logged")
	}
	if logs.FilterMessage("http panic recovered").Len() != 1 {
		t.Fatal("panic was not logged")
	}
}

func TestHealthWorksWithoutAuthConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(config.Config{})
	for _, tc := range []struct {
		path   string
		status int
	}{{"/api/v1/pub/healthz", http.StatusOK}, {"/api/v1/auth/me", http.StatusInternalServerError}} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != tc.status {
			t.Fatalf("%s returned %d", tc.path, recorder.Code)
		}
	}
}
