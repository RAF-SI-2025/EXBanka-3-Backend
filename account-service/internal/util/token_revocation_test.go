package util_test

import (
	"context"
	"os"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// TestTokenRevocation_NoRealRedis exercises every token-revocation code path
// without a live Redis: an empty address yields a nil store, and a dead address
// builds a client whose calls fail fast (timeouts) so the error/fail-open
// branches run too.
func TestTokenRevocation_NoRealRedis(t *testing.T) {
	if util.NewRedisTokenRevocationStore("", "", 0) != nil {
		t.Error("empty addr should yield a nil store")
	}
	store := util.NewRedisTokenRevocationStore("127.0.0.1:1", "", 0)
	if store == nil {
		t.Fatal("expected a store for a non-empty addr")
	}

	if util.RevokedTokenKey("jti") == "" {
		t.Error("RevokedTokenKey should be non-empty")
	}

	// Close: real store and nil receiver.
	_ = store.Close()
	var nilStore *util.TokenRevocationStore
	_ = nilStore.Close()

	ctx := context.Background()
	// IsRevoked guards: nil store, empty jti.
	if ok, _ := nilStore.IsRevoked(ctx, "j"); ok {
		t.Error("nil store should report not revoked")
	}
	if ok, _ := store.IsRevoked(ctx, ""); ok {
		t.Error("empty jti should report not revoked")
	}
	_, _ = store.IsRevoked(ctx, "jti") // dead client -> error path (fails fast)

	// IsTokenRevoked: nil claims, no store configured, store configured (fail-open).
	if util.IsTokenRevoked(ctx, nil) {
		t.Error("nil claims -> not revoked")
	}
	c := &util.Claims{}
	c.ID = "jti-x"
	util.SetTokenRevocationStore(nil)
	if util.IsTokenRevoked(ctx, c) {
		t.Error("no store configured -> not revoked")
	}
	util.SetTokenRevocationStore(store)
	_ = util.IsTokenRevoked(ctx, c) // dead client -> fails open (false)
	util.SetTokenRevocationStore(nil)

	// ConfigureTokenRevocationFromEnv: disabled (no addr) and enabled paths.
	os.Unsetenv("REDIS_ADDR")
	ConfigureAndClose(t)
	os.Setenv("REDIS_ADDR", "127.0.0.1:1")
	os.Setenv("REDIS_DB", "1")
	ConfigureAndClose(t)
	os.Setenv("REDIS_DB", "not-a-number") // invalid -> warn branch
	ConfigureAndClose(t)
	os.Unsetenv("REDIS_ADDR")
	os.Unsetenv("REDIS_DB")
}

func ConfigureAndClose(t *testing.T) {
	t.Helper()
	closeFn := util.ConfigureTokenRevocationFromEnv("test")
	if closeFn == nil {
		t.Fatal("ConfigureTokenRevocationFromEnv returned nil close func")
	}
	closeFn()
}
