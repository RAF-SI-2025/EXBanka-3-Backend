package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
)

// ---- mock notification service ----

type mockNotifSvc struct {
	accountLockedFn func(toEmail, toName string) error
}

func (m *mockNotifSvc) SendConfirmationEmail(toEmail, toName string) error { return nil }
func (m *mockNotifSvc) SendResetPasswordEmail(toEmail, toName, token string) error { return nil }
func (m *mockNotifSvc) SendAccountLockedEmail(toEmail, toName string) error {
	if m.accountLockedFn != nil {
		return m.accountLockedFn(toEmail, toName)
	}
	return nil
}

var _ service.NotificationServiceInterface = (*mockNotifSvc)(nil)

// ---- helpers ----

func newBruteForceTestSvc(
	empRepo repository.EmployeeRepositoryInterface,
	clientRepo repository.ClientRepositoryInterface,
	notif service.NotificationServiceInterface,
) *service.AuthService {
	cfg := &config.Config{
		JWTSecret:          "test-secret",
		JWTAccessDuration:  15,
		JWTRefreshDuration: 24 * 60,
	}
	return service.NewAuthServiceWithRepos(cfg, empRepo, clientRepo, &mockTokenRepo{}, notif)
}

func makeActiveEmployee(failedAttempts int, lockedUntil *time.Time) (*models.Employee, string) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("CorrectPass12", salt)
	return &models.Employee{
		ID:                  1,
		Ime:                 "Test",
		Prezime:             "Employee",
		Email:               "emp@bank.com",
		Username:            "emp01",
		Password:            hash,
		SaltPassword:        salt,
		Aktivan:             true,
		FailedLoginAttempts: failedAttempts,
		AccountLockedUntil:  lockedUntil,
		Permissions:         []models.Permission{},
	}, "CorrectPass12"
}

func makeActiveClient(failedAttempts int, lockedUntil *time.Time) (*models.Client, string) {
	salt, _ := util.GenerateSalt()
	hash, _ := util.HashPassword("CorrectPass12", salt)
	return &models.Client{
		ID:                  10,
		Ime:                 "Test",
		Prezime:             "Client",
		Email:               "client@bank.com",
		Password:            hash,
		SaltPassword:        salt,
		Aktivan:             true,
		FailedLoginAttempts: failedAttempts,
		AccountLockedUntil:  lockedUntil,
		Permissions:         []models.Permission{},
	}, "CorrectPass12"
}

// ---- KORAK 5, scenario 1 ----
// 5. neuspešan pokušaj zaključava nalog i šalje email.

func TestLogin_FifthFailedAttempt_LocksAccountAndSendsEmail(t *testing.T) {
	emp, _ := makeActiveEmployee(4, nil) // already has 4 failed attempts

	var savedFields map[string]interface{}
	var emailSentTo string

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{
			findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				savedFields = fields
				return nil
			},
		},
		&mockClientRepo{},
		&mockNotifSvc{
			accountLockedFn: func(toEmail, toName string) error {
				emailSentTo = toEmail
				return nil
			},
		},
	)

	_, _, _, err := svc.Login("emp@bank.com", "WrongPass99")
	if err == nil {
		t.Fatal("Login() expected error, got nil")
	}

	if savedFields == nil {
		t.Fatal("UpdateFields was not called")
	}
	attempts, ok := savedFields["failed_login_attempts"].(int)
	if !ok || attempts != 5 {
		t.Errorf("failed_login_attempts = %v, want 5", savedFields["failed_login_attempts"])
	}
	if savedFields["account_locked_until"] == nil {
		t.Error("account_locked_until should be set, got nil")
	}
	if emailSentTo != "emp@bank.com" {
		t.Errorf("lockout email sent to %q, want %q", emailSentTo, "emp@bank.com")
	}
}

// ---- KORAK 5, scenario 2 ----
// Login odbijen dok je nalog zaključan — čak i sa ispravnom lozinkom.

func TestLogin_LockedAccount_RejectsEvenCorrectPassword(t *testing.T) {
	lockUntil := time.Now().Add(10 * time.Minute)
	emp, correctPassword := makeActiveEmployee(5, &lockUntil)

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{
			findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil },
		},
		&mockClientRepo{},
		&mockNotifSvc{},
	)

	_, _, _, err := svc.Login("emp@bank.com", correctPassword)
	if err == nil {
		t.Fatal("Login() expected error for locked account, got nil")
	}
	if !strings.Contains(err.Error(), "temporarily locked") {
		t.Errorf("Login() error = %q, want contains %q", err.Error(), "temporarily locked")
	}
}

