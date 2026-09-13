package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Signer issues administrator JWTs using a configured, persistent RSA key pair.
// It never creates keys during service startup.
type Signer struct {
	privateKey *rsa.PrivateKey
	issuer     string
	audience   string
	ttl        time.Duration
}

func NewSigner(privateKey, publicKey, issuer, audience string, ttl time.Duration) (*Signer, error) {
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(audience) == "" {
		return nil, errors.New("jwt issuer and audience are required")
	}
	if ttl < time.Second || ttl > 24*time.Hour {
		return nil, errors.New("jwt token lifetime must be between 1s and 24h")
	}
	key, err := parseRSAPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	verificationKey, err := parseRSAPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	if key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, errors.New("jwt private key must be a valid RSA key of at least 2048 bits")
	}
	if !key.PublicKey.Equal(verificationKey) {
		return nil, errors.New("jwt public and private keys do not match")
	}
	return &Signer{privateKey: key, issuer: issuer, audience: audience, ttl: ttl}, nil
}

func (s *Signer) Sign(userID, name string) (string, time.Time, error) {
	if s == nil || s.privateKey == nil || strings.TrimSpace(userID) == "" || userID != strings.TrimSpace(userID) {
		return "", time.Time{}, errors.New("cannot issue authentication token")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", time.Time{}, errors.New("cannot issue authentication token")
	}
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(s.ttl).Truncate(time.Second)
	issuedClaims := claims{
		Name: name,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.issuer, Subject: userID, Audience: jwt.ClaimStrings{s.audience},
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt), ID: hex.EncodeToString(id[:]),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, issuedClaims).SignedString(s.privateKey)
	if err != nil {
		return "", time.Time{}, errors.New("cannot issue authentication token")
	}
	return token, expiresAt, nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("jwt private key is required")
	}
	var der []byte
	if block, rest := pem.Decode([]byte(raw)); block != nil {
		if strings.TrimSpace(string(rest)) != "" {
			return nil, errors.New("invalid jwt private key encoding")
		}
		der = block.Bytes
	} else {
		var err error
		der, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			der, err = base64.RawStdEncoding.DecodeString(raw)
		}
		if err != nil {
			return nil, errors.New("invalid jwt private key encoding")
		}
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("jwt private key must be RSA")
}
