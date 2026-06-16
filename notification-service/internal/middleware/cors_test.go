package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_Preflight(t *testing.T) {
	called := false
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/notifications", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("preflight: want 204, got %d", rr.Code)
	}
	if called {
		t.Error("preflight should short-circuit before the next handler")
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS allow-origin header")
	}
}

func TestCORS_Passthrough(t *testing.T) {
	called := false
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Error("non-preflight request should reach the next handler")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("passthrough: want 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing CORS allow-methods header")
	}
}
