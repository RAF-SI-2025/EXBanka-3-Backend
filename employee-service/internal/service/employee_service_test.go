package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
)

// ---- mock employee repository ----

type mockEmployeeRepo struct {
	createFn         func(emp *models.Employee) error
	findByIDFn       func(id uint) (*models.Employee, error)
	findByEmailFn    func(email string) (*models.Employee, error)
	listAllFn        func() ([]models.Employee, error)
	listFn           func(filter repository.EmployeeFilter) ([]models.Employee, int64, error)
	updateFn         func(emp *models.Employee) error
	updateFieldsFn   func(id uint, fields map[string]interface{}) error
	setPermissionsFn func(emp *models.Employee, permissions []models.Permission) error
	emailExistsFn    func(email string, excludeID uint) (bool, error)
	usernameExistsFn func(username string, excludeID uint) (bool, error)
}

func (m *mockEmployeeRepo) Create(emp *models.Employee) error {
	if m.createFn != nil {
		return m.createFn(emp)
	}
	return nil
}

func (m *mockEmployeeRepo) FindByID(id uint) (*models.Employee, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEmployeeRepo) FindByEmail(email string) (*models.Employee, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(email)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEmployeeRepo) ListAll() ([]models.Employee, error) {
	if m.listAllFn != nil {
		return m.listAllFn()
	}
	return nil, errors.New("not implemented")
}

func (m *mockEmployeeRepo) List(filter repository.EmployeeFilter) ([]models.Employee, int64, error) {
	if m.listFn != nil {
		return m.listFn(filter)
	}
	return nil, 0, errors.New("not implemented")
}

func (m *mockEmployeeRepo) Update(emp *models.Employee) error {
	if m.updateFn != nil {
		return m.updateFn(emp)
	}
	return nil
}

func (m *mockEmployeeRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	if m.updateFieldsFn != nil {
		return m.updateFieldsFn(id, fields)
	}
	return nil
}

func (m *mockEmployeeRepo) SetPermissions(emp *models.Employee, permissions []models.Permission) error {
	if m.setPermissionsFn != nil {
		return m.setPermissionsFn(emp, permissions)
	}
	return nil
}

func (m *mockEmployeeRepo) ReassignFundsManagedBy(oldManagerID, newManagerID uint) (int64, error) {
	return 0, nil
}

func (m *mockEmployeeRepo) EmailExists(email string, excludeID uint) (bool, error) {
	if m.emailExistsFn != nil {
		return m.emailExistsFn(email, excludeID)
	}
	return false, nil
}

func (m *mockEmployeeRepo) UsernameExists(username string, excludeID uint) (bool, error) {
	if m.usernameExistsFn != nil {
		return m.usernameExistsFn(username, excludeID)
	}
	return false, nil
}

// ---- mock permission repository ----

type mockPermRepo struct {
	findAllBySubjectFn      func(subjectType string) ([]models.Permission, error)
	findByNamesForSubjectFn func(names []string, subjectType string) ([]models.Permission, error)
}

func (m *mockPermRepo) FindAllBySubject(subjectType string) ([]models.Permission, error) {
	if m.findAllBySubjectFn != nil {
		return m.findAllBySubjectFn(subjectType)
	}
	return nil, errors.New("not implemented")
}

func (m *mockPermRepo) FindByNamesForSubject(names []string, subjectType string) ([]models.Permission, error) {
	if m.findByNamesForSubjectFn != nil {
		return m.findByNamesForSubjectFn(names, subjectType)
	}
	return nil, errors.New("not implemented")
}

type mockActuaryProfileRepo struct {
	findByEmployeeIDFn        func(employeeID uint) (*models.ActuaryProfile, error)
	upsertFn                  func(profile *models.ActuaryProfile) error
	deleteByEmployeeIDFn      func(employeeID uint) error
	updateLimitFn             func(employeeID uint, limit *float64) error
	resetUsedLimitFn          func(employeeID uint) error
	setNeedApprovalFn         func(employeeID uint, needApproval bool) error
	resetAllAgentUsedLimitsFn func() (int64, error)
}

func (m *mockActuaryProfileRepo) FindByEmployeeID(employeeID uint) (*models.ActuaryProfile, error) {
	if m.findByEmployeeIDFn != nil {
		return m.findByEmployeeIDFn(employeeID)
	}
	return nil, nil
}

