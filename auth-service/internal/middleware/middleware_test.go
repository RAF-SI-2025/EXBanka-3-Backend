package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// =====================
// CORS
// =====================

func TestCORS_SetsHeaders(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := CORS(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Allow-Origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Allow-Methods to be set")
	}
	if rec.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("expected Allow-Headers to be set")
	}
	if rec.Header().Get("Access-Control-Max-Age") != "3600" {
		t.Errorf("Max-Age = %q", rec.Header().Get("Access-Control-Max-Age"))
	}
}

func TestCORS_OptionsShortCircuits(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	h := CORS(next)

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Error("expected next handler NOT to be called for OPTIONS")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// =====================
// LoggingInterceptor
// =====================

func TestLoggingInterceptor_Success(t *testing.T) {
	interceptor := LoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), "req", info, handler)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want ok", resp)
	}
}

func TestLoggingInterceptor_GRPCError(t *testing.T) {
	interceptor := LoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.InvalidArgument, "bad input")
	}

	_, err := interceptor(context.Background(), "req", info, handler)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoggingInterceptor_PlainError(t *testing.T) {
	interceptor := LoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("non-grpc error")
	}

	_, err := interceptor(context.Background(), "req", info, handler)
	if err == nil {
		t.Fatal("expected error")
	}
}
