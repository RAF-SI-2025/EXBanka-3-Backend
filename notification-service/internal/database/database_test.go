package database

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestConnect_Error covers the Connect error path: a refused connection makes
// gorm.Open fail fast, exercising the DSN build + error wrap without a live DB.
func TestConnect_Error(t *testing.T) {
	cfg := &config.Config{
		DBHost: "127.0.0.1", DBPort: "1", DBUser: "u", DBPassword: "p",
		DBName: "x", DBSSLMode: "disable",
	}
	if db, err := Connect(cfg); err == nil {
		t.Fatalf("expected connection error to 127.0.0.1:1, got db=%v", db)
	}
}

// TestMigrate runs the migration against a sqlite DB. The non-postgres branch of
// withMigrationLock applies the AutoMigrate directly (no advisory lock), creating
// the notifications table.
func TestMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:notif_db_migrate?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !db.Migrator().HasTable(&models.Notification{}) {
		t.Error("expected notifications table to exist after Migrate")
	}
	// Idempotent: a second run is a no-op.
	if err := Migrate(db); err != nil {
		t.Errorf("second migrate: %v", err)
	}
}

// TestMigrate_Error covers the migration-failure wrap: closing the underlying
// connection pool makes the transaction (and thus AutoMigrate) fail.
func TestMigrate_Error(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:notif_db_migrate_err?mode=memory"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	_ = sqlDB.Close()

	if err := Migrate(db); err == nil {
		t.Error("expected migrate error on a closed connection pool")
	}
}