func (m *mockActuaryProfileRepo) Upsert(profile *models.ActuaryProfile) error {
	if m.upsertFn != nil {
		return m.upsertFn(profile)
	}
	return nil
}

func (m *mockActuaryProfileRepo) DeleteByEmployeeID(employeeID uint) error {
	if m.deleteByEmployeeIDFn != nil {
		return m.deleteByEmployeeIDFn(employeeID)
	}
	return nil
}

func (m *mockActuaryProfileRepo) UpdateLimit(employeeID uint, limit *float64) error {
	if m.updateLimitFn != nil {
		return m.updateLimitFn(employeeID, limit)
	}
	return nil
}

func (m *mockActuaryProfileRepo) ResetUsedLimit(employeeID uint) error {
	if m.resetUsedLimitFn != nil {
		return m.resetUsedLimitFn(employeeID)
	}
	return nil
}

func (m *mockActuaryProfileRepo) SetNeedApproval(employeeID uint, needApproval bool) error {
	if m.setNeedApprovalFn != nil {
		return m.setNeedApprovalFn(employeeID, needApproval)
	}
	return nil
}

func (m *mockActuaryProfileRepo) ResetAllAgentUsedLimits() (int64, error) {
	if m.resetAllAgentUsedLimitsFn != nil {
		return m.resetAllAgentUsedLimitsFn()
	}
	return 0, nil
}

// ---- mock token repository ----

type mockTokenRepo struct {
	createFn                   func(token *models.Token) error
	findValidFn                func(tokenStr, tokenType string) (*models.Token, error)
	invalidateEmployeeTokensFn func(employeeID uint, tokenType string) error
}

func (m *mockTokenRepo) Create(token *models.Token) error {
	if m.createFn != nil {
		return m.createFn(token)
	}
	return nil
}

