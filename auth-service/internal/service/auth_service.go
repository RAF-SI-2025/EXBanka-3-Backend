package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
	"gorm.io/gorm"
)

type AuthService struct {
	cfg          *config.Config
	employeeRepo repository.EmployeeRepositoryInterface
	clientRepo   repository.ClientRepositoryInterface
	tokenRepo    repository.TokenRepositoryInterface
	notifSvc     NotificationServiceInterface
}

func NewAuthService(cfg *config.Config, db *gorm.DB, notifSvc NotificationServiceInterface) *AuthService {
	return &AuthService{
		cfg:          cfg,
		employeeRepo: repository.NewEmployeeRepository(db),
		clientRepo:   repository.NewClientRepository(db),
		tokenRepo:    repository.NewTokenRepository(db),
		notifSvc:     notifSvc,
	}
}

// NewAuthServiceWithRepos constructs an AuthService with injected repository interfaces,
// allowing mock implementations to be used in unit tests.
func NewAuthServiceWithRepos(cfg *config.Config, employeeRepo repository.EmployeeRepositoryInterface, clientRepo repository.ClientRepositoryInterface, tokenRepo repository.TokenRepositoryInterface, notifSvc NotificationServiceInterface) *AuthService {
	return &AuthService{
		cfg:          cfg,
		employeeRepo: employeeRepo,
		clientRepo:   clientRepo,
		tokenRepo:    tokenRepo,
		notifSvc:     notifSvc,
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *AuthService) Login(email, password string) (string, string, *models.Employee, error) {
	emp, err := s.employeeRepo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil, fmt.Errorf("invalid credentials")
		}
		return "", "", nil, err
	}

	if !emp.Aktivan {
		return "", "", nil, fmt.Errorf("account is not active")
	}

	// Brute-force check — must happen BEFORE password verification.
	now := time.Now()
	if emp.AccountLockedUntil != nil {
		if emp.AccountLockedUntil.After(now) {
			// Lock is still active: reject immediately, do NOT touch the counter.
			return "", "", nil, fmt.Errorf("account temporarily locked, please try again later or reset your password")
		}
		// Lock has expired: reset state so the user gets fresh attempts.
		_ = s.employeeRepo.UpdateFields(emp.ID, map[string]interface{}{
			"failed_login_attempts": 0,
			"account_locked_until":  nil,
		})
		emp.FailedLoginAttempts = 0
		emp.AccountLockedUntil = nil
	}

	ok, err := util.VerifyPassword(password, emp.SaltPassword, emp.Password)
	if err != nil {
		return "", "", nil, err
	}
	if !ok {
		emp.FailedLoginAttempts++
		updates := map[string]interface{}{
			"failed_login_attempts": emp.FailedLoginAttempts,
		}
		if emp.FailedLoginAttempts >= 5 {
			lockUntil := now.Add(10 * time.Minute)
			updates["account_locked_until"] = lockUntil
			_ = s.employeeRepo.UpdateFields(emp.ID, updates)
			if s.notifSvc != nil {
				_ = s.notifSvc.SendAccountLockedEmail(emp.Email, emp.Ime+" "+emp.Prezime)
			}
		} else {
			_ = s.employeeRepo.UpdateFields(emp.ID, updates)
		}
		return "", "", nil, fmt.Errorf("invalid credentials")
	}

	// Success — clear brute-force state.
	_ = s.employeeRepo.UpdateFields(emp.ID, map[string]interface{}{
		"failed_login_attempts": 0,
		"account_locked_until":  nil,
	})

	perms := emp.PermissionNames()

	accessToken, err := util.GenerateAccessToken(emp.ID, emp.Email, emp.Username, perms, s.cfg.JWTSecret, s.cfg.JWTAccessDuration)
	if err != nil {
		return "", "", nil, err
	}

	refreshToken, err := util.GenerateRefreshToken(emp.ID, emp.Email, emp.Username, s.cfg.JWTSecret, s.cfg.JWTRefreshDuration)
	if err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, emp, nil
}

