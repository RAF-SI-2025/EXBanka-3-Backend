package handler

import (
	"context"

	authv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/gen/proto/auth/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	authsvc "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	infrasvc "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type AuthHandler struct {
	authv1.UnimplementedAuthServiceServer
	svc *authsvc.AuthService
}

func NewAuthHandler(cfg *config.Config, db *gorm.DB, notifSvc *infrasvc.NotificationService) *AuthHandler {
	return &AuthHandler{
		svc: authsvc.NewAuthService(cfg, db, notifSvc),
	}
}

func (h *AuthHandler) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	accessToken, refreshToken, emp, err := h.svc.Login(req.Email, req.Password)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "%s", err.Error())
	}

	return &authv1.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Employee: &authv1.EmployeeInfo{
			Id:          uint64(emp.ID),
			Ime:         emp.Ime,
			Prezime:     emp.Prezime,
			Email:       emp.Email,
			Username:    emp.Username,
			Pozicija:    emp.Pozicija,
			Permissions: emp.PermissionNames(),
		},
	}, nil
}

func (h *AuthHandler) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	accessToken, refreshToken, err := h.svc.RefreshToken(req.RefreshToken)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "%s", err.Error())
	}

	return &authv1.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (h *AuthHandler) ActivateAccount(ctx context.Context, req *authv1.ActivateAccountRequest) (*authv1.ActivateAccountResponse, error) {
	if err := h.svc.ActivateAccount(req.Token, req.Password, req.PasswordConfirm); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &authv1.ActivateAccountResponse{
		Success: true,
		Message: "Account activated successfully",
	}, nil
}

func (h *AuthHandler) RequestPasswordReset(ctx context.Context, req *authv1.RequestPasswordResetRequest) (*authv1.RequestPasswordResetResponse, error) {
	if err := h.svc.RequestPasswordReset(req.Email); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process request")
	}

	return &authv1.RequestPasswordResetResponse{
		Success: true,
		Message: "If the email exists, a reset link has been sent",
	}, nil
}

func (h *AuthHandler) ResetPassword(ctx context.Context, req *authv1.ResetPasswordRequest) (*authv1.ResetPasswordResponse, error) {
	if err := h.svc.ResetPassword(req.Token, req.Password, req.PasswordConfirm); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &authv1.ResetPasswordResponse{
		Success: true,
		Message: "Password reset successfully",
	}, nil
}
