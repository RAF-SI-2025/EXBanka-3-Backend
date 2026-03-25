package handler

import (
	"net/http"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}

func parseHTTPClaims(w http.ResponseWriter, r *http.Request, cfg *config.Config) (*util.Claims, bool) {
	if cfg == nil {
		return nil, true
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		writeAuthError(w, http.StatusUnauthorized, "missing authorization header")
		return nil, false
	}
	if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		writeAuthError(w, http.StatusUnauthorized, "invalid authorization header")
		return nil, false
	}

	tokenStr := strings.TrimSpace(authHeader[len("Bearer "):])
	if tokenStr == "" {
		writeAuthError(w, http.StatusUnauthorized, "invalid authorization header")
		return nil, false
	}

	claims, err := util.ParseToken(tokenStr, cfg.JWTSecret)
	if err != nil || claims.TokenType != "access" {
		writeAuthError(w, http.StatusUnauthorized, "invalid or expired token")
		return nil, false
	}

	return claims, true
}

func requireEmployeePermissionHTTP(w http.ResponseWriter, claims *util.Claims, perm string) bool {
	if claims == nil {
		return true
	}
	if claims.ClientID != 0 || claims.TokenSource == "client" || claims.EmployeeID == 0 {
		writeAuthError(w, http.StatusForbidden, "employee access required")
		return false
	}
	if !util.HasPermission(claims, perm) {
		writeAuthError(w, http.StatusForbidden, "insufficient permissions")
		return false
	}
	return true
}

func requireClientOrEmployeeHTTP(w http.ResponseWriter, claims *util.Claims, clientID uint, clientPerm, employeePerm string) bool {
	if claims == nil {
		return true
	}

	if claims.ClientID != 0 || claims.TokenSource == "client" {
		if claims.ClientID != clientID {
			writeAuthError(w, http.StatusForbidden, "access denied")
			return false
		}
		if clientPerm != "" && !util.HasPermission(claims, clientPerm) {
			writeAuthError(w, http.StatusForbidden, "insufficient permissions")
			return false
		}
		return true
	}

	return requireEmployeePermissionHTTP(w, claims, employeePerm)
}