func (s *AuthService) RefreshToken(refreshTokenStr string) (string, string, error) {
	claims, err := util.ParseToken(refreshTokenStr, s.cfg.JWTSecret)
	if err != nil {
		return "", "", fmt.Errorf("invalid refresh token")
	}

	if claims.TokenType != "refresh" {
		return "", "", fmt.Errorf("wrong token type")
	}
	if util.IsTokenRevoked(context.Background(), claims) {
		return "", "", fmt.Errorf("invalid refresh token")
	}

	emp, err := s.employeeRepo.FindByID(claims.EmployeeID)
	if err != nil {
		return "", "", fmt.Errorf("employee not found")
	}

	if !emp.Aktivan {
		return "", "", fmt.Errorf("account is not active")
	}

	perms := emp.PermissionNames()

	accessToken, err := util.GenerateAccessToken(emp.ID, emp.Email, emp.Username, perms, s.cfg.JWTSecret, s.cfg.JWTAccessDuration)
	if err != nil {
		return "", "", err
	}

	newRefresh, err := util.GenerateRefreshToken(emp.ID, emp.Email, emp.Username, s.cfg.JWTSecret, s.cfg.JWTRefreshDuration)
	if err != nil {
		return "", "", err
	}

	return accessToken, newRefresh, nil
}

func (s *AuthService) ActivateAccount(tokenStr, password, passwordConfirm string) error {
	if password != passwordConfirm {
		return fmt.Errorf("passwords do not match")
	}
	if err := util.ValidatePasswordPolicy(password); err != nil {
		return err
	}

	token, err := s.tokenRepo.FindValid(tokenStr, models.TokenTypeActivation)
	if err != nil {
		return fmt.Errorf("invalid or expired activation token")
	}

	if err := s.tokenRepo.InvalidateEmployeeTokens(token.EmployeeID, models.TokenTypeActivation); err != nil {
		return err
	}

	salt, err := util.GenerateSalt()
	if err != nil {
		return err
	}
	hashed, err := util.HashPassword(password, salt)
	if err != nil {
		return err
	}

	if err := s.employeeRepo.UpdateFields(token.EmployeeID, map[string]interface{}{
		"password":      hashed,
		"salt_password": salt,
		"aktivan":       true,
	}); err != nil {
		return err
	}

	emp, err := s.employeeRepo.FindByID(token.EmployeeID)
	if err != nil {
		return err
	}

	_ = s.notifSvc.SendConfirmationEmail(emp.Email, emp.Ime+" "+emp.Prezime)
	return nil
}

func (s *AuthService) RequestPasswordReset(email string) error {
	emp, err := s.employeeRepo.FindByEmail(email)
	if err != nil {
		return nil
	}
	if !emp.Aktivan {
		return nil
	}

	_ = s.tokenRepo.InvalidateEmployeeTokens(emp.ID, models.TokenTypeReset)

	tokenStr, err := generateToken()
	if err != nil {
		return err
	}

	token := &models.Token{
		EmployeeID: emp.ID,
		Token:      tokenStr,
		Type:       models.TokenTypeReset,
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}
	if err := s.tokenRepo.Create(token); err != nil {
		return err
	}

	_ = s.notifSvc.SendResetPasswordEmail(emp.Email, emp.Ime+" "+emp.Prezime, tokenStr)
	return nil
}