func (m *mockTokenRepo) FindValid(tokenStr, tokenType string) (*models.Token, error) {
	if m.findValidFn != nil {
		return m.findValidFn(tokenStr, tokenType)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTokenRepo) InvalidateEmployeeTokens(employeeID uint, tokenType string) error {
	if m.invalidateEmployeeTokensFn != nil {
		return m.invalidateEmployeeTokensFn(employeeID, tokenType)
	}
	return nil
}

// ---- compile-time interface checks ----

var _ repository.EmployeeRepositoryInterface = (*mockEmployeeRepo)(nil)
var _ repository.PermissionRepositoryInterface = (*mockPermRepo)(nil)
var _ repository.TokenRepositoryInterface = (*mockTokenRepo)(nil)
var _ repository.ActuaryProfileRepositoryInterface = (*mockActuaryProfileRepo)(nil)

// ---- test helper ----

func newTestEmployeeService(empRepo repository.EmployeeRepositoryInterface, permRepo repository.PermissionRepositoryInterface, tokRepo repository.TokenRepositoryInterface) *service.EmployeeService {
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	// notifSvc is passed as nil; tests that reach notification calls are not in scope here
	return service.NewEmployeeServiceWithRepos(cfg, empRepo, &mockActuaryProfileRepo{}, permRepo, tokRepo, nil)
}

// validInput returns a CreateEmployeeInput with all required fields valid.
func validCreateInput() service.CreateEmployeeInput {
	return service.CreateEmployeeInput{
		Ime:           "Marko",
		Prezime:       "Markovic",
		DatumRodjenja: time.Date(1990, 1, 15, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         "marko@bank.com",
		BrojTelefona:  "0641234567",
		Adresa:        "Ulica 1",
		Username:      "mmarkovic",
		Pozicija:      "Analyst",
		Departman:     "IT",
	}
}

// ---- tests ----

func TestCreateEmployee_Success(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		emailExistsFn:    func(email string, excludeID uint) (bool, error) { return false, nil },
		usernameExistsFn: func(username string, excludeID uint) (bool, error) { return false, nil },
		createFn:         func(emp *models.Employee) error { emp.ID = 10; return nil },
	}
	tokRepo := &mockTokenRepo{
		createFn: func(token *models.Token) error { return nil },
	}

	// CreateEmployee calls notifSvc.SendActivationEmail; we construct a real NotificationService with an unreachable SMTP host — the dial error is discarded by the service with `_ =`.
	cfgWithNotif := &config.Config{FrontendURL: "http://localhost:5173", SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "noreply@bank.com"}
	notifSvc := service.NewNotificationService(cfgWithNotif)
	svc := service.NewEmployeeServiceWithRepos(cfgWithNotif, empRepo, &mockActuaryProfileRepo{}, &mockPermRepo{}, tokRepo, notifSvc)

	emp, err := svc.CreateEmployee(validCreateInput())
	if err != nil {
		t.Fatalf("CreateEmployee() unexpected error: %v", err)
	}
	if emp == nil {
		t.Fatal("CreateEmployee() returned nil employee")
	}
	if emp.ID != 10 {
		t.Errorf("CreateEmployee() emp.ID = %d, want 10", emp.ID)
	}
}

func TestCreateEmployee_DuplicateEmail(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return true, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	_, err := svc.CreateEmployee(validCreateInput())
	if err == nil {
		t.Fatal("CreateEmployee() expected error for duplicate email, got nil")
	}
	if !strings.Contains(err.Error(), "email already in use") {
		t.Errorf("CreateEmployee() error = %q, want contains %q", err.Error(), "email already in use")
	}
}

func TestCreateEmployee_InvalidBankEmail(t *testing.T) {
	svc := newTestEmployeeService(&mockEmployeeRepo{}, &mockPermRepo{}, &mockTokenRepo{})

	input := validCreateInput()
	input.Email = "marko@gmail.com" // not @bank.com

	_, err := svc.CreateEmployee(input)
	if err == nil {
		t.Fatal("CreateEmployee() expected error for non-bank email, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "bank.com") {
		t.Errorf("CreateEmployee() error = %q, expected a bank email validation error", err.Error())
	}
}

func TestGetEmployee_Found(t *testing.T) {
	emp := &models.Employee{ID: 5, Email: "emp@bank.com", Aktivan: true}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	got, err := svc.GetEmployee(5)
	if err != nil {
		t.Fatalf("GetEmployee() unexpected error: %v", err)
	}
	if got == nil || got.ID != 5 {
		t.Errorf("GetEmployee() returned wrong employee")
	}
}

func TestGetEmployee_NotFound(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) {
			return nil, errors.New("record not found")
		},
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	_, err := svc.GetEmployee(999)
	if err == nil {
		t.Fatal("GetEmployee() expected error for missing employee, got nil")
	}
}

func TestUpdateEmployee_CannotEditAdmin(t *testing.T) {
	adminEmp := &models.Employee{
		ID:    1,
		Email: "admin@bank.com",
		Permissions: []models.Permission{
			{Name: models.PermEmployeeAdmin},
		},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return adminEmp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	input := service.UpdateEmployeeInput{
		Ime:           "Admin",
		Prezime:       "User",
		DatumRodjenja: time.Date(1985, 5, 10, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         "admin@bank.com",
		BrojTelefona:  "0641234567",
		Adresa:        "Ulica 1",
		Username:      "adminuser",
		Pozicija:      "Admin",
		Departman:     "Management",
		Aktivan:       true,
	}

	_, err := svc.UpdateEmployee(1, input)
	if err == nil {
		t.Fatal("UpdateEmployee() expected error for admin employee, got nil")
	}
	if !strings.Contains(err.Error(), "cannot edit an admin employee") {
		t.Errorf("UpdateEmployee() error = %q, want contains %q", err.Error(), "cannot edit an admin employee")
	}
}

func TestSetEmployeeActive_DeactivateAdmin(t *testing.T) {
	adminEmp := &models.Employee{
		ID:    1,
		Email: "admin@bank.com",
		Permissions: []models.Permission{
			{Name: models.PermEmployeeAdmin},
		},
		Aktivan: true,
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return adminEmp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.SetEmployeeActive(1, false)
	if err == nil {
		t.Fatal("SetEmployeeActive() expected error when deactivating admin, got nil")
	}
	if !strings.Contains(err.Error(), "cannot deactivate an admin employee") {
		t.Errorf("SetEmployeeActive() error = %q, want contains %q", err.Error(), "cannot deactivate an admin employee")
	}
}

func TestUpdateEmployeePermissions_WrongSubjectType(t *testing.T) {
	emp := &models.Employee{ID: 3, Email: "emp@bank.com", Permissions: []models.Permission{}}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	// FindByNamesForSubject returns fewer perms than requested — simulating wrong subject type
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, subjectType string) ([]models.Permission, error) {
			// Return only 1 perm even though 2 were requested
			return []models.Permission{{Name: "employee.read"}}, nil
		},
	}
	svc := newTestEmployeeService(empRepo, permRepo, &mockTokenRepo{})

	_, err := svc.UpdateEmployeePermissions(3, []string{"employee.read", "client.basic"})
	if err == nil {
		t.Fatal("UpdateEmployeePermissions() expected error for wrong subject type, got nil")
	}
	if !strings.Contains(err.Error(), "employee permissions") {
		t.Errorf("UpdateEmployeePermissions() error = %q, want contains %q", err.Error(), "employee permissions")
	}
}

func TestUpdateEmployeePermissions_AgentCreatesActuaryProfile(t *testing.T) {
	emp := &models.Employee{
		ID:    3,
		Email: "agent@bank.com",
		Permissions: []models.Permission{
			{Name: models.PermEmployeeBasic},
		},
	}
	updated := &models.Employee{
		ID:    3,
		Email: "agent@bank.com",
		Permissions: []models.Permission{
			{Name: models.PermEmployeeAgent},
		},
	}

	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) {
			if len(emp.Permissions) == 1 && emp.Permissions[0].Name == models.PermEmployeeBasic {
				emp.Permissions = updated.Permissions
				return emp, nil
			}
			return updated, nil
		},
		setPermissionsFn: func(emp *models.Employee, permissions []models.Permission) error {
			emp.Permissions = permissions
			return nil
		},
	}
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, subjectType string) ([]models.Permission, error) {
			return []models.Permission{{Name: models.PermEmployeeAgent}}, nil
		},
	}

	var saved *models.ActuaryProfile
	actuaryRepo := &mockActuaryProfileRepo{
		findByEmployeeIDFn: func(employeeID uint) (*models.ActuaryProfile, error) { return nil, nil },
		upsertFn: func(profile *models.ActuaryProfile) error {
			saved = profile.Clone()
			return nil
		},
	}

	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, permRepo, &mockTokenRepo{}, nil)

	_, err := svc.UpdateEmployeePermissions(3, []string{models.PermEmployeeAgent})
	if err != nil {
		t.Fatalf("UpdateEmployeePermissions() unexpected error: %v", err)
	}
	if saved == nil {
		t.Fatal("expected actuary profile to be created")
	}
	if saved.Limit == nil || *saved.Limit != 0 {
		t.Fatalf("expected default agent limit 0, got %#v", saved.Limit)
	}
	if !saved.NeedApproval {
		t.Fatal("expected agent profile to require approval by default")
	}
}

