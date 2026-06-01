package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	employeev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/gen/proto/employee/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
	"github.com/glebarez/sqlite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// =====================
// shared test helpers
// =====================

func openHandlerDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Employee{}, &models.ActuaryProfile{}, &models.Permission{}, &models.Token{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func createEmployee(t *testing.T, db *gorm.DB, email, username string, perms ...string) *models.Employee {
	t.Helper()
	emp := &models.Employee{
		Ime:           "Test",
		Prezime:       "User",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         email,
		BrojTelefona:  "0600000000",
		Adresa:        "Test Street 1",
		Username:      username,
		Password:      "hash",
		SaltPassword:  "salt",
		Pozicija:      "Agent",
		Departman:     "Trading",
		Aktivan:       true,
	}
	if err := db.Create(emp).Error; err != nil {
		t.Fatalf("create employee %s: %v", email, err)
	}
	for _, name := range perms {
		p := models.Permission{Name: name, SubjectType: models.PermissionSubjectEmployee}
		if err := db.Where("name = ?", name).FirstOrCreate(&p).Error; err != nil {
			t.Fatalf("create permission %s: %v", name, err)
		}
		if err := db.Model(emp).Association("Permissions").Append(&p); err != nil {
			t.Fatalf("attach permission %s: %v", name, err)
		}
	}
	return emp
}

func supervisorToken(t *testing.T, cfg *config.Config, emp *models.Employee) string {
	t.Helper()
	tok, err := util.GenerateAccessToken(emp.ID, emp.Email, emp.Username, []string{models.PermEmployeeSupervisor}, cfg.JWTSecret, 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func agentToken(t *testing.T, cfg *config.Config, emp *models.Employee) string {
	t.Helper()
	tok, err := util.GenerateAccessToken(emp.ID, emp.Email, emp.Username, []string{models.PermEmployeeAgent}, cfg.JWTSecret, 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// =====================
// ActuaryHTTPHandler additional tests
// =====================

func TestListActuaries_WrongMethod_Returns405(t *testing.T) {
	db := openHandlerDB(t, "list_act_method")
	cfg := &config.Config{JWTSecret: "secret"}
	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/actuaries", nil)
	w := httptest.NewRecorder()
	h.ListActuaries(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestListActuaries_NoAuth_Returns401(t *testing.T) {
	db := openHandlerDB(t, "list_act_noauth")
	cfg := &config.Config{JWTSecret: "secret"}
	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/actuaries", nil)
	w := httptest.NewRecorder()
	h.ListActuaries(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListActuaries_SearchFilter(t *testing.T) {
	db := openHandlerDB(t, "list_act_search")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.search@bank.com", "sup-search", models.PermEmployeeSupervisor)
	createEmployee(t, db, "agent.alpha@bank.com", "alpha", models.PermEmployeeAgent)
	createEmployee(t, db, "agent.beta@bank.com", "beta", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/actuaries?q=alpha", nil)
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ListActuaries(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alpha") {
		t.Error("expected filtered response to contain 'alpha'")
	}
}

// =====================
// ActuaryRoutes tests
// =====================

func TestActuaryRoutes_UpdateLimit_Success(t *testing.T) {
	db := openHandlerDB(t, "act_routes_limit")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.limit@bank.com", "sup-limit", models.PermEmployeeSupervisor)
	agent := createEmployee(t, db, "agent.limit@bank.com", "agent-limit", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	body := `{"limit": 50000.0}`
	r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/actuaries/%d/limit", agent.ID), strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestActuaryRoutes_UpdateLimit_InvalidBody(t *testing.T) {
	db := openHandlerDB(t, "act_routes_limit_bad")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.limitb@bank.com", "sup-limitb", models.PermEmployeeSupervisor)
	agent := createEmployee(t, db, "agent.limitb@bank.com", "agent-limitb", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/actuaries/%d/limit", agent.ID), strings.NewReader("not json"))
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestActuaryRoutes_ResetUsedLimit_Success(t *testing.T) {
	db := openHandlerDB(t, "act_routes_reset")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.reset@bank.com", "sup-reset", models.PermEmployeeSupervisor)
	agent := createEmployee(t, db, "agent.reset@bank.com", "agent-reset", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/actuaries/%d/reset-used-limit", agent.ID), nil)
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestActuaryRoutes_SetNeedApproval_Success(t *testing.T) {
	db := openHandlerDB(t, "act_routes_approval")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.approval@bank.com", "sup-approval", models.PermEmployeeSupervisor)
	agent := createEmployee(t, db, "agent.approval@bank.com", "agent-approval", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	body := `{"needApproval": true}`
	r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/actuaries/%d/need-approval", agent.ID), strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestActuaryRoutes_SetNeedApproval_InvalidBody(t *testing.T) {
	db := openHandlerDB(t, "act_routes_approval_bad")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.approvalbad@bank.com", "sup-approvalbad", models.PermEmployeeSupervisor)
	agent := createEmployee(t, db, "agent.approvalbad@bank.com", "agent-approvalbad", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/actuaries/%d/need-approval", agent.ID), strings.NewReader("not json"))
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestActuaryRoutes_EmptyPath_Returns404(t *testing.T) {
	db := openHandlerDB(t, "act_routes_empty")
	cfg := &config.Config{JWTSecret: "secret"}
	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/actuaries/", nil)
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestActuaryRoutes_InvalidEmployeeID_Returns400(t *testing.T) {
	db := openHandlerDB(t, "act_routes_badid")
	cfg := &config.Config{JWTSecret: "secret"}
	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPut, "/api/v1/actuaries/notanumber/limit", nil)
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestActuaryRoutes_NoAuth_Returns401(t *testing.T) {
	db := openHandlerDB(t, "act_routes_noauth")
	cfg := &config.Config{JWTSecret: "secret"}
	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPut, "/api/v1/actuaries/1/limit", strings.NewReader(`{"limit":100}`))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestActuaryRoutes_AgentForbidden_Returns403(t *testing.T) {
	db := openHandlerDB(t, "act_routes_agentforbidden")
	cfg := &config.Config{JWTSecret: "secret"}

	agent := createEmployee(t, db, "agent.forbidden@bank.com", "agent-forbidden", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/actuaries/%d/limit", agent.ID), strings.NewReader(`{"limit":100}`))
	r.Header.Set("Authorization", "Bearer "+agentToken(t, cfg, agent))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestActuaryRoutes_UnknownSubpath_Returns404(t *testing.T) {
	db := openHandlerDB(t, "act_routes_unknown")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.unknown@bank.com", "sup-unknown", models.PermEmployeeSupervisor)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/actuaries/1/unknown-action", nil)
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestActuaryRoutes_PatchLimit_Success(t *testing.T) {
	db := openHandlerDB(t, "act_routes_patchlimit")
	cfg := &config.Config{JWTSecret: "secret"}

	sup := createEmployee(t, db, "sup.patch@bank.com", "sup-patch", models.PermEmployeeSupervisor)
	agent := createEmployee(t, db, "agent.patch@bank.com", "agent-patch", models.PermEmployeeAgent)

	h := NewActuaryHTTPHandler(cfg, service.NewEmployeeService(cfg, db, nil), nil)

	body := `{"limit": 25000.0}`
	r := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/actuaries/%d/limit", agent.ID), strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+supervisorToken(t, cfg, sup))
	w := httptest.NewRecorder()
	h.ActuaryRoutes(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// =====================
// http_auth helpers
// =====================

func TestParseHTTPClaims_MissingHeader(t *testing.T) {
	cfg := &config.Config{JWTSecret: "secret"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := parseHTTPClaims(r, cfg)
	if err == nil {
		t.Error("expected error for missing auth header")
	}
}

func TestParseHTTPClaims_InvalidToken(t *testing.T) {
	cfg := &config.Config{JWTSecret: "secret"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer invalidtoken")
	_, err := parseHTTPClaims(r, cfg)
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestRequireAuthenticatedEmployeeHTTP_ZeroEmployeeID_Returns403(t *testing.T) {
	cfg := &config.Config{JWTSecret: "secret"}

	// Token with employeeID=0 should be rejected
	tok, err := util.GenerateAccessToken(0, "test@test.com", "test", []string{}, cfg.JWTSecret, 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	_, ok := requireAuthenticatedEmployeeHTTP(w, r, cfg)
	if ok {
		t.Error("expected ok=false for zero employee ID")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================
// EmployeeHandler (gRPC) tests
// =====================

func TestEmployeeHandler_CreateEmployee_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_create")
	cfg := &config.Config{JWTSecret: "secret"}
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.CreateEmployee(context.Background(), &employeev1.CreateEmployeeRequest{
		Ime:           "Ana",
		Prezime:       "Petrovic",
		DatumRodjenja: time.Date(1995, 6, 15, 0, 0, 0, 0, time.UTC).Unix(),
		Pol:           "F",
		Email:         "ana.petrovic@bank.com",
		BrojTelefona:  "0641111111",
		Adresa:        "Test 1",
		Username:      "ana.petrovic",
		Pozicija:      "Analyst",
		Departman:     "Finance",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Employee.Email != "ana.petrovic@bank.com" {
		t.Errorf("expected email, got %s", resp.Employee.Email)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestEmployeeHandler_CreateEmployee_DuplicateEmail_ReturnsInvalidArgument(t *testing.T) {
	db := openHandlerDB(t, "grpc_create_dup")
	cfg := &config.Config{JWTSecret: "secret"}
	createEmployee(t, db, "dup@bank.com", "dup-user")
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	_, err := h.CreateEmployee(context.Background(), &employeev1.CreateEmployeeRequest{
		Ime:      "X",
		Prezime:  "Y",
		Email:    "dup@bank.com",
		Username: "dup-user2",
		Pol:      "M",
	})

	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestEmployeeHandler_GetEmployee_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_get")
	cfg := &config.Config{JWTSecret: "secret"}
	emp := createEmployee(t, db, "get.emp@bank.com", "get-emp")
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.GetEmployee(context.Background(), &employeev1.GetEmployeeRequest{Id: uint64(emp.ID)})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Employee.Email != emp.Email {
		t.Errorf("expected email %s, got %s", emp.Email, resp.Employee.Email)
	}
}

func TestEmployeeHandler_GetEmployee_NotFound(t *testing.T) {
	db := openHandlerDB(t, "grpc_get_nf")
	cfg := &config.Config{JWTSecret: "secret"}
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	_, err := h.GetEmployee(context.Background(), &employeev1.GetEmployeeRequest{Id: 9999})

	if err == nil {
		t.Fatal("expected error for non-existent employee")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestEmployeeHandler_ListEmployees_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_list")
	cfg := &config.Config{JWTSecret: "secret"}
	createEmployee(t, db, "list1@bank.com", "list1-user")
	createEmployee(t, db, "list2@bank.com", "list2-user")
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.ListEmployees(context.Background(), &employeev1.ListEmployeesRequest{
		Page:     1,
		PageSize: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Employees) < 2 {
		t.Errorf("expected at least 2 employees, got %d", len(resp.Employees))
	}
	if resp.Page != 1 || resp.PageSize != 10 {
		t.Errorf("unexpected pagination: page=%d size=%d", resp.Page, resp.PageSize)
	}
}


func TestEmployeeHandler_UpdateEmployee_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_update")
	cfg := &config.Config{JWTSecret: "secret"}
	emp := createEmployee(t, db, "update.emp@bank.com", "update-emp")
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.UpdateEmployee(context.Background(), &employeev1.UpdateEmployeeRequest{
		Id:            uint64(emp.ID),
		Ime:           "Updated",
		Prezime:       "Name",
		Email:         "update.emp@bank.com",
		Username:      "update-emp",
		BrojTelefona:  "0649999999",
		Adresa:        "New Address 1",
		Pol:           "M",
		Pozicija:      "Senior Agent",
		Departman:     "Trading",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		Aktivan:       true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Employee.Ime != "Updated" {
		t.Errorf("expected Ime=Updated, got %s", resp.Employee.Ime)
	}
}

func TestEmployeeHandler_UpdateEmployee_NotFound_ReturnsInvalidArgument(t *testing.T) {
	db := openHandlerDB(t, "grpc_update_nf")
	cfg := &config.Config{JWTSecret: "secret"}
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	_, err := h.UpdateEmployee(context.Background(), &employeev1.UpdateEmployeeRequest{
		Id:  9999,
		Ime: "Ghost",
	})

	if err == nil {
		t.Fatal("expected error for non-existent employee")
	}
}

func TestEmployeeHandler_SetEmployeeActive_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_active")
	cfg := &config.Config{JWTSecret: "secret"}
	emp := createEmployee(t, db, "active.emp@bank.com", "active-emp")
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.SetEmployeeActive(context.Background(), &employeev1.SetEmployeeActiveRequest{
		Id:      uint64(emp.ID),
		Aktivan: false,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Aktivan != false {
		t.Error("expected Aktivan=false")
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestEmployeeHandler_SetEmployeeActive_NotFound(t *testing.T) {
	db := openHandlerDB(t, "grpc_active_nf")
	cfg := &config.Config{JWTSecret: "secret"}
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	_, err := h.SetEmployeeActive(context.Background(), &employeev1.SetEmployeeActiveRequest{
		Id:      9999,
		Aktivan: true,
	})

	if err == nil {
		t.Fatal("expected error for non-existent employee")
	}
}

func TestEmployeeHandler_UpdateEmployeePermissions_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_perms")
	cfg := &config.Config{JWTSecret: "secret"}
	// Seed the permission so UpdateEmployeePermissions can find it
	emp := createEmployee(t, db, "perms.emp@bank.com", "perms-emp", models.PermEmployeeAgent)
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.UpdateEmployeePermissions(context.Background(), &employeev1.UpdateEmployeePermissionsRequest{
		Id:              uint64(emp.ID),
		PermissionNames: []string{models.PermEmployeeAgent},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestEmployeeHandler_UpdateEmployeePermissions_NotFound_ReturnsInvalidArgument(t *testing.T) {
	db := openHandlerDB(t, "grpc_perms_nf")
	cfg := &config.Config{JWTSecret: "secret"}
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	_, err := h.UpdateEmployeePermissions(context.Background(), &employeev1.UpdateEmployeePermissionsRequest{
		Id:              9999,
		PermissionNames: []string{models.PermEmployeeAgent},
	})

	if err == nil {
		t.Fatal("expected error for non-existent employee")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestEmployeeHandler_GetAllPermissions_Success(t *testing.T) {
	db := openHandlerDB(t, "grpc_allperms")
	cfg := &config.Config{JWTSecret: "secret"}

	// Seed some permissions
	for _, name := range []string{models.PermEmployeeBasic, models.PermEmployeeAgent} {
		p := models.Permission{Name: name, SubjectType: models.PermissionSubjectEmployee}
		db.Where("name = ?", name).FirstOrCreate(&p)
	}

	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.GetAllPermissions(context.Background(), &employeev1.GetAllPermissionsRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Permissions) < 2 {
		t.Errorf("expected at least 2 permissions, got %d", len(resp.Permissions))
	}
}

func TestEmployeeHandler_ToEmployeeProto_WithPermissions(t *testing.T) {
	db := openHandlerDB(t, "grpc_proto_perms")
	cfg := &config.Config{JWTSecret: "secret"}
	emp := createEmployee(t, db, "proto.emp@bank.com", "proto-emp", models.PermEmployeeAgent)
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.GetEmployee(context.Background(), &employeev1.GetEmployeeRequest{Id: uint64(emp.ID)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Employee.Permissions) == 0 {
		t.Error("expected permissions to be mapped in proto")
	}
}

func TestEmployeeHandler_ListEmployees_ToListItem(t *testing.T) {
	db := openHandlerDB(t, "grpc_listitem")
	cfg := &config.Config{JWTSecret: "secret"}
	createEmployee(t, db, "item.emp@bank.com", "item-emp", models.PermEmployeeAgent)
	svc := service.NewEmployeeService(cfg, db, nil)
	h := NewEmployeeHandlerWithService(svc, nil)

	resp, err := h.ListEmployees(context.Background(), &employeev1.ListEmployeesRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Employees) == 0 {
		t.Fatal("expected at least one employee in list")
	}

	item := resp.Employees[0]
	if item.Email == "" {
		t.Error("expected non-empty email in list item")
	}
}

