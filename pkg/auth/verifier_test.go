package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifierPolicy(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(base64.StdEncoding.EncodeToString(der), "test-issuer", "test-audience")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		mutate  func(jwt.MapClaims)
		method  jwt.SigningMethod
		allowed bool
		userID  string
	}{
		{name: "valid staff ID", allowed: true, userID: "staff-1"},
		{name: "subject fallback", allowed: true, userID: "subject-1", mutate: func(c jwt.MapClaims) { delete(c, "staffuserid") }},
		{name: "expired", mutate: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{name: "missing expiration", mutate: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "wrong issuer", mutate: func(c jwt.MapClaims) { c["iss"] = "other" }},
		{name: "missing issuer", mutate: func(c jwt.MapClaims) { delete(c, "iss") }},
		{name: "wrong audience", mutate: func(c jwt.MapClaims) { c["aud"] = "other" }},
		{name: "missing audience", mutate: func(c jwt.MapClaims) { delete(c, "aud") }},
		{name: "missing identity", mutate: func(c jwt.MapClaims) { c["staffuserid"] = " "; c["sub"] = " " }},
		{name: "non-string identity", mutate: func(c jwt.MapClaims) { c["staffuserid"] = 42 }},
		{name: "future token", mutate: func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }},
		{name: "RS384 rejected", method: jwt.SigningMethodRS384},
		{name: "HS256 rejected", method: jwt.SigningMethodHS256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{"iss": "test-issuer", "aud": "test-audience", "exp": time.Now().Add(time.Hour).Unix(), "staffuserid": "staff-1", "sub": "subject-1", "name": "Test User"}
			if tc.mutate != nil {
				tc.mutate(claims)
			}
			method := tc.method
			if method == nil {
				method = jwt.SigningMethodRS256
			}
			var signingKey any = key
			if method == jwt.SigningMethodHS256 {
				signingKey = []byte("fake-test-secret")
			}
			token, err := jwt.NewWithClaims(method, claims).SignedString(signingKey)
			if err != nil {
				t.Fatal(err)
			}
			user, err := verifier.Authenticate("Bearer "+token, "fake-test-cookie")
			if !tc.allowed {
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("expected unauthorized, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if user.UserID != tc.userID || user.Jwt != token || user.Cookie != "fake-test-cookie" {
				t.Fatal("identity or internal credential propagation failed")
			}
			encoded, err := json.Marshal(user)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "cookie") || strings.Contains(string(encoded), "jwt") {
				t.Fatal("serialized user leaked credentials")
			}
		})
	}
	for _, header := range []string{"", "Bearer", "Basic fake", "Bearer one two"} {
		if _, err := verifier.Authenticate(header, ""); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("malformed authorization accepted")
		}
	}
	for _, rawKey := range []string{
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
		base64.RawStdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)),
	} {
		if _, err := NewVerifier(rawKey, "test-issuer", "test-audience"); err != nil {
			t.Fatalf("supported public key format rejected: %v", err)
		}
	}
	for _, config := range [][3]string{{"", "issuer", "audience"}, {"invalid-key", "issuer", "audience"}, {base64.StdEncoding.EncodeToString(der), "", "audience"}, {base64.StdEncoding.EncodeToString(der), "issuer", ""}} {
		if _, err := NewVerifier(config[0], config[1], config[2]); err == nil {
			t.Fatal("incomplete authentication configuration accepted")
		}
	}
}
