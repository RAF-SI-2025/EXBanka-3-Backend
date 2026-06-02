package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"gorm.io/gorm"
)

// AuditHTTPHandler serves GET /api/v1/audit-logs.
// Access is restricted to supervisor and admin roles.
type AuditHTTPHandler struct {
	cfg *config.Config
	db  *gorm.DB
}

func NewAuditHTTPHandler(cfg *config.Config, db *gorm.DB) *AuditHTTPHandler {
	return &AuditHTTPHandler{cfg: cfg, db: db}
}

// ListAuditLogs handles GET /api/v1/audit-logs with optional filters:
//
//	?action=AGENT_LIMIT_CHANGED
//	?actorId=42
//	?from=2025-01-01T00:00:00Z
//	?to=2025-12-31T23:59:59Z
func (h *AuditHTTPHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, ok := requireAuthenticatedEmployeeHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeSupervisor) {
		return
	}

	q := r.URL.Query()

	query := h.db.Model(&models.AuditLog{}).Order("created_at DESC")

	if action := q.Get("action"); action != "" {
		query = query.Where("action = ?", action)
	}

	if actorIDStr := q.Get("actorId"); actorIDStr != "" {
		actorID, err := strconv.ParseUint(actorIDStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid actorId"})
			return
		}
		query = query.Where("actor_id = ?", actorID)
	}

	if fromStr := q.Get("from"); fromStr != "" {
		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid from date, use RFC3339"})
			return
		}
		query = query.Where("created_at >= ?", from.UTC())
	}

	if toStr := q.Get("to"); toStr != "" {
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid to date, use RFC3339"})
			return
		}
		query = query.Where("created_at <= ?", to.UTC())
	}

	var logs []models.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load audit logs"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"audit_logs": logs,
		"count":      len(logs),
	})
}
