package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
)

func parseHTTPClaims(r *http.Request, cfg *config.Config) (*util.Claims, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, http.ErrNoCookie
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return util.ParseToken(token, cfg.JWTSecret)
}

func requireAuthenticatedEmployeeHTTP(w http.ResponseWriter, r *http.Request, cfg *config.Config) (*util.Claims, bool) {
	claims, err := parseHTTPClaims(r, cfg)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
		return nil, false
	}

	if claims.TokenSource != "employee" || claims.EmployeeID == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "employee access required"})
		return nil, false
	}

	if claims.TokenType != "access" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid token"})
		return nil, false
	}
	if util.IsTokenRevoked(r.Context(), claims) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid token"})
		return nil, false
	}

	return claims, true
}

func requireEmployeePermissionHTTP(w http.ResponseWriter, claims *util.Claims, permission string) bool {
	if !util.HasPermission(claims, permission) {
		message := "missing permission"
		if permission == models.PermEmployeeSupervisor {
			message = "supervisor access required"
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"message": message})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
