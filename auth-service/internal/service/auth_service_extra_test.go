package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
)

// notifSvc backed by an unreachable SMTP — errors are silently discarded by AuthService,
// so the full success path can run end-to-end.
func newSilentNotifSvc() *service.NotificationService {
	return service.NewNotificationService(&config.Config{
		SMTPHost: "127.0.0.1",
		SMTPPort: 1, // closed port
		SMTPFrom: "noreply@bank.com",
	})
}

func newAuthSvc(emp repository.EmployeeRepositoryInterface, cli repository.ClientRepositoryInterface, tok repository.TokenRepositoryInterface) *service.AuthService {
	cfg := &config.Config{JWTSecret: "test-secret", JWTAccessDuration: 15, JWTRefreshDuration: 24 * 60}
	return service.NewAuthServiceWithRepos(cfg, emp, cli, tok, newSilentNotifSvc())
}

// =====================
// RefreshToken success
// =====================

func TestRefreshToken_Success(t *testing.T) {
	refresh, err := util.GenerateRefreshToken(7, "u@b.c", "u", "test-secret", 24)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}

	emp := &models.Employee{ID: 7, Email: "u@b.c", Username: "u", Aktivan: true}
	svc := newAuthSvc(
		&mockEmployeeRepo{findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil }},
		&mockClientRepo{},
		&mockTokenRepo{},
	)

	access, newRefresh, err := svc.RefreshToken(refresh)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if access == "" || newRefresh == "" {
		t.Error("expected non-empty tokens")
	}
}

func TestRefreshToken_InactiveEmployee(t *testing.T) {
	refresh, _ := util.GenerateRefreshToken(8, "u@b.c", "u", "test-secret", 24)
	emp := &models.Employee{ID: 8, Email: "u@b.c", Aktivan: false}
	svc := newAuthSvc(
		&mockEmployeeRepo{findByIDFn: func(id uint) (*models.Employee, error) { return emp, nil }},
		&mockClientRepo{},
		&mockTokenRepo{},
	)

	if _, _, err := svc.RefreshToken(refresh); err == nil || !strings.Contains(err.Error(), "not active") {
		t.Errorf("expected 'not active' error, got %v", err)
	}
}

