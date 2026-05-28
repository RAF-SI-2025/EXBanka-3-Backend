package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultTokenRevocationTimeout = 500 * time.Millisecond
	revokedTokenKeyPrefix         = "auth:revoked:jti:"
)

var (
	ErrTokenRevocationUnavailable = errors.New("token revocation store unavailable")
	ErrTokenNotRevocable          = errors.New("token has no jti")

	tokenRevocationStoreMu sync.RWMutex
	tokenRevocationStore   *TokenRevocationStore
)

type TokenRevocationStore struct {
	client *redis.Client
}

func NewRedisTokenRevocationStore(addr, password string, db int) *TokenRevocationStore {
	if addr == "" {
		return nil
	}
	return &TokenRevocationStore{
		client: redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     password,
			DB:           db,
			DialTimeout:  defaultTokenRevocationTimeout,
			ReadTimeout:  defaultTokenRevocationTimeout,
			WriteTimeout: defaultTokenRevocationTimeout,
		}),
	}
}

func ConfigureTokenRevocationFromEnv(serviceName string) func() {
	db := 0
	if raw := os.Getenv("REDIS_DB"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			db = parsed
		} else {
			slog.Warn("invalid REDIS_DB, using default", "service", serviceName, "raw", raw)
		}
	}

	store := NewRedisTokenRevocationStore(os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"), db)
	if store == nil {
		return func() {}
	}

	SetTokenRevocationStore(store)
	slog.Info("Redis token revocation configured", "service", serviceName, "addr", os.Getenv("REDIS_ADDR"), "db", db)
	return func() {
		_ = store.Close()
		SetTokenRevocationStore(nil)
	}
}

func SetTokenRevocationStore(store *TokenRevocationStore) {
	tokenRevocationStoreMu.Lock()
	defer tokenRevocationStoreMu.Unlock()
	tokenRevocationStore = store
}

func currentTokenRevocationStore() *TokenRevocationStore {
	tokenRevocationStoreMu.RLock()
	defer tokenRevocationStoreMu.RUnlock()
	return tokenRevocationStore
}

func (s *TokenRevocationStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *TokenRevocationStore) RevokeJTI(ctx context.Context, jti string, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return ErrTokenRevocationUnavailable
	}
	if jti == "" {
		return ErrTokenNotRevocable
	}
	if ttl <= 0 {
		return nil
	}
	return s.client.Set(ctx, RevokedTokenKey(jti), "1", ttl).Err()
}

func (s *TokenRevocationStore) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if s == nil || s.client == nil || jti == "" {
		return false, nil
	}
	count, err := s.client.Exists(ctx, RevokedTokenKey(jti)).Result()
	return count > 0, err
}

func RevokedTokenKey(jti string) string {
	sum := sha256.Sum256([]byte(jti))
	return revokedTokenKeyPrefix + hex.EncodeToString(sum[:])
}

func RevokeTokenClaims(ctx context.Context, claims *Claims) error {
	if claims == nil || claims.ID == "" {
		return ErrTokenNotRevocable
	}
	if claims.ExpiresAt == nil {
		return ErrTokenNotRevocable
	}

	store := currentTokenRevocationStore()
	if store == nil {
		return ErrTokenRevocationUnavailable
	}

	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTokenRevocationTimeout)
	defer cancel()
	return store.RevokeJTI(ctx, claims.ID, ttl)
}

func IsTokenRevoked(ctx context.Context, claims *Claims) bool {
	if claims == nil || claims.ID == "" {
		return false
	}

	store := currentTokenRevocationStore()
	if store == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTokenRevocationTimeout)
	defer cancel()

	revoked, err := store.IsRevoked(ctx, claims.ID)
	if err != nil {
		slog.Warn("token revocation check failed, failing open", "error", err)
		return false
	}
	return revoked
}
