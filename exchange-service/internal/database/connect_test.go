package database

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
)

// TestConnect_Error covers the Connect error path: a refused connection
// (localhost:1) makes gorm.Open fail fast, exercising the DSN build + error
// wrap without needing a live PostgreSQL.
func TestConnect_Error(t *testing.T) {
	cfg := &config.Config{
		DBHost: "127.0.0.1", DBPort: "1", DBUser: "u", DBPassword: "p",
		DBName: "x", DBSSLMode: "disable",
	}
	db, err := Connect(cfg)
	if err == nil {
		t.Fatalf("expected connection error to 127.0.0.1:1, got db=%v", db)
	}
}
