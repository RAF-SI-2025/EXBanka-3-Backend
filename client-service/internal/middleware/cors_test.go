package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/middleware"
)

func TestCORS_AddsHeadersAndPassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	srv := middleware.CORS(next)
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if !called {
		t.Error("expected next handler to be called")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected ACAO=*, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods set")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("expected Access-Control-Allow-Headers set")
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("expected Max-Age=3600, got %q", got)
	}
}

func TestCORS_OPTIONSReturns204AndSkipsNext(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	srv := middleware.CORS(next)
	r := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if called {
		t.Error("expected next NOT to be called for OPTIONS preflight")
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected ACAO header on preflight, got %q", got)
	}
}
