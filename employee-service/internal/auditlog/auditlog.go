package auditlog

import (
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// Action constants for all audited operations.
const (
	ActionAgentLimitChanged          = "AGENT_LIMIT_CHANGED"
	ActionAgentUsedLimitReset        = "AGENT_USED_LIMIT_RESET"
	ActionOrderApproved              = "ORDER_APPROVED"
	ActionOrderDeclined              = "ORDER_DECLINED"
	ActionEmployeePermissionsChanged = "EMPLOYEE_PERMISSIONS_CHANGED"
	ActionTaxCollectionTriggered     = "TAX_COLLECTION_TRIGGERED"
)

// AuditEntry carries the data for a single audit record.
type AuditEntry struct {
	Action       string
	ActorID      uint
	ResourceID   *uint
	ResourceType string
	OldValue     string
	NewValue     string
}

// Record writes an audit log entry to the database.
// It is best-effort: if db is nil or the write fails, the error is only logged
// and never propagated — the caller's main operation is never interrupted.
func Record(db *gorm.DB, e AuditEntry) {
	if db == nil {
		return
	}

	row := map[string]interface{}{
		"created_at":    time.Now().UTC(),
		"action":        e.Action,
		"actor_id":      e.ActorID,
		"resource_type": e.ResourceType,
		"old_value":     e.OldValue,
		"new_value":     e.NewValue,
	}
	if e.ResourceID != nil {
		row["resource_id"] = *e.ResourceID
	}

	if err := db.Table("audit_logs").Create(row).Error; err != nil {
		slog.Error("audit log write failed",
			"action", e.Action,
			"actor_id", e.ActorID,
			"error", err,
		)
	}
}
