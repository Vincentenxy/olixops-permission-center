// Package auth provides the shared inbound JWT policy for HTTP and gRPC.
package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"olixops-permission-center/pkg/userctx"
)

var ErrUnauthorized = errors.New("unauthorized")

type Verifier struct {
	publicKey *rsa.PublicKey
	parser    *jwt.Parser
}

type claims struct {
	StaffUserID string `json:"staffuserid"`
	Name        string `json:"name"`
	jwt.RegisteredClaims
}

func NewVerifier(rawPublicKey, issuer, audience string) (*Verifier, error) {
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(audience) == "" {
		return nil, errors.New("jwt issuer and audience are required")
	}
	key, err := parseRSAPublicKey(rawPublicKey)
	if err != nil {
		return nil, err
	}
	return &Verifier{
		publicKey: key,
		parser: jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}),
			jwt.WithExpirationRequired(), jwt.WithIssuer(issuer), jwt.WithAudience(audience)),
	}, nil
}

// Authenticate never returns token, claim or cryptographic error details.
func (v *Verifier) Authenticate(header, cookie string) (userctx.User, error) {
	parts := strings.Fields(header)
	if v == nil || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return userctx.User{}, ErrUnauthorized
	}
	parsed := &claims{}
	token, err := v.parser.ParseWithClaims(parts[1], parsed, func(*jwt.Token) (any, error) {
		return v.publicKey, nil
	})
	if err != nil || !token.Valid {
		return userctx.User{}, ErrUnauthorized
	}
	userID := strings.TrimSpace(parsed.StaffUserID)
	if userID == "" {
		userID = strings.TrimSpace(parsed.Subject)
	}
	if userID == "" {
		return userctx.User{}, ErrUnauthorized
	}
	return userctx.User{UserID: userID, Name: parsed.Name, Jwt: parts[1], Cookie: cookie}, nil
}

func parseRSAPublicKey(raw string) (*rsa.PublicKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("jwt public key is required")
	}
	var der []byte
	if block, _ := pem.Decode([]byte(raw)); block != nil {
		der = block.Bytes
	} else {
		var err error
		der, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			der, err = base64.RawStdEncoding.DecodeString(raw)
		}
		if err != nil {
			return nil, errors.New("invalid jwt public key encoding")
		}
	}
	if key, err := x509.ParsePKIXPublicKey(der); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("jwt public key must be RSA")
}
