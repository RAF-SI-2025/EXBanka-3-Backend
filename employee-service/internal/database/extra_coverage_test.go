package database

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
)

func TestBackfillActuaryProfiles_AgentWithExistingProfile_PreservesLimitAndNeedApproval(t *testing.T) {
	db := openEmployeeBackfillTestDB(t, "employee_backfill_existing_agent")

	agentPerm := models.Permission{Name: models.PermEmployeeAgent, SubjectType: models.PermissionSubjectEmployee}
	if err := db.Create(&agentPerm).Error; err != nil {
		t.Fatalf("create perm: %v", err)
	}

	agent := models.Employee{
		Ime: "Existing", Prezime: "Agent", Email: "ea@b.com", Username: "ea",
		Password: "p", SaltPassword: "p", Pol: "M", Pozicija: "Agent", Aktivan: true,
		Limit:     999, // should be ignored because existing profile already has a limit
		UsedLimit: 10,
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	attachEmployeePermissions(t, db, &agent, agentPerm)

	custom := 77777.0
	existing := models.ActuaryProfile{EmployeeID: agent.ID, Limit: &custom, UsedLimit: 250, NeedApproval: false}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed existing: %v", err)
	}

	if err := BackfillActuaryProfiles(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got models.ActuaryProfile
	if err := db.Where("employee_id = ?", agent.ID).First(&got).Error; err != nil {
		t.Fatalf("expected profile: %v", err)
	}
	if got.Limit == nil || *got.Limit != 77777 {
		t.Fatalf("expected preserved limit 77777, got %#v", got.Limit)
	}
	if got.NeedApproval {
		t.Fatal("expected NeedApproval false to be preserved")
	}
}

func TestBackfillActuaryProfiles_AgentNoExistingNoLimit_UsesDefault(t *testing.T) {
	db := openEmployeeBackfillTestDB(t, "employee_backfill_default_limit")

	agentPerm := models.Permission{Name: models.PermEmployeeAgent, SubjectType: models.PermissionSubjectEmployee}
	if err := db.Create(&agentPerm).Error; err != nil {
		t.Fatalf("create perm: %v", err)
	}

	agent := models.Employee{
		Ime: "Zero", Prezime: "Limit", Email: "zl@b.com", Username: "zl",
		Password: "p", SaltPassword: "p", Pol: "M", Pozicija: "Agent", Aktivan: true,
		// Limit zero → should fall back to DefaultAgentTradingLimit
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	attachEmployeePermissions(t, db, &agent, agentPerm)

	if err := BackfillActuaryProfiles(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got models.ActuaryProfile
	if err := db.Where("employee_id = ?", agent.ID).First(&got).Error; err != nil {
		t.Fatalf("expected profile: %v", err)
	}
	if got.Limit == nil || *got.Limit != models.DefaultAgentTradingLimit {
		t.Fatalf("expected default limit, got %#v", got.Limit)
	}
}

func TestMigrate_DropsTablesError_PropagatesNil(t *testing.T) {
	db := openSQLite(t, "employee_migrate_double")
	if err := Migrate(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Second migrate on same DB should still be a no-op success.
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