// ---- KORAK 5, extra (VAŽNO iz specifikacije) ----
// Pokušaj tokom aktivnog lock-a NE sme da pomera AccountLockedUntil dalje.

func TestLogin_LockedAccount_DoesNotExtendLockout(t *testing.T) {
	lockUntil := time.Now().Add(10 * time.Minute)
	emp, correctPassword := makeActiveEmployee(5, &lockUntil)

	var updateFieldsCalled bool

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{
			findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				updateFieldsCalled = true
				return nil
			},
		},
		&mockClientRepo{},
		&mockNotifSvc{},
	)

	// Pokušaj sa ispravnom lozinkom dok je nalog zaključan.
	_, _, _, err := svc.Login("emp@bank.com", correctPassword)
	if err == nil {
		t.Fatal("Login() expected error for locked account, got nil")
	}
	if updateFieldsCalled {
		t.Error("UpdateFields was called during active lock — must NOT modify counter or lockout time")
	}
}

// ---- KORAK 5, scenario 3 ----
// Login uspeva posle isteka lock-a i resetuje brojač na 0.

func TestLogin_AfterLockExpires_SuccessResetsCounter(t *testing.T) {
	expired := time.Now().Add(-1 * time.Minute) // lock je istekao minut ranije
	emp, correctPassword := makeActiveEmployee(5, &expired)

	var savedFields map[string]interface{}
	callCount := 0

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{
			findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				callCount++
				savedFields = fields // poslednji poziv
				return nil
			},
		},
		&mockClientRepo{},
		&mockNotifSvc{},
	)

	access, _, _, err := svc.Login("emp@bank.com", correctPassword)
	if err != nil {
		t.Fatalf("Login() unexpected error after lock expiry: %v", err)
	}
	if access == "" {
		t.Error("Login() returned empty access token")
	}
	// Poslednji UpdateFields poziv (success path) mora da resetuje brojač.
	if savedFields["failed_login_attempts"] != 0 {
		t.Errorf("failed_login_attempts = %v after successful login, want 0", savedFields["failed_login_attempts"])
	}
	if savedFields["account_locked_until"] != nil {
		t.Errorf("account_locked_until = %v after successful login, want nil", savedFields["account_locked_until"])
	}
}

// ---- KORAK 5, scenario 4 ----
// ResetPassword otključava nalog i resetuje brojač.

func TestResetPassword_UnlocksAccountAndResetsCounter(t *testing.T) {
	lockUntil := time.Now().Add(5 * time.Minute)
	emp, _ := makeActiveEmployee(5, &lockUntil)

	var savedFields map[string]interface{}

	token := &models.Token{
		EmployeeID: emp.ID,
		Token:      "valid-reset-token",
		Type:       models.TokenTypeReset,
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	cfg := &config.Config{
		JWTSecret:          "test-secret",
		JWTAccessDuration:  15,
		JWTRefreshDuration: 24 * 60,
	}
	svcFull := service.NewAuthServiceWithRepos(
		cfg,
		&mockEmployeeRepo{
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				savedFields = fields
				return nil
			},
		},
		&mockClientRepo{},
		&mockTokenRepo{
			findValidFn: func(tokenStr, tokenType string) (*models.Token, error) {
				return token, nil
			},
			invalidateEmployeeTokensFn: func(employeeID uint, tokenType string) error {
				return nil
			},
		},
		&mockNotifSvc{},
	)

	err := svcFull.ResetPassword("valid-reset-token", "NewPass12!", "NewPass12!")
	if err != nil {
		t.Fatalf("ResetPassword() unexpected error: %v", err)
	}

	if savedFields == nil {
		t.Fatal("UpdateFields was not called")
	}
	if savedFields["failed_login_attempts"] != 0 {
		t.Errorf("failed_login_attempts = %v after reset, want 0", savedFields["failed_login_attempts"])
	}
	if savedFields["account_locked_until"] != nil {
		t.Errorf("account_locked_until = %v after reset, want nil", savedFields["account_locked_until"])
	}
}

