package database

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSQLite(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	return db
}

// --- Connect ---

func TestConnect_BadConfig_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		DBHost:     "127.0.0.1",
		DBPort:     "1", // closed port
		DBUser:     "x",
		DBPassword: "x",
		DBName:     "x",
		DBSSLMode:  "disable",
	}
	if _, err := Connect(cfg); err == nil {
		t.Fatal("expected error connecting to closed port")
	}
}

// --- Migrate ---

func TestMigrate_CreatesTablesAndRunsBackfill(t *testing.T) {
	db := openSQLite(t, "employee_migrate_test")

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	// Tables must now exist; insert/find round-trips will work.
	emp := models.Employee{
		Ime: "Ana", Prezime: "Test", Email: "a@b.com", Username: "ana",
		Password: "x", SaltPassword: "x", Pol: "F", Pozicija: "Clerk", Aktivan: true,
	}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("expected employee table to exist: %v", err)
	}
	// ActuaryProfile table must also exist (created by Migrate via AutoMigrate).
	var count int64
	if err := db.Model(&models.ActuaryProfile{}).Count(&count).Error; err != nil {
		t.Fatalf("expected actuary_profile table to exist: %v", err)
	}
}

// --- SeedPermissions ---

func TestSeedPermissions_InsertsAllDefaults(t *testing.T) {
	db := openSQLite(t, "employee_seed_perms_test")
	if err := db.AutoMigrate(&models.Permission{}); err != nil {
		t.Fatalf("migrate permission: %v", err)
	}

	if err := SeedPermissions(db); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	var count int64
	if err := db.Model(&models.Permission{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if int(count) != len(models.DefaultPermissions) {
		t.Errorf("expected %d permissions, got %d", len(models.DefaultPermissions), count)
	}

	// All permissions should have SubjectType set.
	var noSubject int64
	if err := db.Model(&models.Permission{}).Where("subject_type = ''").Count(&noSubject).Error; err != nil {
		t.Fatalf("count without subject: %v", err)
	}
	if noSubject != 0 {
		t.Errorf("expected all permissions to have subject_type, got %d without", noSubject)
	}
}

func TestSeedPermissions_BackfillsLegacyEmptySubjectType(t *testing.T) {
	db := openSQLite(t, "employee_seed_perms_legacy")
	if err := db.AutoMigrate(&models.Permission{}); err != nil {
		t.Fatalf("migrate permission: %v", err)
	}
	// Pre-seed a legacy permission with empty subject_type.
	if err := db.Exec(
		"INSERT INTO permissions (name, description, subject_type) VALUES (?, ?, ?)",
		"legacy.perm", "Legacy", "",
	).Error; err != nil {
		t.Fatalf("insert legacy: %v", err)
	}

	if err := SeedPermissions(db); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	var legacy models.Permission
	if err := db.Where("name = ?", "legacy.perm").First(&legacy).Error; err != nil {
		t.Fatalf("legacy lookup: %v", err)
	}
	if legacy.SubjectType != models.PermissionSubjectEmployee {
		t.Errorf("expected subject_type backfilled to %q, got %q",
			models.PermissionSubjectEmployee, legacy.SubjectType)
	}
}

// --- BackfillActuaryProfiles edge cases ---

func TestBackfillActuaryProfiles_DeletesProfileWhenEmployeeNoLongerActuary(t *testing.T) {
	db := openSQLite(t, "employee_backfill_delete")
	if err := db.AutoMigrate(&models.Employee{}, &models.ActuaryProfile{}, &models.Permission{}, &models.Token{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	emp := models.Employee{
		Ime: "X", Prezime: "Y", Email: "x@y.com", Username: "xy",
		Password: "p", SaltPassword: "p", Pol: "M", Pozicija: "Clerk", Aktivan: true,
	}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	// Pre-existing actuary profile from a previous role.
	limit := 50000.0
	stale := models.ActuaryProfile{EmployeeID: emp.ID, Limit: &limit, UsedLimit: 100, NeedApproval: true}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("create stale profile: %v", err)
	}
	// Employee has no actuary permissions → IsActuaryRole() returns false.

	if err := BackfillActuaryProfiles(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var remaining int64
	if err := db.Model(&models.ActuaryProfile{}).Where("employee_id = ?", emp.ID).Count(&remaining).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected stale profile to be deleted, got %d", remaining)
	}
}

func TestSeedPermissions_Idempotent(t *testing.T) {
	db := openSQLite(t, "employee_seed_perms_idempotent")
	if err := db.AutoMigrate(&models.Permission{}); err != nil {
		t.Fatalf("migrate permission: %v", err)
	}

	if err := SeedPermissions(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	var count int64
	if err := db.Model(&models.Permission{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if int(count) != len(models.DefaultPermissions) {
		t.Errorf("expected %d permissions after re-seed, got %d", len(models.DefaultPermissions), count)
	}
}
