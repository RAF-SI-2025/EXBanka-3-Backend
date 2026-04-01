package middleware_test

import (
	"context"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testSecret = "test-secret"

func newTestConfig() *config.Config {
	return &config.Config{JWTSecret: testSecret}
}

func employeeToken(t *testing.T, perms []string) string {
	t.Helper()
	tok, err := util.GenerateAccessToken(1, "admin@bank.com", "admin", perms, testSecret, 60)
	if err != nil {
		t.Fatalf("generate employee token: %v", err)
	}
	return tok
}

func clientToken(t *testing.T, perms []string) string {
	t.Helper()
	tok, err := util.GenerateClientAccessToken(1, "client@gmail.com", perms, testSecret, 60)
	if err != nil {
		t.Fatalf("generate client token: %v", err)
	}
	return tok
}

func callInterceptor(t *testing.T, cfg *config.Config, method, token string) error {
	t.Helper()
	interceptor := middleware.AuthInterceptor(cfg)
	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	info := &grpc.UnaryServerInfo{FullMethod: method}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}
	_, err := interceptor(ctx, nil, info, handler)
	return err
}

func TestAuthInterceptor_EmployeeJWT_PassesForEmployeeEndpoint(t *testing.T) {
	cfg := newTestConfig()
	tok := employeeToken(t, []string{models.PermEmployeeAdmin})
	err := callInterceptor(t, cfg, "/employee.v1.EmployeeService/GetEmployee", tok)
	if err != nil {
		t.Errorf("expected no error for admin employee token, got %v", err)
	}
}

func TestAuthInterceptor_ClientJWT_RejectedOnEmployeeService(t *testing.T) {
	cfg := newTestConfig()
	tok := clientToken(t, []string{models.PermClientBasic})
	err := callInterceptor(t, cfg, "/employee.v1.EmployeeService/GetEmployee", tok)
	if err == nil {
		t.Fatal("expected PermissionDenied for client JWT on employee service, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestAuthInterceptor_ExpiredToken_Rejected(t *testing.T) {
	cfg := newTestConfig()
	expired, err := util.GenerateAccessToken(1, "admin@bank.com", "admin", []string{models.PermEmployeeAdmin}, testSecret, -1)
	if err != nil {
		t.Fatalf("generate expired token: %v", err)
	}
	err = callInterceptor(t, cfg, "/employee.v1.EmployeeService/GetEmployee", expired)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestAuthInterceptor_MissingToken_Rejected(t *testing.T) {
	cfg := newTestConfig()
	interceptor := middleware.AuthInterceptor(cfg)
	md := metadata.Pairs()
	ctx := metadata.NewIncomingContext(context.Background(), md)
	info := &grpc.UnaryServerInfo{FullMethod: "/employee.v1.EmployeeService/GetEmployee"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}
	_, err := interceptor(ctx, nil, info, handler)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestAuthInterceptor_EmployeeJWT_PermissionDeniedWithoutRequiredPerm(t *testing.T) {
	cfg := newTestConfig()
	tok := employeeToken(t, []string{models.PermEmployeeBasic}) // not admin, not create
	err := callInterceptor(t, cfg, "/employee.v1.EmployeeService/CreateEmployee", tok)
	if err == nil {
		t.Fatal("expected PermissionDenied for employee without create permission, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}
