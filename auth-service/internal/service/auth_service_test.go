package service_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
	"gorm.io/gorm"
)

// ---- mock implementations ----

type mockEmployeeRepo struct {
	findByEmailFn  func(email string) (*models.Employee, error)
	findByIDFn     func(id uint) (*models.Employee, error)
	updateFieldsFn func(id uint, fields map[string]interface{}) error
}

func (m *mockEmployeeRepo) FindByEmail(email string) (*models.Employee, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(email)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEmployeeRepo) FindByID(id uint) (*models.Employee, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEmployeeRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	if m.updateFieldsFn != nil {
		return m.updateFieldsFn(id, fields)
	}
	return nil
}

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

// ---- mock client repository ----

type mockClientRepo struct {
	findByEmailFn  func(email string) (*models.Client, error)
	findByIDFn     func(id uint) (*models.Client, error)
	updateFieldsFn func(id uint, fields map[string]interface{}) error
}

func (m *mockClientRepo) FindByEmail(email string) (*models.Client, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(email)
	}
	return nil, errors.New("not implemented")
}

func (m *mockClientRepo) FindByID(id uint) (*models.Client, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockClientRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	if m.updateFieldsFn != nil {
		return m.updateFieldsFn(id, fields)
	}
	return nil
}

// ---- compile-time interface checks ----

var _ repository.EmployeeRepositoryInterface = (*mockEmployeeRepo)(nil)
var _ repository.TokenRepositoryInterface = (*mockTokenRepo)(nil)
var _ repository.ClientRepositoryInterface = (*mockClientRepo)(nil)

// ---- test helper ----

// newTestAuthService creates an AuthService for unit testing.
// notifSvc is nil — safe because unit tests only exercise paths that return
// before the notification call (password mismatch, policy failure, etc.).
func newTestAuthService(empRepo repository.EmployeeRepositoryInterface, tokRepo repository.TokenRepositoryInterface) *service.AuthService {
	cfg := &config.Config{
		JWTSecret:          "test-secret",
		JWTAccessDuration:  15,
		JWTRefreshDuration: 24 * 60,
	}
	return service.NewAuthServiceWithRepos(cfg, empRepo, &mockClientRepo{}, tokRepo, nil)
}

// ---- tests ----

func TestLogin_Success(t *testing.T) {
	salt, err := util.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}
	hash, err := util.HashPassword("TestPass12", salt)
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	emp := &models.Employee{
		ID:           1,
		Email:        "emp@bank.com",
		Username:     "emp01",
		Password:     hash,
		SaltPassword: salt,
		Aktivan:      true,
		Permissions:  []models.Permission{},
	}

	svc := newTestAuthService(
		&mockEmployeeRepo{findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil }},
		&mockTokenRepo{},
	)

	access, refresh, gotEmp, err := svc.Login("emp@bank.com", "TestPass12")
	if err != nil {
		t.Fatalf("Login() unexpected error: %v", err)
	}
	if access == "" {
		t.Error("Login() returned empty access token")
	}
	if refresh == "" {
		t.Error("Login() returned empty refresh token")
	}
	if gotEmp == nil || gotEmp.ID != 1 {
		t.Error("Login() returned wrong employee")
	}
}

