package util

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key"

func signWith(t *testing.T, method jwt.SigningMethod, key interface{}, claims *Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestParseToken_Valid(t *testing.T) {
	claims := &Claims{
		ClientID: 100, TokenSource: "client", TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	signed := signWith(t, jwt.SigningMethodHS256, []byte(testSecret), claims)

	got, err := ParseToken(signed, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ClientID != 100 || got.TokenSource != "client" || got.TokenType != "access" {
		t.Errorf("claims mismatch: %+v", got)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	signed := signWith(t, jwt.SigningMethodHS256, []byte(testSecret), &Claims{
		ClientID: 1, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	})
	if _, err := ParseToken(signed, "a-different-secret"); err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestParseToken_Expired(t *testing.T) {
	signed := signWith(t, jwt.SigningMethodHS256, []byte(testSecret), &Claims{
		ClientID: 1, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))},
	})
	if _, err := ParseToken(signed, testSecret); err == nil {
		t.Error("expected error for expired token")
	}
}

func TestParseToken_WrongSigningMethod(t *testing.T) {
	// A token signed with "none" must be rejected by the HMAC-only guard.
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{ClientID: 1})
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := ParseToken(signed, testSecret); err == nil {
		t.Error("expected error for none-signed token")
	}
}

func TestParseToken_Garbage(t *testing.T) {
	if _, err := ParseToken("not-a-jwt", testSecret); err == nil {
		t.Error("expected error for malformed token")
	}
}
