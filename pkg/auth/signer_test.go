package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSignerFormatsAndClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	public := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	formats := []string{
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		base64.StdEncoding.EncodeToString(pkcs8),
		base64.RawStdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(key)),
	}
	for _, private := range formats {
		signer, err := NewSigner(private, public, "test-issuer", "test-audience", 8*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		token, expiresAt, err := signer.Sign("internal-user-id", "Test User")
		if err != nil {
			t.Fatal(err)
		}
		parsedClaims := &claims{}
		parsed, err := jwt.ParseWithClaims(token, parsedClaims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
			jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("test-issuer"), jwt.WithAudience("test-audience"), jwt.WithExpirationRequired())
		if err != nil || !parsed.Valid || parsedClaims.Subject != "internal-user-id" || parsedClaims.Name != "Test User" {
			t.Fatal("issued token claims are invalid")
		}
		if !parsedClaims.ExpiresAt.Time.Equal(expiresAt) || expiresAt.Sub(parsedClaims.IssuedAt.Time) != 8*time.Hour || len(parsedClaims.ID) != 32 {
			t.Fatal("issued lifetime or token ID is invalid")
		}
		verifier, err := NewVerifier(public, "test-issuer", "test-audience")
		if err != nil {
			t.Fatal(err)
		}
		user, err := verifier.Authenticate("Bearer "+token, "")
		if err != nil || user.UserID != "internal-user-id" {
			t.Fatal("shared verifier did not accept issued token")
		}
		second, _, err := signer.Sign("internal-user-id", "Test User")
		if err != nil || second == token {
			t.Fatal("token IDs were reused")
		}
		if _, _, err := signer.Sign(" ", "User"); err == nil {
			t.Fatal("blank subject accepted")
		}
	}
}

func TestSignerRejectsInvalidConfiguration(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	private := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(key))
	public := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	for _, tc := range []struct {
		name, private, public, issuer, audience string
		ttl                                     time.Duration
	}{
		{"missing private", "", public, "issuer", "audience", time.Hour},
		{"malformed private", "fake-sensitive-value", public, "issuer", "audience", time.Hour},
		{"missing public", private, "", "issuer", "audience", time.Hour},
		{"mismatched keys", private, base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&other.PublicKey)), "issuer", "audience", time.Hour},
		{"weak key", base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(weak)), base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&weak.PublicKey)), "issuer", "audience", time.Hour},
		{"non RSA", base64.StdEncoding.EncodeToString(ecDER), public, "issuer", "audience", time.Hour},
		{"issuer", private, public, " ", "audience", time.Hour},
		{"audience", private, public, "issuer", " ", time.Hour},
		{"zero TTL", private, public, "issuer", "audience", 0},
		{"negative TTL", private, public, "issuer", "audience", -time.Hour},
		{"subsecond TTL", private, public, "issuer", "audience", time.Millisecond},
		{"long TTL", private, public, "issuer", "audience", 25 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSigner(tc.private, tc.public, tc.issuer, tc.audience, tc.ttl)
			if err == nil || strings.Contains(err.Error(), "fake-sensitive-value") || strings.Contains(err.Error(), private) {
				t.Fatal("configuration did not fail safely")
			}
		})
	}
	var unavailable *Signer
	if _, _, err := unavailable.Sign("user-id", "Name"); err == nil {
		t.Fatal("unavailable signer did not fail closed")
	}
}