func TestLogin_InvalidEmail(t *testing.T) {
	svc := newTestAuthService(
		&mockEmployeeRepo{findByEmailFn: func(email string) (*models.Employee, error) {
			return nil, gorm.ErrRecordNotFound
		}},
		&mockTokenRepo{},
	)

	_, _, _, err := svc.Login("noone@bank.com", "TestPass12")
	if err == nil {
		t.Fatal("Login() expected error for non-existent email, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("Login() error = %q, want contains %q", err.Error(), "invalid credentials")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("CorrectPass12", salt)

	emp := &models.Employee{
		ID:           2,
		Email:        "emp@bank.com",
		Username:     "emp02",
		Password:     hash,
		SaltPassword: salt,
		Aktivan:      true,
		Permissions:  []models.Permission{},
	}

	svc := newTestAuthService(
		&mockEmployeeRepo{findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil }},
		&mockTokenRepo{},
	)

	_, _, _, err := svc.Login("emp@bank.com", "WrongPass99")
	if err == nil {
		t.Fatal("Login() expected error for wrong password, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("Login() error = %q, want contains %q", err.Error(), "invalid credentials")
	}
}

func TestLogin_InactiveEmployee(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("TestPass12", salt)

	emp := &models.Employee{
		ID:           3,
		Email:        "inactive@bank.com",
		Username:     "inactive01",
		Password:     hash,
		SaltPassword: salt,
		Aktivan:      false,
		Permissions:  []models.Permission{},
	}

	svc := newTestAuthService(
		&mockEmployeeRepo{findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil }},
		&mockTokenRepo{},
	)

	_, _, _, err := svc.Login("inactive@bank.com", "TestPass12")
	if err == nil {
		t.Fatal("Login() expected error for inactive employee, got nil")
	}
	if !strings.Contains(err.Error(), "account is not active") {
		t.Errorf("Login() error = %q, want contains %q", err.Error(), "account is not active")
	}
}

func TestRefreshToken_WrongTokenType(t *testing.T) {
	// Generate an access token (type="access") and pass it to RefreshToken
	accessToken, err := util.GenerateAccessToken(1, "emp@bank.com", "emp01", []string{}, "test-secret", 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error: %v", err)
	}

	svc := newTestAuthService(&mockEmployeeRepo{}, &mockTokenRepo{})

	_, _, err = svc.RefreshToken(accessToken)
	if err == nil {
		t.Fatal("RefreshToken() expected error for access token, got nil")
	}
	if !strings.Contains(err.Error(), "wrong token type") {
		t.Errorf("RefreshToken() error = %q, want contains %q", err.Error(), "wrong token type")
	}
}

func TestActivateAccount_PasswordMismatch(t *testing.T) {
	svc := newTestAuthService(&mockEmployeeRepo{}, &mockTokenRepo{})

	err := svc.ActivateAccount("sometoken", "TestPass12", "DifferentPass12")
	if err == nil {
		t.Fatal("ActivateAccount() expected error for mismatched passwords, got nil")
	}
	if !strings.Contains(err.Error(), "passwords do not match") {
		t.Errorf("ActivateAccount() error = %q, want contains %q", err.Error(), "passwords do not match")
	}
}

func TestActivateAccount_InvalidPasswordPolicy(t *testing.T) {
	svc := newTestAuthService(&mockEmployeeRepo{}, &mockTokenRepo{})

	// "weak" has no digits, no uppercase — fails policy
	err := svc.ActivateAccount("sometoken", "weak", "weak")
	if err == nil {
		t.Fatal("ActivateAccount() expected error for weak password, got nil")
	}
}

// ---- ClientLogin tests ----

func newTestAuthServiceWithClient(empRepo repository.EmployeeRepositoryInterface, clientRepo repository.ClientRepositoryInterface, tokRepo repository.TokenRepositoryInterface) *service.AuthService {
	cfg := &config.Config{
		JWTSecret:          "test-secret",
		JWTAccessDuration:  15,
		JWTRefreshDuration: 24 * 60,
	}
	return service.NewAuthServiceWithRepos(cfg, empRepo, clientRepo, tokRepo, nil)
}

func TestClientLogin_Success(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("ClientPass12", salt)

	client := &models.Client{
		ID:           10,
		Email:        "user@gmail.com",
		Password:     hash,
		SaltPassword: salt,
		Aktivan:      true,
		Permissions:  []models.Permission{},
	}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{findByEmailFn: func(email string) (*models.Client, error) { return client, nil }},
		&mockTokenRepo{},
	)

	access, refresh, gotClient, err := svc.ClientLogin("user@gmail.com", "ClientPass12")
	if err != nil {
		t.Fatalf("ClientLogin() unexpected error: %v", err)
	}
	if access == "" {
		t.Error("ClientLogin() returned empty access token")
	}
	if refresh == "" {
		t.Error("ClientLogin() returned empty refresh token")
	}
	if gotClient == nil || gotClient.ID != 10 {
		t.Error("ClientLogin() returned wrong client")
	}
}

func TestClientLogin_WrongPassword(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("CorrectPass12", salt)

	client := &models.Client{
		ID:           11,
		Email:        "user@gmail.com",
		Password:     hash,
		SaltPassword: salt,
		Aktivan:      true,
	}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{findByEmailFn: func(email string) (*models.Client, error) { return client, nil }},
		&mockTokenRepo{},
	)

	_, _, _, err := svc.ClientLogin("user@gmail.com", "WrongPass99")
	if err == nil {
		t.Fatal("ClientLogin() expected error for wrong password, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("ClientLogin() error = %q, want contains %q", err.Error(), "invalid credentials")
	}
}

func TestClientLogin_NonExistentEmail(t *testing.T) {
	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{findByEmailFn: func(email string) (*models.Client, error) {
			return nil, gorm.ErrRecordNotFound
		}},
		&mockTokenRepo{},
	)

	_, _, _, err := svc.ClientLogin("nobody@gmail.com", "AnyPass12")
	if err == nil {
		t.Fatal("ClientLogin() expected error for non-existent email, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("ClientLogin() error = %q, want contains %q", err.Error(), "invalid credentials")
	}
}

