package middleware

import (
	"context"
	"log"
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

		log.Printf("[gRPC] %-60s | %-20s | %v", info.FullMethod, code, duration)
		return resp, err
	}
}
