package middleware

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := codes.OK
		if err != nil {
			if s, ok := status.FromError(err); ok {
				code = s.Code()
			}
		}

		if code != codes.OK {
			slog.Warn("gRPC request failed",
				"method", info.FullMethod,
				"status", code.String(),
				"duration_ms", duration.Milliseconds(),
				"error", err,
			)
		} else {
			slog.Info("gRPC request",
				"method", info.FullMethod,
				"status", code.String(),
				"duration_ms", duration.Milliseconds(),
			)
		}
		return resp, err
	}
}