func TestClientLogin_InactiveClient(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("ClientPass12", salt)

	client := &models.Client{
		ID:           12,
		Email:        "inactive@gmail.com",
		Password:     hash,
		SaltPassword: salt,
		Aktivan:      false,
	}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{findByEmailFn: func(email string) (*models.Client, error) { return client, nil }},
		&mockTokenRepo{},
	)

	_, _, _, err := svc.ClientLogin("inactive@gmail.com", "ClientPass12")
	if err == nil {
		t.Fatal("ClientLogin() expected error for inactive client, got nil")
	}
	if !strings.Contains(err.Error(), "account is not active") {
		t.Errorf("ClientLogin() error = %q, want contains %q", err.Error(), "account is not active")
	}
}

func TestClientLogin_JWTContainsClientID(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("ClientPass12", salt)

	client := &models.Client{ID: 42, Email: "user@gmail.com", Password: hash, SaltPassword: salt, Aktivan: true, Permissions: []models.Permission{}}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{findByEmailFn: func(email string) (*models.Client, error) { return client, nil }},
		&mockTokenRepo{},
	)

	access, _, _, err := svc.ClientLogin("user@gmail.com", "ClientPass12")
	if err != nil {
		t.Fatalf("ClientLogin() unexpected error: %v", err)
	}

	claims, err := util.ParseToken(access, "test-secret")
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	if claims.ClientID != 42 {
		t.Errorf("JWT ClientID = %d, want 42", claims.ClientID)
	}
}

func TestClientLogin_JWTTokenSourceIsClient(t *testing.T) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("ClientPass12", salt)

	client := &models.Client{ID: 7, Email: "user@gmail.com", Password: hash, SaltPassword: salt, Aktivan: true, Permissions: []models.Permission{}}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{findByEmailFn: func(email string) (*models.Client, error) { return client, nil }},
		&mockTokenRepo{},
	)

	access, _, _, err := svc.ClientLogin("user@gmail.com", "ClientPass12")
	if err != nil {
		t.Fatalf("ClientLogin() unexpected error: %v", err)
	}

	claims, err := util.ParseToken(access, "test-secret")
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	if claims.TokenSource != "client" {
		t.Errorf("JWT TokenSource = %q, want %q", claims.TokenSource, "client")
	}
}

func TestRequestPasswordReset_NonExistentEmail(t *testing.T) {
	svc := newTestAuthService(
		&mockEmployeeRepo{findByEmailFn: func(email string) (*models.Employee, error) {
			return nil, errors.New("record not found")
		}},
		&mockTokenRepo{},
	)

	// Service silently returns nil when employee not found
	err := svc.RequestPasswordReset("nobody@bank.com")
	if err != nil {
		t.Errorf("RequestPasswordReset() expected nil for non-existent email, got: %v", err)
	}
}

func TestActivateClientAccount_Success(t *testing.T) {
	token, err := util.GenerateClientSetupToken(55, "newclient@gmail.com", "test-secret", 24)
	if err != nil {
		t.Fatalf("GenerateClientSetupToken() error: %v", err)
	}

	var updatedFields map[string]interface{}
	client := &models.Client{
		ID:      55,
		Ime:     "Novi",
		Prezime: "Klijent",
		Email:   "newclient@gmail.com",
		Aktivan: false,
	}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{
			findByIDFn: func(id uint) (*models.Client, error) { return client, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				updatedFields = fields
				return nil
			},
		},
		&mockTokenRepo{},
	)

	err = svc.ActivateClientAccount(token, "ClientPass12", "ClientPass12")
	if err != nil {
		t.Fatalf("ActivateClientAccount() unexpected error: %v", err)
	}
	if updatedFields == nil {
		t.Fatal("ActivateClientAccount() did not update client fields")
	}
	if updatedFields["aktivan"] != true {
		t.Errorf("expected aktivan=true, got %#v", updatedFields["aktivan"])
	}
	if updatedFields["password"] == "" || updatedFields["salt_password"] == "" {
		t.Error("ActivateClientAccount() did not set password hash and salt")
	}
}

func TestActivateClientAccount_RejectsActiveClient(t *testing.T) {
	token, err := util.GenerateClientSetupToken(77, "active@gmail.com", "test-secret", 24)
	if err != nil {
		t.Fatalf("GenerateClientSetupToken() error: %v", err)
	}

	svc := newTestAuthServiceWithClient(
		&mockEmployeeRepo{},
		&mockClientRepo{
			findByIDFn: func(id uint) (*models.Client, error) {
				return &models.Client{ID: 77, Email: "active@gmail.com", Aktivan: true}, nil
			},
		},
		&mockTokenRepo{},
	)

	err = svc.ActivateClientAccount(token, "ClientPass12", "ClientPass12")
	if err == nil {
		t.Fatal("ActivateClientAccount() expected error for active client, got nil")
	}
	if !strings.Contains(err.Error(), "already active") {
		t.Errorf("ActivateClientAccount() error = %q, want contains %q", err.Error(), "already active")
	}
}
