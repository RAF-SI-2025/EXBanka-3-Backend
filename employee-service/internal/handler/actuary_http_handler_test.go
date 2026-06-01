package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openActuaryHandlerTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.Employee{}, &models.ActuaryProfile{}, &models.Permission{}, &models.Token{}); err != nil {
		t.Fatalf("failed to migrate employee tables: %v", err)
	}
	return db
}

func mustCreateEmployeeWithPermissions(t *testing.T, db *gorm.DB, email, username string, permissionNames ...string) *models.Employee {
	t.Helper()

	emp := &models.Employee{
		Ime:           "Test",
		Prezime:       username,
		DatumRodjenja: time.Date(1991, 5, 20, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         email,
		BrojTelefona:  "0641234567",
		Adresa:        "Actuary Test 1",
		Username:      username,
		Password:      "pending",
		SaltPassword:  "pending",
		Pozicija:      "Analyst",
		Departman:     "Trading",
		Aktivan:       true,
	}
	if err := db.Create(emp).Error; err != nil {
		t.Fatalf("failed to create employee %s: %v", email, err)
	}

	if len(permissionNames) == 0 {
		return emp
	}

	perms := make([]models.Permission, 0, len(permissionNames))
	for _, name := range permissionNames {
		perm := models.Permission{Name: name, SubjectType: models.PermissionSubjectEmployee}
		if err := db.Where("name = ?", name).FirstOrCreate(&perm).Error; err != nil {
			t.Fatalf("failed to create permission %s: %v", name, err)
		}
		perms = append(perms, perm)
	}
	if err := db.Model(emp).Association("Permissions").Append(perms); err != nil {
		t.Fatalf("failed to attach permissions for %s: %v", email, err)
	}

	return emp
}

func mustEmployeeAccessToken(t *testing.T, cfg *config.Config, employeeID uint, email, username string, permissions []string) string {
	t.Helper()

	token, err := util.GenerateAccessToken(employeeID, email, username, permissions, cfg.JWTSecret, 60)
	if err != nil {
		t.Fatalf("failed to generate employee token: %v", err)
	}
	return token
}

func TestActuaryHTTPHandler_ListActuaries_AllowsSupervisorAndAdmin(t *testing.T) {
	db := openActuaryHandlerTestDB(t, "actuary_handler_supervisor")
	cfg := &config.Config{JWTSecret: "test-secret"}

	_ = mustCreateEmployeeWithPermissions(t, db, "agent.market@bank.com", "agent-market", models.PermEmployeeAgent)
	supervisor := mustCreateEmployeeWithPermissions(t, db, "supervisor.market@bank.com", "supervisor-market", models.PermEmployeeSupervisor)
	admin := mustCreateEmployeeWithPermissions(t, db, "admin.market@bank.com", "admin-market", models.PermEmployeeAdmin)

	svc := service.NewEmployeeService(cfg, db, nil)
	handler := NewActuaryHTTPHandler(cfg, svc, nil)

	for _, testCase := range []struct {
		name        string
		employee    *models.Employee
		permissions []string
	}{
		{
			name:        "supervisor",
			employee:    supervisor,
			permissions: []string{models.PermEmployeeSupervisor},
		},
		{
			name:        "admin",
			employee:    admin,
			permissions: []string{models.PermEmployeeAdmin},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/actuaries", nil)
			req.Header.Set("Authorization", "Bearer "+mustEmployeeAccessToken(t, cfg, testCase.employee.ID, testCase.employee.Email, testCase.employee.Username, testCase.permissions))
			rec := httptest.NewRecorder()

			handler.ListActuaries(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
			}

			var payload struct {
				Actuaries []map[string]interface{} `json:"actuaries"`
				Count     int                      `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if payload.Count < 2 {
				t.Fatalf("expected supervisor/admin to receive actuaries list, got %+v", payload)
			}
		})
	}
}

func TestActuaryHTTPHandler_ListActuaries_RejectsNonSupervisorEmployee(t *testing.T) {
	db := openActuaryHandlerTestDB(t, "actuary_handler_agent")
	cfg := &config.Config{JWTSecret: "test-secret"}

	agent := mustCreateEmployeeWithPermissions(t, db, "agent.only@bank.com", "agent-only", models.PermEmployeeAgent)
	svc := service.NewEmployeeService(cfg, db, nil)
	handler := NewActuaryHTTPHandler(cfg, svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/actuaries", nil)
	req.Header.Set("Authorization", "Bearer "+mustEmployeeAccessToken(t, cfg, agent.ID, agent.Email, agent.Username, []string{models.PermEmployeeAgent}))
	rec := httptest.NewRecorder()

	handler.ListActuaries(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d with body %s", rec.Code, rec.Body.String())
	}
}
