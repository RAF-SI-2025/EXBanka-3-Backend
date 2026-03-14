package middleware

import (
	"context"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var requiredPermissions = map[string]string{
	"/client.v1.ClientService/CreateClient":            models.PermAdmin,
	"/client.v1.ClientService/GetClient":               models.PermAdmin,
	"/client.v1.ClientService/ListClients":             models.PermAdmin,
	"/client.v1.ClientService/UpdateClient":            models.PermAdmin,
	"/client.v1.ClientService/UpdateClientPermissions": models.PermAdmin,
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

		if requiredPerm, exists := requiredPermissions[info.FullMethod]; exists {
			isAdmin := util.HasPermission(claims, models.PermAdmin)
			hasPerm := util.HasPermission(claims, requiredPerm)
			if !isAdmin && !hasPerm {
				return nil, status.Errorf(codes.PermissionDenied, "permission %q required", requiredPerm)
			}
		}

		ctx = context.WithValue(ctx, ClaimsKey, claims)
		return handler(ctx, req)
	}
}
