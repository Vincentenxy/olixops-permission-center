package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordPolicyAndVerification(t *testing.T) {
	for _, password := range []string{"", strings.Repeat("a", 11), strings.Repeat("a", 73), strings.Repeat("界", 25)} {
		if _, err := HashPassword(password); !errors.Is(err, ErrPasswordPolicy) {
			t.Fatal("invalid byte length accepted")
		}
	}
	for _, password := range []string{strings.Repeat("a", 12), strings.Repeat("a", 72), "  test-password  ", strings.Repeat("界", 4)} {
		hash, err := HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil || cost != passwordCost || !VerifyPassword(password, hash) {
			t.Fatal("password failed to round trip with expected cost")
		}
		if VerifyPassword("incorrect-password", hash) {
			t.Fatal("incorrect password accepted")
		}
	}
}

func TestMissingAndMalformedPasswordHashesFailClosed(t *testing.T) {
	if cost, err := bcrypt.Cost([]byte(dummyPasswordHash)); err != nil || cost != passwordCost {
		t.Fatal("invalid dummy bcrypt cost")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte("dummy-comparison-password")); !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Fatal("dummy hash does not perform a valid bcrypt comparison")
	}
	for _, hash := range []string{"", "invalid", "$2a$31$" + strings.Repeat("a", 53), "$2a$12$" + strings.Repeat("!", 53)} {
		if VerifyPassword("test-password-value", hash) {
			t.Fatal("malformed password hash accepted")
		}
	}
	if VerifyPassword(strings.Repeat("a", 73), dummyPasswordHash) {
		t.Fatal("oversized password accepted")
	}
}