func TestGetActuaryState_SupervisorHasNoLimit(t *testing.T) {
	emp := &models.Employee{
		ID:    5,
		Email: "supervisor@bank.com",
		Permissions: []models.Permission{
			{Name: models.PermEmployeeSupervisor},
		},
	}
	actuaryRepo := &mockActuaryProfileRepo{
		findByEmployeeIDFn: func(employeeID uint) (*models.ActuaryProfile, error) {
			return &models.ActuaryProfile{
				EmployeeID:   employeeID,
				UsedLimit:    2500,
				NeedApproval: false,
				Limit:        nil,
			}, nil
		},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}

	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	state, err := svc.GetActuaryState(5)
	if err != nil {
		t.Fatalf("GetActuaryState() unexpected error: %v", err)
	}
	if !state.IsActuary || !state.IsSupervisor {
		t.Fatalf("expected supervisor actuarial state, got %+v", state)
	}
	if state.Limit != nil {
		t.Fatalf("expected supervisor limit to be nil, got %#v", state.Limit)
	}
}

func TestListActuaryStates_ReturnsAgentsAndSupervisors(t *testing.T) {
	employees := []models.Employee{
		{
			ID:       1,
			Ime:      "Ana",
			Prezime:  "Agent",
			Email:    "ana@bank.com",
			Username: "aagent",
			Pozicija: "Agent",
			Permissions: []models.Permission{
				{Name: models.PermEmployeeAgent},
			},
			Aktivan: true,
		},
		{
			ID:       2,
			Ime:      "Sara",
			Prezime:  "Supervisor",
			Email:    "sara@bank.com",
			Username: "ssupervisor",
			Pozicija: "Supervisor",
			Permissions: []models.Permission{
				{Name: models.PermEmployeeSupervisor},
			},
			Aktivan: true,
		},
		{
			ID:       3,
			Ime:      "Boris",
			Prezime:  "Basic",
			Email:    "boris@bank.com",
			Username: "bbasic",
			Pozicija: "Clerk",
			Permissions: []models.Permission{
				{Name: models.PermEmployeeBasic},
			},
			Aktivan: true,
		},
	}

	empRepo := &mockEmployeeRepo{
		listAllFn: func() ([]models.Employee, error) { return employees, nil },
	}
	actuaryRepo := &mockActuaryProfileRepo{
		findByEmployeeIDFn: func(employeeID uint) (*models.ActuaryProfile, error) {
			switch employeeID {
			case 1:
				limit := 150000.0
				return &models.ActuaryProfile{
					EmployeeID:   employeeID,
					Limit:        &limit,
					UsedLimit:    35000,
					NeedApproval: true,
				}, nil
			case 2:
				return &models.ActuaryProfile{
					EmployeeID:   employeeID,
					Limit:        nil,
					UsedLimit:    0,
					NeedApproval: false,
				}, nil
			default:
				return nil, nil
			}
		},
	}

	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	items, err := svc.ListActuaryStates()
	if err != nil {
		t.Fatalf("ListActuaryStates() unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 actuaries, got %d", len(items))
	}
	if !items[0].IsSupervisor {
		t.Fatalf("expected supervisor to be listed first, got %+v", items[0])
	}
	if items[1].Limit == nil || *items[1].Limit != 150000 {
		t.Fatalf("expected agent limit to be preserved, got %+v", items[1])
	}
}

