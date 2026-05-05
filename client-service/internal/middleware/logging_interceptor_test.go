package middleware_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoggingInterceptor_Success_PassesThrough(t *testing.T) {
	interceptor := middleware.LoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}
	want := "ok-response"
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return want, nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != want {
		t.Errorf("expected response %q, got %v", want, resp)
	}
}

func TestLoggingInterceptor_GrpcStatusError_PreservesError(t *testing.T) {
	interceptor := middleware.LoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Failing"}
	want := status.Error(codes.NotFound, "missing")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, want
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if resp != nil {
		t.Errorf("expected nil resp, got %v", resp)
	}
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

func TestLoggingInterceptor_PlainError_PreservesError(t *testing.T) {
	interceptor := middleware.LoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/PlainError"}
	want := errors.New("boom")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, want
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if !errors.Is(err, want) {
		t.Errorf("expected original error preserved, got %v", err)
	}
}
