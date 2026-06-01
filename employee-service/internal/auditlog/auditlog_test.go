package auditlog_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/auditlog"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func loadLogs(t *testing.T, db *gorm.DB) []models.AuditLog {
	t.Helper()
	var logs []models.AuditLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatalf("load logs: %v", err)
	}
	return logs
}

// TestRecord_WritesCorrectFields verifies that Record() persists all fields.
func TestRecord_WritesCorrectFields(t *testing.T) {
	db := openTestDB(t, "auditlog_fields")
	resID := uint(99)

	auditlog.Record(db, auditlog.AuditEntry{
		Action:       auditlog.ActionAgentLimitChanged,
		ActorID:      7,
		ResourceID:   &resID,
		ResourceType: "employee",
		OldValue:     "1000.0000",
		NewValue:     "2000.0000",
	})

	logs := loadLogs(t, db)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	got := logs[0]

	if got.Action != auditlog.ActionAgentLimitChanged {
		t.Errorf("Action: got %q, want %q", got.Action, auditlog.ActionAgentLimitChanged)
	}
	if got.ActorID != 7 {
		t.Errorf("ActorID: got %d, want 7", got.ActorID)
	}
	if got.ResourceID == nil || *got.ResourceID != 99 {
		t.Errorf("ResourceID: got %v, want 99", got.ResourceID)
	}
	if got.ResourceType != "employee" {
		t.Errorf("ResourceType: got %q, want %q", got.ResourceType, "employee")
	}
	if got.OldValue != "1000.0000" {
		t.Errorf("OldValue: got %q, want %q", got.OldValue, "1000.0000")
	}
	if got.NewValue != "2000.0000" {
		t.Errorf("NewValue: got %q, want %q", got.NewValue, "2000.0000")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

// TestRecord_NilDB_DoesNotPanic verifies that Record() is a no-op when db is nil.
func TestRecord_NilDB_DoesNotPanic(t *testing.T) {
	// Should not panic
	auditlog.Record(nil, auditlog.AuditEntry{
		Action:  auditlog.ActionOrderApproved,
		ActorID: 1,
	})
}

// TestRecord_MultipleEntries_AllPersisted verifies that multiple calls each
// produce a separate row with the correct Action and ActorID.
func TestRecord_MultipleEntries_AllPersisted(t *testing.T) {
	db := openTestDB(t, "auditlog_multi")

	auditlog.Record(db, auditlog.AuditEntry{
		Action:  auditlog.ActionOrderApproved,
		ActorID: 10,
	})
	auditlog.Record(db, auditlog.AuditEntry{
		Action:  auditlog.ActionOrderDeclined,
		ActorID: 11,
	})
	auditlog.Record(db, auditlog.AuditEntry{
		Action:  auditlog.ActionTaxCollectionTriggered,
		ActorID: 12,
	})

	logs := loadLogs(t, db)
	if len(logs) != 3 {
		t.Fatalf("expected 3 audit logs, got %d", len(logs))
	}

	actionActors := map[string]uint{}
	for _, l := range logs {
		actionActors[l.Action] = l.ActorID
	}

	if actionActors[auditlog.ActionOrderApproved] != 10 {
		t.Errorf("OrderApproved actor: got %d, want 10", actionActors[auditlog.ActionOrderApproved])
	}
	if actionActors[auditlog.ActionOrderDeclined] != 11 {
		t.Errorf("OrderDeclined actor: got %d, want 11", actionActors[auditlog.ActionOrderDeclined])
	}
	if actionActors[auditlog.ActionTaxCollectionTriggered] != 12 {
		t.Errorf("TaxCollectionTriggered actor: got %d, want 12", actionActors[auditlog.ActionTaxCollectionTriggered])
	}
}

// TestRecord_NilResourceID_OmittedFromRow verifies that when ResourceID is nil
// no resource_id value is written (remains NULL / zero in the DB).
func TestRecord_NilResourceID_OmittedFromRow(t *testing.T) {
	db := openTestDB(t, "auditlog_nil_res")

	auditlog.Record(db, auditlog.AuditEntry{
		Action:  auditlog.ActionTaxCollectionTriggered,
		ActorID: 5,
	})

	logs := loadLogs(t, db)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].ResourceID != nil {
		t.Errorf("ResourceID should be nil, got %v", logs[0].ResourceID)
	}
}
