package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordBytes = 12
	MaxPasswordBytes = 72
	passwordCost     = 12
	// A public, syntactically valid bcrypt value used only for absent/invalid hashes.
	// No account uses this value. Its purpose is to perform the same expensive work.
	dummyPasswordHash = "$2a$12$abcdefghijklmnopqrstuuq1kJUmwVOFyFa.TAeJu.CnpREzEXdUe"
)

var ErrPasswordPolicy = errors.New("password must contain 12 to 72 bytes")

// HashPassword validates byte length without trimming or normalizing passwords.
func HashPassword(password string) (string, error) {
	if !validPasswordLength(password) {
		return "", ErrPasswordPolicy
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return "", errors.New("cannot hash password")
	}
	return string(hash), nil
}

// VerifyPassword performs a cost-matched bcrypt comparison even for a missing
// account. Callers must also check account status and never expose which failed.
func VerifyPassword(password, encodedHash string) bool {
	cost, err := bcrypt.Cost([]byte(encodedHash))
	validHash := err == nil && cost == passwordCost && validHashEncoding(encodedHash)
	if !validHash {
		encodedHash = dummyPasswordHash
	}
	validPassword := validPasswordLength(password)
	if !validPassword {
		password = "invalid-password-placeholder"
	}
	err = bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password))
	return validHash && validPassword && err == nil
}

func validPasswordLength(password string) bool {
	return len(password) >= MinPasswordBytes && len(password) <= MaxPasswordBytes
}

func validHashEncoding(hash string) bool {
	if len(hash) != 60 {
		return false
	}
	for _, char := range hash[7:] {
		if char != '.' && char != '/' && !(char >= 'A' && char <= 'Z') && !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
