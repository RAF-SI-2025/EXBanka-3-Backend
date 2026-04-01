package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

func parseHTTPClaims(r *http.Request, cfg *config.Config) (*util.Claims, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, http.ErrNoCookie
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return util.ParseToken(token, cfg.JWTSecret)
}

func requireAuthenticatedHTTP(w http.ResponseWriter, r *http.Request, cfg *config.Config) (*util.Claims, bool) {
	claims, err := parseHTTPClaims(r, cfg)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
		return nil, false
	}
	return claims, true
}

func requireClientPermissionHTTP(w http.ResponseWriter, claims *util.Claims, permission string) bool {
	if claims.TokenSource != "client" || claims.ClientID == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "client access required"})
		return false
	}
	if !util.HasPermission(claims, permission) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "missing permission"})
		return false
	}
	return true
}

func requireMarketReadAccessHTTP(w http.ResponseWriter, claims *util.Claims) bool {
	switch claims.TokenSource {
	case "client":
		if claims.ClientID == 0 {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "client access required"})
			return false
		}
		if !util.HasPermission(claims, models.PermClientTrading) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "trading permission required"})
			return false
		}
		return true
	case "employee":
		if claims.EmployeeID == 0 {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "employee access required"})
			return false
		}
		if !util.HasPermission(claims, models.PermEmployeeAgent) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "actuary access required"})
			return false
		}
		return true
	default:
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return false
	}
}

func requireSupervisorHTTP(w http.ResponseWriter, claims *util.Claims) bool {
	if claims.TokenSource != "employee" || claims.EmployeeID == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "employee access required"})
		return false
	}
	if !util.HasPermission(claims, models.PermEmployeeSupervisor) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "supervisor permission required"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
