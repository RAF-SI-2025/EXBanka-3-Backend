package database

import "testing"

// TestMigrate_ClosedDB_Errors covers the Migrate error-wrap path: a closed
// connection makes AutoMigrate fail, which propagates through withMigrationLock
// (sqlite branch) and is wrapped by Migrate.
func TestMigrate_ClosedDB_Errors(t *testing.T) {
	db := openSQLite(t, "employee_migrate_closed")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	_ = sqlDB.Close()
	if err := Migrate(db); err == nil {
		t.Fatal("expected migrate error on a closed connection")
	}
}

// TestSeedPermissions_Unmigrated_Errors covers the backfill error branch:
// seeding before the permissions table exists fails.
func TestSeedPermissions_Unmigrated_Errors(t *testing.T) {
	db := openSQLite(t, "employee_seedperm_unmigrated")
	if err := SeedPermissions(db); err == nil {
		t.Fatal("expected error seeding permissions without a table")
	}
}
