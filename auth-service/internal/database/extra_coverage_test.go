package database

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
)

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

func TestSeedDefaultAdmin_ExistingWithBadSalt_ReturnsVerifyError(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin_bad_salt")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}
	existing := models.Employee{
		Ime: "Bad", Prezime: "Admin", Email: "admin@bank.com", Username: "admin",
		Password: "x", SaltPassword: "not-valid-base64-???", Pol: "M",
		Pozicija: "Administrator", Departman: "IT", Aktivan: true,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SeedDefaultAdmin(db); err == nil {
		t.Fatal("expected verify error from bad base64 salt")
	}
}

func TestSeedDefaultAdmin_ExistingActiveButPermissionMissing_Errors(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin_existing_no_perm")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Create admin row WITHOUT seeding permissions first. Use a valid base64 salt
	// so VerifyPassword runs to completion (rather than erroring on bad encoding).
	salt, err := util.GenerateSalt()
	if err != nil {
		t.Fatalf("salt: %v", err)
	}
	hashedPwd, err := util.HashPassword("Admin123!", salt)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	existing := models.Employee{
		Ime: "Old", Prezime: "Admin", Email: "admin@bank.com", Username: "admin",
		Password: hashedPwd, SaltPassword: salt, Pol: "M",
		Pozicija: "Administrator", Departman: "IT", Aktivan: true,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Now SeedDefaultAdmin should hit the existing-path permission fetch and error.
	if err := SeedDefaultAdmin(db); err == nil {
		t.Fatal("expected error when employeeAdmin permission missing in existing-admin branch")
	}
}

func TestSeedDefaultEmployees_PartialFailure_PermissionMissingForOne(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_emps_partial")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Seed only the Basic permission — Agent will be missing, so second iteration fails.
	if err := db.Create(&models.Permission{Name: models.PermEmployeeBasic, SubjectType: models.PermissionSubjectEmployee}).Error; err != nil {
		t.Fatalf("seed perm: %v", err)
	}
	if err := SeedDefaultEmployees(db); err == nil {
		t.Fatal("expected error when one permission is missing")
	}
}

func TestSeedDefaultAdmin_RepairsInactiveExisting(t *testing.T) {
	db := newInMemoryDB(t, "auth_seed_admin_repair")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedPermissions(db); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}

	// Pre-create an inactive admin with a valid-but-wrong password — must be repaired.
	salt, err := util.GenerateSalt()
	if err != nil {
		t.Fatalf("salt: %v", err)
	}
	wrongHash, err := util.HashPassword("WrongPassword123!", salt)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	existing := models.Employee{
		Ime: "Old", Prezime: "Admin", Email: "admin@bank.com", Username: "admin",
		Password: wrongHash, SaltPassword: salt, Pol: "M",
		Pozicija: "Administrator", Departman: "IT",
		Aktivan: false,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := SeedDefaultAdmin(db); err != nil {
		t.Fatalf("SeedDefaultAdmin: %v", err)
	}

	var got models.Employee
	if err := db.Preload("Permissions").Where("email = ?", "admin@bank.com").First(&got).Error; err != nil {
		t.Fatalf("post: %v", err)
	}
	if !got.Aktivan {
		t.Fatal("expected admin to be activated")
	}
	if got.Password == wrongHash {
		t.Fatal("expected admin password to be rehashed")
	}
	if len(got.Permissions) == 0 {
		t.Fatal("expected admin to have permissions attached")
	}
}
