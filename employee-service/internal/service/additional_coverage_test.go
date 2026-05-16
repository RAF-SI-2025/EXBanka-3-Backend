package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestEmployeeDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestNewEmployeeService_Constructs(t *testing.T) {
	db := newTestEmployeeDB(t, "empsvc_ctor")
	cfg := &config.Config{}
	if svc := service.NewEmployeeService(cfg, db, nil); svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestStartCronJobs_StartsAndStopsCleanly(t *testing.T) {
	db := newTestEmployeeDB(t, "empsvc_cron")
	cfg := &config.Config{}
	svc := service.NewEmployeeService(cfg, db, nil)

	c := service.StartCronJobs(svc)
	if c == nil {
		t.Fatal("expected non-nil cron")
	}
	c.Stop()
}

func TestGetActuaryState_NonActuary_ReturnsState(t *testing.T) {
	emp := &models.Employee{ID: 100, Email: "x@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}}}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	state, err := svc.GetActuaryState(100)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if state.IsActuary {
		t.Fatal("expected IsActuary=false for non-actuary")
	}
}

func TestGetActuaryState_AgentNoProfileTriggersSync(t *testing.T) {
	emp := &models.Employee{ID: 101, Email: "agent@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeAgent}},
		Limit:       12345,
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	upserted := 0
	actuaryRepo := &mockActuaryProfileRepo{
		findByEmployeeIDFn: func(employeeID uint) (*models.ActuaryProfile, error) {
			return nil, nil
		},
		upsertFn: func(p *models.ActuaryProfile) error {
			upserted++
			return nil
		},
	}
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	state, err := svc.GetActuaryState(101)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !state.IsActuary {
		t.Fatal("expected IsActuary=true")
	}
	if upserted == 0 {
		t.Fatal("expected upsert called")
	}
}

func TestGetActuaryState_FindErr_ReturnsErr(t *testing.T) {
	emp := &models.Employee{ID: 1, Permissions: []models.Permission{{Name: models.PermEmployeeAgent}}}
	empRepo := &mockEmployeeRepo{findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil }}
	actuaryRepo := &mockActuaryProfileRepo{
		findByEmployeeIDFn: func(uint) (*models.ActuaryProfile, error) { return nil, errors.New("db") },
	}
	cfg := &config.Config{}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepo, actuaryRepo, &mockPermRepo{}, &mockTokenRepo{}, nil)

	if _, err := svc.GetActuaryState(1); err == nil {
		t.Fatal("expected error")
	}
}

func TestListActuaryStates_PropagatesListAllError(t *testing.T) {
	empRepo := &mockEmployeeRepo{listAllFn: func() ([]models.Employee, error) { return nil, errors.New("x") }}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})
	if _, err := svc.ListActuaryStates(); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetEmployeeActive_NonAdminSuccess(t *testing.T) {
	emp := &models.Employee{ID: 1, Permissions: []models.Permission{{Name: models.PermEmployeeBasic}}, Aktivan: true}
	var updateCalled bool
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
		updateFieldsFn: func(id uint, fields map[string]interface{}) error {
			updateCalled = true
			return nil
		},
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})
	if err := svc.SetEmployeeActive(1, false); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !updateCalled {
		t.Fatal("expected updateFields called")
	}
}

