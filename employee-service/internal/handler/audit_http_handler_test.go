package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/auditlog"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openAuditTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedAuditLogs(t *testing.T, db *gorm.DB, entries []models.AuditLog) {
	t.Helper()
	for i := range entries {
		if err := db.Create(&entries[i]).Error; err != nil {
			t.Fatalf("seed audit log: %v", err)
		}
	}
}

func newAuditHandler(cfg *config.Config, db *gorm.DB) *AuditHTTPHandler {
	return NewAuditHTTPHandler(cfg, db)
}

// Scenario 44 — Unauthorized access is rejected with 401
func TestAuditHTTP_Unauthorized(t *testing.T) {
	db := openAuditTestDB(t, "audit_unauth")
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := newAuditHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	rec := httptest.NewRecorder()
	h.ListAuditLogs(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Scenario 45 — Agent (non-supervisor) is rejected with 403
func TestAuditHTTP_AgentForbidden(t *testing.T) {
	db := openAuditTestDB(t, "audit_agent_403")
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := newAuditHandler(cfg, db)

	token := auditTestToken(t, cfg, 99, "agent@bank.com", "agent", []string{models.PermEmployeeAgent})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ListAuditLogs(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Scenario 46 — Supervisor gets all logs, admin also gets all logs.
// Also verifies that action/actorId/date filters narrow results correctly.
func TestAuditHTTP_SupervisorSeesAllLogs(t *testing.T) {
	db := openAuditTestDB(t, "audit_supervisor_all")
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := newAuditHandler(cfg, db)

	now := time.Now().UTC()
	res1 := uint(1)
	res2 := uint(2)
	seedAuditLogs(t, db, []models.AuditLog{
		{
			CreatedAt:    now.Add(-2 * time.Hour),
			Action:       auditlog.ActionAgentLimitChanged,
			ActorID:      10,
			ResourceID:   &res1,
			ResourceType: "employee",
			OldValue:     "1000.0000",
			NewValue:     "2000.0000",
		},
		{
			CreatedAt:    now.Add(-1 * time.Hour),
			Action:       auditlog.ActionOrderApproved,
			ActorID:      20,
			ResourceID:   &res2,
			ResourceType: "order",
			OldValue:     "pending",
			NewValue:     "approved_by=20",
		},
	})

	token := auditTestToken(t, cfg, 1, "sup@bank.com", "supervisor", []string{models.PermEmployeeSupervisor})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ListAuditLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		AuditLogs []models.AuditLog `json:"audit_logs"`
		Count     int               `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("expected 2 logs, got %d", payload.Count)
	}
}

// TestAuditHTTP_FilterByAction verifies that ?action= narrows results.
func TestAuditHTTP_FilterByAction(t *testing.T) {
	db := openAuditTestDB(t, "audit_filter_action")
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := newAuditHandler(cfg, db)

	now := time.Now().UTC()
	res := uint(5)
	seedAuditLogs(t, db, []models.AuditLog{
		{CreatedAt: now, Action: auditlog.ActionAgentLimitChanged, ActorID: 1, ResourceID: &res, ResourceType: "employee"},
		{CreatedAt: now, Action: auditlog.ActionOrderApproved, ActorID: 2},
		{CreatedAt: now, Action: auditlog.ActionOrderApproved, ActorID: 3},
	})

	token := auditTestToken(t, cfg, 1, "sup@bank.com", "supervisor", []string{models.PermEmployeeSupervisor})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action="+auditlog.ActionOrderApproved, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ListAuditLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("expected 2 ORDER_APPROVED logs, got %d", payload.Count)
	}
}

// TestAuditHTTP_FilterByActorID verifies that ?actorId= narrows results.
func TestAuditHTTP_FilterByActorID(t *testing.T) {
	db := openAuditTestDB(t, "audit_filter_actor")
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := newAuditHandler(cfg, db)

	now := time.Now().UTC()
	seedAuditLogs(t, db, []models.AuditLog{
		{CreatedAt: now, Action: auditlog.ActionOrderApproved, ActorID: 42},
		{CreatedAt: now, Action: auditlog.ActionOrderApproved, ActorID: 99},
		{CreatedAt: now, Action: auditlog.ActionOrderDeclined, ActorID: 42},
	})

	token := auditTestToken(t, cfg, 1, "sup@bank.com", "supervisor", []string{models.PermEmployeeSupervisor})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?actorId=42", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ListAuditLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("expected 2 logs for actor 42, got %d", payload.Count)
	}
}

// auditTestToken generates a signed employee JWT for use in audit handler tests.
func auditTestToken(t *testing.T, cfg *config.Config, id uint, email, username string, perms []string) string {
	t.Helper()
	tok, err := util.GenerateAccessToken(id, email, username, perms, cfg.JWTSecret, 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}