// ---- UpdateAgentLimit ----

func TestUpdateAgentLimit_Success(t *testing.T) {
	emp := &models.Employee{
		ID:    7,
		Email: "agent@bank.com",
		Permissions: []models.Permission{
			{Name: models.PermEmployeeAgent},
		},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	called := false
	actuaryRepo := &mockActuaryProfileRepo{
		updateLimitFn: func(employeeID uint, limit *float64) error {
			called = true
			if employeeID != 7 {
				t.Errorf("UpdateLimit got employeeID=%d, want 7", employeeID)
			}
			return nil
		},
	}
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	limit := 5000.0
	if err := svc.UpdateAgentLimit(7, &limit); err != nil {
		t.Fatalf("UpdateAgentLimit() unexpected error: %v", err)
	}
	if !called {
		t.Fatal("UpdateLimit was not called on the actuary repo")
	}
}

func TestUpdateAgentLimit_NotFound(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) {
			return nil, errors.New("record not found")
		},
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.UpdateAgentLimit(99, nil)
	if err == nil || !strings.Contains(err.Error(), "employee not found") {
		t.Fatalf("UpdateAgentLimit() error = %v, want contains 'employee not found'", err)
	}
}

func TestUpdateAgentLimit_NotActuary(t *testing.T) {
	emp := &models.Employee{
		ID:          1,
		Email:       "basic@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.UpdateAgentLimit(1, nil)
	if err == nil || !strings.Contains(err.Error(), "not an actuary") {
		t.Fatalf("UpdateAgentLimit() error = %v, want contains 'not an actuary'", err)
	}
}

func TestUpdateAgentLimit_SupervisorRejected(t *testing.T) {
	emp := &models.Employee{
		ID:          2,
		Email:       "sup@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeSupervisor}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.UpdateAgentLimit(2, nil)
	if err == nil || !strings.Contains(err.Error(), "supervisors do not have limits") {
		t.Fatalf("UpdateAgentLimit() error = %v, want contains 'supervisors do not have limits'", err)
	}
}

// ---- ResetAgentUsedLimit ----

func TestResetAgentUsedLimit_Success(t *testing.T) {
	emp := &models.Employee{
		ID:          3,
		Email:       "agent@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeAgent}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	called := false
	actuaryRepo := &mockActuaryProfileRepo{
		resetUsedLimitFn: func(employeeID uint) error {
			called = true
			return nil
		},
	}
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	if err := svc.ResetAgentUsedLimit(3); err != nil {
		t.Fatalf("ResetAgentUsedLimit() unexpected error: %v", err)
	}
	if !called {
		t.Fatal("ResetUsedLimit was not called on the actuary repo")
	}
}

func TestResetAgentUsedLimit_NotActuary(t *testing.T) {
	emp := &models.Employee{
		ID:          4,
		Email:       "basic@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.ResetAgentUsedLimit(4)
	if err == nil || !strings.Contains(err.Error(), "not an actuary") {
		t.Fatalf("ResetAgentUsedLimit() error = %v, want contains 'not an actuary'", err)
	}
}

func TestResetAgentUsedLimit_NotFound(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) {
			return nil, errors.New("record not found")
		},
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.ResetAgentUsedLimit(404)
	if err == nil || !strings.Contains(err.Error(), "employee not found") {
		t.Fatalf("ResetAgentUsedLimit() error = %v, want contains 'employee not found'", err)
	}
}

// ---- SetNeedApproval ----

func TestSetNeedApproval_Success(t *testing.T) {
	emp := &models.Employee{
		ID:          5,
		Email:       "agent@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeAgent}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	var gotNeed bool
	actuaryRepo := &mockActuaryProfileRepo{
		setNeedApprovalFn: func(employeeID uint, needApproval bool) error {
			gotNeed = needApproval
			return nil
		},
	}
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	if err := svc.SetNeedApproval(5, true); err != nil {
		t.Fatalf("SetNeedApproval() unexpected error: %v", err)
	}
	if !gotNeed {
		t.Fatal("expected SetNeedApproval to forward needApproval=true")
	}
}

func TestSetNeedApproval_SupervisorRejected(t *testing.T) {
	emp := &models.Employee{
		ID:          6,
		Email:       "sup@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeSupervisor}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.SetNeedApproval(6, true)
	if err == nil || !strings.Contains(err.Error(), "supervisors always have need_approval=false") {
		t.Fatalf("SetNeedApproval() error = %v, want supervisor rejection", err)
	}
}

func TestSetNeedApproval_NotActuary(t *testing.T) {
	emp := &models.Employee{
		ID:          7,
		Email:       "basic@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.SetNeedApproval(7, false)
	if err == nil || !strings.Contains(err.Error(), "not an actuary") {
		t.Fatalf("SetNeedApproval() error = %v, want contains 'not an actuary'", err)
	}
}

func TestSetNeedApproval_NotFound(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) {
			return nil, errors.New("record not found")
		},
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	err := svc.SetNeedApproval(404, true)
	if err == nil || !strings.Contains(err.Error(), "employee not found") {
		t.Fatalf("SetNeedApproval() error = %v, want contains 'employee not found'", err)
	}
}

// ---- ResetAllAgentUsedLimits ----

func TestResetAllAgentUsedLimits_DelegatesToRepo(t *testing.T) {
	called := false
	actuaryRepo := &mockActuaryProfileRepo{
		resetAllAgentUsedLimitsFn: func() (int64, error) {
			called = true
			return 12, nil
		},
	}
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, &mockEmployeeRepo{}, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	n, err := svc.ResetAllAgentUsedLimits()
	if err != nil {
		t.Fatalf("ResetAllAgentUsedLimits() unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected ResetAllAgentUsedLimits to delegate to the repo")
	}
	if n != 12 {
		t.Fatalf("ResetAllAgentUsedLimits() n = %d, want 12", n)
	}
}

// ---- ListEmployees / GetAllPermissions ----

func TestListEmployees_DelegatesToRepo(t *testing.T) {
	wantFilter := repository.EmployeeFilter{Email: "x@bank.com", Page: 2, PageSize: 5}
	var gotFilter repository.EmployeeFilter
	empRepo := &mockEmployeeRepo{
		listFn: func(filter repository.EmployeeFilter) ([]models.Employee, int64, error) {
			gotFilter = filter
			return []models.Employee{{ID: 1}, {ID: 2}}, 42, nil
		},
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	items, total, err := svc.ListEmployees(wantFilter)
	if err != nil {
		t.Fatalf("ListEmployees() unexpected error: %v", err)
	}
	if total != 42 || len(items) != 2 {
		t.Fatalf("ListEmployees() = (len=%d, total=%d), want (2, 42)", len(items), total)
	}
	if gotFilter != wantFilter {
		t.Fatalf("ListEmployees() filter = %+v, want %+v", gotFilter, wantFilter)
	}
}

func TestGetAllPermissions_FiltersToEmployeeSubject(t *testing.T) {
	var gotSubject string
	permRepo := &mockPermRepo{
		findAllBySubjectFn: func(subjectType string) ([]models.Permission, error) {
			gotSubject = subjectType
			return []models.Permission{{Name: models.PermEmployeeBasic}}, nil
		},
	}
	svc := newTestEmployeeService(&mockEmployeeRepo{}, permRepo, &mockTokenRepo{})

	perms, err := svc.GetAllPermissions()
	if err != nil {
		t.Fatalf("GetAllPermissions() unexpected error: %v", err)
	}
	if len(perms) != 1 {
		t.Fatalf("GetAllPermissions() len = %d, want 1", len(perms))
	}
	if gotSubject != models.PermissionSubjectEmployee {
		t.Fatalf("GetAllPermissions() subject = %q, want %q", gotSubject, models.PermissionSubjectEmployee)
	}
}

// ---- UpdateEmployee success path ----

func TestUpdateEmployee_Success(t *testing.T) {
	emp := &models.Employee{
		ID:           5,
		Email:        "old@bank.com",
		Username:     "olduser",
		Permissions:  []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	var updated *models.Employee
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
		emailExistsFn:    func(email string, excludeID uint) (bool, error) { return false, nil },
		usernameExistsFn: func(username string, excludeID uint) (bool, error) { return false, nil },
		updateFn: func(e *models.Employee) error { updated = e; return nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	input := service.UpdateEmployeeInput{
		Ime:           "Pera",
		Prezime:       "Peric",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         "new@bank.com",
		BrojTelefona:  "0641234567",
		Adresa:        "Adresa 1",
		Username:      "newuser",
		Pozicija:      "Clerk",
		Departman:     "IT",
		Aktivan:       true,
	}

	got, err := svc.UpdateEmployee(5, input)
	if err != nil {
		t.Fatalf("UpdateEmployee() unexpected error: %v", err)
	}
	if got == nil || got.Email != "new@bank.com" || got.Username != "newuser" {
		t.Fatalf("UpdateEmployee() returned unexpected employee: %+v", got)
	}
	if updated == nil {
		t.Fatal("expected Update to be called on the repo")
	}
}

func TestUpdateEmployee_DuplicateEmail(t *testing.T) {
	emp := &models.Employee{
		ID:          5,
		Email:       "old@bank.com",
		Username:    "olduser",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn:    func(id uint) (*models.Employee, error) { return emp, nil },
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return true, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	input := service.UpdateEmployeeInput{
		Ime: "X", Prezime: "Y",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         "taken@bank.com",
		BrojTelefona:  "0641234567",
		Adresa:        "A",
		Username:      "olduser",
		Pozicija:      "Clerk",
		Departman:     "IT",
		Aktivan:       true,
	}

	_, err := svc.UpdateEmployee(5, input)
	if err == nil || !strings.Contains(err.Error(), "email already in use") {
		t.Fatalf("UpdateEmployee() error = %v, want 'email already in use'", err)
	}
}

// ---- NotificationService extra paths ----

func TestNotificationService_ExtraEmails(t *testing.T) {
	cfg := &config.Config{FrontendURL: "http://localhost:5173", SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "noreply@bank.com"}
	notif := service.NewNotificationService(cfg)

	// All three end up calling sendEmail which will fail to dial — that's fine,
	// we just want to exercise the body-building branches.
	if err := notif.SendResetPasswordEmail("user@bank.com", "User", "tok"); err == nil {
		t.Fatal("SendResetPasswordEmail() expected dial error, got nil")
	}
	if err := notif.SendConfirmationEmail("user@bank.com", "User"); err == nil {
		t.Fatal("SendConfirmationEmail() expected dial error, got nil")
	}
}