func TestSetEmployeeActive_NotFound(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return nil, errors.New("nope") },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})
	if err := svc.SetEmployeeActive(1, false); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateEmployee_Success_BasicEmployee(t *testing.T) {
	emp := &models.Employee{
		ID: 5, Email: "old@bank.com", Username: "old",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
		emailExistsFn: func(email string, excludeID uint) (bool, error) {
			return false, nil
		},
		usernameExistsFn: func(username string, excludeID uint) (bool, error) {
			return false, nil
		},
		updateFn: func(e *models.Employee) error { return nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	_, err := svc.UpdateEmployee(5, service.UpdateEmployeeInput{
		Ime: "I", Prezime: "P", DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Pol: "M", Email: "new@bank.com", BrojTelefona: "0641234567", Adresa: "A", Username: "new",
		Pozicija: "P", Departman: "D", Aktivan: true,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestUpdateEmployee_EmailDuplicate(t *testing.T) {
	emp := &models.Employee{
		ID: 5, Email: "old@bank.com", Username: "old",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn:    func(id uint) (*models.Employee, error) { return emp, nil },
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return true, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	_, err := svc.UpdateEmployee(5, service.UpdateEmployeeInput{
		Email: "dup@bank.com", BrojTelefona: "0641234567",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Pol: "M",
		Username: "old",
	})
	if err == nil {
		t.Fatal("expected duplicate email error")
	}
}

func TestUpdateEmployee_UsernameDuplicate(t *testing.T) {
	emp := &models.Employee{
		ID: 5, Email: "old@bank.com", Username: "old",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn:       func(id uint) (*models.Employee, error) { return emp, nil },
		emailExistsFn:    func(string, uint) (bool, error) { return false, nil },
		usernameExistsFn: func(string, uint) (bool, error) { return true, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	_, err := svc.UpdateEmployee(5, service.UpdateEmployeeInput{
		Email: "old@bank.com", Username: "newdup", BrojTelefona: "0641234567",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Pol: "M",
	})
	if err == nil {
		t.Fatal("expected duplicate username error")
	}
}

func TestUpdateEmployee_InvalidPhone(t *testing.T) {
	emp := &models.Employee{
		ID: 5, Email: "old@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})

	_, err := svc.UpdateEmployee(5, service.UpdateEmployeeInput{Email: "x@bank.com", BrojTelefona: "abc"})
	if err == nil {
		t.Fatal("expected phone validation error")
	}
}

func TestUpdateEmployeePermissionsBy_SupervisorDemoted_ReassignsFunds(t *testing.T) {
	emp := &models.Employee{
		ID: 9, Email: "sup@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeSupervisor}},
	}
	updated := &models.Employee{
		ID: 9, Email: "sup@bank.com",
		Permissions: []models.Permission{{Name: models.PermEmployeeBasic}},
	}
	calls := 0
	reassigned := uint(0)
	empRepo := &mockEmployeeRepo{
		findByIDFn: func(id uint) (*models.Employee, error) {
			calls++
			if calls == 1 {
				return emp, nil
			}
			return updated, nil
		},
		setPermissionsFn: func(e *models.Employee, perms []models.Permission) error {
			e.Permissions = perms
			return nil
		},
	}
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, sub string) ([]models.Permission, error) {
			return []models.Permission{{Name: models.PermEmployeeBasic}}, nil
		},
	}
	cfg := &config.Config{FrontendURL: "http://localhost:5173"}
	svc := service.NewEmployeeServiceWithRepos(cfg, empRepoWithReassign(empRepo, &reassigned), &mockActuaryProfileRepo{}, permRepo, &mockTokenRepo{}, nil)

	_, err := svc.UpdateEmployeePermissionsBy(9, []string{models.PermEmployeeBasic}, 42)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if reassigned != 42 {
		t.Fatalf("expected reassign to 42, got %d", reassigned)
	}
}

type employeeRepoWithReassign struct {
	*mockEmployeeRepo
	reassignTarget *uint
}

func (e *employeeRepoWithReassign) ReassignFundsManagedBy(oldID, newID uint) (int64, error) {
	*e.reassignTarget = newID
	return 1, nil
}

func empRepoWithReassign(base *mockEmployeeRepo, target *uint) *employeeRepoWithReassign {
	return &employeeRepoWithReassign{mockEmployeeRepo: base, reassignTarget: target}
}

func TestUpdateEmployeePermissionsBy_PermFindError(t *testing.T) {
	emp := &models.Employee{ID: 1, Permissions: []models.Permission{}}
	empRepo := &mockEmployeeRepo{findByIDFn: func(uint) (*models.Employee, error) { return emp, nil }}
	permRepo := &mockPermRepo{findByNamesForSubjectFn: func([]string, string) ([]models.Permission, error) {
		return nil, errors.New("db")
	}}
	svc := newTestEmployeeService(empRepo, permRepo, &mockTokenRepo{})
	if _, err := svc.UpdateEmployeePermissions(1, []string{"x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateEmployee_InvalidPhone(t *testing.T) {
	svc := newTestEmployeeService(&mockEmployeeRepo{}, &mockPermRepo{}, &mockTokenRepo{})
	input := validCreateInput()
	input.BrojTelefona = "abc"
	if _, err := svc.CreateEmployee(input); err == nil {
		t.Fatal("expected phone validation error")
	}
}

func TestCreateEmployee_DuplicateUsername(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		emailExistsFn:    func(string, uint) (bool, error) { return false, nil },
		usernameExistsFn: func(string, uint) (bool, error) { return true, nil },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})
	if _, err := svc.CreateEmployee(validCreateInput()); err == nil {
		t.Fatal("expected duplicate username error")
	}
}

func TestCreateEmployee_RepoCreateErr(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		emailExistsFn:    func(string, uint) (bool, error) { return false, nil },
		usernameExistsFn: func(string, uint) (bool, error) { return false, nil },
		createFn:         func(*models.Employee) error { return errors.New("db") },
	}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, &mockTokenRepo{})
	if _, err := svc.CreateEmployee(validCreateInput()); err == nil {
		t.Fatal("expected create error")
	}
}

func TestCreateEmployee_TokenCreateErr(t *testing.T) {
	empRepo := &mockEmployeeRepo{
		emailExistsFn:    func(string, uint) (bool, error) { return false, nil },
		usernameExistsFn: func(string, uint) (bool, error) { return false, nil },
		createFn:         func(emp *models.Employee) error { emp.ID = 1; return nil },
	}
	tokRepo := &mockTokenRepo{createFn: func(*models.Token) error { return errors.New("tok") }}
	svc := newTestEmployeeService(empRepo, &mockPermRepo{}, tokRepo)
	if _, err := svc.CreateEmployee(validCreateInput()); err == nil {
		t.Fatal("expected token create error")
	}
}
