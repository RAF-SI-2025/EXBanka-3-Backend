package database

import "testing"

func TestMigrate_ClosedDB_Errors(t *testing.T) {
	db := newInMemoryDB(t, "auth_migrate_closed")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	_ = sqlDB.Close()
	if err := Migrate(db); err == nil {
		t.Fatal("expected migrate error on a closed connection")
	}
}

func TestSeedPermissions_Unmigrated_Errors(t *testing.T) {
	db := newInMemoryDB(t, "auth_seedperm_unmigrated")
	if err := SeedPermissions(db); err == nil {
		t.Fatal("expected error seeding permissions without a table")
	}
}

func TestSeedDefaultAdmin_Unmigrated_Errors(t *testing.T) {
	db := newInMemoryDB(t, "auth_admin_unmigrated")
	if err := SeedDefaultAdmin(db); err == nil {
		t.Fatal("expected error seeding admin without tables")
	}
}
