package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
)

type LogoutHandler struct {
	cfg *config.Config
}

type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func NewLogoutHandler(cfg *config.Config) *LogoutHandler {
	return &LogoutHandler{cfg: cfg}
}

func (h *LogoutHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeLogoutJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}

	accessToken := bearerToken(r.Header.Get("Authorization"))
	if accessToken == "" {
		writeLogoutJSON(w, http.StatusUnauthorized, map[string]string{"message": "missing authorization header"})
		return
	}

	accessClaims, err := util.ParseToken(accessToken, h.cfg.JWTSecret)
	if err != nil || accessClaims.TokenType != "access" {
		writeLogoutJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid or expired token"})
		return
	}

	if err := util.RevokeTokenClaims(r.Context(), accessClaims); err != nil {
		writeLogoutRevocationError(w, err)
		return
	}

	var req logoutRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeLogoutJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
			return
		}
	}

	if strings.TrimSpace(req.RefreshToken) != "" {
		refreshClaims, err := util.ParseToken(strings.TrimSpace(req.RefreshToken), h.cfg.JWTSecret)
		if err != nil || refreshClaims.TokenType != "refresh" {
			writeLogoutJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid refresh token"})
			return
		}
		if !sameTokenSubject(accessClaims, refreshClaims) {
			writeLogoutJSON(w, http.StatusForbidden, map[string]string{"message": "refresh token does not match current user"})
			return
		}
		if err := util.RevokeTokenClaims(r.Context(), refreshClaims); err != nil {
			writeLogoutRevocationError(w, err)
			return
		}
	}

	writeLogoutJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}

func sameTokenSubject(accessClaims, refreshClaims *util.Claims) bool {
	if accessClaims == nil || refreshClaims == nil {
		return false
	}
	if accessClaims.TokenSource != refreshClaims.TokenSource {
		return false
	}
	if accessClaims.TokenSource == "client" {
		return accessClaims.ClientID != 0 && accessClaims.ClientID == refreshClaims.ClientID
	}
	return accessClaims.EmployeeID != 0 && accessClaims.EmployeeID == refreshClaims.EmployeeID
}

func writeLogoutRevocationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, util.ErrTokenRevocationUnavailable):
		writeLogoutJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "token revocation unavailable"})
	case errors.Is(err, util.ErrTokenNotRevocable):
		writeLogoutJSON(w, http.StatusBadRequest, map[string]string{"message": "token cannot be revoked"})
	default:
		writeLogoutJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "token revocation failed"})
	}
}

func writeLogoutJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