// ---- KORAK 5, scenario 5 ----
// Uspešan login posle nekoliko neuspešnih resetuje brojač na 0.

func TestLogin_SuccessAfterSomeFailures_ResetsCounter(t *testing.T) {
	emp, correctPassword := makeActiveEmployee(3, nil) // 3 neuspešna, nije zaključan

	var lastSavedFields map[string]interface{}

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{
			findByEmailFn: func(email string) (*models.Employee, error) { return emp, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				lastSavedFields = fields
				return nil
			},
		},
		&mockClientRepo{},
		&mockNotifSvc{},
	)

	access, _, _, err := svc.Login("emp@bank.com", correctPassword)
	if err != nil {
		t.Fatalf("Login() unexpected error: %v", err)
	}
	if access == "" {
		t.Error("Login() returned empty access token")
	}
	if lastSavedFields["failed_login_attempts"] != 0 {
		t.Errorf("failed_login_attempts = %v after successful login, want 0", lastSavedFields["failed_login_attempts"])
	}
	if lastSavedFields["account_locked_until"] != nil {
		t.Errorf("account_locked_until = %v after successful login, want nil", lastSavedFields["account_locked_until"])
	}
}

// ---- Isti scenario 1 za Client ----

func TestClientLogin_FifthFailedAttempt_LocksAccountAndSendsEmail(t *testing.T) {
	client, _ := makeActiveClient(4, nil)

	var savedFields map[string]interface{}
	var emailSentTo string

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{},
		&mockClientRepo{
			findByEmailFn: func(email string) (*models.Client, error) { return client, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				savedFields = fields
				return nil
			},
		},
		&mockNotifSvc{
			accountLockedFn: func(toEmail, toName string) error {
				emailSentTo = toEmail
				return nil
			},
		},
	)

	_, _, _, err := svc.ClientLogin("client@bank.com", "WrongPass99")
	if err == nil {
		t.Fatal("ClientLogin() expected error, got nil")
	}

	attempts, ok := savedFields["failed_login_attempts"].(int)
	if !ok || attempts != 5 {
		t.Errorf("failed_login_attempts = %v, want 5", savedFields["failed_login_attempts"])
	}
	if savedFields["account_locked_until"] == nil {
		t.Error("account_locked_until should be set, got nil")
	}
	if emailSentTo != "client@bank.com" {
		t.Errorf("lockout email sent to %q, want %q", emailSentTo, "client@bank.com")
	}
}

// ---- Isti scenario 2 za Client ----

func TestClientLogin_LockedAccount_RejectsEvenCorrectPassword(t *testing.T) {
	lockUntil := time.Now().Add(10 * time.Minute)
	client, correctPassword := makeActiveClient(5, &lockUntil)

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{},
		&mockClientRepo{
			findByEmailFn: func(email string) (*models.Client, error) { return client, nil },
		},
		&mockNotifSvc{},
	)

	_, _, _, err := svc.ClientLogin("client@bank.com", correctPassword)
	if err == nil {
		t.Fatal("ClientLogin() expected error for locked account, got nil")
	}
	if !strings.Contains(err.Error(), "temporarily locked") {
		t.Errorf("ClientLogin() error = %q, want contains %q", err.Error(), "temporarily locked")
	}
}

// ---- Extend-lock test za Client ----

func TestClientLogin_LockedAccount_DoesNotExtendLockout(t *testing.T) {
	lockUntil := time.Now().Add(10 * time.Minute)
	client, correctPassword := makeActiveClient(5, &lockUntil)

	var updateFieldsCalled bool

	svc := newBruteForceTestSvc(
		&mockEmployeeRepo{},
		&mockClientRepo{
			findByEmailFn: func(email string) (*models.Client, error) { return client, nil },
			updateFieldsFn: func(id uint, fields map[string]interface{}) error {
				updateFieldsCalled = true
				return nil
			},
		},
		&mockNotifSvc{},
	)

	_, _, _, err := svc.ClientLogin("client@bank.com", correctPassword)
	if err == nil {
		t.Fatal("ClientLogin() expected error for locked account, got nil")
	}
	if updateFieldsCalled {
		t.Error("UpdateFields was called during active client lock — must NOT modify counter or lockout time")
	}
}
