package middleware_test

import (
	"context"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGetClaimsFromContext_NotPresent(t *testing.T) {
	if _, ok := middleware.GetClaimsFromContext(context.Background()); ok {
		t.Error("expected ok=false on empty context")
	}
}

func TestGetClaimsFromContext_PresentAfterInterceptor(t *testing.T) {
	cfg := newTestConfig()
	tok := employeeToken(t, []string{models.PermEmployeeBasic})

	interceptor := middleware.AuthInterceptor(cfg)
	md := metadata.Pairs("authorization", "Bearer "+tok)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	info := &grpc.UnaryServerInfo{FullMethod: "/client.v1.ClientService/GetClient"}

	var seen *util.Claims
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		c, ok := middleware.GetClaimsFromContext(ctx)
		if !ok {
			t.Error("expected claims in handler context")
		}
		seen = c
		return nil, nil
	}
	if _, err := interceptor(ctx, nil, info, handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen == nil || seen.Email != "admin@bank.com" {
		t.Errorf("unexpected claims: %+v", seen)
	}
}

func TestAuthInterceptor_NoMetadata_Rejected(t *testing.T) {
	cfg := newTestConfig()
	interceptor := middleware.AuthInterceptor(cfg)
	info := &grpc.UnaryServerInfo{FullMethod: "/client.v1.ClientService/GetClient"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) { return nil, nil }

	// No metadata attached at all (raw context, not metadata.NewIncomingContext).
	_, err := interceptor(context.Background(), nil, info, handler)
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestAuthInterceptor_RefreshToken_Rejected(t *testing.T) {
	cfg := newTestConfig()
	tok, err := util.GenerateRefreshToken(1, "admin@bank.com", "admin", testSecret, 1)
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}
	gotErr := callInterceptor(t, cfg, "/client.v1.ClientService/GetClient", tok)
	if gotErr == nil {
		t.Fatal("expected error for refresh token used as access token")
	}
	if st, _ := status.FromError(gotErr); st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestAuthInterceptor_EmployeeMissingPerm_Rejected(t *testing.T) {
	cfg := newTestConfig()
	// Admin endpoint requires PermEmployeeAdmin, give only basic.
	tok := employeeToken(t, []string{models.PermEmployeeBasic})
	err := callInterceptor(t, cfg, "/client.v1.ClientService/UpdateClientPermissions", tok)
	if err == nil {
		t.Fatal("expected PermissionDenied")
	}
	if st, _ := status.FromError(err); st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestAuthInterceptor_ClientMissingPerm_Rejected(t *testing.T) {
	cfg := newTestConfig()
	tok := clientToken(t, []string{}) // no permissions
	err := callInterceptor(t, cfg, "/client.v1.ClientService/GetClient", tok)
	if err == nil {
		t.Fatal("expected PermissionDenied")
	}
	if st, _ := status.FromError(err); st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestAuthInterceptor_EmployeeOnUnmappedEndpoint_Allowed(t *testing.T) {
	cfg := newTestConfig()
	tok := employeeToken(t, []string{models.PermEmployeeBasic})
	// Unmapped methods: no required permission for employees → should pass through.
	err := callInterceptor(t, cfg, "/client.v1.ClientService/SomeOtherMethod", tok)
	if err != nil {
		t.Errorf("expected employee call to pass through unmapped endpoint, got %v", err)
	}
}