// ClientLogin authenticates a client by email/password and returns JWT tokens with client_id.
func (s *AuthService) ClientLogin(email, password string) (string, string, *models.Client, error) {
	client, err := s.clientRepo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil, fmt.Errorf("invalid credentials")
		}
		return "", "", nil, err
	}

	if !client.Aktivan {
		return "", "", nil, fmt.Errorf("account is not active")
	}

	// Brute-force check — must happen BEFORE password verification.
	now := time.Now()
	if client.AccountLockedUntil != nil {
		if client.AccountLockedUntil.After(now) {
			// Lock is still active: reject immediately, do NOT touch the counter.
			return "", "", nil, fmt.Errorf("account temporarily locked, please try again later or reset your password")
		}
		// Lock has expired: reset state so the user gets fresh attempts.
		_ = s.clientRepo.UpdateFields(client.ID, map[string]interface{}{
			"failed_login_attempts": 0,
			"account_locked_until":  nil,
		})
		client.FailedLoginAttempts = 0
		client.AccountLockedUntil = nil
	}

	ok, err := util.VerifyPassword(password, client.SaltPassword, client.Password)
	if err != nil {
		return "", "", nil, err
	}
	if !ok {
		client.FailedLoginAttempts++
		updates := map[string]interface{}{
			"failed_login_attempts": client.FailedLoginAttempts,
		}
		if client.FailedLoginAttempts >= 5 {
			lockUntil := now.Add(10 * time.Minute)
			updates["account_locked_until"] = lockUntil
			_ = s.clientRepo.UpdateFields(client.ID, updates)
			if s.notifSvc != nil {
				_ = s.notifSvc.SendAccountLockedEmail(client.Email, client.Ime+" "+client.Prezime)
			}
		} else {
			_ = s.clientRepo.UpdateFields(client.ID, updates)
		}
		return "", "", nil, fmt.Errorf("invalid credentials")
	}

	// Success — clear brute-force state.
	_ = s.clientRepo.UpdateFields(client.ID, map[string]interface{}{
		"failed_login_attempts": 0,
		"account_locked_until":  nil,
	})

	perms := client.PermissionNames()

	accessToken, err := util.GenerateClientAccessToken(client.ID, client.Email, perms, s.cfg.JWTSecret, s.cfg.JWTAccessDuration)
	if err != nil {
		return "", "", nil, err
	}

	refreshToken, err := util.GenerateClientRefreshToken(client.ID, client.Email, s.cfg.JWTSecret, s.cfg.JWTRefreshDuration)
	if err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, client, nil
}

func (s *AuthService) ActivateClientAccount(tokenStr, password, passwordConfirm string) error {
	if password != passwordConfirm {
		return fmt.Errorf("passwords do not match")
	}
	if err := util.ValidatePasswordPolicy(password); err != nil {
		return err
	}

	claims, err := util.ParseToken(tokenStr, s.cfg.JWTSecret)
	if err != nil {
		return fmt.Errorf("invalid or expired activation token")
	}
	if claims.TokenType != "setup" || claims.TokenSource != "client_setup" || claims.ClientID == 0 {
		return fmt.Errorf("invalid activation token")
	}

	client, err := s.clientRepo.FindByID(claims.ClientID)
	if err != nil {
		return fmt.Errorf("invalid activation token")
	}
	if client.Email != claims.Email {
		return fmt.Errorf("invalid activation token")
	}
	if client.Aktivan {
		return fmt.Errorf("account is already active")
	}

	salt, err := util.GenerateSalt()
	if err != nil {
		return err
	}
	hashed, err := util.HashPassword(password, salt)
	if err != nil {
		return err
	}

	if err := s.clientRepo.UpdateFields(client.ID, map[string]interface{}{
		"password":      hashed,
		"salt_password": salt,
		"aktivan":       true,
	}); err != nil {
		return err
	}

	if s.notifSvc != nil {
		_ = s.notifSvc.SendConfirmationEmail(client.Email, client.Ime+" "+client.Prezime)
	}

	return nil
}

func (s *AuthService) ResetPassword(tokenStr, password, passwordConfirm string) error {
	if password != passwordConfirm {
		return fmt.Errorf("passwords do not match")
	}
	if err := util.ValidatePasswordPolicy(password); err != nil {
		return err
	}

	token, err := s.tokenRepo.FindValid(tokenStr, models.TokenTypeReset)
	if err != nil {
		return fmt.Errorf("invalid or expired reset token")
	}

	if err := s.tokenRepo.InvalidateEmployeeTokens(token.EmployeeID, models.TokenTypeReset); err != nil {
		return err
	}

	salt, err := util.GenerateSalt()
	if err != nil {
		return err
	}
	hashed, err := util.HashPassword(password, salt)
	if err != nil {
		return err
	}

	return s.employeeRepo.UpdateFields(token.EmployeeID, map[string]interface{}{
		"password":              hashed,
		"salt_password":         salt,
		"failed_login_attempts": 0,
		"account_locked_until":  nil,
	})
}
