package middleware

import (
	"context"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// requiredPermissions maps employee-only RPCs to the required permission.
var requiredPermissions = map[string]string{
	"/account.v1.AccountService/CreateAccount":       models.PermEmployeeBasic,
	"/account.v1.AccountService/ListAllAccounts":     models.PermEmployeeBasic,
	"/account.v1.AccountService/UpdateAccountLimits": models.PermEmployeeBasic,
}

// clientRequiredPermissions maps RPCs accessible to clients.
var clientRequiredPermissions = map[string]string{
	"/account.v1.AccountService/GetAccount":          models.PermClientBasic,
	"/account.v1.AccountService/ListClientAccounts":  models.PermClientBasic,
	"/account.v1.AccountService/UpdateAccountName":   models.PermClientBasic,
	"/account.v1.AccountService/UpdateAccountLimits": models.PermClientBasic,
}

type claimsContextKey struct{}

var ClaimsKey = claimsContextKey{}

func GetClaimsFromContext(ctx context.Context) (*util.Claims, bool) {
	claims, ok := ctx.Value(ClaimsKey).(*util.Claims)
	return claims, ok
}

func AuthInterceptor(cfg *config.Config) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		tokenStr := strings.TrimPrefix(authHeaders[0], "Bearer ")
		claims, err := util.ParseToken(tokenStr, cfg.JWTSecret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}

		if claims.TokenType != "access" {
			return nil, status.Error(codes.Unauthenticated, "wrong token type: expected access token")
		}

		if claims.TokenSource == "client" {
			requiredPerm, exists := clientRequiredPermissions[info.FullMethod]
			if !exists {
				return nil, status.Error(codes.PermissionDenied, "endpoint not accessible to clients")
			}
			if !util.HasPermission(claims, requiredPerm) {
				return nil, status.Errorf(codes.PermissionDenied, "permission %q required", requiredPerm)
			}
		} else {
			if requiredPerm, exists := requiredPermissions[info.FullMethod]; exists {
				if !util.HasPermission(claims, requiredPerm) {
					return nil, status.Errorf(codes.PermissionDenied, "permission %q required", requiredPerm)
				}
			}
		}

		ctx = context.WithValue(ctx, ClaimsKey, claims)
		return handler(ctx, req)
	}
}
