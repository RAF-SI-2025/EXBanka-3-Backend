package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
	"gorm.io/gorm"
)

type EmployeeService struct {
	cfg          *config.Config
	employeeRepo *repository.EmployeeRepository
	permRepo     *repository.PermissionRepository
	tokenRepo    *repository.TokenRepository
	notifSvc     *NotificationService
}

func NewEmployeeService(cfg *config.Config, db *gorm.DB, notifSvc *NotificationService) *EmployeeService {
	return &EmployeeService{
		cfg:          cfg,
		employeeRepo: repository.NewEmployeeRepository(db),
		permRepo:     repository.NewPermissionRepository(db),
		tokenRepo:    repository.NewTokenRepository(db),
		notifSvc:     notifSvc,
	}
}

type CreateEmployeeInput struct {
	Ime           string
	Prezime       string
	DatumRodjenja time.Time
	Pol           string
	Email         string
	BrojTelefona  string
	Adresa        string
	Username      string
	Pozicija      string
	Departman     string
}

type UpdateEmployeeInput struct {
	Ime           string
	Prezime       string
	DatumRodjenja time.Time
	Pol           string
	Email         string
	BrojTelefona  string
	Adresa        string
	Username      string
	Pozicija      string
	Departman     string
	Aktivan       bool
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *EmployeeService) CreateEmployee(input CreateEmployeeInput) (*models.Employee, error) {
	if err := util.ValidatePhoneNumber(input.BrojTelefona); err != nil {
		return nil, err
	}
	if err := util.ValidateBankEmail(input.Email); err != nil {
		return nil, err
	}
	if err := util.ValidateDateOfBirth(input.DatumRodjenja); err != nil {
		return nil, err
	}

	emailExists, err := s.employeeRepo.EmailExists(input.Email, 0)
	if err != nil {
		return nil, err
	}
	if emailExists {
		return nil, fmt.Errorf("email already in use")
	}

	usernameExists, err := s.employeeRepo.UsernameExists(input.Username, 0)
	if err != nil {
		return nil, err
	}
	if usernameExists {
		return nil, fmt.Errorf("username already in use")
	}

	emp := &models.Employee{
		Ime:           input.Ime,
		Prezime:       input.Prezime,
		DatumRodjenja: input.DatumRodjenja,
		Pol:           input.Pol,
		Email:         input.Email,
		BrojTelefona:  input.BrojTelefona,
		Adresa:        input.Adresa,
		Username:      input.Username,
		Pozicija:      input.Pozicija,
		Departman:     input.Departman,
		Aktivan:       false,
		Password:      "pending",
		SaltPassword:  "pending",
	}

	if err := s.employeeRepo.Create(emp); err != nil {
		return nil, err
	}

	tokenStr, err := generateToken()
	if err != nil {
		return nil, err
	}

	token := &models.Token{
		EmployeeID: emp.ID,
		Token:      tokenStr,
		Type:       models.TokenTypeActivation,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	if err := s.tokenRepo.Create(token); err != nil {
		return nil, err
	}

	_ = s.notifSvc.SendActivationEmail(emp.Email, emp.Ime+" "+emp.Prezime, tokenStr)
	return emp, nil
}

func (s *EmployeeService) GetEmployee(id uint) (*models.Employee, error) {
	return s.employeeRepo.FindByID(id)
}

func (s *EmployeeService) ListEmployees(filter repository.EmployeeFilter) ([]models.Employee, int64, error) {
	return s.employeeRepo.List(filter)
}

func (s *EmployeeService) UpdateEmployee(id uint, input UpdateEmployeeInput) (*models.Employee, error) {
	emp, err := s.employeeRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("employee not found")
	}

	if emp.IsAdmin() {
		return nil, fmt.Errorf("cannot edit an admin employee")
	}

	if err := util.ValidatePhoneNumber(input.BrojTelefona); err != nil {
		return nil, err
	}
	if err := util.ValidateBankEmail(input.Email); err != nil {
		return nil, err
	}
	if err := util.ValidateDateOfBirth(input.DatumRodjenja); err != nil {
		return nil, err
	}

	if input.Email != emp.Email {
		exists, err := s.employeeRepo.EmailExists(input.Email, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("email already in use")
		}
	}

	if input.Username != emp.Username {
		exists, err := s.employeeRepo.UsernameExists(input.Username, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("username already in use")
		}
	}

	emp.Ime = input.Ime
	emp.Prezime = input.Prezime
	emp.DatumRodjenja = input.DatumRodjenja
	emp.Pol = input.Pol
	emp.Email = input.Email
	emp.BrojTelefona = input.BrojTelefona
	emp.Adresa = input.Adresa
	emp.Username = input.Username
	emp.Pozicija = input.Pozicija
	emp.Departman = input.Departman
	emp.Aktivan = input.Aktivan

	if err := s.employeeRepo.Update(emp); err != nil {
		return nil, err
	}

	return emp, nil
}

func (s *EmployeeService) SetEmployeeActive(id uint, aktivan bool) error {
	emp, err := s.employeeRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("employee not found")
	}
	if !aktivan && emp.IsAdmin() {
		return fmt.Errorf("cannot deactivate an admin employee")
	}

	return s.employeeRepo.UpdateFields(id, map[string]interface{}{"aktivan": aktivan})
}

func (s *EmployeeService) UpdateEmployeePermissions(id uint, permissionNames []string) (*models.Employee, error) {
	emp, err := s.employeeRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("employee not found")
	}

	perms, err := s.permRepo.FindByNamesForSubject(permissionNames, models.PermissionSubjectEmployee)
	if err != nil {
		return nil, err
	}
	if len(perms) != len(permissionNames) {
		return nil, fmt.Errorf("employees can only be assigned employee permissions")
	}

	if err := s.employeeRepo.SetPermissions(emp, perms); err != nil {
		return nil, err
	}

	return s.employeeRepo.FindByID(id)
}

func (s *EmployeeService) GetAllPermissions() ([]models.Permission, error) {
	return s.permRepo.FindAllBySubject(models.PermissionSubjectEmployee)
}
