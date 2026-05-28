package util

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestRevokedTokenKeyHashesJTI(t *testing.T) {
	key := RevokedTokenKey("token-id-123")

	if key != RevokedTokenKey("token-id-123") {
		t.Fatal("expected revocation key to be deterministic")
	}
	if !strings.HasPrefix(key, revokedTokenKeyPrefix) {
		t.Fatalf("expected key prefix %q, got %q", revokedTokenKeyPrefix, key)
	}
	if strings.Contains(key, "token-id-123") {
		t.Fatal("expected raw jti not to be stored in Redis key")
	}
}

func TestRevokeTokenClaimsRequiresStoreAndJTI(t *testing.T) {
	SetTokenRevocationStore(nil)

	err := RevokeTokenClaims(context.Background(), &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	if !errors.Is(err, ErrTokenRevocationUnavailable) {
		t.Fatalf("expected unavailable store error, got %v", err)
	}

	err = RevokeTokenClaims(context.Background(), &Claims{})
	if !errors.Is(err, ErrTokenNotRevocable) {
		t.Fatalf("expected not revocable error, got %v", err)
	}
}

func TestIsTokenRevokedWithoutStoreFailsOpen(t *testing.T) {
	SetTokenRevocationStore(nil)

	if IsTokenRevoked(context.Background(), &Claims{
		RegisteredClaims: jwt.RegisteredClaims{ID: "jti"},
	}) {
		t.Fatal("expected revoked check without store to fail open")
	}
}
