package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
	"github.com/alicebob/miniredis/v2"
)

const logoutSecret = "logout-test-secret"

// withRevocationStore points the global token-revocation store at an
// in-process miniredis for the duration of the test.
func withRevocationStore(t *testing.T) {
	t.Helper()
	mr := miniredis.RunT(t)
	store := util.NewRedisTokenRevocationStore(mr.Addr(), "", 0)
	util.SetTokenRevocationStore(store)
	t.Cleanup(func() { util.SetTokenRevocationStore(nil) })
}

func logoutRequest(t *testing.T, h *handler.LogoutHandler, method, auth, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/logout", strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, "/logout", nil)
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.Logout(rec, r)
	return rec
}

func TestLogout_MethodNotAllowed(t *testing.T) {
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	if rec := logoutRequest(t, h, http.MethodGet, "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestLogout_MissingAndInvalidToken(t *testing.T) {
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})

	// No Authorization header.
	if rec := logoutRequest(t, h, http.MethodPost, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("missing header status = %d, want 401", rec.Code)
	}
	// Non-bearer header.
	if rec := logoutRequest(t, h, http.MethodPost, "Basic abc", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("non-bearer status = %d, want 401", rec.Code)
	}
	// Garbage bearer token.
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer not.a.jwt", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("garbage token status = %d, want 401", rec.Code)
	}
	// Valid signature but wrong token type (refresh used as access).
	refresh, _ := util.GenerateRefreshToken(1, "e@x.com", "emp", logoutSecret, 24)
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+refresh, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong-type token status = %d, want 401", rec.Code)
	}
}

func TestLogout_RevocationUnavailable(t *testing.T) {
	util.SetTokenRevocationStore(nil) // no store
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateAccessToken(1, "e@x.com", "emp", nil, logoutSecret, 15)
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestLogout_AccessOnly_Success(t *testing.T) {
	withRevocationStore(t)
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateAccessToken(1, "e@x.com", "emp", nil, logoutSecret, 15)

	// No body.
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestLogout_InvalidBody(t *testing.T) {
	withRevocationStore(t)
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateAccessToken(1, "e@x.com", "emp", nil, logoutSecret, 15)
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, "{not json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLogout_WithRefreshToken_Success(t *testing.T) {
	withRevocationStore(t)
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateAccessToken(2, "e@x.com", "emp", nil, logoutSecret, 15)
	refresh, _ := util.GenerateRefreshToken(2, "e@x.com", "emp", logoutSecret, 24)
	body := `{"refreshToken":"` + refresh + `"}`
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, body); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestLogout_RefreshTokenInvalid(t *testing.T) {
	withRevocationStore(t)
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateAccessToken(2, "e@x.com", "emp", nil, logoutSecret, 15)

	// Refresh token isn't a valid JWT.
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, `{"refreshToken":"garbage"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad refresh status = %d, want 400", rec.Code)
	}

	// Refresh token is actually an access token (wrong type).
	access2, _ := util.GenerateAccessToken(2, "e@x.com", "emp", nil, logoutSecret, 15)
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, `{"refreshToken":"`+access2+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong-type refresh status = %d, want 400", rec.Code)
	}
}

func TestLogout_RefreshTokenSubjectMismatch(t *testing.T) {
	withRevocationStore(t)
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateAccessToken(2, "e@x.com", "emp", nil, logoutSecret, 15)
	// Refresh token for a different employee.
	otherRefresh, _ := util.GenerateRefreshToken(99, "o@x.com", "other", logoutSecret, 24)
	body := `{"refreshToken":"` + otherRefresh + `"}`
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, body); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestLogout_ClientTokens_Success(t *testing.T) {
	withRevocationStore(t)
	h := handler.NewLogoutHandler(&config.Config{JWTSecret: logoutSecret})
	access, _ := util.GenerateClientAccessToken(5, "c@x.com", nil, logoutSecret, 15)
	refresh, _ := util.GenerateClientRefreshToken(5, "c@x.com", logoutSecret, 24)
	body := `{"refreshToken":"` + refresh + `"}`
	if rec := logoutRequest(t, h, http.MethodPost, "Bearer "+access, body); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
