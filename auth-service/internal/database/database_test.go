package database

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newInMemoryDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// =====================
// Migrate
// =====================

func TestMigrate_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "auth_migrate")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

func TestMigrate_CreatesExpectedTables(t *testing.T) {
	db := newInMemoryDB(t, "auth_migrate_tables")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tables := []interface{}{&models.Employee{}, &models.Permission{}, &models.Token{}}
	for _, m := range tables {
		if !db.Migrator().HasTable(m) {
			t.Errorf("missing table for %T", m)
		}
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "auth_migrate_idem")
	if err := Migrate(db); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second: %v", err)
	}
}

// =====================
// SeedPermissions
// =====================

func TestSeedPermissions_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_perm")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
}

func TestSeedPermissions_InsertsAllDefaults(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_perm_count")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	var count int64
	db.Model(&models.Permission{}).Count(&count)
	if int(count) != len(models.DefaultPermissions) {
		t.Errorf("expected %d permissions, got %d", len(models.DefaultPermissions), count)
	}
}

func TestSeedPermissions_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_perm_idem")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("first: %v", err)
	}
	var before int64
	db.Model(&models.Permission{}).Count(&before)
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("second: %v", err)
	}
	var after int64
	db.Model(&models.Permission{}).Count(&after)
	if before != after {
		t.Errorf("idempotency broken: before=%d after=%d", before, after)
	}
}

func TestSeedPermissions_BackfillsSubjectType(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_perm_backfill")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Insert a permission with empty subject_type
	if err := db.Exec("INSERT INTO permissions (name, description, subject_type) VALUES (?, ?, ?)", "legacyPerm", "old", "").Error; err != nil {
		t.Fatalf("insert legacy: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	var p models.Permission
	if err := db.Where("name = ?", "legacyPerm").First(&p).Error; err != nil {
		t.Fatalf("find legacy: %v", err)
	}
	if p.SubjectType != models.PermissionSubjectEmployee {
		t.Errorf("expected backfilled subject_type=employee, got %s", p.SubjectType)
	}
}

// =====================
// SeedDefaultAdmin
// =====================

func TestSeedDefaultAdmin_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	if err := SeedDefaultAdmin(db); err != nil {
		t.Fatalf("SeedDefaultAdmin: %v", err)
	}
}

func TestSeedDefaultAdmin_CreatesAdmin(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin_create")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	if err := SeedDefaultAdmin(db); err != nil {
		t.Fatalf("SeedDefaultAdmin: %v", err)
	}
	var emp models.Employee
	if err := db.Where("email = ?", "admin@bank.com").First(&emp).Error; err != nil {
		t.Errorf("expected admin to exist: %v", err)
	}
	if !emp.Aktivan {
		t.Error("expected admin to be active")
	}
}

func TestSeedDefaultAdmin_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin_idem")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	if err := SeedDefaultAdmin(db); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := SeedDefaultAdmin(db); err != nil {
		t.Fatalf("second: %v", err)
	}
	var count int64
	db.Model(&models.Employee{}).Where("email = ?", "admin@bank.com").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 admin, got %d", count)
	}
}

func TestSeedDefaultAdmin_FailsWithoutPermission(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin_no_perm")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedDefaultAdmin(db); err == nil {
		t.Error("expected error when employeeAdmin permission missing")
	}
}

// =====================
// SeedDefaultEmployees
// =====================

func TestSeedDefaultEmployees_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_emps")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	if err := SeedDefaultEmployees(db); err != nil {
		t.Fatalf("SeedDefaultEmployees: %v", err)
	}
}

func TestSeedDefaultEmployees_CreatesEmployees(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_emps_create")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	if err := SeedDefaultEmployees(db); err != nil {
		t.Fatalf("SeedDefaultEmployees: %v", err)
	}
	for _, email := range []string{"marko.petrovic@bank.com", "ana.jovic@bank.com"} {
		var e models.Employee
		if err := db.Where("email = ?", email).First(&e).Error; err != nil {
			t.Errorf("expected employee %s: %v", email, err)
		}
	}
}

func TestSeedDefaultEmployees_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_emps_idem")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	if err := SeedDefaultEmployees(db); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := SeedDefaultEmployees(db); err != nil {
		t.Fatalf("second: %v", err)
	}
	var count int64
	db.Model(&models.Employee{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 employees, got %d", count)
	}
}

func TestSeedDefaultEmployees_FailsWithoutPermissions(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_emps_no_perm")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedDefaultEmployees(db); err == nil {
		t.Error("expected error when permissions missing")
	}
}
