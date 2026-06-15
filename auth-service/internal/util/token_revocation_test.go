package util

import (
	"context"
	"errors"
	"os"
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

// TestTokenRevocationStore_NoRealRedis covers the store constructor, Close,
// RevokeJTI, IsRevoked and the env configuration without a live Redis: a dead
// address builds a client whose calls fail fast, exercising the error paths.
func TestTokenRevocationStore_NoRealRedis(t *testing.T) {
	if NewRedisTokenRevocationStore("", "", 0) != nil {
		t.Error("empty addr should yield a nil store")
	}
	store := NewRedisTokenRevocationStore("127.0.0.1:1", "", 0)
	if store == nil {
		t.Fatal("expected a store for a non-empty addr")
	}
	ctx := context.Background()

	// Close: real + nil receiver.
	_ = store.Close()
	var nilStore *TokenRevocationStore
	_ = nilStore.Close()

	// RevokeJTI branches.
	if err := nilStore.RevokeJTI(ctx, "j", time.Minute); !errors.Is(err, ErrTokenRevocationUnavailable) {
		t.Errorf("nil store -> unavailable, got %v", err)
	}
	if err := store.RevokeJTI(ctx, "", time.Minute); !errors.Is(err, ErrTokenNotRevocable) {
		t.Errorf("empty jti -> not-revocable, got %v", err)
	}
	if err := store.RevokeJTI(ctx, "j", 0); err != nil {
		t.Errorf("ttl<=0 -> no-op, got %v", err)
	}
	_ = store.RevokeJTI(ctx, "j", time.Minute) // dead client -> set error

	// IsRevoked branches.
	if ok, _ := nilStore.IsRevoked(ctx, "j"); ok {
		t.Error("nil store -> not revoked")
	}
	if ok, _ := store.IsRevoked(ctx, ""); ok {
		t.Error("empty jti -> not revoked")
	}
	_, _ = store.IsRevoked(ctx, "j")

	// RevokeTokenClaims/IsTokenRevoked with a (dead) store configured.
	c := &Claims{RegisteredClaims: jwt.RegisteredClaims{ID: "x", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	SetTokenRevocationStore(store)
	_ = RevokeTokenClaims(ctx, c)
	_ = IsTokenRevoked(ctx, c)
	// Already-expired token -> ttl<=0 no-op.
	expired := &Claims{RegisteredClaims: jwt.RegisteredClaims{ID: "y", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}}
	if err := RevokeTokenClaims(ctx, expired); err != nil {
		t.Errorf("expired token revoke -> no-op, got %v", err)
	}
	SetTokenRevocationStore(nil)

	// ConfigureTokenRevocationFromEnv: disabled, enabled, invalid REDIS_DB.
	os.Unsetenv("REDIS_ADDR")
	ConfigureTokenRevocationFromEnv("t")()
	os.Setenv("REDIS_ADDR", "127.0.0.1:1")
	os.Setenv("REDIS_DB", "2")
	ConfigureTokenRevocationFromEnv("t")()
	os.Setenv("REDIS_DB", "not-a-number")
	ConfigureTokenRevocationFromEnv("t")()
	os.Unsetenv("REDIS_ADDR")
	os.Unsetenv("REDIS_DB")
}