func TestRefreshToken_EmployeeNotFound(t *testing.T) {
	refresh, _ := util.GenerateRefreshToken(9, "u@b.c", "u", "test-secret", 24)
	svc := newAuthSvc(
		&mockEmployeeRepo{findByIDFn: func(id uint) (*models.Employee, error) { return nil, errors.New("not found") }},
		&mockClientRepo{},
		&mockTokenRepo{},
	)

	if _, _, err := svc.RefreshToken(refresh); err == nil {
		t.Error("expected error")
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if _, _, err := svc.RefreshToken("garbage"); err == nil {
		t.Error("expected error")
	}
}

// =====================
// ActivateAccount success path
// =====================

func TestActivateAccount_Success(t *testing.T) {
	updateCalled := false
	tokenRepo := &mockTokenRepo{
		findValidFn: func(tokenStr, tokenType string) (*models.Token, error) {
			return &models.Token{ID: 1, EmployeeID: 5, Token: tokenStr, Type: tokenType, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	empRepo := &mockEmployeeRepo{
		updateFieldsFn: func(id uint, fields map[string]interface{}) error {
			updateCalled = true
			if fields["aktivan"] != true {
				t.Errorf("expected aktivan=true, got %v", fields["aktivan"])
			}
			return nil
		},
		findByIDFn: func(id uint) (*models.Employee, error) {
			return &models.Employee{ID: 5, Email: "e@b.c", Ime: "E", Prezime: "M"}, nil
		},
	}

	svc := newAuthSvc(empRepo, &mockClientRepo{}, tokenRepo)

	if err := svc.ActivateAccount("activation-tok", "Password12", "Password12"); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	if !updateCalled {
		t.Error("expected UpdateFields to be called")
	}
}

func TestActivateAccount_InvalidToken(t *testing.T) {
	tokenRepo := &mockTokenRepo{
		findValidFn: func(string, string) (*models.Token, error) { return nil, errors.New("not found") },
	}
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, tokenRepo)

	if err := svc.ActivateAccount("bad-tok", "Password12", "Password12"); err == nil {
		t.Error("expected error")
	}
}

// =====================
// RequestPasswordReset success
// =====================

func TestRequestPasswordReset_Success(t *testing.T) {
	emp := &models.Employee{ID: 11, Email: "u@b.c", Aktivan: true}
	createCalled := false

	empRepo := &mockEmployeeRepo{findByEmailFn: func(string) (*models.Employee, error) { return emp, nil }}
	tokenRepo := &mockTokenRepo{
		createFn: func(token *models.Token) error {
			createCalled = true
			if token.EmployeeID != 11 {
				t.Errorf("EmployeeID = %d", token.EmployeeID)
			}
			if token.Type != models.TokenTypeReset {
				t.Errorf("Type = %s", token.Type)
			}
			return nil
		},
	}

	svc := newAuthSvc(empRepo, &mockClientRepo{}, tokenRepo)
	if err := svc.RequestPasswordReset("u@b.c"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if !createCalled {
		t.Error("expected token Create to be called")
	}
}

func TestRequestPasswordReset_InactiveEmployeeNoOp(t *testing.T) {
	emp := &models.Employee{ID: 12, Email: "u@b.c", Aktivan: false}
	createCalled := false

	empRepo := &mockEmployeeRepo{findByEmailFn: func(string) (*models.Employee, error) { return emp, nil }}
	tokenRepo := &mockTokenRepo{createFn: func(token *models.Token) error { createCalled = true; return nil }}

	svc := newAuthSvc(empRepo, &mockClientRepo{}, tokenRepo)
	if err := svc.RequestPasswordReset("u@b.c"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if createCalled {
		t.Error("expected no token created for inactive employee")
	}
}

// =====================
// ResetPassword
// =====================

func TestResetPassword_Success(t *testing.T) {
	updateCalled := false
	tokenRepo := &mockTokenRepo{
		findValidFn: func(string, string) (*models.Token, error) {
			return &models.Token{ID: 1, EmployeeID: 22, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	empRepo := &mockEmployeeRepo{updateFieldsFn: func(id uint, fields map[string]interface{}) error {
		updateCalled = true
		if id != 22 {
			t.Errorf("id = %d", id)
		}
		return nil
	}}

	svc := newAuthSvc(empRepo, &mockClientRepo{}, tokenRepo)

	if err := svc.ResetPassword("tok", "Password12", "Password12"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if !updateCalled {
		t.Error("expected UpdateFields call")
	}
}

func TestResetPassword_PasswordMismatch(t *testing.T) {
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if err := svc.ResetPassword("tok", "Password12", "Different12"); err == nil {
		t.Error("expected mismatch error")
	}
}

func TestResetPassword_WeakPassword(t *testing.T) {
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if err := svc.ResetPassword("tok", "weak", "weak"); err == nil {
		t.Error("expected policy error")
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	tokenRepo := &mockTokenRepo{findValidFn: func(string, string) (*models.Token, error) { return nil, errors.New("nope") }}
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, tokenRepo)

	if err := svc.ResetPassword("bad", "Password12", "Password12"); err == nil {
		t.Error("expected error")
	}
}

// =====================
// ActivateClientAccount edge cases
// =====================

func TestActivateClientAccount_PasswordMismatch(t *testing.T) {
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if err := svc.ActivateClientAccount("tok", "Password12", "Different12"); err == nil {
		t.Error("expected mismatch error")
	}
}

func TestActivateClientAccount_WeakPassword(t *testing.T) {
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if err := svc.ActivateClientAccount("tok", "weak", "weak"); err == nil {
		t.Error("expected policy error")
	}
}

func TestActivateClientAccount_InvalidToken(t *testing.T) {
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if err := svc.ActivateClientAccount("garbage", "Password12", "Password12"); err == nil {
		t.Error("expected error")
	}
}

func TestActivateClientAccount_WrongTokenType(t *testing.T) {
	tok, _ := util.GenerateAccessToken(1, "x@y.z", "x", []string{}, "test-secret", 15)
	svc := newAuthSvc(&mockEmployeeRepo{}, &mockClientRepo{}, &mockTokenRepo{})
	if err := svc.ActivateClientAccount(tok, "Password12", "Password12"); err == nil {
		t.Error("expected error for wrong token type")
	}
}

func TestActivateClientAccount_EmailMismatch(t *testing.T) {
	tok, _ := util.GenerateClientSetupToken(50, "expected@gmail.com", "test-secret", 24)
	cli := &mockClientRepo{
		findByIDFn: func(uint) (*models.Client, error) {
			return &models.Client{ID: 50, Email: "different@gmail.com", Aktivan: false}, nil
		},
	}
	svc := newAuthSvc(&mockEmployeeRepo{}, cli, &mockTokenRepo{})
	if err := svc.ActivateClientAccount(tok, "Password12", "Password12"); err == nil {
		t.Error("expected error for email mismatch")
	}
}

// =====================
// NotificationService construction
// =====================

func TestNewNotificationService(t *testing.T) {
	cfg := &config.Config{SMTPHost: "localhost"}
	if service.NewNotificationService(cfg) == nil {
		t.Error("expected non-nil notification service")
	}
}
