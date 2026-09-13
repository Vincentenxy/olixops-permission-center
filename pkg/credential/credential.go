// Package credential generates and verifies opaque system API keys without
// storage or transport dependencies. The caller handles expiry and revocation.
package credential

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

const (
	prefix       = "opc_"
	idBytes      = 16
	secretBytes  = 32
	idLength     = idBytes * 2
	secretLength = 43
	tokenLength  = len(prefix) + idLength + 1 + secretLength
)

var (
	ErrInvalidToken     = errors.New("invalid API key")
	errGenerationFailed = errors.New("cannot generate API key")
)

// Issued contains a new API key. ID is the public lookup identifier without
// opc_. Token is shown once through an explicit response DTO; only ID and Digest
// are intended for storage. Never log this value, including with formatting.
type Issued struct {
	ID     string `json:"id"`
	Token  string `json:"-"`
	Digest string `json:"-"`
}

// Generate creates a 128-bit public identifier and a 256-bit random secret.
func Generate() (Issued, error) { return generate(rand.Reader) }

func generate(random io.Reader) (Issued, error) {
	var data [idBytes + secretBytes]byte
	if _, err := io.ReadFull(random, data[:]); err != nil {
		return Issued{}, errGenerationFailed
	}
	id := hex.EncodeToString(data[:idBytes])
	token := prefix + id + "." + base64.RawURLEncoding.EncodeToString(data[idBytes:])
	digest := sha256.Sum256([]byte(token))
	return Issued{ID: id, Token: token, Digest: hex.EncodeToString(digest[:])}, nil
}

// ParseID validates the full canonical token format and returns its public ID.
// Successful parsing does not imply that a token is authentic or active.
func ParseID(token string) (string, error) {
	if len(token) != tokenLength || !strings.HasPrefix(token, prefix) {
		return "", ErrInvalidToken
	}
	separator := len(prefix) + idLength
	if token[separator] != '.' {
		return "", ErrInvalidToken
	}
	id := token[len(prefix):separator]
	if !isLowerHex(id) {
		return "", ErrInvalidToken
	}
	secret := token[separator+1:]
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(secret)
	if err != nil || len(decoded) != secretBytes {
		return "", ErrInvalidToken
	}
	// Reject alternate encodings and newlines otherwise tolerated by base64.
	if base64.RawURLEncoding.EncodeToString(decoded) != secret {
		return "", ErrInvalidToken
	}
	return id, nil
}

// Verify compares the complete token SHA-256 digest in constant time after
// validating both inputs. It does not check record status, expiry or scope.
func Verify(token, digest string) bool {
	if _, err := ParseID(token); err != nil {
		return false
	}
	if len(digest) != sha256.Size*2 || !isLowerHex(digest) {
		return false
	}
	expected, err := hex.DecodeString(digest)
	if err != nil {
		return false
	}
	actual := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(actual[:], expected) == 1
}

func isLowerHex(value string) bool {
	for i := range len(value) {
		if !(value[i] >= '0' && value[i] <= '9') && !(value[i] >= 'a' && value[i] <= 'f') {
			return false
		}
	}
	return true
}
