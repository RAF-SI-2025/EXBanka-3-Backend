package handler_test

import (
	"context"
	"errors"
	"testing"

	authv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/gen/proto/auth/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mock service ---

type mockAuthSvc struct {
	access, refresh string
	emp             *models.Employee
	client          *models.Client
	loginErr        error
	refreshErr      error
	activateErr     error
	resetReqErr     error
	resetErr        error
	clientLoginErr  error
	clientActErr    error
}

func (m *mockAuthSvc) Login(email, password string) (string, string, *models.Employee, error) {
	return m.access, m.refresh, m.emp, m.loginErr
}
func (m *mockAuthSvc) RefreshToken(refreshToken string) (string, string, error) {
	return m.access, m.refresh, m.refreshErr
}
func (m *mockAuthSvc) ActivateAccount(token, password, passwordConfirm string) error {
	return m.activateErr
}
func (m *mockAuthSvc) RequestPasswordReset(email string) error { return m.resetReqErr }
func (m *mockAuthSvc) ResetPassword(token, password, passwordConfirm string) error {
	return m.resetErr
}
func (m *mockAuthSvc) ClientLogin(email, password string) (string, string, *models.Client, error) {
	return m.access, m.refresh, m.client, m.clientLoginErr
}
func (m *mockAuthSvc) ActivateClientAccount(token, password, passwordConfirm string) error {
	return m.clientActErr
}

// --- helpers ---

func makeEmployee() *models.Employee {
	return &models.Employee{
		ID:       7,
		Ime:      "Ana",
		Prezime:  "Marković",
		Email:    "ana@example.com",
		Username: "ana",
		Pozicija: "menadzer",
		Permissions: []models.Permission{
			{Name: "employee.read"},
			{Name: "employee.write"},
		},
	}
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	svc := &mockAuthSvc{access: "atok", refresh: "rtok", emp: makeEmployee()}
	h := handler.NewAuthHandlerWithService(svc)

	resp, err := h.Login(context.Background(), &authv1.LoginRequest{
		Email: "ana@example.com", Password: "x",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken != "atok" || resp.RefreshToken != "rtok" {
		t.Errorf("expected tokens to propagate, got %q / %q", resp.AccessToken, resp.RefreshToken)
	}
	if resp.Employee.Id != 7 || resp.Employee.Email != "ana@example.com" {
		t.Errorf("unexpected employee: %+v", resp.Employee)
	}
	if len(resp.Employee.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(resp.Employee.Permissions))
	}
}

func TestLogin_BadCredentials_ReturnsUnauthenticated(t *testing.T) {
	svc := &mockAuthSvc{loginErr: errors.New("invalid credentials")}
	h := handler.NewAuthHandlerWithService(svc)

	_, err := h.Login(context.Background(), &authv1.LoginRequest{Email: "a", Password: "b"})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

// --- RefreshToken ---

func TestRefreshToken_Success(t *testing.T) {
	svc := &mockAuthSvc{access: "newA", refresh: "newR"}
	h := handler.NewAuthHandlerWithService(svc)

	resp, err := h.RefreshToken(context.Background(), &authv1.RefreshTokenRequest{RefreshToken: "old"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken != "newA" || resp.RefreshToken != "newR" {
		t.Errorf("unexpected tokens: %q / %q", resp.AccessToken, resp.RefreshToken)
	}
}

func TestRefreshToken_Invalid_ReturnsUnauthenticated(t *testing.T) {
	svc := &mockAuthSvc{refreshErr: errors.New("invalid refresh token")}
	h := handler.NewAuthHandlerWithService(svc)

	_, err := h.RefreshToken(context.Background(), &authv1.RefreshTokenRequest{RefreshToken: "bad"})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

// --- ActivateAccount ---

func TestActivateAccount_Success(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewAuthHandlerWithService(svc)

	resp, err := h.ActivateAccount(context.Background(), &authv1.ActivateAccountRequest{
		Token: "t", Password: "p", PasswordConfirm: "p",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
}

func TestActivateAccount_Error_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockAuthSvc{activateErr: errors.New("passwords do not match")}
	h := handler.NewAuthHandlerWithService(svc)

	_, err := h.ActivateAccount(context.Background(), &authv1.ActivateAccountRequest{
		Token: "t", Password: "p", PasswordConfirm: "q",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

// --- RequestPasswordReset ---

func TestRequestPasswordReset_Success(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewAuthHandlerWithService(svc)

	resp, err := h.RequestPasswordReset(context.Background(), &authv1.RequestPasswordResetRequest{
		Email: "x@y.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
}

func TestRequestPasswordReset_Error_ReturnsInternal(t *testing.T) {
	svc := &mockAuthSvc{resetReqErr: errors.New("token gen failed")}
	h := handler.NewAuthHandlerWithService(svc)

	_, err := h.RequestPasswordReset(context.Background(), &authv1.RequestPasswordResetRequest{
		Email: "x@y.com",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", st.Code())
	}
}

// --- ResetPassword ---

func TestResetPassword_Success(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewAuthHandlerWithService(svc)

	resp, err := h.ResetPassword(context.Background(), &authv1.ResetPasswordRequest{
		Token: "t", Password: "p", PasswordConfirm: "p",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
}

func TestResetPassword_Error_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockAuthSvc{resetErr: errors.New("invalid or expired reset token")}
	h := handler.NewAuthHandlerWithService(svc)

	_, err := h.ResetPassword(context.Background(), &authv1.ResetPasswordRequest{
		Token: "bad", Password: "p", PasswordConfirm: "p",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}
